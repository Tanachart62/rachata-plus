package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var errAccountChanged = errors.New("account_changed")

type accountOwner struct {
	ExpectedUserID int64 `json:"expectedUserId" binding:"required,gt=0"`
}
type lockedUser struct {
	User     accountResponse
	Password string
}
type rowQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func readSessionUser(ctx context.Context, db rowQuerier, hash []byte) (accountResponse, error) {
	var u accountResponse
	err := db.QueryRow(ctx, `SELECT u.id,u.username,u.name,u.email,u.role,`+membershipColumns+
		` FROM sessions t JOIN users u ON u.id=t.user_id `+membershipJoin+` WHERE t.token_hash=$1 AND t.expires_at>NOW() AND u.account_status='active'`, hash).
		Scan(&u.ID, &u.Username, &u.Name, &u.Email, &u.Role, &u.Member, &u.Plan, &u.ExpiresAt, &u.MembershipRevision, &u.AccountStatus)
	return u, err
}

func rollback(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}

// The caller owns the deadline. Recheck the session in a fresh statement after
// waiting for the user lock: the JOIN snapshot can predate session revocation.
func lockedAccount(ctx context.Context, db *pgxpool.Pool, hash []byte, expected int64) (pgx.Tx, lockedUser, error) {
	var u lockedUser
	tx, err := db.Begin(ctx)
	if err != nil {
		return nil, u, err
	}
	err = tx.QueryRow(ctx, `SELECT u.id,u.username,u.name,u.email,u.role,u.password_hash
 FROM users u JOIN sessions t ON t.user_id=u.id WHERE t.token_hash=$1 AND t.expires_at>NOW() FOR UPDATE OF u`, hash).
		Scan(&u.User.ID, &u.User.Username, &u.User.Name, &u.User.Email, &u.User.Role, &u.Password)
	if err == nil {
		u.User, err = readSessionUser(ctx, tx, hash)
	}
	if err == nil && u.User.ID != expected {
		err = errAccountChanged
	}
	if err != nil {
		rollback(tx)
		return nil, u, err
	}
	return tx, u, nil
}

