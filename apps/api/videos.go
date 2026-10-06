package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const maxVideoBytes int64 = 100 * 1024 * 1024
const videoColumns = `id,title,description,category,duration_seconds,publication_status,status,processing_error`

var storageKeyPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)
var mediaFilePattern = regexp.MustCompile(`^(index\.m3u8|segment_[0-9]{6}\.ts)$`)

type videoRecord struct {
	ID                int64  `json:"id"`
	Title             string `json:"title"`
	Description       string `json:"description"`
	Category          string `json:"category"`
	DurationSeconds   int    `json:"durationSeconds"`
	PublicationStatus string `json:"publicationStatus"`
	ProcessingStatus  string `json:"processingStatus"`
	ProcessingError   string `json:"processingError,omitempty"`
	PlaybackURL       string `json:"playbackUrl,omitempty"`
	PreviewURL        string `json:"previewUrl,omitempty"`
}

func (v *videoRecord) destinations() []any {
	return []any{&v.ID, &v.Title, &v.Description, &v.Category, &v.DurationSeconds, &v.PublicationStatus, &v.ProcessingStatus, &v.ProcessingError}
}
func (v *videoRecord) deliveryURLs() {
	if v.ProcessingStatus == "ready" {
		v.PlaybackURL = fmt.Sprintf("/media/%d/full/index.m3u8", v.ID)
		v.PreviewURL = fmt.Sprintf("/media/%d/preview/index.m3u8", v.ID)
	}
}
func (v *videoRecord) scan(row pgx.Row) error {
	err := row.Scan(v.destinations()...)
	if err == nil {
		v.deliveryURLs()
	}
	return err
}

type videoService struct {
	db            *pgxpool.Pool
	auth          *authConfig
	root          string
	ffmpeg        string
	ffprobe       string
	mu            sync.Mutex
	running       map[int64]context.CancelFunc
	nginxDelivery bool
	leaseActive   atomic.Bool
	threads       int
	budget        mediaBudget
}

func newVideoService(db *pgxpool.Pool, a *authConfig, dir string) (*videoService, error) {
	if dir == "" {
		dir = "../../storage"
	}
	root, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	budget, err := readMediaBudget()
	if err != nil {
		return nil, err
	}
	return &videoService{budget: budget, db: db, auth: a, root: root, running: make(map[int64]context.CancelFunc)}, nil
}

func (s *videoService) initTranscoder(threads int) error {
	fm, err := exec.LookPath("ffmpeg")
	if err != nil {
		return fmt.Errorf("FFmpeg is required for the video worker")
	}
	fp, err := exec.LookPath("ffprobe")
	if err != nil {
		return fmt.Errorf("ffprobe is required for the video worker")
	}
	s.ffmpeg, s.ffprobe, s.threads = fm, fp, threads
	return nil
}

