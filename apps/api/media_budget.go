package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var errMediaBudget = errors.New("media storage budget exceeded")

type mediaBudget struct{ Total, Job, MinFree int64 }

func readMediaBudget() (mediaBudget, error) {
	b := mediaBudget{Total: 10 << 30, Job: 1 << 30, MinFree: 512 << 20}
	for name, target := range map[string]*int64{"MEDIA_TOTAL_BYTES": &b.Total, "MEDIA_JOB_BYTES": &b.Job, "MEDIA_MIN_FREE_BYTES": &b.MinFree} {
		if value := os.Getenv(name); value != "" {
			n, err := strconv.ParseInt(value, 10, 64)
			if err != nil || n <= 0 {
				return b, fmt.Errorf("invalid %s", name)
			}
			*target = n
		}
	}
	if b.Job <= maxVideoBytes || b.Total < b.Job || b.Total > 1<<50 || b.MinFree > 1<<50 {
		return b, fmt.Errorf("invalid media budget bounds")
	}
	return b, nil
}
func directoryBytes(ctx context.Context, root string, skip map[string]int64) (int64, error) {
	var size int64
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if path == root {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink in media storage")
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		key := strings.Split(rel, string(filepath.Separator))[0]
		if _, ok := skip[key]; ok {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.IsDir() {
			info, err := d.Info()
			if err != nil {
				return err
			}
			size += info.Size()
		}
		return nil
	})
	return size, err
}

// Database advisory locking coordinates API and worker reservations across
// processes. Count untracked/legacy files as well, not just new DB records.
func (s *videoService) reserveMedia(ctx context.Context, key string, upload bool) error {
	if !storageKeyPattern.MatchString(key) {
		return fmt.Errorf("invalid storage key")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(tx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(724286002)`); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT storage_key,reserved_bytes FROM media_reservations`)
	if err != nil {
		return err
	}
	reservations := map[string]int64{}
	var total int64
	for rows.Next() {
		var k string
		var n int64
		if err = rows.Scan(&k, &n); err != nil {
			break
		}
		reservations[k] = n
		if k != key {
			total += n
		}
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return err
	}
	// The current video's existing bytes are included in its new full budget.
	reservations[key] = s.budget.Job
	untracked, err := directoryBytes(ctx, s.root, reservations)
	if err != nil {
		return err
	}
	projected := total + untracked + s.budget.Job
	if projected > s.budget.Total {
		return errMediaBudget
	}
	used, err := directoryBytes(ctx, s.root, nil)
	if err != nil {
		return err
	}
	free, err := diskFree(s.root)
	if err != nil {
		return err
	}
	growth := projected - used
	if growth < 0 {
		growth = 0
	}
	if free < growth+s.budget.MinFree {
		return errMediaBudget
	}
	var lease any
	if upload {
		lease = time.Now().Add(20 * time.Minute)
	}
	_, err = tx.Exec(ctx, `INSERT INTO media_reservations(storage_key,reserved_bytes,lease_until) VALUES($1,$2,$3)
 ON CONFLICT(storage_key) DO UPDATE SET reserved_bytes=EXCLUDED.reserved_bytes,lease_until=EXCLUDED.lease_until`, key, s.budget.Job, lease)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *videoService) releaseMedia(key string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, _ = s.db.Exec(ctx, `DELETE FROM media_reservations WHERE storage_key=$1`, key)
}
func (s *videoService) settleMedia(key string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	size, err := directoryBytes(ctx, filepath.Join(s.root, key), nil)
	if err == nil && size > 0 {
		_, _ = s.db.Exec(ctx, `UPDATE media_reservations SET reserved_bytes=$2,lease_until=NULL WHERE storage_key=$1`, key, size)
	}
}
func (s *videoService) cleanAbandonedUploads(ctx context.Context) {
	query, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	rows, err := s.db.Query(query, `SELECT storage_key FROM media_reservations r WHERE lease_until<NOW() AND NOT EXISTS(SELECT 1 FROM videos v WHERE v.storage_key=r.storage_key) LIMIT 10`)
	if err != nil {
		return
	}
	keys := []string{}
	for rows.Next() {
		var key string
		if rows.Scan(&key) == nil {
			keys = append(keys, key)
		}
	}
	rows.Close()
	for _, key := range keys {
		if storageKeyPattern.MatchString(key) && os.RemoveAll(filepath.Join(s.root, key)) == nil {
			s.releaseMedia(key)
		}
	}
}
func (s *videoService) transcode(ctx context.Context, key string) (duration int, processErr error) {
	reserve, cancelReserve := context.WithTimeout(ctx, 10*time.Second)
	err := s.reserveMedia(reserve, key, false)
	cancelReserve()
	if err != nil {
		return 0, err
	}
	job, cancel := context.WithCancel(ctx)
	monitored := make(chan struct{})
	failure := make(chan error, 1)
	go func() {
		defer close(monitored)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-job.Done():
				return
			case <-ticker.C:
				size, err := directoryBytes(job, filepath.Join(s.root, key), nil)
				if err == nil && size > s.budget.Job {
					err = errMediaBudget
				}
				if err == nil {
					var free int64
					free, err = diskFree(s.root)
					if err == nil && free < s.budget.MinFree {
						err = errMediaBudget
					}
				}
				if err != nil {
					failure <- err
					cancel()
					return
				}
			}
		}
	}()
	defer func() {
		cancel()
		<-monitored
		select {
		case err := <-failure:
			processErr = err
		default:
		}
		if processErr == nil {
			check, stop := context.WithTimeout(context.Background(), 5*time.Second)
			size, err := directoryBytes(check, filepath.Join(s.root, key), nil)
			stop()
			if err != nil {
				processErr = err
			} else if size > s.budget.Job {
				processErr = errMediaBudget
			}
		}
		if processErr != nil {
			for _, kind := range []string{"full", "preview"} {
				_ = os.RemoveAll(filepath.Join(s.root, key, kind))
			}
		}
		s.settleMedia(key)
	}()
	return s.transcodeFiles(job, key)
}