func accountFailure(c *gin.Context, err error) {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		c.JSON(401, gin.H{"error": "unauthenticated"})
	case errors.Is(err, errAccountChanged):
		c.JSON(409, gin.H{"error": "account_changed"})
	default:
		c.JSON(503, gin.H{"error": "database_unavailable"})
	}
}
func requireSession(c *gin.Context, db *pgxpool.Pool) (accountResponse, bool) {
	hash, ok := sessionHash(c)
	if !ok {
		accountFailure(c, pgx.ErrNoRows)
		return accountResponse{}, false
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	u, err := readSessionUser(ctx, db, hash)
	if err != nil {
		accountFailure(c, err)
		return u, false
	}
	return u, true
}
func checkOwner(c *gin.Context, u accountResponse, expected int64) bool {
	if u.ID != expected {
		accountFailure(c, errAccountChanged)
		return false
	}
	return true
}
func (a *authConfig) accountRoutes(g *gin.RouterGroup, db *pgxpool.Pool) {
	a.membershipRoutes(g, db)
	g.GET("/sessions", a.listSessions(db, true))
	g.POST("/sessions/:sessionId/revoke", a.revokeOwnSession(db))
	g.POST("/profile", a.updateProfile(db))
	g.POST("/password", a.changePassword(db))
	g.GET("/library", a.library(db))
	g.GET("/watchlist/:id", a.savedVideo(db))
	g.POST("/watchlist/:id", a.saveVideo(db))
	g.GET("/history/:id", a.readProgress(db))
	g.POST("/history/:id", a.watchProgress(db))
}
func (a *authConfig) replaceSessions(ctx context.Context, tx pgx.Tx, id int64, agent string) (string, error) {
	token, hash, err := newSessionToken()
	if err != nil {
		return "", err
	}
	_, err = tx.Exec(ctx, `DELETE FROM sessions WHERE user_id=$1`, id)
	if err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO sessions(token_hash,user_id,expires_at,user_agent) VALUES($1,$2,$3,$4)`, hash, id, time.Now().Add(sessionLifetime), agent)
	}
	return token, err
}
func beginAccountMutation(c *gin.Context, db *pgxpool.Pool, ctx context.Context, expected int64) (pgx.Tx, lockedUser, bool) {
	hash, ok := sessionHash(c)
	if !ok {
		accountFailure(c, pgx.ErrNoRows)
		return nil, lockedUser{}, false
	}
	tx, u, err := lockedAccount(ctx, db, hash, expected)
	if err != nil {
		accountFailure(c, err)
		return nil, u, false
	}
	return tx, u, true
}
func (a *authConfig) updateProfile(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, ok := sessionHash(c); !ok {
			accountFailure(c, pgx.ErrNoRows)
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8192)
		var req struct {
			accountOwner
			Name            string `json:"name" binding:"required"`
			Email           string `json:"email" binding:"required,email,max=254"`
			CurrentPassword string `json:"currentPassword"`
		}
		if c.ShouldBindJSON(&req) != nil || utf8.RuneCountInString(req.CurrentPassword) > 128 {
			c.JSON(400, gin.H{"error": "invalid_request"})
			return
		}
		req.Name, req.Email = strings.TrimSpace(req.Name), strings.ToLower(strings.TrimSpace(req.Email))
		if n := utf8.RuneCountInString(req.Name); n < 1 || n > 100 {
			c.JSON(400, gin.H{"error": "invalid_name"})
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		tx, locked, ok := beginAccountMutation(c, db, ctx, req.ExpectedUserID)
		if !ok {
			return
		}
		defer rollback(tx)
		u := locked.User
		changed := req.Email != u.Email
		if changed && !verifyPassword(req.CurrentPassword, locked.Password) {
			c.JSON(400, gin.H{"error": "invalid_current_password"})
			return
		}
		_, err := tx.Exec(ctx, `UPDATE users SET name=$2,email=$3,updated_at=NOW() WHERE id=$1`, u.ID, req.Name, req.Email)
		var token string
		if err == nil && changed {
			token, err = a.replaceSessions(ctx, tx, u.ID, sessionAgent(c))
		}
		if err == nil {
			err = tx.Commit(ctx)
		}
		if err != nil {
			var e *pgconn.PgError
			if errors.As(err, &e) && e.Code == "23505" && e.ConstraintName == "users_email_unique" {
				c.JSON(409, gin.H{"error": "email_already_exists"})
			} else {
				accountFailure(c, err)
			}
			return
		}
		if changed {
			a.cookie(c, token, int(sessionLifetime.Seconds()))
		}
		u.Name, u.Email = req.Name, req.Email
		c.JSON(200, gin.H{"user": u})
	}
}
func (a *authConfig) changePassword(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8192)
		var req struct {
			accountOwner
			Current string `json:"currentPassword" binding:"required"`
			Next    string `json:"newPassword" binding:"required"`
		}
		if c.ShouldBindJSON(&req) != nil || utf8.RuneCountInString(req.Current) > 128 {
			c.JSON(400, gin.H{"error": "invalid_request"})
			return
		}
		if n := utf8.RuneCountInString(req.Next); n < 15 || n > 128 {
			c.JSON(400, gin.H{"error": "invalid_password_length"})
			return
		}
		if req.Current == req.Next {
			c.JSON(400, gin.H{"error": "password_unchanged"})
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		tx, u, ok := beginAccountMutation(c, db, ctx, req.ExpectedUserID)
		if !ok {
			return
		}
		defer rollback(tx)
		if !verifyPassword(req.Current, u.Password) {
			c.JSON(400, gin.H{"error": "invalid_current_password"})
			return
		}
		next, err := hashPassword(req.Next)
		if err != nil {
			c.JSON(500, gin.H{"error": "internal_error"})
			return
		}
		_, err = tx.Exec(ctx, `UPDATE users SET password_hash=$2,updated_at=NOW() WHERE id=$1`, u.User.ID, next)
		var token string
		if err == nil {
			token, err = a.replaceSessions(ctx, tx, u.User.ID, sessionAgent(c))
		}
		if err == nil {
			err = tx.Commit(ctx)
		}
		if err != nil {
			accountFailure(c, err)
			return
		}
		a.cookie(c, token, int(sessionLifetime.Seconds()))
		c.Status(204)
	}
}
