package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

func (s *videoService) upload(c *gin.Context) {
	if s.store != nil {
		c.JSON(409, gin.H{"error": "direct_upload_required"})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, s.budget.Job)
	reader, err := c.Request.MultipartReader()
	if err != nil {
		c.JSON(400, gin.H{"error": "multipart_required"})
		return
	}
	bytes := make([]byte, 16)
	if _, err = rand.Read(bytes); err != nil {
		c.JSON(500, gin.H{"error": "internal_error"})
		return
	}
	key := hex.EncodeToString(bytes)
	reserve, cancelReserve := context.WithTimeout(c.Request.Context(), 10*time.Second)
	err = s.reserveMedia(reserve, key, true)
	cancelReserve()
	if err != nil {
		if errors.Is(err, errMediaBudget) {
			c.JSON(507, gin.H{"error": "storage_budget_exceeded"})
		} else {
			c.JSON(503, gin.H{"error": "storage_unavailable"})
		}
		return
	}
	dir := filepath.Join(s.root, key)
	if err = os.Mkdir(dir, 0700); err != nil {
		s.releaseMedia(key)
		c.JSON(500, gin.H{"error": "storage_unavailable"})
		return
	}
	keep := false
	defer func() {
		if !keep {
			os.RemoveAll(dir)
			s.releaseMedia(key)
		}
	}()
	fields := map[string]string{}
	hasFile := false
	hasCover := false
	for {
		part, readErr := reader.NextPart()
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			c.JSON(413, gin.H{"error": "upload_incomplete_or_too_large"})
			return
		}
		if part.FormName() == "file" && part.FileName() != "" {
			ext := strings.ToLower(filepath.Ext(part.FileName()))
			if hasFile || (ext != ".mp4" && ext != ".mov" && ext != ".webm") {
				part.Close()
				c.JSON(400, gin.H{"error": "invalid_video_file"})
				return
			}
			hasFile = true
			file, openErr := os.OpenFile(filepath.Join(dir, "source"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if openErr != nil {
				part.Close()
				c.JSON(500, gin.H{"error": "storage_unavailable"})
				return
			}
			n, copyErr := io.Copy(file, io.LimitReader(part, s.budget.Job+1))
			closeErr := file.Close()
			part.Close()
			if copyErr != nil || n > s.budget.Job {
				c.JSON(413, gin.H{"error": "upload_incomplete_or_too_large"})
				return
			}
			if closeErr != nil {
				c.JSON(500, gin.H{"error": "storage_unavailable"})
				return
			}
			if n == 0 {
				c.JSON(400, gin.H{"error": "invalid_video_file"})
				return
			}
		} else if part.FormName() == "cover" && part.FileName() != "" {
			if hasCover {
				part.Close()
				c.JSON(400, gin.H{"error": "invalid_cover"})
				return
			}
			coverErr := saveCover(part, filepath.Join(dir, "cover.jpg"))
			part.Close()
			if coverErr != nil {
				c.JSON(400, gin.H{"error": "invalid_cover"})
				return
			}
			hasCover = true
		} else {
			name := part.FormName()
			if name != "title" && name != "description" && name != "category" {
				part.Close()
				c.JSON(400, gin.H{"error": "invalid_metadata"})
				return
			}
			if _, exists := fields[name]; exists {
				part.Close()
				c.JSON(400, gin.H{"error": "invalid_metadata"})
				return
			}
			data, e := io.ReadAll(io.LimitReader(part, 8193))
			part.Close()
			if e != nil || len(data) > 8192 || !utf8.Valid(data) {
				c.JSON(400, gin.H{"error": "invalid_metadata"})
				return
			}
			fields[name] = strings.TrimSpace(string(data))
		}
	}
	title, description, category := fields["title"], fields["description"], fields["category"]
	if !hasFile || utf8.RuneCountInString(title) < 1 || utf8.RuneCountInString(title) > 120 || utf8.RuneCountInString(description) > 2000 || (category != "บันเทิง" && category != "ความรู้" && category != "ไลฟ์สไตล์" && category != "อื่น ๆ") {
		c.JSON(400, gin.H{"error": "invalid_metadata"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	tx, ok := s.beginAdminMutation(c, ctx)
	if !ok {
		return
	}
	defer rollback(tx)
	var v videoRecord
	err = v.scan(tx.QueryRow(ctx, `INSERT INTO videos(title,description,category,storage_key,has_cover) VALUES($1,$2,$3,$4,$5) RETURNING `+videoColumns, title, description, category, key, hasCover))
	if err == nil {
		_, err = tx.Exec(ctx, `UPDATE media_reservations SET lease_until=NULL WHERE storage_key=$1`, key)
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		c.JSON(503, gin.H{"error": "database_unavailable"})
		return
	}
	keep = true
	c.JSON(202, gin.H{"video": v})
}
