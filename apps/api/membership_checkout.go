package main

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func planRank(plan string) int {
	switch plan {
	case "basic":
		return 1
	case "standard":
		return 2
	case "premium":
		return 3
	default:
		return 0
	}
}

// Simulated payment is available only in the local environment. Production
// must grant paid membership from a verified payment-provider event instead.
func (a *authConfig) demoCheckout(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if envDefault("APP_ENV", "local") != "local" {
			c.JSON(403, gin.H{"error": "demo_checkout_disabled"})
			return
		}
		actor, ok := requireSession(c, db)
		if !ok {
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
		var req struct {
			accountOwner
			Plan     string `json:"plan"`
			Revision *int64 `json:"expectedRevision"`
		}
		if c.ShouldBindJSON(&req) != nil || planRank(req.Plan) == 0 || req.Revision == nil || *req.Revision < 0 {
			c.JSON(400, gin.H{"error": "invalid_request"})
			return
		}
		if !checkOwner(c, actor, req.ExpectedUserID) {
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		hash, _ := sessionHash(c)
		tx, locked, err := lockedAccount(ctx, db, hash, req.ExpectedUserID)
		if err != nil {
			accountFailure(c, err)
			return
		}
		defer rollback(tx)
		u := locked.User
		if u.Role == "admin" {
			c.JSON(403, gin.H{"error": "admin_membership_not_needed"})
			return
		}
		if u.MembershipRevision != *req.Revision {
			c.JSON(409, gin.H{"error": "membership_conflict"})
			return
		}
		if planRank(req.Plan) <= planRank(u.Plan) {
			c.JSON(409, gin.H{"error": "upgrade_only"})
			return
		}
		var expiry time.Time
		if u.Member {
			expiry = time.UnixMilli(u.ExpiresAt)
		} else if err = tx.QueryRow(ctx, `SELECT NOW()+interval '30 days'`).Scan(&expiry); err != nil {
			accountFailure(c, err)
			return
		}
		_, err = tx.Exec(ctx, `UPDATE subscriptions SET status='revoked',updated_at=NOW() WHERE user_id=$1 AND status='active'`, u.ID)
		if err == nil {
			_, err = tx.Exec(ctx, `INSERT INTO subscriptions(user_id,plan,starts_at,expires_at) VALUES($1,$2,NOW(),$3)`, u.ID, req.Plan, expiry)
		}
		if err == nil {
			_, err = tx.Exec(ctx, `UPDATE users SET membership_revision=membership_revision+1,updated_at=NOW() WHERE id=$1`, u.ID)
		}
		if err == nil {
			_, err = tx.Exec(ctx, `INSERT INTO membership_events(user_id,actor_id,action,plan,expires_at,revision) VALUES($1,$1,'grant',$2,$3,$4)`, u.ID, req.Plan, expiry, u.MembershipRevision+1)
		}
		if err == nil {
			u, err = readMembershipAccount(ctx, tx, u.ID)
		}
		if err == nil {
			err = tx.Commit(ctx)
		}
		if err != nil {
			accountFailure(c, err)
			return
		}
		c.JSON(200, gin.H{"user": u, "simulated": true})
	}
}
