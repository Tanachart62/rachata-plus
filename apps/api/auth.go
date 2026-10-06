package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const sessionCookie = "rachata_session"
const sessionLifetime = 24 * time.Hour

type attemptWindow struct {
	count int
	reset time.Time
}
type authConfig struct {
	origin            string
	secure            bool
	dummyHash         string
	mu                sync.Mutex
	attempts          map[string]attemptWindow
	slots             chan struct{}
	nextSweep         time.Time
	nextCapacitySweep time.Time
}

func newAuthConfig(origin, secureValue string) (*authConfig, error) {
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, fmt.Errorf("AUTH_ORIGIN must be an exact origin, for example http://localhost:5173")
	}
	if secureValue != "true" && secureValue != "false" {
		return nil, fmt.Errorf("COOKIE_SECURE must be true or false")
	}
	if secureValue == "false" && (parsed.Scheme != "http" || (parsed.Hostname() != "localhost" && parsed.Hostname() != "127.0.0.1" && parsed.Hostname() != "::1")) {
		return nil, fmt.Errorf("insecure cookies are allowed only for local HTTP development")
	}
	if secureValue == "true" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("secure cookies require an HTTPS AUTH_ORIGIN")
	}
	dummy, err := hashPassword("dummy-password-never-used-for-login")
	if err != nil {
		return nil, err
	}
	return &authConfig{origin: origin, secure: secureValue == "true", dummyHash: dummy, attempts: make(map[string]attemptWindow), slots: make(chan struct{}, 2)}, nil
}

// Same-origin JSON writes plus SameSite cookies protect against browser CSRF.
// A bounded local limiter also limits expensive password hashing requests.
func (a *authConfig) protectWrites() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		if c.Request.Method == http.MethodGet {
			c.Next()
			return
		}
		if c.GetHeader("Origin") != a.origin || c.GetHeader("Sec-Fetch-Site") == "cross-site" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "invalid_origin"})
			return
		}
		mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
		if err != nil || mediaType != "application/json" {
			c.AbortWithStatusJSON(http.StatusUnsupportedMediaType, gin.H{"error": "json_required"})
			return
		}
		if strings.HasSuffix(c.Request.URL.Path, "/logout") {
			c.Next()
			return
		}
		now := time.Now()
		a.mu.Lock()
		if !now.Before(a.nextSweep) {
			a.sweepAttempts(now)
		}
		ip := c.ClientIP()
		libraryWrite := strings.HasPrefix(c.Request.URL.Path, "/auth/history/") || strings.HasPrefix(c.Request.URL.Path, "/auth/watchlist/")
		limit := 10
		if libraryWrite {
			ip = "library:" + ip
			limit = 120
		}
		entry, exists := a.attempts[ip]
		if exists && !now.Before(entry.reset) {
			delete(a.attempts, ip)
			entry, exists = attemptWindow{}, false
		}
		if !exists && len(a.attempts) >= 10000 && !now.Before(a.nextCapacitySweep) {
			a.sweepAttempts(now)
		}
		denied := (!exists && len(a.attempts) >= 10000) || entry.count >= limit
		if !denied {
			if !exists {
				entry.reset = now.Add(time.Minute)
			}
			entry.count++
			a.attempts[ip] = entry
		}
		a.mu.Unlock()
		if denied {
			c.Header("Retry-After", "60")
			c.AbortWithStatusJSON(429, gin.H{"error": "too_many_requests"})
			return
		}
		if libraryWrite {
			c.Next()
			return
		}
		select {
		case a.slots <- struct{}{}:
			defer func() { <-a.slots }()
			c.Next()
		default:
			c.Header("Retry-After", "1")
			c.AbortWithStatusJSON(429, gin.H{"error": "too_many_requests"})
		}
	}
}

type accountResponse struct {
	AccountStatus      string `json:"accountStatus"`
	ID                 int64  `json:"id"`
	Username           string `json:"username"`
	Name               string `json:"name"`
	Email              string `json:"email"`
	Role               string `json:"role"`
	Member             bool   `json:"member"`
	Plan               string `json:"plan"`
	ExpiresAt          int64  `json:"expiresAt"`
	MembershipRevision int64  `json:"membershipRevision"`
}

func newSessionToken() (string, []byte, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	hash := sha256.Sum256([]byte(token))
	return token, hash[:], nil
}

