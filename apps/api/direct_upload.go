package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

func validVideoMetadata(title, description, category string) bool {
	return utf8.RuneCountInString(title) > 0 && utf8.RuneCountInString(title) <= 120 && utf8.RuneCountInString(description) <= 2000 && (category == "บันเทิง" || category == "ความรู้" || category == "ไลฟ์สไตล์" || category == "อื่น ๆ")
}
func randomStorageKey() (string, error) {
	var data [16]byte
	_, err := rand.Read(data[:])
	return hex.EncodeToString(data[:]), err
}
func (s *videoService) startDirectUpload(c *gin.Context) {
	if s.store == nil {
		c.JSON(200, gin.H{"mode": "local"})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16384)
	var req struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Category    string `json:"category"`
		Filename    string `json:"filename"`
		Size        int64  `json:"size"`
	}
	if c.ShouldBindJSON(&req) != nil {
		c.JSON(400, gin.H{"error": "invalid_metadata"})
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	req.Description = strings.TrimSpace(req.Description)
	ext := strings.ToLower(filepath.Ext(req.Filename))
	if !validVideoMetadata(req.Title, req.Description, req.Category) {
		c.JSON(400, gin.H{"error": "invalid_metadata"})
		return
	}
	if req.Size <= 0 || req.Size >= s.budget.Job || (ext != ".mp4" && ext != ".mov" && ext != ".webm") {
		c.JSON(400, gin.H{"error": "invalid_video_file"})
		return
	}
	key, err := randomStorageKey()
	if err != nil {
		c.Status(500)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	tx, ok := s.beginAdminMutation(c, ctx)
	if !ok {
		return
	}
	defer rollback(tx)
	actor := c.MustGet(videoAdminActorKey).(int64)
	hash, _ := sessionHash(c)
	var active int
	err = tx.QueryRow(ctx, `SELECT count(*) FROM direct_uploads WHERE actor_id=$1 AND video_id IS NULL AND expires_at>NOW()`, actor).Scan(&active)
	if err != nil {
		accountFailure(c, err)
		return
	}
	if active >= 3 {
		c.JSON(429, gin.H{"error": "too_many_uploads"})
		return
	}
	partSize := max(int64(16<<20), (req.Size+9998)/9999)
	out, err := s.store.client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{Bucket: aws.String(s.store.bucket), Key: aws.String(s.store.key(key + "/source")), ContentType: aws.String("application/octet-stream")})
	if err != nil {
		c.JSON(503, gin.H{"error": "storage_unavailable"})
		return
	}
	saved := false
	defer func() {
		if !saved {
			cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
			defer stop()
			_, _ = s.store.client.AbortMultipartUpload(cleanup, &s3.AbortMultipartUploadInput{Bucket: aws.String(s.store.bucket), Key: aws.String(s.store.key(key + "/source")), UploadId: out.UploadId})
		}
	}()
	_, err = tx.Exec(ctx, `INSERT INTO direct_uploads(id,actor_id,session_hash,upload_id,title,description,category,size_bytes,part_size,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,NOW()+interval '24 hours')`, key, actor, hash, aws.ToString(out.UploadId), req.Title, req.Description, req.Category, req.Size, partSize)
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		accountFailure(c, err)
		return
	}
	saved = true
	c.JSON(201, gin.H{"mode": "s3", "id": key, "partSize": partSize, "parts": (req.Size + partSize - 1) / partSize})
}

type directUpload struct {
	ID, UploadID, Title, Description, Category string
	Size, PartSize                             int64
	VideoID                                    *int64
}

