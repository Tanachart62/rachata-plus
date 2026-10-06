package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func (s *videoService) worker(ctx context.Context) {
	// Only the lock owner may recover/claim jobs from the persistent queue.
	for ctx.Err() == nil {
		conn, err := s.db.Acquire(ctx)
		if err != nil {
			if !waitVideo(ctx) {
				return
			}
			continue
		}
		var locked bool
		err = conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(724286001)`).Scan(&locked)
		if err != nil || !locked {
			conn.Release()
			if !waitVideo(ctx) {
				return
			}
			continue
		}
		lease, cancel := context.WithCancel(ctx)
		checked := make(chan struct{})
		s.leaseActive.Store(true)
		go func() {
			defer close(checked)
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-lease.Done():
					return
				case <-ticker.C:
					ping, done := context.WithTimeout(lease, 3*time.Second)
					err := conn.Conn().Ping(ping)
					done()
					if err != nil {
						s.leaseActive.Store(false)
						cancel()
						return
					}
				}
			}
		}()
		s.workQueue(lease)
		cancel()
		<-checked
		s.leaseActive.Store(false)
		cleanup, stop := context.WithTimeout(context.Background(), 3*time.Second)
		conn.Exec(cleanup, `SELECT pg_advisory_unlock(724286001)`)
		stop()
		conn.Release()
		if !waitVideo(ctx) {
			return
		}
	}
}
func waitVideo(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(time.Second):
		return true
	}
}
func (s *videoService) workQueue(ctx context.Context) {
	if _, err := s.db.Exec(ctx, `UPDATE videos SET status='pending',processing_error='',updated_at=NOW() WHERE status='processing' AND deleted_at IS NULL AND storage_key IS NOT NULL`); err != nil {
		return
	}
	if _, err := s.db.Exec(ctx, `UPDATE videos SET status='failed' WHERE status='processing' AND deleted_at IS NOT NULL`); err != nil {
		return
	}
	for ctx.Err() == nil {
		s.cleanDeleted(ctx)
		s.cleanAbandonedUploads(ctx)
		var id int64
		var key string
		err := s.db.QueryRow(ctx, `WITH job AS (SELECT id FROM videos WHERE status='pending' AND deleted_at IS NULL AND storage_key IS NOT NULL ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1)
  UPDATE videos v SET status='processing',updated_at=NOW() FROM job WHERE v.id=job.id RETURNING v.id,v.storage_key`).Scan(&id, &key)
		if err != nil {
			if !waitVideo(ctx) {
				return
			}
			continue
		}
		job, stop := context.WithTimeout(ctx, 45*time.Minute)
		s.mu.Lock()
		s.running[id] = stop
		s.mu.Unlock()
		monitored := make(chan struct{})
		go func() {
			defer close(monitored)
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-job.Done():
					return
				case <-ticker.C:
					query, done := context.WithTimeout(job, 3*time.Second)
					var deleted bool
					err := s.db.QueryRow(query, `SELECT deleted_at IS NOT NULL FROM videos WHERE id=$1`, id).Scan(&deleted)
					done()
					if err == nil && deleted {
						stop()
						return
					}
				}
			}
		}()
		duration, processErr := s.transcode(job, key)
		stop()
		<-monitored
		s.mu.Lock()
		delete(s.running, id)
		s.mu.Unlock()
		if ctx.Err() != nil {
			return
		} // Keep processing for recovery after shutdown.
		for ctx.Err() == nil {
			update, done := context.WithTimeout(ctx, 5*time.Second)
			var saveErr error
			if processErr != nil {
				_, saveErr = s.db.Exec(update, `UPDATE videos SET status='failed',processing_error='แปลงวิดีโอไม่สำเร็จ กรุณาตรวจไฟล์แล้วลองใหม่',updated_at=NOW() WHERE id=$1`, id)
			} else {
				_, saveErr = s.db.Exec(update, `UPDATE videos SET status=CASE WHEN deleted_at IS NULL THEN 'ready' ELSE 'failed' END,duration_seconds=$2,processing_error='',updated_at=NOW() WHERE id=$1`, id, duration)
			}
			done()
			if saveErr == nil {
				break
			}
			log.Printf("video %d: retrying status update", id)
			if !waitVideo(ctx) {
				return
			}
		}
	}
}
func (s *videoService) cleanDeleted(ctx context.Context) {
	rows, err := s.db.Query(ctx, `SELECT id,storage_key FROM videos WHERE deleted_at IS NOT NULL AND storage_key IS NOT NULL AND status<>'processing' LIMIT 50`)
	if err != nil {
		return
	}
	type item struct {
		id  int64
		key string
	}
	items := []item{}
	for rows.Next() {
		var v item
		if rows.Scan(&v.id, &v.key) == nil {
			items = append(items, v)
		}
	}
	rows.Close()
	for _, v := range items {
		if storageKeyPattern.MatchString(v.key) && os.RemoveAll(filepath.Join(s.root, v.key)) == nil {
			s.db.Exec(ctx, `UPDATE videos SET storage_key=NULL WHERE id=$1 AND deleted_at IS NOT NULL`, v.id)
			s.releaseMedia(v.key)
		}
	}
}
func (s *videoService) transcodeFiles(ctx context.Context, key string) (int, error) {
	if !storageKeyPattern.MatchString(key) {
		return 0, fmt.Errorf("invalid storage key")
	}
	dir := filepath.Join(s.root, key)
	source := filepath.Join(dir, "source")
	probe, stop := context.WithTimeout(ctx, 30*time.Second)
	data, err := exec.CommandContext(probe, s.ffprobe, "-v", "error", "-protocol_whitelist", "file,pipe", "-format_whitelist", "mov,matroska,webm", "-select_streams", "v:0", "-show_entries", "format=duration:stream=codec_type,width,height", "-of", "json", source).Output()
	stop()
	if err != nil {
		return 0, fmt.Errorf("invalid source")
	}
	var metadata struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
		Streams []struct {
			Type   string `json:"codec_type"`
			Width  int    `json:"width"`
			Height int    `json:"height"`
		} `json:"streams"`
	}
	if json.Unmarshal(data, &metadata) != nil {
		return 0, fmt.Errorf("invalid probe")
	}
	seconds, err := strconv.ParseFloat(metadata.Format.Duration, 64)
	if err != nil || math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds <= 0 || seconds > 10800 {
		return 0, fmt.Errorf("invalid duration")
	}
	video := false
	for _, stream := range metadata.Streams {
		if stream.Type == "video" && stream.Width >= 2 && stream.Height >= 2 && stream.Width <= 7680 && stream.Height <= 4320 {
			video = true
		}
	}
	if !video {
		return 0, fmt.Errorf("no supported video stream")
	}
	for _, kind := range []string{"full", "preview"} {
		output := filepath.Join(dir, kind)
		if err = os.RemoveAll(output); err != nil {
			return 0, err
		}
		if err = os.Mkdir(output, 0700); err != nil {
			return 0, err
		}
		args := []string{"-nostdin", "-hide_banner", "-loglevel", "error", "-y", "-protocol_whitelist", "file,pipe", "-format_whitelist", "mov,matroska,webm", "-i", source, "-map", "0:v:0", "-map", "0:a:0?", "-vf", "scale=w='min(1280,iw)':h='min(720,ih)':force_original_aspect_ratio=decrease:force_divisible_by=2,setsar=1", "-c:v", "libx264", "-preset", "veryfast", "-crf", "23", "-r", "30", "-maxrate", "2500k", "-bufsize", "5000k", "-pix_fmt", "yuv420p", "-threads", strconv.Itoa(s.threads), "-force_key_frames", "expr:gte(t,n_forced*4)", "-c:a", "aac", "-b:a", "128k", "-ac", "2"}
		limit := seconds
		if kind == "preview" && limit > 20 {
			limit = 20
		}
		args = append(args, "-t", strconv.FormatFloat(limit, 'f', 3, 64))
		args = append(args, "-f", "hls", "-hls_time", "4", "-hls_playlist_type", "vod", "-hls_flags", "independent_segments", "-hls_segment_filename", filepath.Join(output, "segment_%06d.ts"), filepath.Join(output, "index.m3u8"))
		if err = exec.CommandContext(ctx, s.ffmpeg, args...).Run(); err != nil {
			return 0, fmt.Errorf("transcoding failed")
		}
		playlist, err := os.ReadFile(filepath.Join(output, "index.m3u8"))
		if err != nil || !strings.Contains(string(playlist), "#EXT-X-ENDLIST") {
			return 0, fmt.Errorf("incomplete HLS")
		}
	}
	return int(math.Ceil(seconds)), nil
}
