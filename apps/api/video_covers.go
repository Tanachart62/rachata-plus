package main

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

const maxCoverBytes int64 = 5 * 1024 * 1024

// Decode before storing: reject disguised files and excessive pixel allocations.
// Re-encoding removes embedded metadata and always serves a known image format.
func saveCover(reader io.Reader, path string) error {
	data, err := io.ReadAll(io.LimitReader(reader, maxCoverBytes+1))
	if err != nil || len(data) == 0 || int64(len(data)) > maxCoverBytes {
		return errors.New("invalid cover size")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "jpeg" && format != "png") || config.Width < 1 || config.Height < 1 || config.Width > 4096 || config.Height > 4096 || int64(config.Width)*int64(config.Height) > 8_000_000 {
		return errors.New("invalid cover dimensions or format")
	}
	picture, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	encodeErr := jpeg.Encode(file, picture, &jpeg.Options{Quality: 85})
	closeErr := file.Close()
	if encodeErr != nil {
		return encodeErr
	}
	return closeErr
}

func (s *videoService) cover(c *gin.Context) {
	id, ok := videoID(c)
	if !ok {
		return
	}
	user, ok := s.user(c, false)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	var key string
	err := s.db.QueryRow(ctx, `SELECT storage_key FROM videos WHERE id=$1 AND deleted_at IS NULL AND has_cover AND ($2 OR (publication_status='published' AND status='ready'))`, id, user.Role == "admin").Scan(&key)
	if errors.Is(err, pgx.ErrNoRows) {
		c.Status(404)
		return
	}
	if err != nil {
		c.Status(503)
		return
	}
	if !storageKeyPattern.MatchString(key) {
		c.Status(404)
		return
	}
	root, err := os.OpenRoot(s.root)
	if err != nil {
		c.Status(503)
		return
	}
	defer root.Close()
	file, err := root.Open(filepath.Join(key, "cover.jpg"))
	if err != nil {
		c.Status(404)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		c.Status(404)
		return
	}
	c.Header("Content-Type", "image/jpeg")
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cross-Origin-Resource-Policy", "same-origin")
	http.ServeContent(c.Writer, c.Request, "cover.jpg", info.ModTime(), file)
}