func sessionHash(c *gin.Context) ([]byte, bool) {
	token, err := c.Cookie(sessionCookie)
	if err != nil || len(token) != 43 {
		return nil, false
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(token)
	if err != nil || len(raw) != 32 {
		return nil, false
	}
	hash := sha256.Sum256([]byte(token))
	return hash[:], true
}

func (a *authConfig) cookie(c *gin.Context, token string, age int) {
	expires := time.Now().Add(sessionLifetime)
	if age < 0 {
		expires = time.Unix(1, 0)
	}
	// Remove the legacy /auth cookie before widening scope to protected media/API.
	http.SetCookie(c.Writer, &http.Cookie{Name: sessionCookie, Value: "", Path: "/auth", HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteStrictMode, MaxAge: -1, Expires: time.Unix(1, 0)})
	http.SetCookie(c.Writer, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteStrictMode, MaxAge: age, Expires: expires})
}

func (a *authConfig) loginHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8*1024)
		var req struct {
			Email    string `json:"email" binding:"required,email,max=254"`
			Password string `json:"password" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || utf8.RuneCountInString(req.Password) > 128 {
			c.JSON(400, gin.H{"error": "invalid_request"})
			return
		}
		req.Email = strings.ToLower(strings.TrimSpace(req.Email))
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		var user accountResponse
		var passwordHash string
		tx, err := db.Begin(ctx)
		if err != nil {
			c.JSON(503, gin.H{"error": "database_unavailable"})
			return
		}
		defer tx.Rollback(context.Background())
		err = tx.QueryRow(ctx, `SELECT u.id,u.username,u.name,u.email,u.role,u.password_hash,`+membershipColumns+
			` FROM users u `+membershipJoin+` WHERE lower(trim(u.email))=$1 FOR UPDATE OF u`, req.Email).
			Scan(&user.ID, &user.Username, &user.Name, &user.Email, &user.Role, &passwordHash, &user.Member, &user.Plan, &user.ExpiresAt, &user.MembershipRevision, &user.AccountStatus)
		missing := errors.Is(err, pgx.ErrNoRows)
		if err != nil && !missing {
			c.JSON(503, gin.H{"error": "database_unavailable"})
			return
		}
		if missing {
			passwordHash = a.dummyHash
		}
		valid := verifyPassword(req.Password, passwordHash)
		if missing || !valid {
			c.JSON(401, gin.H{"error": "invalid_credentials"})
			return
		}
		if user.AccountStatus == "suspended" {
			c.JSON(403, gin.H{"error": "account_suspended"})
			return
		}
		token, hash, err := newSessionToken()
		if err != nil {
			c.JSON(500, gin.H{"error": "internal_error"})
			return
		}
		if old, ok := sessionHash(c); ok {
			if _, err = tx.Exec(ctx, `DELETE FROM sessions WHERE token_hash=$1`, old); err != nil {
				c.JSON(503, gin.H{"error": "database_unavailable"})
				return
			}
		}
		_, err = tx.Exec(ctx, `DELETE FROM sessions WHERE user_id=$1 AND expires_at<=NOW()`, user.ID)
		if err == nil {
			_, err = tx.Exec(ctx, `INSERT INTO sessions(token_hash,user_id,expires_at,user_agent) VALUES($1,$2,$3,$4)`, hash, user.ID, time.Now().Add(sessionLifetime), sessionAgent(c))
		}
		if err == nil {
			err = tx.Commit(ctx)
		}
		if err != nil {
			c.JSON(503, gin.H{"error": "database_unavailable"})
			return
		}
		a.cookie(c, token, int(sessionLifetime.Seconds()))
		c.JSON(200, gin.H{"user": user})
	}
}

func (a *authConfig) sweepAttempts(now time.Time) {
	for key, entry := range a.attempts {
		if !now.Before(entry.reset) {
			delete(a.attempts, key)
		}
	}
	a.nextSweep = now.Add(time.Minute)
	a.nextCapacitySweep = now.Add(time.Second)
}
func (a *authConfig) meHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		u, ok := requireSession(c, db)
		if ok {
			c.JSON(200, gin.H{"user": u})
		}
	}
}
func (a *authConfig) logoutHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
		var req accountOwner
		if c.ShouldBindJSON(&req) != nil {
			c.JSON(400, gin.H{"error": "invalid_request"})
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()
		hash, ok := sessionHash(c)
		if ok {
			u, err := readSessionUser(ctx, db, hash)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				accountFailure(c, err)
				return
			}
			if err == nil {
				if !checkOwner(c, u, req.ExpectedUserID) {
					return
				}
				if _, err = db.Exec(ctx, `DELETE FROM sessions WHERE token_hash=$1`, hash); err != nil {
					accountFailure(c, err)
					return
				}
			}
		}
		// Revoke server-side only: a late logout response must not erase a
		// newer login cookie established concurrently in another tab.
		c.Status(http.StatusNoContent)
	}
}