func (s *videoService) ownedUpload(c *gin.Context, ctx context.Context, tx pgx.Tx) (directUpload, bool) {
	var u directUpload
	if s.store == nil || !storageKeyPattern.MatchString(c.Param("upload")) {
		c.Status(404)
		return u, false
	}
	hash, _ := sessionHash(c)
	actor := c.MustGet(videoAdminActorKey).(int64)
	err := tx.QueryRow(ctx, `SELECT id,upload_id,title,description,category,size_bytes,part_size,video_id FROM direct_uploads WHERE id=$1 AND actor_id=$2 AND session_hash=$3 AND expires_at>NOW() FOR UPDATE`, c.Param("upload"), actor, hash).Scan(&u.ID, &u.UploadID, &u.Title, &u.Description, &u.Category, &u.Size, &u.PartSize, &u.VideoID)
	if errors.Is(err, pgx.ErrNoRows) {
		c.Status(404)
		return u, false
	}
	if err != nil {
		accountFailure(c, err)
		return u, false
	}
	return u, true
}
func (s *videoService) signDirectPart(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	var req struct {
		Part int32 `json:"part"`
	}
	if c.ShouldBindJSON(&req) != nil {
		c.Status(400)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	tx, ok := s.beginAdminMutation(c, ctx)
	if !ok {
		return
	}
	defer rollback(tx)
	u, ok := s.ownedUpload(c, ctx, tx)
	if !ok {
		return
	}
	parts := (u.Size + u.PartSize - 1) / u.PartSize
	if u.VideoID != nil || req.Part < 1 || int64(req.Part) > parts {
		c.Status(409)
		return
	}
	size := min(u.PartSize, u.Size-int64(req.Part-1)*u.PartSize)
	out, err := s.store.signer.PresignUploadPart(ctx, &s3.UploadPartInput{Bucket: aws.String(s.store.bucket), Key: aws.String(s.store.key(u.ID + "/source")), UploadId: aws.String(u.UploadID), PartNumber: aws.Int32(req.Part), ContentLength: aws.Int64(size)}, s3.WithPresignExpires(10*time.Minute))
	if err != nil {
		c.Status(503)
		return
	}
	if err = tx.Commit(ctx); err != nil {
		accountFailure(c, err)
		return
	}
	c.JSON(200, gin.H{"url": out.URL})
}
func (s *videoService) completeDirectUpload(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4<<20)
	var req struct {
		Cover string `json:"cover"`
	}
	if c.ShouldBindJSON(&req) != nil {
		c.JSON(400, gin.H{"error": "invalid_cover"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()
	tx, ok := s.beginAdminMutation(c, ctx)
	if !ok {
		return
	}
	defer rollback(tx)
	u, ok := s.ownedUpload(c, ctx, tx)
	if !ok {
		return
	}
	if u.VideoID != nil {
		var v videoRecord
		err := v.scan(tx.QueryRow(ctx, `SELECT `+videoColumns+` FROM videos WHERE id=$1 AND deleted_at IS NULL`, *u.VideoID))
		if err != nil {
			accountFailure(c, err)
			return
		}
		c.JSON(200, gin.H{"video": v})
		return
	}
	hasCover := req.Cover != ""
	if hasCover {
		raw, err := base64.StdEncoding.Strict().DecodeString(req.Cover)
		if err != nil {
			c.JSON(400, gin.H{"error": "invalid_cover"})
			return
		}
		cover, err := encodedCover(bytes.NewReader(raw))
		if err != nil {
			c.JSON(400, gin.H{"error": "invalid_cover"})
			return
		}
		if err = s.store.bytes(ctx, u.ID+"/cover.jpg", cover, "image/jpeg"); err != nil {
			c.Status(503)
			return
		}
	}
	// A previous Complete may have succeeded while its response or DB commit was lost.
	// The source is immutable once complete (UploadId cannot be used again).
	head, headErr := s.store.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(s.store.bucket), Key: aws.String(s.store.key(u.ID + "/source"))})
	if headErr != nil {
		var parts []types.CompletedPart
		pager := s3.NewListPartsPaginator(s.store.client, &s3.ListPartsInput{Bucket: aws.String(s.store.bucket), Key: aws.String(s.store.key(u.ID + "/source")), UploadId: aws.String(u.UploadID)})
		var n int64
		for pager.HasMorePages() {
			out, err := pager.NextPage(ctx)
			if err != nil {
				c.JSON(409, gin.H{"error": "upload_incomplete_or_too_large"})
				return
			}
			for _, p := range out.Parts {
				number := int64(len(parts) + 1)
				want := min(u.PartSize, u.Size-(number-1)*u.PartSize)
				if want <= 0 || int64(aws.ToInt32(p.PartNumber)) != number || aws.ToInt64(p.Size) != want {
					c.JSON(409, gin.H{"error": "upload_incomplete_or_too_large"})
					return
				}
				n += aws.ToInt64(p.Size)
				parts = append(parts, types.CompletedPart{ETag: p.ETag, PartNumber: p.PartNumber})
			}
		}
		if n != u.Size || len(parts) == 0 {
			c.JSON(409, gin.H{"error": "upload_incomplete_or_too_large"})
			return
		}
		_, err := s.store.client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{Bucket: aws.String(s.store.bucket), Key: aws.String(s.store.key(u.ID + "/source")), UploadId: aws.String(u.UploadID), MultipartUpload: &types.CompletedMultipartUpload{Parts: parts}})
		if err != nil {
			c.Status(503)
			return
		}
	} else if aws.ToInt64(head.ContentLength) != u.Size {
		c.Status(409)
		return
	}
	var v videoRecord
	err := v.scan(tx.QueryRow(ctx, `INSERT INTO videos(title,description,category,storage_key,has_cover) VALUES($1,$2,$3,$4,$5) RETURNING `+videoColumns, u.Title, u.Description, u.Category, u.ID, hasCover))
	if err == nil {
		_, err = tx.Exec(ctx, `UPDATE direct_uploads SET video_id=$2 WHERE id=$1`, u.ID, v.ID)
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		accountFailure(c, err)
		return
	}
	c.JSON(202, gin.H{"video": v})
}
func (s *videoService) abortDirectUpload(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	tx, ok := s.beginAdminMutation(c, ctx)
	if !ok {
		return
	}
	defer rollback(tx)
	u, ok := s.ownedUpload(c, ctx, tx)
	if !ok {
		return
	}
	if u.VideoID != nil {
		c.Status(409)
		return
	}
	_, err := s.store.client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{Bucket: aws.String(s.store.bucket), Key: aws.String(s.store.key(u.ID + "/source")), UploadId: aws.String(u.UploadID)})
	if err != nil {
		var missing *types.NoSuchUpload
		if !errors.As(err, &missing) {
			c.Status(503)
			return
		}
	}
	if err = s.store.deletePrefix(ctx, u.ID); err != nil {
		c.Status(503)
		return
	}
	_, err = tx.Exec(ctx, `DELETE FROM direct_uploads WHERE id=$1 AND video_id IS NULL`, u.ID)
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		accountFailure(c, err)
		return
	}
	c.Status(204)
}
func (s *videoService) s3Media(c *gin.Context, directory, kind, name string, maxHeight int) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	if kind == "full" && name != "index.m3u8" {
		master, err := s.store.read(ctx, directory+"/"+kind+"/index.m3u8", 1<<20)
		if err != nil {
			objectError(c, err)
			return
		}
		if !renditionAllowed(master, name, maxHeight) {
			c.JSON(403, gin.H{"error": "quality_not_allowed"})
			return
		}
	}
	if strings.HasSuffix(name, ".m3u8") {
		data, err := s.store.read(ctx, directory+"/"+kind+"/"+name, 1<<20)
		if err != nil {
			objectError(c, err)
			return
		}
		c.Header("Cache-Control", "private, no-store")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Cross-Origin-Resource-Policy", "same-origin")
		token := ""
		if kind == "full" {
			token = c.Query("playback")
		}
		servePlaylist(c, bytes.NewReader(data), maxHeight, token)
		return
	}
	s.store.serve(c, directory+"/"+kind+"/"+name, "video/mp2t")
}
