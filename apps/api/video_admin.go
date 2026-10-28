package main

import (
	"context"
	"errors"
	"mime"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

func (s *videoService) publish(c *gin.Context) {
	id, ok := videoID(c)
	if !ok {
		return
	}
	mediaType, _, _ := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if mediaType != "application/json" {
		c.JSON(415, gin.H{"error": "json_required"})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	var req struct {
		Status string `json:"publicationStatus"`
	}
	if c.ShouldBindJSON(&req) != nil || (req.Status != "draft" && req.Status != "published" && req.Status != "hidden") {
		c.JSON(400, gin.H{"error": "invalid_status"})
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
	err := v.scan(tx.QueryRow(ctx, `UPDATE videos SET publication_status=$2,updated_at=NOW() WHERE id=$1 AND deleted_at IS NULL AND ($2<>'published' OR status='ready') RETURNING `+videoColumns, id, req.Status))
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(409, gin.H{"error": "video_not_ready_or_missing"})
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
func (s *videoService) remove(c *gin.Context) {
	id, ok := videoID(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	tx, ok := s.beginAdminMutation(c, ctx)
	if !ok {
		return
	}
	defer rollback(tx)
	tag, err := tx.Exec(ctx, `UPDATE videos SET deleted_at=NOW(),publication_status='hidden',updated_at=NOW() WHERE id=$1 AND deleted_at IS NULL`, id)
	if err != nil {
		c.JSON(503, gin.H{"error": "database_unavailable"})
		return
	}
	if tag.RowsAffected() == 0 {
		c.JSON(404, gin.H{"error": "video_not_found"})
		return
	}
	if err = tx.Commit(ctx); err != nil {
		accountFailure(c, err)
		return
	}
	s.mu.Lock()
	if stop := s.running[id]; stop != nil {
		stop()
	}
	s.mu.Unlock()
	c.Status(204)
}
func (s *videoService) retry(c *gin.Context) {
	id, ok := videoID(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	tx, ok := s.beginAdminMutation(c, ctx)
	if !ok {
		return
	}
	defer rollback(tx)
	tag, err := tx.Exec(ctx, `UPDATE videos SET status='pending',processing_error='',updated_at=NOW() WHERE id=$1 AND deleted_at IS NULL AND status IN ('failed','ready') AND storage_key IS NOT NULL`, id)
	if err != nil {
		c.JSON(503, gin.H{"error": "database_unavailable"})
		return
	}
	if tag.RowsAffected() == 0 {
		c.JSON(409, gin.H{"error": "video_not_retryable"})
		return
	}
	if err = tx.Commit(ctx); err != nil {
		accountFailure(c, err)
		return
	}
	c.Status(202)
}