func (s *videoService) user(c *gin.Context, required bool) (accountResponse, bool) {
	var user accountResponse
	hash, ok := sessionHash(c)
	if !ok {
		if required {
			c.AbortWithStatusJSON(401, gin.H{"error": "unauthenticated"})
		}
		return user, !required
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	var err error
	user, err = readSessionUser(ctx, s.db, hash)
	if errors.Is(err, pgx.ErrNoRows) {
		if required {
			c.AbortWithStatusJSON(401, gin.H{"error": "unauthenticated"})
		}
		return accountResponse{}, !required
	}
	if err != nil {
		c.AbortWithStatusJSON(503, gin.H{"error": "database_unavailable"})
		return user, false
	}
	return user, true
}
func (s *videoService) admin() gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := s.user(c, true)
		if !ok {
			return
		}
		if user.Role != "admin" {
			c.AbortWithStatusJSON(403, gin.H{"error": "admin_required"})
			return
		}
		if c.Request.Method != "GET" && (c.GetHeader("Origin") != s.auth.origin || c.GetHeader("Sec-Fetch-Site") == "cross-site") {
			c.AbortWithStatusJSON(403, gin.H{"error": "invalid_origin"})
			return
		}
		c.Header("Cache-Control", "no-store")
		c.Next()
	}
}
func (s *videoService) routes(router *gin.Engine) {
	router.GET("/api/videos", s.list(false))
	router.GET("/api/videos/:id", s.detail)
	admin := router.Group("/api/admin")
	admin.Use(s.admin())
	admin.GET("/videos", s.list(true))
	admin.POST("/videos", s.upload)
	admin.PATCH("/videos/:id", s.publish)
	admin.DELETE("/videos/:id", s.remove)
	admin.POST("/videos/:id/retry", s.retry)
	router.GET("/media/:id/:kind/:file", s.media)
	router.HEAD("/media/:id/:kind/:file", s.media)
}
func videoID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(404, gin.H{"error": "video_not_found"})
		return 0, false
	}
	return id, true
}
func (s *videoService) list(admin bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()
		where := `deleted_at IS NULL`
		if !admin {
			where += ` AND publication_status='published' AND status='ready'`
		}
		rows, err := s.db.Query(ctx, `SELECT `+videoColumns+` FROM videos WHERE `+where+` ORDER BY created_at DESC LIMIT 500`)
		if err != nil {
			c.JSON(503, gin.H{"error": "database_unavailable"})
			return
		}
		defer rows.Close()
		videos := []videoRecord{}
		for rows.Next() {
			var v videoRecord
			if err = v.scan(rows); err != nil {
				c.JSON(503, gin.H{"error": "database_unavailable"})
				return
			}
			videos = append(videos, v)
		}
		if rows.Err() != nil {
			c.JSON(503, gin.H{"error": "database_unavailable"})
			return
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(200, gin.H{"videos": videos})
	}
}
func (s *videoService) detail(c *gin.Context) {
	id, ok := videoID(c)
	if !ok {
		return
	}
	user, ok := s.user(c, false)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	var v videoRecord
	err := v.scan(s.db.QueryRow(ctx, `SELECT `+videoColumns+` FROM videos WHERE id=$1 AND deleted_at IS NULL AND ($2 OR (publication_status='published' AND status='ready'))`, id, user.Role == "admin"))
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(404, gin.H{"error": "video_not_found"})
		return
	}
	if err != nil {
		c.JSON(503, gin.H{"error": "database_unavailable"})
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(200, gin.H{"video": v})
}
func (s *videoService) upload(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxVideoBytes+64*1024)
	reader, err := c.Request.MultipartReader()
	if err != nil {
		c.JSON(400, gin.H{"error": "multipart_required"})
		return
	}
	bytes := make([]byte, 16)
	if _, err = rand.Read(bytes); err != nil {
		c.JSON(500, gin.H{"error": "internal_error"})
		return
	}
	key := hex.EncodeToString(bytes)
	reserve, cancelReserve := context.WithTimeout(c.Request.Context(), 10*time.Second)
	err = s.reserveMedia(reserve, key, true)
	cancelReserve()
	if err != nil {
		if errors.Is(err, errMediaBudget) {
			c.JSON(507, gin.H{"error": "storage_budget_exceeded"})
		} else {
			c.JSON(503, gin.H{"error": "storage_unavailable"})
		}
		return
	}
	dir := filepath.Join(s.root, key)
	if err = os.Mkdir(dir, 0700); err != nil {
		s.releaseMedia(key)
		c.JSON(500, gin.H{"error": "storage_unavailable"})
		return
	}
	keep := false
	defer func() {
		if !keep {
			os.RemoveAll(dir)
			s.releaseMedia(key)
		}
	}()
	fields := map[string]string{}
	hasFile := false
	for {
		part, readErr := reader.NextPart()
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			c.JSON(413, gin.H{"error": "upload_incomplete_or_too_large"})
			return
		}
		if part.FormName() == "file" && part.FileName() != "" {
			ext := strings.ToLower(filepath.Ext(part.FileName()))
			if hasFile || (ext != ".mp4" && ext != ".mov" && ext != ".webm") {
				part.Close()
				c.JSON(400, gin.H{"error": "invalid_video_file"})
				return
			}
			hasFile = true
			file, openErr := os.OpenFile(filepath.Join(dir, "source"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if openErr != nil {
				part.Close()
				c.JSON(500, gin.H{"error": "storage_unavailable"})
				return
			}
			n, copyErr := io.Copy(file, io.LimitReader(part, maxVideoBytes+1))
			closeErr := file.Close()
			part.Close()
			if copyErr != nil || n > maxVideoBytes {
				c.JSON(413, gin.H{"error": "upload_incomplete_or_too_large"})
				return
			}
			if closeErr != nil {
				c.JSON(500, gin.H{"error": "storage_unavailable"})
				return
			}
			if n == 0 {
				c.JSON(400, gin.H{"error": "invalid_video_file"})
				return
			}
		} else {
			name := part.FormName()
			if name != "title" && name != "description" && name != "category" {
				part.Close()
				c.JSON(400, gin.H{"error": "invalid_metadata"})
				return
			}
			if _, exists := fields[name]; exists {
				part.Close()
				c.JSON(400, gin.H{"error": "invalid_metadata"})
				return
			}
			data, e := io.ReadAll(io.LimitReader(part, 8193))
			part.Close()
			if e != nil || len(data) > 8192 || !utf8.Valid(data) {
				c.JSON(400, gin.H{"error": "invalid_metadata"})
				return
			}
			fields[name] = strings.TrimSpace(string(data))
		}
	}
	title, description, category := fields["title"], fields["description"], fields["category"]
	if !hasFile || utf8.RuneCountInString(title) < 1 || utf8.RuneCountInString(title) > 120 || utf8.RuneCountInString(description) > 2000 || (category != "บันเทิง" && category != "ความรู้" && category != "ไลฟ์สไตล์" && category != "อื่น ๆ") {
		c.JSON(400, gin.H{"error": "invalid_metadata"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	var v videoRecord
	err = v.scan(s.db.QueryRow(ctx, `INSERT INTO videos(title,description,category,storage_key) VALUES($1,$2,$3,$4) RETURNING `+videoColumns, title, description, category, key))
	if err != nil {
		c.JSON(503, gin.H{"error": "database_unavailable"})
		return
	}
	keep = true
	_, _ = s.db.Exec(ctx, `UPDATE media_reservations SET lease_until=NULL WHERE storage_key=$1`, key)
	c.JSON(202, gin.H{"video": v})
}
func (s *videoService) publish(c *gin.Context) {
	id, ok := videoID(c)
	if !ok {
		return
	}
	mediaType, _, _ := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if mediaType != "application/json" {
		c.JSON(415, gin.H{"error": "json_required"})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	var req struct {
		Status string `json:"publicationStatus"`
	}
	if c.ShouldBindJSON(&req) != nil || (req.Status != "draft" && req.Status != "published" && req.Status != "hidden") {
		c.JSON(400, gin.H{"error": "invalid_status"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	var v videoRecord
	err := v.scan(s.db.QueryRow(ctx, `UPDATE videos SET publication_status=$2,updated_at=NOW() WHERE id=$1 AND deleted_at IS NULL AND ($2<>'published' OR status='ready') RETURNING `+videoColumns, id, req.Status))
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(409, gin.H{"error": "video_not_ready_or_missing"})
		return
	}
	if err != nil {
		c.JSON(503, gin.H{"error": "database_unavailable"})
		return
	}
	c.JSON(200, gin.H{"video": v})
}
func (s *videoService) remove(c *gin.Context) {
	id, ok := videoID(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	tag, err := s.db.Exec(ctx, `UPDATE videos SET deleted_at=NOW(),publication_status='hidden',updated_at=NOW() WHERE id=$1 AND deleted_at IS NULL`, id)
	if err != nil {
		c.JSON(503, gin.H{"error": "database_unavailable"})
		return
	}
	if tag.RowsAffected() == 0 {
		c.JSON(404, gin.H{"error": "video_not_found"})
		return
	}
	s.mu.Lock()
	if stop := s.running[id]; stop != nil {
		stop()
	}
	s.mu.Unlock()
	c.Status(204)
}
func (s *videoService) retry(c *gin.Context) {
	id, ok := videoID(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	tag, err := s.db.Exec(ctx, `UPDATE videos SET status='pending',processing_error='',updated_at=NOW() WHERE id=$1 AND deleted_at IS NULL AND status='failed' AND storage_key IS NOT NULL`, id)
	if err != nil {
		c.JSON(503, gin.H{"error": "database_unavailable"})
		return
	}
	if tag.RowsAffected() == 0 {
		c.JSON(409, gin.H{"error": "video_not_retryable"})
		return
	}
	c.Status(202)
}
func (s *videoService) media(c *gin.Context) {
	id, ok := videoID(c)
	if !ok {
		return
	}
	kind, name := c.Param("kind"), c.Param("file")
	if (kind != "full" && kind != "preview") || !mediaFilePattern.MatchString(name) {
		c.Status(404)
		return
	}
	user, ok := s.user(c, kind == "full")
	if !ok {
		return
	}
	if kind == "full" && user.Role != "admin" && !user.Member {
		c.JSON(403, gin.H{"error": "membership_required"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	var key string
	err := s.db.QueryRow(ctx, `SELECT storage_key FROM videos WHERE id=$1 AND deleted_at IS NULL AND status='ready' AND ($2 OR publication_status='published')`, id, user.Role == "admin").Scan(&key)
	if errors.Is(err, pgx.ErrNoRows) {
		c.Status(404)
		return
	}
	if err != nil {
		c.Status(503)
		return
	}
	if !storageKeyPattern.MatchString(key) {
		c.Status(404)
		return
	}
	root, err := os.OpenRoot(s.root)
	if err != nil {
		c.Status(503)
		return
	}
	defer root.Close()
	file, err := root.Open(filepath.Join(key, kind, name))
	if err != nil {
		c.Status(404)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		c.Status(404)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cross-Origin-Resource-Policy", "same-origin")
	if strings.HasSuffix(name, ".m3u8") {
		c.Header("Content-Type", "application/vnd.apple.mpegurl")
	} else {
		c.Header("Content-Type", "video/mp2t")
	}
	if s.nginxDelivery {
		// Only validated, authorized paths reach Nginx's internal-only location.
		c.Header("X-Accel-Redirect", "/_hls/"+key+"/"+kind+"/"+name)
		c.Status(200)
		return
	}
	http.ServeContent(c.Writer, c.Request, name, info.ModTime(), file)
}
