package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Explicit, offline import. No server/worker starts, and local files are never removed.
// Re-run is safe: matching SHA256 metadata skips unchanged objects.
func importMedia(ctx context.Context, db *pgxpool.Pool, apply bool) error {
	store, err := newObjectStorage(ctx)
	if err != nil {
		return err
	}
	if store == nil {
		return fmt.Errorf("import requires MEDIA_BACKEND=s3")
	}
	root, err := filepath.Abs(os.Getenv("MEDIA_IMPORT_DIR"))
	if err != nil || os.Getenv("MEDIA_IMPORT_DIR") == "" {
		return fmt.Errorf("MEDIA_IMPORT_DIR must identify restored local storage")
	}
	handle, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer handle.Close()
	rows, err := db.Query(ctx, `SELECT id,storage_key,cover_jpeg,has_cover,cover_s3_key FROM videos WHERE deleted_at IS NULL AND storage_key IS NOT NULL ORDER BY id`)
	if err != nil {
		return err
	}
	type video struct {
		id          int64
		key         string
		cover       []byte
		hasCover    bool
		coverObject *string
	}
	var videos []video
	for rows.Next() {
		var v video
		if err = rows.Scan(&v.id, &v.key, &v.cover, &v.hasCover, &v.coverObject); err != nil {
			break
		}
		videos = append(videos, v)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return err
	}
	count := 0
	for _, v := range videos {
		if !storageKeyPattern.MatchString(v.key) {
			return fmt.Errorf("invalid storage key for video %d", v.id)
		}
		// Only an existing DB row's directory is imported, never arbitrary backup files.
		err = filepath.WalkDir(filepath.Join(root, v.key), func(filePath string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("symlink in import")
			}
			if entry.IsDir() {
				return nil
			}
			rel, e := filepath.Rel(root, filePath)
			if e != nil {
				return e
			}
			rel = filepath.ToSlash(rel)
			// Old edited covers in DB take precedence over the older on-disk cover.
			if rel == v.key+"/cover.jpg" && (len(v.cover) > 0 || v.coverObject != nil) {
				return nil
			}
			file, e := handle.Open(rel)
			if e != nil {
				return e
			}
			defer file.Close()
			info, e := file.Stat()
			if e != nil {
				return e
			}
			count++
			if !apply {
				return nil
			}
			contentType := "application/octet-stream"
			if strings.HasSuffix(rel, ".jpg") {
				contentType = "image/jpeg"
			} else if strings.HasSuffix(rel, ".m3u8") {
				contentType = "application/vnd.apple.mpegurl"
			} else if strings.HasSuffix(rel, ".ts") {
				contentType = "video/mp2t"
			}
			return importObject(ctx, store, rel, file, info.Size(), contentType)
		})
		if err != nil {
			return fmt.Errorf("import video %d: %w", v.id, err)
		}
		if len(v.cover) > 0 {
			count++
			if apply {
				if err = importObject(ctx, store, v.key+"/cover.jpg", bytes.NewReader(v.cover), int64(len(v.cover)), "image/jpeg"); err != nil {
					return err
				}
			}
		}
		if apply && v.hasCover {
			coverKey := v.key + "/cover.jpg"
			if v.coverObject != nil {
				coverKey = *v.coverObject
			}
			if _, err = store.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(store.bucket), Key: aws.String(store.key(coverKey))}); err != nil {
				return fmt.Errorf("missing cover for video %d", v.id)
			}
			if _, err = db.Exec(ctx, `UPDATE videos SET cover_s3_key=$2,cover_jpeg=NULL WHERE id=$1`, v.id, coverKey); err != nil {
				return err
			}
		}
	}
	rows, err = db.Query(ctx, `SELECT id,avatar_jpeg FROM users WHERE avatar_jpeg IS NOT NULL AND account_status<>'deleted'`)
	if err != nil {
		return err
	}
	type avatar struct {
		id   int64
		data []byte
	}
	var avatars []avatar
	for rows.Next() {
		var a avatar
		if err = rows.Scan(&a.id, &a.data); err != nil {
			break
		}
		avatars = append(avatars, a)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return err
	}
	for _, a := range avatars {
		if len(a.data) == 0 {
			continue
		}
		count++
		if !apply {
			continue
		}
		key := fmt.Sprintf("avatars/%d/imported.jpg", a.id)
		if err = importObject(ctx, store, key, bytes.NewReader(a.data), int64(len(a.data)), "image/jpeg"); err != nil {
			return err
		}
		if _, err = db.Exec(ctx, `UPDATE users SET avatar_s3_key=$2,avatar_jpeg=NULL WHERE id=$1`, a.id, key); err != nil {
			return err
		}
	}
	fmt.Printf("Media import: videos=%d objects=%d apply=%t; local files retained\n", len(videos), count, apply)
	return nil
}
func importObject(ctx context.Context, store *objectStorage, key string, body io.ReadSeeker, size int64, contentType string) error {
	hash := sha256.New()
	if _, err := io.Copy(hash, body); err != nil {
		return err
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	if _, err := body.Seek(0, io.SeekStart); err != nil {
		return err
	}
	check, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	current, err := store.client.HeadObject(check, &s3.HeadObjectInput{Bucket: aws.String(store.bucket), Key: aws.String(store.key(key))})
	if err == nil && current.Metadata["sha256"] == digest && aws.ToInt64(current.ContentLength) == size {
		return nil
	}
	_, err = store.client.PutObject(check, &s3.PutObjectInput{Bucket: aws.String(store.bucket), Key: aws.String(store.key(key)), Body: body, ContentLength: aws.Int64(size), ContentType: aws.String(contentType), Metadata: map[string]string{"sha256": digest}})
	if err != nil {
		return err
	}
	verified, err := store.client.HeadObject(check, &s3.HeadObjectInput{Bucket: aws.String(store.bucket), Key: aws.String(store.key(key))})
	if err != nil {
		return err
	}
	if verified.Metadata["sha256"] != digest || aws.ToInt64(verified.ContentLength) != size {
		return fmt.Errorf("import verification failed")
	}
	return nil
}
