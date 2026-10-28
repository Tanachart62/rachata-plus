package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

func (s *videoService) editVideo(c *gin.Context) {
	id, ok := videoID(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, s.budget.Job)
	reader, err := c.Request.MultipartReader()
	if err != nil {
		c.JSON(400, gin.H{"error": "multipart_required"})
		return
	}
	fields := map[string]string{}
	var cover []byte
	for {
		part, readErr := reader.NextPart()
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			c.JSON(413, gin.H{"error": "upload_incomplete_or_too_large"})
			return
		}
		name := part.FormName()
		if name == "cover" && part.FileName() != "" {
			if cover != nil {
				part.Close()
				c.JSON(400, gin.H{"error": "invalid_cover"})
				return
			}
			cover, err = encodedCover(part)
			part.Close()
			if err != nil {
				c.JSON(400, gin.H{"error": "invalid_cover"})
				return
			}
			continue
		}
		if part.FileName() != "" || (name != "title" && name != "description" && name != "category" && name != "expectedUserId" && name != "expectedRevision") {
			part.Close()
			c.JSON(400, gin.H{"error": "invalid_metadata"})
			return
		}
		if _, duplicate := fields[name]; duplicate {
			part.Close()
			c.JSON(400, gin.H{"error": "invalid_metadata"})
			return
		}
		data, readErr := io.ReadAll(io.LimitReader(part, 8193))
		part.Close()
		if readErr != nil || len(data) > 8192 || !utf8.Valid(data) {
			c.JSON(400, gin.H{"error": "invalid_metadata"})
			return
		}
		fields[name] = strings.TrimSpace(string(data))
	}
	title, description, category := fields["title"], fields["description"], fields["category"]
	if n := utf8.RuneCountInString(title); n < 1 || n > 120 || utf8.RuneCountInString(description) > 2000 ||
		(category != "บันเทิง" && category != "ความรู้" && category != "ไลฟ์สไตล์" && category != "อื่น ๆ") {
		c.JSON(400, gin.H{"error": "invalid_metadata"})
		return
	}
	expected, ownerErr := strconv.ParseInt(fields["expectedUserId"], 10, 64)
	revision, revisionErr := strconv.ParseInt(fields["expectedRevision"], 10, 64)
	if ownerErr != nil || expected < 1 || revisionErr != nil || revision < 0 {
		c.JSON(400, gin.H{"error": "invalid_metadata"})
		return
	}
	actor, _ := c.Get(videoAdminActorKey)
	if actor != expected {
		c.JSON(409, gin.H{"error": "account_changed"})
		return
	}
	if s.store != nil {
		s.editS3Video(c, id, revision, title, description, category, cover)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	tx, ok := s.beginAdminMutation(c, ctx)
	if !ok {
		return
	}
	defer rollback(tx)
	var v videoRecord
	err = v.scan(tx.QueryRow(ctx, `UPDATE videos SET title=$2,description=$3,category=$4,
	 cover_jpeg=COALESCE($6::bytea,cover_jpeg),has_cover=(has_cover OR $6::bytea IS NOT NULL),
	 metadata_revision=metadata_revision+1,updated_at=NOW()
	 WHERE id=$1 AND deleted_at IS NULL AND metadata_revision=$5 RETURNING `+videoColumns,
		id, title, description, category, revision, cover))
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(409, gin.H{"error": "video_edit_conflict"})
		return
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		c.JSON(503, gin.H{"error": "database_unavailable"})
		return
	}
	c.JSON(200, gin.H{"video": v})
}
