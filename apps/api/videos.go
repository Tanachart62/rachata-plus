package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const videoColumns = `id,title,description,category,duration_seconds,publication_status,status,processing_error,has_cover,metadata_revision`

var qualifiedVideoColumns = "v." + strings.ReplaceAll(videoColumns, ",", ",v.")

var storageKeyPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)
var mediaFilePattern = regexp.MustCompile(`^(index\.m3u8|[0-9]{1,4}p\.m3u8|segment_([0-9]{1,4}p_)?[0-9]{6}\.ts)$`)

type videoRecord struct {
	MetadataRevision  int64  `json:"metadataRevision"`
	HasCover          bool   `json:"-"`
	CoverURL          string `json:"coverUrl,omitempty"`
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
	return []any{&v.ID, &v.Title, &v.Description, &v.Category, &v.DurationSeconds, &v.PublicationStatus, &v.ProcessingStatus, &v.ProcessingError, &v.HasCover, &v.MetadataRevision}
}
func (v *videoRecord) deliveryURLs() {
	if v.HasCover {
		v.CoverURL = fmt.Sprintf("/api/videos/%d/cover?v=%d", v.ID, v.MetadataRevision)
	}
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
	jobTimeout    time.Duration
	store         *objectStorage
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
	if !(os.Getenv("MEDIA_BACKEND") == "s3" && os.Getenv("APP_MODE") == "api") {
		if err = os.MkdirAll(root, 0700); err != nil {
			return nil, err
		}
	}
	budget, err := readMediaBudget()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	store, err := newObjectStorage(ctx)
	if err != nil {
		return nil, err
	}
	jobTimeout, err := time.ParseDuration(envDefault("VIDEO_JOB_TIMEOUT", "45m"))
	if err != nil || jobTimeout < time.Minute || jobTimeout > 24*time.Hour {
		return nil, fmt.Errorf("VIDEO_JOB_TIMEOUT must be 1m..24h")
	}
	return &videoService{jobTimeout: jobTimeout, store: store, budget: budget, db: db, auth: a, root: root, running: make(map[int64]context.CancelFunc)}, nil
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

const videoAdminActorKey = "video_admin_actor_id"

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
		c.Set(videoAdminActorKey, user.ID)
		c.Header("Cache-Control", "no-store")
		c.Next()
	}
}

// Acquire authority only after request parsing. The user lock is shared with
// suspension/session revocation, and remains held through the media write.
func (s *videoService) beginAdminMutation(c *gin.Context, ctx context.Context) (pgx.Tx, bool) {
	actor, exists := c.Get(videoAdminActorKey)
	id, valid := actor.(int64)
	if !exists || !valid || id <= 0 {
		accountFailure(c, pgx.ErrNoRows)
		return nil, false
	}
	tx, locked, ok := beginAccountMutation(c, s.db, ctx, id)
	if !ok {
		return nil, false
	}
	if locked.User.Role != "admin" {
		rollback(tx)
		c.JSON(403, gin.H{"error": "admin_required"})
		return nil, false
	}
	return tx, true
}
func (s *videoService) routes(router *gin.Engine) {
	router.GET("/api/videos", s.list(false))
	router.GET("/api/videos/:id", s.detail)
	router.GET("/api/videos/:id/cover", s.cover)
	admin := router.Group("/api/admin")
	admin.Use(s.admin())
	admin.GET("/videos", s.list(true))
	admin.POST("/uploads", s.startDirectUpload)
	admin.POST("/uploads/:upload/part", s.signDirectPart)
	admin.POST("/uploads/:upload/complete", s.completeDirectUpload)
	admin.DELETE("/uploads/:upload", s.abortDirectUpload)
	admin.POST("/videos", s.upload)
	admin.PATCH("/videos/:id", s.publish)
	admin.POST("/videos/:id/edit", s.editVideo)
	admin.DELETE("/videos/:id", s.remove)
	admin.POST("/videos/:id/retry", s.retry)
	router.GET("/media/:id/:kind/:file", s.media)
	router.HEAD("/media/:id/:kind/:file", s.media)
	router.POST("/api/playback/:id", s.playback)
	router.DELETE("/api/playback/:id", s.playback)
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
