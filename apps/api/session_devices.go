package main

import (
	"bytes"
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func sessionAgent(c *gin.Context) string {
	chars := []rune(strings.ToValidUTF8(c.Request.UserAgent(), ""))
	if len(chars) > 512 {
		chars = chars[:512]
	}
	return string(chars)
}

func deviceLabel(agent string) string {
	browser, os := "ไม่ทราบเบราว์เซอร์", "ไม่ทราบระบบปฏิบัติการ"
	switch {
	case strings.Contains(agent, "Edg/") || strings.Contains(agent, "EdgA/") || strings.Contains(agent, "EdgiOS/"):
		browser = "Edge"
	case strings.Contains(agent, "OPR/") || strings.Contains(agent, "OPiOS/"):
		browser = "Opera"
	case strings.Contains(agent, "Firefox/") || strings.Contains(agent, "FxiOS/"):
		browser = "Firefox"
	case strings.Contains(agent, "Chrome/") || strings.Contains(agent, "CriOS/"):
		browser = "Chrome"
	case strings.Contains(agent, "Safari/"):
		browser = "Safari"
	}
	switch {
	case strings.Contains(agent, "Android"):
		os = "Android"
	case strings.Contains(agent, "iPhone"):
		os = "iPhone / iOS"
	case strings.Contains(agent, "iPad"):
		os = "iPad / iPadOS"
	case strings.Contains(agent, "Windows NT"):
		os = "Windows"
	case strings.Contains(agent, "Macintosh"):
		os = "macOS"
	case strings.Contains(agent, "Linux"):
		os = "Linux"
	}
	return browser + " · " + os
}

type deviceSession struct {
	ID        string `json:"id"`
	Device    string `json:"device"`
	CreatedAt int64  `json:"createdAt"`
	ExpiresAt int64  `json:"expiresAt"`
	Current   bool   `json:"current"`
}

func (a *authConfig) listSessions(db *pgxpool.Pool, own bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		actor, ok := requireSession(c, db)
		if !ok {
			return
		}
		if !own && actor.Role != "admin" {
			c.JSON(403, gin.H{"error": "admin_required"})
			return
		}
		id := actor.ID
		var err error
		if !own {
			id, err = strconv.ParseInt(c.Param("id"), 10, 64)
		}
		var cursor int64
		if err == nil && c.Query("cursor") != "" {
			cursor, err = strconv.ParseInt(c.Query("cursor"), 10, 64)
		}
		if err != nil || id < 1 || cursor < 0 {
			c.JSON(400, gin.H{"error": "invalid_request"})
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()
		var exists bool
		if err = db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id=$1)`, id).Scan(&exists); err != nil {
			accountFailure(c, err)
			return
		}
		if !exists {
			c.JSON(404, gin.H{"error": "user_not_found"})
			return
		}
		rows, err := db.Query(ctx, `SELECT session_id::text,user_agent,(EXTRACT(EPOCH FROM created_at)*1000)::bigint,
   (EXTRACT(EPOCH FROM expires_at)*1000)::bigint,token_hash FROM sessions WHERE user_id=$1 AND expires_at>NOW()
   AND ($2::bigint=0 OR session_id<$2) ORDER BY session_id DESC LIMIT 51`, id, cursor)
		if err != nil {
			accountFailure(c, err)
			return
		}
		defer rows.Close()
		hash, _ := sessionHash(c)
		sessions := make([]deviceSession, 0)
		for rows.Next() {
			var s deviceSession
			var agent string
			var tokenHash []byte
			if err = rows.Scan(&s.ID, &agent, &s.CreatedAt, &s.ExpiresAt, &tokenHash); err != nil {
				accountFailure(c, err)
				return
			}
			s.Device = deviceLabel(agent)
			s.Current = bytes.Equal(hash, tokenHash)
			sessions = append(sessions, s)
		}
		if err = rows.Err(); err != nil {
			accountFailure(c, err)
			return
		}
		next := ""
		if len(sessions) > 50 {
			sessions = sessions[:50]
			next = sessions[49].ID
		}
		c.JSON(200, gin.H{"actorId": actor.ID, "userId": id, "sessions": sessions, "nextCursor": next})
	}
}

func (a *authConfig) revokeOwnSession(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("sessionId"), 10, 64)
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
		var req accountOwner
		if err != nil || id < 1 || c.ShouldBindJSON(&req) != nil {
			c.JSON(400, gin.H{"error": "invalid_request"})
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		tx, locked, ok := beginAccountMutation(c, db, ctx, req.ExpectedUserID)
		if !ok {
			return
		}
		defer rollback(tx)
		result, err := tx.Exec(ctx, `DELETE FROM sessions WHERE session_id=$1 AND user_id=$2 AND expires_at>NOW()`, id, locked.User.ID)
		if err != nil {
			accountFailure(c, err)
			return
		}
		if result.RowsAffected() == 0 {
			c.JSON(404, gin.H{"error": "session_not_found"})
			return
		}
		_, err = tx.Exec(ctx, `UPDATE users SET membership_revision=membership_revision+1,updated_at=NOW() WHERE id=$1`, locked.User.ID)
		if err == nil {
			_, err = tx.Exec(ctx, `INSERT INTO membership_events(user_id,actor_id,action,revision,session_id)
    VALUES($1,$1,'logout_session',$2,$3)`, locked.User.ID, locked.User.MembershipRevision+1, id)
		}
		if err == nil {
			err = tx.Commit(ctx)
		}
		if err != nil {
			accountFailure(c, err)
			return
		}
		locked.User.MembershipRevision++
		c.JSON(200, gin.H{"user": locked.User, "actorId": locked.User.ID})
	}
}
