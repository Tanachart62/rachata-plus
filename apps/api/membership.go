package main

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// One deterministic active subscription drives both playback and account UI.
const membershipJoin = ` LEFT JOIN LATERAL (
 SELECT plan, expires_at FROM subscriptions WHERE user_id=u.id AND status='active'
 AND starts_at<=NOW() AND expires_at>NOW() ORDER BY expires_at DESC,id DESC LIMIT 1
) m ON true `
const membershipColumns = `m.expires_at IS NOT NULL,COALESCE(m.plan,''),COALESCE((EXTRACT(EPOCH FROM m.expires_at)*1000)::bigint,0),u.membership_revision,u.account_status`

func readMembershipAccount(ctx context.Context, db rowQuerier, id int64) (accountResponse, error) {
	var u accountResponse
	err := db.QueryRow(ctx, `SELECT u.id,u.username,u.name,u.email,u.role,`+membershipColumns+
		` FROM users u `+membershipJoin+` WHERE u.id=$1`, id).
		Scan(&u.ID, &u.Username, &u.Name, &u.Email, &u.Role, &u.Member, &u.Plan, &u.ExpiresAt, &u.MembershipRevision, &u.AccountStatus)
	return u, err
}

func (a *authConfig) membershipRoutes(g *gin.RouterGroup, db *pgxpool.Pool) {
	g.GET("/admin/memberships/:id/sessions", a.listSessions(db, false))
	g.GET("/admin/memberships/accounts", a.listMembershipAccounts(db))
	g.GET("/admin/memberships", a.findMembership(db))
	g.POST("/admin/memberships/:id", a.manageMembership(db))
}

func (a *authConfig) listMembershipAccounts(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		actor, ok := requireSession(c, db)
		if !ok {
			return
		}
		if actor.Role != "admin" {
			c.JSON(403, gin.H{"error": "admin_required"})
			return
		}
		query := strings.ToLower(strings.TrimSpace(c.Query("q")))
		var cursor int64
		var err error
		if value := c.Query("cursor"); value != "" {
			cursor, err = strconv.ParseInt(value, 10, 64)
		}
		if err != nil || cursor < 0 || len(query) > 254 {
			c.JSON(400, gin.H{"error": "invalid_request"})
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()
		rows, err := db.Query(ctx, `SELECT u.id,u.username,u.name,u.email,u.role,`+membershipColumns+
			` FROM users u `+membershipJoin+` WHERE u.id>$1 AND ($2='' OR
			 strpos(lower(u.name || ' ' || u.username || ' ' || u.email),$2)>0) ORDER BY u.id LIMIT 51`, cursor, query)
		if err != nil {
			accountFailure(c, err)
			return
		}
		defer rows.Close()
		users := make([]accountResponse, 0)
		for rows.Next() {
			var u accountResponse
			if err = rows.Scan(&u.ID, &u.Username, &u.Name, &u.Email, &u.Role, &u.Member, &u.Plan, &u.ExpiresAt, &u.MembershipRevision, &u.AccountStatus); err != nil {
				accountFailure(c, err)
				return
			}
			users = append(users, u)
		}
		if err = rows.Err(); err != nil {
			accountFailure(c, err)
			return
		}
		next := ""
		if len(users) > 50 {
			users = users[:50]
			next = strconv.FormatInt(users[49].ID, 10)
		}
		c.JSON(200, gin.H{"users": users, "nextCursor": next, "actorId": actor.ID})
	}
}

func (a *authConfig) findMembership(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		actor, ok := requireSession(c, db)
		if !ok {
			return
		}
		if actor.Role != "admin" {
			c.JSON(403, gin.H{"error": "admin_required"})
			return
		}
		email := strings.ToLower(strings.TrimSpace(c.Query("email")))
		if len(email) == 0 || len(email) > 254 {
			c.JSON(400, gin.H{"error": "invalid_request"})
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()
		var id int64
		err := db.QueryRow(ctx, `SELECT id FROM users WHERE lower(trim(email))=$1`, email).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(404, gin.H{"error": "user_not_found"})
			return
		}
		if err != nil {
			accountFailure(c, err)
			return
		}
		u, err := readMembershipAccount(ctx, db, id)
		if err != nil {
			accountFailure(c, err)
			return
		}
		rows, err := db.Query(ctx, `SELECT e.action,COALESCE(e.plan,''),COALESCE((EXTRACT(EPOCH FROM e.expires_at)*1000)::bigint,0),
          (EXTRACT(EPOCH FROM e.created_at)*1000)::bigint,a.username,COALESCE(e.session_id::text,'')
          FROM membership_events e JOIN users a ON a.id=e.actor_id WHERE e.user_id=$1 ORDER BY e.id DESC LIMIT 20`, id)
		if err != nil {
			accountFailure(c, err)
			return
		}
		defer rows.Close()
		events := make([]membershipEvent, 0)
		for rows.Next() {
			var e membershipEvent
			if err = rows.Scan(&e.Action, &e.Plan, &e.ExpiresAt, &e.CreatedAt, &e.Actor, &e.SessionID); err != nil {
				accountFailure(c, err)
				return
			}
			events = append(events, e)
		}
		if err = rows.Err(); err != nil {
			accountFailure(c, err)
			return
		}
		c.JSON(200, gin.H{"user": u, "events": events, "actorId": actor.ID})
	}
}

type membershipEvent struct {
	SessionID string `json:"sessionId"`
	Action    string `json:"action"`
	Plan      string `json:"plan"`
	ExpiresAt int64  `json:"expiresAt"`
	CreatedAt int64  `json:"createdAt"`
	Actor     string `json:"actor"`
}

