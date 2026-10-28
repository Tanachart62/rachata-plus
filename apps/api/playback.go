package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

const playbackLifetime = 90 * time.Second

type playbackLimits struct{ Height, Screens int }

func limitsFor(u accountResponse) playbackLimits {
	if u.Role == "admin" {
		return playbackLimits{2160, 0}
	}
	if !u.Member {
		return playbackLimits{}
	}
	switch u.Plan {
	case "basic":
		return playbackLimits{1080, 2}
	case "standard":
		return playbackLimits{1440, 2}
	case "premium":
		return playbackLimits{2160, 4}
	default:
		return playbackLimits{}
	}
}

// Each open player owns a short lease, bound to its login session and video.
// User-row locking serializes acquisition with other players and revocation.
func (s *videoService) playback(c *gin.Context) {
	if c.GetHeader("Origin") != s.auth.origin || c.GetHeader("Sec-Fetch-Site") == "cross-site" {
		c.JSON(403, gin.H{"error": "invalid_origin"})
		return
	}
	mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || mediaType != "application/json" {
		c.JSON(415, gin.H{"error": "json_required"})
		return
	}
	id, ok := videoID(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	var req struct {
		accountOwner
		Token string `json:"token"`
	}
	if c.ShouldBindJSON(&req) != nil {
		c.JSON(400, gin.H{"error": "invalid_request"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	tx, locked, ok := beginAccountMutation(c, s.db, ctx, req.ExpectedUserID)
	if !ok {
		return
	}
	defer rollback(tx)
	session, _ := sessionHash(c)
	if c.Request.Method == http.MethodDelete {
		hash, valid := playbackHash(req.Token)
		if !valid {
			c.JSON(400, gin.H{"error": "invalid_request"})
			return
		}
		_, err := tx.Exec(ctx, `DELETE FROM playback_leases WHERE token_hash=$1 AND session_hash=$2 AND user_id=$3 AND video_id=$4`, hash, session, locked.User.ID, id)
		if err == nil {
			err = tx.Commit(ctx)
		}
		if err != nil {
			accountFailure(c, err)
			return
		}
		c.Status(204)
		return
	}
	limits := limitsFor(locked.User)
	if limits.Height == 0 {
		c.JSON(403, gin.H{"error": "membership_required"})
		return
	}
	var generation int64
	err = tx.QueryRow(ctx, `SELECT hls_generation FROM videos WHERE id=$1 AND storage_key IS NOT NULL AND deleted_at IS NULL AND status='ready' AND ($2 OR publication_status='published')`, id, locked.User.Role == "admin").Scan(&generation)
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(404, gin.H{"error": "video_not_found"})
		return
	}
	if err != nil {
		accountFailure(c, err)
		return
	}
	token := req.Token
	var hash []byte
	if token != "" {
		var valid bool
		hash, valid = playbackHash(token)
		if !valid {
			c.JSON(400, gin.H{"error": "invalid_request"})
			return
		}
		var savedGeneration int64
		err = tx.QueryRow(ctx, `SELECT generation FROM playback_leases WHERE token_hash=$1 AND session_hash=$2 AND user_id=$3 AND video_id=$4 AND expires_at>NOW()`, hash, session, locked.User.ID, id).Scan(&savedGeneration)
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(409, gin.H{"error": "playback_expired"})
			return
		}
		if err != nil {
			accountFailure(c, err)
			return
		}
		if generation != savedGeneration {
			c.JSON(409, gin.H{"error": "video_changed"})
			return
		}
		// A downgrade by an administrator immediately limits existing players too.
		if ok, err := leaseWithinLimit(ctx, tx, hash, locked.User.ID, limits.Screens); err != nil {
			accountFailure(c, err)
			return
		} else if !ok {
			c.JSON(409, gin.H{"error": "stream_limit_reached"})
			return
		}
	} else {
		if _, err = tx.Exec(ctx, `DELETE FROM playback_leases WHERE expires_at<=NOW() AND user_id=$1`, locked.User.ID); err != nil {
			accountFailure(c, err)
			return
		}
		var count int
		if err = tx.QueryRow(ctx, `SELECT COUNT(*) FROM playback_leases WHERE user_id=$1 AND expires_at>NOW()`, locked.User.ID).Scan(&count); err != nil {
			accountFailure(c, err)
			return
		}
		if limits.Screens > 0 && count >= limits.Screens {
			c.JSON(409, gin.H{"error": "stream_limit_reached", "screens": limits.Screens})
			return
		}
		token, hash, err = newSessionToken()
		if err != nil {
			c.JSON(500, gin.H{"error": "internal_error"})
			return
		}
		_, err = tx.Exec(ctx, `INSERT INTO playback_leases(token_hash,session_hash,user_id,video_id,generation,expires_at) VALUES($1,$2,$3,$4,$5,NOW()+interval '90 seconds')`, hash, session, locked.User.ID, id, generation)
		if err != nil {
			accountFailure(c, err)
			return
		}
	}
	_, err = tx.Exec(ctx, `UPDATE playback_leases SET expires_at=NOW()+interval '90 seconds' WHERE token_hash=$1`, hash)
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		accountFailure(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(200, gin.H{"token": token, "url": fmt.Sprintf("/media/%d/full/index.m3u8?playback=%s", id, url.QueryEscape(token)), "maxHeight": limits.Height, "screens": limits.Screens, "expiresIn": int(playbackLifetime.Seconds())})
}

func playbackHash(token string) ([]byte, bool) {
	// Session tokens are generated as 32 random bytes encoded with RawURLEncoding.
	if len(token) != 43 {
		return nil, false
	}
	for _, c := range token {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return nil, false
		}
	}
	h := sha256.Sum256([]byte(token))
	return h[:], true
}
func leaseWithinLimit(ctx context.Context, db rowQuerier, hash []byte, user int64, limit int) (bool, error) {
	if limit == 0 {
		return true, nil
	}
	var allowed bool
	err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM (SELECT token_hash FROM playback_leases WHERE user_id=$1 AND expires_at>NOW() ORDER BY created_at,token_hash LIMIT $2) p WHERE token_hash=$3)`, user, limit, hash).Scan(&allowed)
	return allowed, err
}
func (s *videoService) authorizePlayback(c *gin.Context, ctx context.Context, u accountResponse, id, generation int64) bool {
	hash, ok := playbackHash(c.Query("playback"))
	if !ok {
		c.JSON(403, gin.H{"error": "playback_required"})
		return false
	}
	session, _ := sessionHash(c)
	var saved int64
	err := s.db.QueryRow(ctx, `SELECT p.generation FROM playback_leases p JOIN sessions t ON t.token_hash=p.session_hash JOIN users u ON u.id=p.user_id WHERE p.token_hash=$1 AND p.session_hash=$2 AND p.user_id=$3 AND p.video_id=$4 AND p.expires_at>NOW() AND t.expires_at>NOW() AND u.account_status='active'`, hash, session, u.ID, id).Scan(&saved)
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(403, gin.H{"error": "playback_expired"})
		return false
	}
	if err != nil {
		accountFailure(c, err)
		return false
	}
	if saved != generation {
		c.JSON(409, gin.H{"error": "video_changed"})
		return false
	}
	allowed, err := leaseWithinLimit(ctx, s.db, hash, u.ID, limitsFor(u).Screens)
	if err != nil {
		accountFailure(c, err)
		return false
	}
	if !allowed {
		c.JSON(409, gin.H{"error": "stream_limit_reached"})
		return false
	}
	return true
}

var heightInName = regexp.MustCompile(`^(?:segment_)?([0-9]{1,4})p(?:\.m3u8|_[0-9]{6}\.ts)$`)

func mediaHeight(name string) int {
	matches := heightInName.FindStringSubmatch(name)
	if len(matches) != 2 {
		return 0
	}
	n, _ := strconv.Atoi(matches[1])
	return n
}
func hlsDirectory(key string, generation int64) string {
	if generation == 0 {
		return key
	}
	return key + "/jobs/g" + strconv.FormatInt(generation, 10)
}

func withinQuality(width, height, maxHeight int) bool {
	maxWidth := map[int]int{720: 1280, 1080: 1920, 1440: 2560, 2160: 3840}[maxHeight]
	return width > 0 && height > 0 && height <= maxHeight && width <= maxWidth
}

// Height in the filename alone is insufficient for ultrawide footage: a
// 2560x960 rendition must not be served to Basic by directly guessing its URL.
func renditionAllowed(master []byte, name string, maxHeight int) bool {
	height := mediaHeight(name)
	if !strings.Contains(string(master), "#EXT-X-STREAM-INF:") {
		// The original single-rendition pipeline produced at most 720p.
		return height == 0 && strings.HasPrefix(name, "segment_")
	}
	lines := strings.Split(strings.ReplaceAll(string(master), "\r\n", "\n"), "\n")
	for i, line := range lines {
		if !strings.HasPrefix(line, "#EXT-X-STREAM-INF:") || i+1 >= len(lines) {
			continue
		}
		matches := regexpResolution.FindStringSubmatch(line)
		if len(matches) != 3 || mediaHeight(strings.TrimSpace(lines[i+1])) != height {
			continue
		}
		w, _ := strconv.Atoi(matches[1])
		h, _ := strconv.Atoi(matches[2])
		return height == h && withinQuality(w, h, maxHeight)
	}
	return false
}

// Only trusted transcoder output is accepted. Rewriting every URI propagates
// the lease through native HLS as well as hls.js, without exposing source files.
func filteredPlaylist(data []byte, maxHeight int, token string) ([]byte, error) {
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	var result strings.Builder
	skipNext := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#EXT-X-STREAM-INF:") {
			matches := regexpResolution.FindStringSubmatch(line)
			if len(matches) != 3 {
				return nil, fmt.Errorf("invalid HLS resolution")
			}
			w, _ := strconv.Atoi(matches[1])
			h, _ := strconv.Atoi(matches[2])
			skipNext = !withinQuality(w, h, maxHeight)
			if skipNext {
				continue
			}
		}
		if line != "" && !strings.HasPrefix(line, "#") {
			if skipNext {
				skipNext = false
				continue
			}
			if !mediaFilePattern.MatchString(line) || line == "index.m3u8" {
				return nil, fmt.Errorf("invalid HLS URI")
			}
			if h := mediaHeight(line); h > maxHeight {
				return nil, fmt.Errorf("invalid HLS quality")
			}
			if token != "" {
				line += "?playback=" + url.QueryEscape(token)
			}
		}
		// Generated HLS never uses external keys, maps or URI attributes.
		if strings.Contains(line, "URI=") {
			return nil, fmt.Errorf("unsupported HLS URI")
		}
		result.WriteString(line + "\n")
	}
	return []byte(result.String()), nil
}

var regexpResolution = regexp.MustCompile(`RESOLUTION=([0-9]+)x([0-9]+)(?:,|$)`)

func servePlaylist(c *gin.Context, file io.Reader, maxHeight int, token string) {
	data, err := io.ReadAll(io.LimitReader(file, 1<<20+1))
	if err != nil || len(data) > 1<<20 {
		c.Status(503)
		return
	}
	data, err = filteredPlaylist(data, maxHeight, token)
	if err != nil {
		c.Status(503)
		return
	}
	c.Header("Content-Length", strconv.Itoa(len(data)))
	if c.Request.Method == http.MethodHead {
		c.Status(200)
		return
	}
	c.Data(200, "application/vnd.apple.mpegurl", data)
}
