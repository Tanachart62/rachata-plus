package main

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// Decode before storing to reject disguised files.
// Re-encoding removes embedded metadata and always serves a known image format.
var coverDecodeSlots = make(chan struct{}, 1)

func encodedCover(reader io.Reader) ([]byte, error) {
	select {
	case coverDecodeSlots <- struct{}{}:
		defer func() { <-coverDecodeSlots }()
	default:
		return nil, errors.New("image decoder busy")
	}
	// Normalize browser uploads; bound memory for clients bypassing the UI.
	data, err := io.ReadAll(io.LimitReader(reader, (32<<20)+1))
	if err != nil || len(data) == 0 || len(data) > 32<<20 {
		return nil, errors.New("invalid cover size")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "jpeg" && format != "png") || config.Width < 1 || config.Height < 1 || int64(config.Width)*int64(config.Height) > 32_000_000 {
		return nil, errors.New("invalid cover dimensions or format")
	}
	picture, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	var output bytes.Buffer
	bounds := picture.Bounds()
	scale := min(1.0, 2048.0/float64(max(bounds.Dx(), bounds.Dy())))
	w, h := max(1, int(float64(bounds.Dx())*scale)), max(1, int(float64(bounds.Dy())*scale))
	thumbnail := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, b, a := picture.At(bounds.Min.X+x*bounds.Dx()/w, bounds.Min.Y+y*bounds.Dy()/h).RGBA()
			thumbnail.SetRGBA(x, y, color.RGBA{uint8((r + 65535 - a) >> 8), uint8((g + 65535 - a) >> 8), uint8((b + 65535 - a) >> 8), 255})
		}
	}
	picture = thumbnail
	if err := jpeg.Encode(&output, picture, &jpeg.Options{Quality: 85}); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func saveCover(reader io.Reader, path string) error {
	data, err := encodedCover(reader)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
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
	var cover []byte
	if s.store != nil {
		var object *string
		err := s.db.QueryRow(ctx, `SELECT storage_key,cover_s3_key FROM videos WHERE id=$1 AND deleted_at IS NULL AND has_cover AND ($2 OR (publication_status='published' AND status='ready'))`, id, user.Role == "admin").Scan(&key, &object)
		if errors.Is(err, pgx.ErrNoRows) {
			c.Status(404)
			return
		}
		if err != nil {
			c.Status(503)
			return
		}
		relative := key + "/cover.jpg"
		if object != nil {
			relative = *object
		}
		if !storageKeyPattern.MatchString(key) || !strings.HasPrefix(relative, key+"/") {
			c.Status(404)
			return
		}
		s.store.serve(c, relative, "image/jpeg")
		return
	}
	err := s.db.QueryRow(ctx, `SELECT storage_key,cover_jpeg FROM videos WHERE id=$1 AND deleted_at IS NULL AND has_cover AND ($2 OR (publication_status='published' AND status='ready'))`, id, user.Role == "admin").Scan(&key, &cover)
	if errors.Is(err, pgx.ErrNoRows) {
		c.Status(404)
		return
	}
	if err != nil {
		c.Status(503)
		return
	}
	if len(cover) > 0 {
		c.Header("Content-Type", "image/jpeg")
		c.Header("Cache-Control", "private, no-store")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Cross-Origin-Resource-Policy", "same-origin")
		http.ServeContent(c.Writer, c.Request, "cover.jpg", time.Time{}, bytes.NewReader(cover))
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