func (a *authConfig) manageMembership(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Authorize before parsing target data. Recheck the session under lock below.
		actor, ok := requireSession(c, db)
		if !ok {
			return
		}
		if actor.Role != "admin" {
			c.JSON(403, gin.H{"error": "admin_required"})
			return
		}
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
		var req struct {
			accountOwner
			Action    string `json:"action"`
			SessionID string `json:"sessionId"`
			Plan      string `json:"plan"`
			Days      int    `json:"days"`
			Revision  *int64 `json:"expectedRevision"`
		}
		if err != nil || id < 1 || c.ShouldBindJSON(&req) != nil || req.Revision == nil || *req.Revision < 0 ||
			(req.Action != "grant" && req.Action != "revoke" && req.Action != "suspend" && req.Action != "reactivate" && req.Action != "logout_all" && req.Action != "logout_session") ||
			(req.Action == "grant" && (req.Days < 1 || req.Days > 365 || (req.Plan != "basic" && req.Plan != "standard" && req.Plan != "premium"))) {
			c.JSON(400, gin.H{"error": "invalid_request"})
			return
		}
		var deviceID int64
		if req.Action == "logout_session" {
			deviceID, err = strconv.ParseInt(req.SessionID, 10, 64)
			if err != nil || deviceID < 1 {
				c.JSON(400, gin.H{"error": "invalid_request"})
				return
			}
		}
		if !checkOwner(c, actor, req.ExpectedUserID) {
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		tx, err := db.Begin(ctx)
		if err != nil {
			accountFailure(c, err)
			return
		}
		defer rollback(tx)
		// Stable lock order allows two admins to edit each other without deadlock.
		rows, err := tx.Query(ctx, `SELECT id FROM users WHERE id=$1 OR id=$2 ORDER BY id FOR UPDATE`, actor.ID, id)
		if err != nil {
			accountFailure(c, err)
			return
		}
		found := false
		for rows.Next() {
			var locked int64
			err = rows.Scan(&locked)
			if err != nil {
				break
			}
			if locked == id {
				found = true
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
		if !found {
			c.JSON(404, gin.H{"error": "user_not_found"})
			return
		}
		target, err := readMembershipAccount(ctx, tx, id)
		if err != nil {
			accountFailure(c, err)
			return
		}
		if target.MembershipRevision != *req.Revision {
			c.JSON(409, gin.H{"error": "membership_conflict"})
			return
		}
		if req.Action == "suspend" && id == actor.ID {
			c.JSON(400, gin.H{"error": "cannot_suspend_self"})
			return
		}
		if req.Action == "grant" && target.AccountStatus == "suspended" {
			c.JSON(409, gin.H{"error": "account_suspended"})
			return
		}
		var now time.Time
		if err = tx.QueryRow(ctx, `SELECT NOW()`).Scan(&now); err != nil {
			accountFailure(c, err)
			return
		}
		var expiry any
		auditPlan := ""
		if req.Action == "grant" {
			base := now
			if target.Member {
				base = time.UnixMilli(target.ExpiresAt)
			}
			until := base.Add(time.Duration(req.Days) * 24 * time.Hour)
			if until.After(now.Add(730 * 24 * time.Hour)) {
				c.JSON(400, gin.H{"error": "membership_too_long"})
				return
			}
			expiry = until
			auditPlan = req.Plan
		}
		switch req.Action {
		case "grant", "revoke":
			_, err = tx.Exec(ctx, `UPDATE subscriptions SET status='revoked',updated_at=NOW() WHERE user_id=$1 AND status='active'`, id)
		case "suspend", "reactivate":
			status := "suspended"
			if req.Action == "reactivate" {
				status = "active"
			}
			_, err = tx.Exec(ctx, `UPDATE users SET account_status=$2 WHERE id=$1`, id, status)
		}
		if err == nil && req.Action == "logout_session" {
			result, deleteErr := tx.Exec(ctx, `DELETE FROM sessions WHERE session_id=$1 AND user_id=$2 AND expires_at>NOW()`, deviceID, id)
			err = deleteErr
			if err == nil && result.RowsAffected() == 0 {
				c.JSON(404, gin.H{"error": "session_not_found"})
				return
			}
		}
		if err == nil && (req.Action == "suspend" || req.Action == "logout_all") {
			_, err = tx.Exec(ctx, `DELETE FROM sessions WHERE user_id=$1`, id)
		}
		if err == nil && req.Action == "grant" {
			_, err = tx.Exec(ctx, `INSERT INTO subscriptions(user_id,plan,starts_at,expires_at) VALUES($1,$2,NOW(),$3)`, id, req.Plan, expiry)
		}
		if err == nil {
			_, err = tx.Exec(ctx, `UPDATE users SET membership_revision=membership_revision+1,updated_at=NOW() WHERE id=$1`, id)
		}
		if err == nil {
			_, err = tx.Exec(ctx, `INSERT INTO membership_events(user_id,actor_id,action,plan,expires_at,revision,session_id)
            VALUES($1,$2,$3,NULLIF($4,''),$5,$6,NULLIF($7::bigint,0))`, id, actor.ID, req.Action, auditPlan, expiry, target.MembershipRevision+1, deviceID)
		}
		if err == nil {
			target, err = readMembershipAccount(ctx, tx, id)
		}
		if err == nil {
			err = tx.Commit(ctx)
		}
		if err != nil {
			accountFailure(c, err)
			return
		}
		c.JSON(200, gin.H{"user": target, "actorId": actor.ID})
	}
}
