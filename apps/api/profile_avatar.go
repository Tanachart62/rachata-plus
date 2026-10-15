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
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

const maxAvatarBytes = 2 * 1024 * 1024

// Only decoded JPEG/PNG pixels are retained, never metadata or uploaded names.
func avatarJPEG(reader io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxAvatarBytes+1))
	if err != nil || len(data) == 0 || len(data) > maxAvatarBytes {
		return nil, errors.New("invalid avatar size")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "jpeg" && format != "png") || config.Width < 1 || config.Height < 1 || config.Width > 4096 || config.Height > 4096 || int64(config.Width)*int64(config.Height) > 8_000_000 {
		return nil, errors.New("invalid avatar image")
	}
	picture, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	// Center crop to a small square so each stored profile has bounded size.
	bounds := picture.Bounds()
	side := min(bounds.Dx(), bounds.Dy())
	x0, y0 := bounds.Min.X+(bounds.Dx()-side)/2, bounds.Min.Y+(bounds.Dy()-side)/2
	size := min(256, side)
	thumbnail := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			r, g, b, a := picture.At(x0+x*side/size, y0+y*side/size).RGBA()
			thumbnail.SetRGBA(x, y, color.RGBA{uint8((r + 65535 - a) >> 8), uint8((g + 65535 - a) >> 8), uint8((b + 65535 - a) >> 8), 255})
		}
	}
	var output bytes.Buffer
	err = jpeg.Encode(&output, thumbnail, &jpeg.Options{Quality: 85})
	return output.Bytes(), err
}

func (a *authConfig) profileAvatar(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		u, ok := requireSession(c, db)
		if !ok {
			return
		}
		id, err := strconv.ParseInt(c.Query("expectedUserId"), 10, 64)
		if err != nil || id < 1 {
			c.JSON(400, gin.H{"error": "invalid_request"})
			return
		}
		if !checkOwner(c, u, id) {
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()
		var data []byte
		err = db.QueryRow(ctx, `SELECT avatar_jpeg FROM users WHERE id=$1`, id).Scan(&data)
		if err != nil {
			accountFailure(c, err)
			return
		}
		if len(data) == 0 {
			c.Status(404)
			return
		}
		c.Header("Cache-Control", "private, no-store")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Cross-Origin-Resource-Policy", "same-origin")
		c.Data(200, "image/jpeg", data)
	}
}

func (a *authConfig) uploadAvatar(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		actor, ok := requireSession(c, db)
		if !ok {
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxAvatarBytes+8192)
		err := c.Request.ParseMultipartForm(64 * 1024)
		if c.Request.MultipartForm != nil {
			defer c.Request.MultipartForm.RemoveAll()
		}
		if err != nil {
			c.JSON(400, gin.H{"error": "invalid_avatar"})
			return
		}
		id, err := strconv.ParseInt(c.PostForm("expectedUserId"), 10, 64)
		if err != nil || id < 1 {
			c.JSON(400, gin.H{"error": "invalid_request"})
			return
		}
		if !checkOwner(c, actor, id) {
			return
		}
		file, header, err := c.Request.FormFile("file")
		if err != nil {
			c.JSON(400, gin.H{"error": "invalid_avatar"})
			return
		}
		defer file.Close()
		if header.Size > maxAvatarBytes {
			c.JSON(400, gin.H{"error": "invalid_avatar"})
			return
		}
		data, err := avatarJPEG(file)
		if err != nil {
			c.JSON(400, gin.H{"error": "invalid_avatar"})
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		hash, _ := sessionHash(c)
		tx, _, err := lockedAccount(ctx, db, hash, id)
		if err != nil {
			accountFailure(c, err)
			return
		}
		defer rollback(tx)
		_, err = tx.Exec(ctx, `UPDATE users SET avatar_jpeg=$2,updated_at=NOW() WHERE id=$1`, id, data)
		if err == nil {
			err = tx.Commit(ctx)
		}
		if err != nil {
			accountFailure(c, err)
			return
		}
		c.Status(204)
	}
}
