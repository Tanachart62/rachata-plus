package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"time"
)

const libraryPageSize = 50

type libraryCursor struct {
	At time.Time `json:"at"`
	ID int64     `json:"id"`
}

func parseLibraryCursor(value string) (libraryCursor, error) {
	if value == "" {
		return libraryCursor{At: time.Unix(1<<40, 0), ID: 1 << 62}, nil
	}
	var cur libraryCursor
	if len(value) > 256 {
		return cur, errors.New("invalid cursor")
	}
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return cur, err
	}
	err = json.Unmarshal(data, &cur)
	if err == nil && (cur.ID <= 0 || cur.At.IsZero()) {
		err = errors.New("invalid cursor")
	}
	return cur, err
}
func encodeLibraryCursor(at time.Time, id int64) string {
	data, _ := json.Marshal(libraryCursor{At: at, ID: id})
	return base64.RawURLEncoding.EncodeToString(data)
}

type historyEntry struct {
	VideoID   int64 `json:"videoId"`
	Position  int   `json:"position"`
	WatchedAt int64 `json:"watchedAt"`
	Revision  int64 `json:"revision"`
}

func (a *authConfig) library(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		u, ok := requireSession(c, db)
		if !ok {
			return
		}
		sc, err := parseLibraryCursor(c.Query("savedCursor"))
		if err != nil {
			c.JSON(400, gin.H{"error": "invalid_cursor"})
			return
		}
		hc, err := parseLibraryCursor(c.Query("historyCursor"))
		if err != nil {
			c.JSON(400, gin.H{"error": "invalid_cursor"})
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()
		rows, err := db.Query(ctx, `SELECT w.video_id,w.created_at,v.id,v.title,v.description,v.category,v.duration_seconds,v.publication_status,v.status,v.processing_error FROM watchlist w JOIN videos v ON v.id=w.video_id
 WHERE w.user_id=$1 AND v.deleted_at IS NULL AND (v.publication_status='published' OR $2) AND (w.created_at,w.video_id)<($3,$4)
 ORDER BY w.created_at DESC,w.video_id DESC LIMIT $5`, u.ID, u.Role == "admin", sc.At, sc.ID, libraryPageSize+1)
		if err != nil {
			accountFailure(c, err)
			return
		}
		saved := []int64{}
		videos := []videoRecord{}
		var last libraryCursor
		var savedNext string
		for rows.Next() {
			var id int64
			var at time.Time
			var video videoRecord
			if err = rows.Scan(append([]any{&id, &at}, video.destinations()...)...); err != nil {
				break
			}
			if len(saved) == libraryPageSize {
				savedNext = encodeLibraryCursor(last.At, last.ID)
				break
			}
			saved = append(saved, id)
			video.deliveryURLs()
			videos = append(videos, video)
			last = libraryCursor{At: at, ID: id}
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil {
			accountFailure(c, err)
			return
		}
		rows, err = db.Query(ctx, `SELECT h.video_id,h.position_seconds,h.watched_at,h.revision,v.id,v.title,v.description,v.category,v.duration_seconds,v.publication_status,v.status,v.processing_error FROM watch_history h JOIN videos v ON v.id=h.video_id
 WHERE h.user_id=$1 AND v.deleted_at IS NULL AND (v.publication_status='published' OR $2) AND (h.watched_at,h.video_id)<($3,$4)
 ORDER BY h.watched_at DESC,h.video_id DESC LIMIT $5`, u.ID, u.Role == "admin", hc.At, hc.ID, libraryPageSize+1)
		if err != nil {
			accountFailure(c, err)
			return
		}
		history := []historyEntry{}
		var historyNext string
		for rows.Next() {
			var e historyEntry
			var at time.Time
			var video videoRecord
			if err = rows.Scan(append([]any{&e.VideoID, &e.Position, &at, &e.Revision}, video.destinations()...)...); err != nil {
				break
			}
			if len(history) == libraryPageSize {
				historyNext = encodeLibraryCursor(last.At, last.ID)
				break
			}
			e.WatchedAt = at.UnixMilli()
			history = append(history, e)
			video.deliveryURLs()
			videos = append(videos, video)
			last = libraryCursor{At: at, ID: e.VideoID}
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil {
			accountFailure(c, err)
			return
		}
		c.JSON(200, gin.H{"userId": u.ID, "saved": saved, "history": history, "savedNext": savedNext, "historyNext": historyNext, "videos": videos})
	}
}
func (a *authConfig) savedVideo(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		u, ok := requireSession(c, db)
		if !ok {
			return
		}
		id, ok := videoID(c)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()
		var saved bool
		err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM watchlist WHERE user_id=$1 AND video_id=$2)`, u.ID, id).Scan(&saved)
		if err != nil {
			accountFailure(c, err)
			return
		}
		c.JSON(200, gin.H{"userId": u.ID, "saved": saved})
	}
}
func (a *authConfig) saveVideo(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		u, ok := requireSession(c, db)
		if !ok {
			return
		}
		id, ok := videoID(c)
		if !ok {
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
		var req struct {
			accountOwner
			Saved *bool `json:"saved"`
		}
		if c.ShouldBindJSON(&req) != nil || req.Saved == nil {
			c.JSON(400, gin.H{"error": "invalid_request"})
			return
		}
		if !checkOwner(c, u, req.ExpectedUserID) {
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()
		var err error
		if !*req.Saved {
			_, err = db.Exec(ctx, `DELETE FROM watchlist WHERE user_id=$1 AND video_id=$2`, u.ID, id)
		} else {
			var available bool
			err = db.QueryRow(ctx, `WITH available AS (SELECT id FROM videos WHERE id=$2 AND deleted_at IS NULL AND (publication_status='published' OR $3)), inserted AS (
 INSERT INTO watchlist(user_id,video_id) SELECT $1,id FROM available ON CONFLICT(user_id,video_id) DO NOTHING RETURNING video_id)
 SELECT EXISTS(SELECT 1 FROM available)`, u.ID, id, u.Role == "admin").Scan(&available)
			if err == nil && !available {
				c.JSON(404, gin.H{"error": "video_not_found"})
				return
			}
		}
		if err != nil {
			accountFailure(c, err)
			return
		}
		c.Status(204)
	}
}
func (a *authConfig) readProgress(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		u, ok := requireSession(c, db)
		if !ok {
			return
		}
		id, ok := videoID(c)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()
		e := historyEntry{VideoID: id}
		var at time.Time
		err := db.QueryRow(ctx, `SELECT COALESCE(h.position_seconds,0),COALESCE(h.revision,0),COALESCE(h.watched_at,to_timestamp(0)) FROM videos v
 LEFT JOIN watch_history h ON h.video_id=v.id AND h.user_id=$1 WHERE v.id=$2 AND v.deleted_at IS NULL AND (v.publication_status='published' OR $3)`, u.ID, id, u.Role == "admin").Scan(&e.Position, &e.Revision, &at)
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(404, gin.H{"error": "video_not_found"})
			return
		}
		if err != nil {
			accountFailure(c, err)
			return
		}
		e.WatchedAt = at.UnixMilli()
		c.JSON(200, gin.H{"userId": u.ID, "history": e})
	}
}
func (a *authConfig) watchProgress(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		u, ok := requireSession(c, db)
		if !ok {
			return
		}
		if u.Role != "admin" && !u.Member {
			c.JSON(403, gin.H{"error": "membership_required"})
			return
		}
		id, ok := videoID(c)
		if !ok {
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
		var req struct {
			accountOwner
			Position         *int   `json:"position"`
			ExpectedRevision *int64 `json:"expectedRevision"`
		}
		if c.ShouldBindJSON(&req) != nil || req.Position == nil || *req.Position < 0 || *req.Position > 2147483647 || req.ExpectedRevision == nil || *req.ExpectedRevision < 0 || *req.ExpectedRevision >= 9007199254740991 {
			c.JSON(400, gin.H{"error": "invalid_request"})
			return
		}
		if !checkOwner(c, u, req.ExpectedUserID) {
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()
		var revision int64
		var position int
		var when time.Time
		err := db.QueryRow(ctx, `WITH available AS (SELECT id,LEAST($3,duration_seconds) AS position FROM videos
 WHERE id=$2 AND status='ready' AND deleted_at IS NULL AND (publication_status='published' OR $4))
 INSERT INTO watch_history(user_id,video_id,position_seconds,revision)
 SELECT $1,id,position,$5+1 FROM available
 WHERE $5=0 OR EXISTS(SELECT 1 FROM watch_history WHERE user_id=$1 AND video_id=$2 AND revision=$5)
 ON CONFLICT(user_id,video_id) DO UPDATE SET position_seconds=EXCLUDED.position_seconds,watched_at=NOW(),revision=watch_history.revision+1
 WHERE watch_history.revision=$5 RETURNING revision,position_seconds,watched_at`, u.ID, id, *req.Position, u.Role == "admin", *req.ExpectedRevision).Scan(&revision, &position, &when)
		if errors.Is(err, pgx.ErrNoRows) {
			var available bool
			err = db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM videos WHERE id=$1 AND status='ready' AND deleted_at IS NULL AND (publication_status='published' OR $2))`, id, u.Role == "admin").Scan(&available)
			if err != nil {
				accountFailure(c, err)
				return
			}
			if !available {
				c.JSON(404, gin.H{"error": "video_not_found"})
			} else {
				c.JSON(409, gin.H{"error": "progress_conflict"})
			}
			return
		}
		if err != nil {
			accountFailure(c, err)
			return
		}
		c.JSON(200, gin.H{"userId": u.ID, "history": historyEntry{VideoID: id, Position: position, WatchedAt: when.UnixMilli(), Revision: revision}})
	}
}
