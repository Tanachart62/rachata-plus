package main

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Account deletion erases profile/library data and revokes access. A pseudonymous
// row remains so previous administration events retain their audit references.
func (a *authConfig) controlAccount(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		actor, ok := requireSession(c, db)
		if !ok {
			return
		}
		if actor.Role != "admin" {
			c.JSON(403, gin.H{"error": "admin_required"})
			return
		}
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
		var req struct {
			accountOwner
			Action       string `json:"action"`
			Level        string `json:"level"`
			Password     string `json:"currentPassword"`
			NewPassword  string `json:"newPassword"`
			ConfirmEmail string `json:"confirmEmail"`
			Revision     *int64 `json:"expectedRevision"`
		}
		if err != nil || id < 1 || c.ShouldBindJSON(&req) != nil || req.Revision == nil || *req.Revision < 0 ||
			(req.Action != "set_role" && req.Action != "reset_password" && req.Action != "delete_account") ||
			utf8.RuneCountInString(req.Password) > 128 || req.Password == "" ||
			(req.Action == "set_role" && req.Level != "user" && req.Level != "admin" && req.Level != "owner") {
			c.JSON(400, gin.H{"error": "invalid_request"})
			return
		}
		if !checkOwner(c, actor, req.ExpectedUserID) {
			return
		}
		if id == actor.ID {
			c.JSON(400, gin.H{"error": "cannot_control_self"})
			return
		}
		var newHash string
		if req.Action == "reset_password" {
			if n := utf8.RuneCountInString(req.NewPassword); n < 8 || n > 128 {
				c.JSON(400, gin.H{"error": "invalid_password_length"})
				return
			}
			newHash, err = hashPassword(req.NewPassword)
			if err != nil {
				c.JSON(500, gin.H{"error": "internal_error"})
				return
			}
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		tx, err := db.Begin(ctx)
		if err != nil {
			accountFailure(c, err)
			return
		}
		defer rollback(tx)
		rows, err := tx.Query(ctx, `SELECT id FROM users WHERE id=$1 OR id=$2 ORDER BY id FOR UPDATE`, actor.ID, id)
		if err != nil {
			accountFailure(c, err)
			return
		}
		for rows.Next() {
			var locked int64
			if err = rows.Scan(&locked); err != nil {
				break
			}
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil {
			accountFailure(c, err)
			return
		}
		hash, _ := sessionHash(c)
		var passwordHash string
		err = tx.QueryRow(ctx, `SELECT u.password_hash FROM users u JOIN sessions t ON t.user_id=u.id
		 WHERE u.id=$1 AND t.token_hash=$2 AND t.expires_at>NOW() AND u.account_status='active' FOR UPDATE OF t`, actor.ID, hash).Scan(&passwordHash)
		if err != nil {
			accountFailure(c, err)
			return
		}
		fresh, err := readSessionUser(ctx, tx, hash)
		if err != nil {
			accountFailure(c, err)
			return
		}
		if !checkOwner(c, fresh, req.ExpectedUserID) {
			return
		}
		if fresh.Role != "admin" {
			c.JSON(403, gin.H{"error": "admin_required"})
			return
		}
		target, err := readMembershipAccount(ctx, tx, id)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && target.AccountStatus == "deleted") {
			c.JSON(404, gin.H{"error": "user_not_found"})
			return
		}
		if err != nil {
			accountFailure(c, err)
			return
		}
		if fresh.AdminLevel != "owner" && (req.Action != "reset_password" || target.Role == "admin") {
			c.JSON(403, gin.H{"error": "owner_required"})
			return
		}
		if !verifyPassword(req.Password, passwordHash) {
			c.JSON(400, gin.H{"error": "invalid_current_password"})
			return
		}
		if target.MembershipRevision != *req.Revision {
			c.JSON(409, gin.H{"error": "membership_conflict"})
			return
		}
		if req.Action == "delete_account" && strings.ToLower(strings.TrimSpace(req.ConfirmEmail)) != target.Email {
			c.JSON(400, gin.H{"error": "confirmation_mismatch"})
			return
		}
		switch req.Action {
		case "set_role":
			role, level := "admin", req.Level
			if level == "user" {
				role, level = "user", ""
			}
			if target.Role == role && target.AdminLevel == level {
				c.JSON(409, gin.H{"error": "role_unchanged"})
				return
			}
			_, err = tx.Exec(ctx, `UPDATE users SET role=$2,admin_level=$3 WHERE id=$1`, id, role, level)
		case "reset_password":
			_, err = tx.Exec(ctx, `UPDATE users SET password_hash=$2 WHERE id=$1`, id, newHash)
		case "delete_account":
			// Keep no reusable login identifier or password for a deleted account.
			nonce, _, tokenErr := newSessionToken()
			if tokenErr != nil {
				c.JSON(500, gin.H{"error": "internal_error"})
				return
			}
			_, err = tx.Exec(ctx, `UPDATE users SET account_status='deleted',role='user',admin_level='',
			 username=$3,name='บัญชีที่ลบแล้ว',email=$2,password_hash='',avatar_jpeg=NULL WHERE id=$1`, id, nonce+"@deleted.invalid", "deleted_"+strings.ToLower(strings.ReplaceAll(nonce, "-", "_"))[:22])
			if err == nil {
				_, err = tx.Exec(ctx, `DELETE FROM watchlist WHERE user_id=$1`, id)
			}
			if err == nil {
				_, err = tx.Exec(ctx, `DELETE FROM watch_history WHERE user_id=$1`, id)
			}
			if err == nil {
				_, err = tx.Exec(ctx, `UPDATE subscriptions SET status='revoked',updated_at=NOW() WHERE user_id=$1`, id)
			}
		}
		if err == nil {
			_, err = tx.Exec(ctx, `DELETE FROM sessions WHERE user_id=$1`, id)
		}
		if err == nil {
			_, err = tx.Exec(ctx, `UPDATE users SET membership_revision=membership_revision+1,updated_at=NOW() WHERE id=$1`, id)
		}
		if err == nil {
			_, err = tx.Exec(ctx, `INSERT INTO membership_events(user_id,actor_id,action,revision,admin_level)
		 VALUES($1,$2,$3,$4,NULLIF($5,''))`, id, fresh.ID, req.Action, target.MembershipRevision+1, req.Level)
		}
		if err == nil && req.Action != "delete_account" {
			target, err = readMembershipAccount(ctx, tx, id)
		}
		if err == nil {
			err = tx.Commit(ctx)
		}
		if err != nil {
			accountFailure(c, err)
			return
		}
		if req.Action == "delete_account" {
			c.JSON(200, gin.H{"deleted": true, "userId": id, "actorId": fresh.ID})
			return
		}
		c.JSON(200, gin.H{"user": target, "actorId": fresh.ID})
	}
}
