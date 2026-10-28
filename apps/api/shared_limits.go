package main

import (
	"context"
	"crypto/sha256"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"strings"
	"time"
)

// A shared production limiter plus the local Argon2 semaphore bounds all nodes.
func (a *authConfig) sharedWriteLimit(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodGet || strings.HasSuffix(c.Request.URL.Path, "/logout") {
			c.Next()
			return
		}
		if c.GetHeader("Origin") != a.origin || c.GetHeader("Sec-Fetch-Site") == "cross-site" {
			c.AbortWithStatusJSON(403, gin.H{"error": "invalid_origin"})
			return
		}
		group := "account"
		limit := 10
		if strings.HasPrefix(c.Request.URL.Path, "/auth/history/") || strings.HasPrefix(c.Request.URL.Path, "/auth/watchlist/") {
			group = "library"
			limit = 120
		}
		key := sha256.Sum256([]byte(group + ":" + c.ClientIP()))
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()
		var hits int
		err := db.QueryRow(ctx, `INSERT INTO request_limits(key_hash,hits,resets_at) VALUES($1,1,NOW()+interval '1 minute')
   ON CONFLICT(key_hash) DO UPDATE SET hits=CASE WHEN request_limits.resets_at<=NOW() THEN 1 ELSE request_limits.hits+1 END,
   resets_at=CASE WHEN request_limits.resets_at<=NOW() THEN NOW()+interval '1 minute' ELSE request_limits.resets_at END RETURNING hits`, key[:]).Scan(&hits)
		if err != nil {
			c.AbortWithStatusJSON(503, gin.H{"error": "database_unavailable"})
			return
		}
		if hits > limit {
			c.Header("Retry-After", "60")
			c.AbortWithStatusJSON(429, gin.H{"error": "too_many_requests"})
			return
		}
		c.Next()
	}
}
