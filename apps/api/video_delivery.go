package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

func (s *videoService) media(c *gin.Context) {
	id, ok := videoID(c)
	if !ok {
		return
	}
	kind, name := c.Param("kind"), c.Param("file")
	if (kind != "full" && kind != "preview") || !mediaFilePattern.MatchString(name) {
		c.Status(404)
		return
	}
	user, ok := s.user(c, kind == "full")
	if !ok {
		return
	}
	if kind == "full" && user.Role != "admin" && !user.Member {
		c.JSON(403, gin.H{"error": "membership_required"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	var key string
	var generation int64
	err := s.db.QueryRow(ctx, `SELECT storage_key,hls_generation FROM videos WHERE id=$1 AND storage_key IS NOT NULL AND deleted_at IS NULL AND status='ready' AND ($2 OR publication_status='published')`, id, user.Role == "admin").Scan(&key, &generation)
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
	maxHeight := 2160
	if kind == "full" {
		maxHeight = limitsFor(user).Height
		if maxHeight == 0 {
			c.JSON(403, gin.H{"error": "membership_required"})
			return
		}
		if !s.authorizePlayback(c, ctx, user, id, generation) {
			return
		}
		if mediaHeight(name) > maxHeight {
			c.JSON(403, gin.H{"error": "quality_not_allowed"})
			return
		}
	}
	directory := hlsDirectory(key, generation)
	if s.store != nil {
		s.s3Media(c, directory, kind, name, maxHeight)
		return
	}
	root, err := os.OpenRoot(s.root)
	if err != nil {
		c.Status(503)
		return
	}
	defer root.Close()
	if kind == "full" && name != "index.m3u8" {
		master, openErr := root.Open(filepath.Join(directory, kind, "index.m3u8"))
		if openErr != nil {
			c.Status(404)
			return
		}
		data, readErr := io.ReadAll(io.LimitReader(master, (1<<20)+1))
		master.Close()
		if readErr != nil || len(data) > 1<<20 {
			c.Status(503)
			return
		}
		if !renditionAllowed(data, name, maxHeight) {
			c.JSON(403, gin.H{"error": "quality_not_allowed"})
			return
		}
	}
	file, err := root.Open(filepath.Join(directory, kind, name))
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
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cross-Origin-Resource-Policy", "same-origin")
	if strings.HasSuffix(name, ".m3u8") {
		token := ""
		if kind == "full" {
			token = c.Query("playback")
		}
		servePlaylist(c, file, maxHeight, token)
		return
	} else {
		c.Header("Content-Type", "video/mp2t")
	}
	if s.nginxDelivery {
		// Only validated, authorized paths reach Nginx's internal-only location.
		c.Header("X-Accel-Redirect", "/_hls/"+directory+"/"+kind+"/"+name)
		c.Status(200)
		return
	}
	http.ServeContent(c.Writer, c.Request, name, info.ModTime(), file)
}
