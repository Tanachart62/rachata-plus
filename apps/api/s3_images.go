package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"strings"
	"time"
)

func (s *videoService) editS3Video(c *gin.Context, id, revision int64, title, description, category string, cover []byte) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	tx, ok := s.beginAdminMutation(c, ctx)
	if !ok {
		return
	}
	defer rollback(tx)
	var key string
	err := tx.QueryRow(ctx, `SELECT storage_key FROM videos WHERE id=$1 AND deleted_at IS NULL AND metadata_revision=$2 FOR UPDATE`, id, revision).Scan(&key)
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(409, gin.H{"error": "video_edit_conflict"})
		return
	}
	if err != nil {
		accountFailure(c, err)
		return
	}
	var coverKey any
	if cover != nil {
		nonce, err := randomStorageKey()
		if err != nil {
			c.Status(500)
			return
		}
		relative := key + "/covers/" + nonce + ".jpg"
		if err = s.store.bytes(ctx, relative, cover, "image/jpeg"); err != nil {
			c.Status(503)
			return
		}
		coverKey = relative
	}
	var v videoRecord
	err = v.scan(tx.QueryRow(ctx, `UPDATE videos SET title=$2,description=$3,category=$4,
  cover_s3_key=COALESCE($5::text,cover_s3_key),cover_jpeg=CASE WHEN $5::text IS NULL THEN cover_jpeg ELSE NULL END,
  has_cover=has_cover OR $5::text IS NOT NULL,metadata_revision=metadata_revision+1,updated_at=NOW() WHERE id=$1 RETURNING `+videoColumns, id, title, description, category, coverKey))
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		accountFailure(c, err)
		return
	}
	c.JSON(200, gin.H{"video": v})
}
func (a *authConfig) s3Avatar(c *gin.Context, db *pgxpool.Pool, id int64) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	var key *string
	if err := db.QueryRow(ctx, `SELECT avatar_s3_key FROM users WHERE id=$1 AND account_status='active'`, id).Scan(&key); err != nil {
		accountFailure(c, err)
		return
	}
	if key == nil || !strings.HasPrefix(*key, fmt.Sprintf("avatars/%d/", id)) {
		c.Status(404)
		return
	}
	a.store.serve(c, *key, "image/jpeg")
}
func (a *authConfig) saveS3Avatar(c *gin.Context, db *pgxpool.Pool, id int64, data []byte) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	hash, _ := sessionHash(c)
	tx, _, err := lockedAccount(ctx, db, hash, id)
	if err != nil {
		accountFailure(c, err)
		return
	}
	defer rollback(tx)
	nonce, err := randomStorageKey()
	if err != nil {
		c.Status(500)
		return
	}
	key := fmt.Sprintf("avatars/%d/%s.jpg", id, nonce)
	if err = a.store.bytes(ctx, key, data, "image/jpeg"); err != nil {
		c.Status(503)
		return
	}
	_, err = tx.Exec(ctx, `UPDATE users SET avatar_s3_key=$2,avatar_jpeg=NULL,updated_at=NOW() WHERE id=$1`, id, key)
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		accountFailure(c, err)
		return
	}
	c.Status(204)
}
