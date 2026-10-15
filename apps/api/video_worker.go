package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// All worker DB commands use the connection holding the advisory lock.
// A lost connection cannot commit through a replacement pool connection.
func (s *videoService) worker(ctx context.Context) {
	for ctx.Err() == nil {
		s.workQueue(ctx)
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
	acquire, cancelAcquire := context.WithTimeout(ctx, 5*time.Second)
	conn, err := s.db.Acquire(acquire)
	cancelAcquire()
	if err != nil {
		return
	}
	defer conn.Release()
	initial, cancelInitial := context.WithTimeout(ctx, 5*time.Second)
	defer cancelInitial()
	var locked bool
	if err = conn.QueryRow(initial, `SELECT pg_try_advisory_lock(724286001)`).Scan(&locked); err != nil || !locked {
		return
	}
	s.leaseActive.Store(true)
	defer func() {
		s.leaseActive.Store(false)
		cleanup, stop := context.WithTimeout(context.Background(), 3*time.Second)
		defer stop()
		if _, err := conn.Exec(cleanup, `SELECT pg_advisory_unlock(724286001)`); err != nil {
			_ = conn.Conn().Close(cleanup)
		}
	}()
	if _, err = conn.Exec(initial, `UPDATE videos SET status='pending',processing_error='',updated_at=NOW() WHERE status='processing' AND deleted_at IS NULL AND storage_key IS NOT NULL`); err != nil {
		return
	}
	if _, err = conn.Exec(initial, `UPDATE videos SET status='failed' WHERE status='processing' AND deleted_at IS NOT NULL`); err != nil {
		return
	}
	for ctx.Err() == nil {
		id, key, generation, err := s.nextJob(ctx, conn)
		if errors.Is(err, pgx.ErrNoRows) {
			if !waitVideo(ctx) {
				return
			}
			continue
		}
		if err != nil {
			return
		}
		duration, processErr, owned := s.processOwnedJob(ctx, conn, id, key, generation)
		if !owned || ctx.Err() != nil {
			return
		} // Recovery belongs to the next owner.
		if processErr != nil {
			// Internal errors only; never include source metadata, codec stderr,
			// session cookies or playback tokens in operational logs.
			log.Printf("video worker: processing failed id=%d generation=%d: %v", id, generation, processErr)
		}
		update, done := context.WithTimeout(ctx, 10*time.Second)
		err = s.finishJob(update, conn, id, generation, duration, processErr)
		if err == nil {
			err = s.settleMediaOn(update, conn, key)
		}
		done()
		if err != nil {
			return
		}
	}
}
func (s *videoService) nextJob(ctx context.Context, conn rowExecutor) (id int64, key string, generation int64, err error) {
	query, done := context.WithTimeout(ctx, 5*time.Second)
	defer done()
	if err = s.cleanDeleted(query, conn); err != nil {
		return
	}
	if err = s.cleanAbandonedUploads(query, conn); err != nil {
		return
	}
	err = conn.QueryRow(query, `WITH job AS (SELECT id FROM videos WHERE status='pending' AND deleted_at IS NULL AND storage_key IS NOT NULL ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1)
	UPDATE videos v SET status='processing',processing_generation=v.processing_generation+1,updated_at=NOW() FROM job WHERE v.id=job.id RETURNING v.id,v.storage_key,v.processing_generation`).Scan(&id, &key, &generation)
	return
}
func (s *videoService) finishJob(ctx context.Context, conn rowExecutor, id, generation int64, duration int, processErr error) error {
	if processErr != nil {
		_, err := conn.Exec(ctx, `UPDATE videos SET status='failed',processing_error='แปลงวิดีโอไม่สำเร็จ กรุณาตรวจไฟล์แล้วลองใหม่',updated_at=NOW() WHERE id=$1 AND processing_generation=$2 AND status='processing'`, id, generation)
		return err
	}
	_, err := conn.Exec(ctx, `UPDATE videos SET status=CASE WHEN deleted_at IS NULL THEN 'ready' ELSE 'failed' END,duration_seconds=$3,hls_generation=$2,processing_error='',updated_at=NOW() WHERE id=$1 AND processing_generation=$2 AND status='processing'`, id, generation, duration)
	return err
}
func (s *videoService) processOwnedJob(ctx context.Context, conn *pgxpool.Conn, id int64, key string, generation int64) (int, error, bool) {
	reserve, done := context.WithTimeout(ctx, 10*time.Second)
	err := s.reserveMediaOn(reserve, conn, key, false)
	done()
	if err != nil {
		return 0, err, !conn.Conn().IsClosed()
	}
	job, stop := context.WithTimeout(ctx, 45*time.Minute)
	defer stop()
	// Create only this generation before starting codecs. Codecs cannot recreate
	// a deleted video's parent directory after another owner cleans its tombstone.
	if !storageKeyPattern.MatchString(key) {
		return 0, fmt.Errorf("invalid storage key"), true
	}
	jobs := filepath.Join(s.root, key, "jobs")
	if err = os.Mkdir(jobs, 0700); err != nil && !os.IsExist(err) {
		return 0, err, true
	}
	// A stale generation must never delete a newer directory, even if ownership
	// was lost immediately after its last DB command.
	entries, err := os.ReadDir(jobs)
	if err != nil {
		return 0, err, true
	}
	for _, entry := range entries {
		n, parseErr := strconv.ParseInt(strings.TrimPrefix(entry.Name(), "g"), 10, 64)
		if parseErr == nil && entry.Name() == "g"+strconv.FormatInt(n, 10) && n > 0 && n < generation {
			if err = os.RemoveAll(filepath.Join(jobs, entry.Name())); err != nil {
				return 0, err, true
			}
		}
	}
	for _, kind := range []string{"full", "preview"} {
		if err = os.RemoveAll(filepath.Join(s.root, key, kind)); err != nil {
			return 0, err, true
		}
	}
	output := filepath.Join(s.root, hlsDirectory(key, generation))
	if err = os.Mkdir(output, 0700); err != nil {
		return 0, err, true
	}
	s.mu.Lock()
	s.running[id] = stop
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.running, id); s.mu.Unlock() }()
	type result struct {
		duration int
		err      error
	}
	finished := make(chan result, 1)
	go func() { d, e := s.transcodeGeneration(job, key, generation); finished <- result{d, e} }()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case result := <-finished:
			// The completion write still uses this exact connection; no reconnect.
			return result.duration, result.err, true
		case <-ctx.Done():
			stop()
			<-finished
			return 0, ctx.Err(), false
		case <-ticker.C:
			check, cancel := context.WithTimeout(ctx, 3*time.Second)
			var deleted bool
			var current int64
			err = conn.QueryRow(check, `SELECT deleted_at IS NOT NULL,processing_generation FROM videos WHERE id=$1`, id).Scan(&deleted, &current)
			cancel()
			if err != nil {
				stop()
				<-finished
				return 0, err, false
			}
			if deleted || current != generation {
				stop()
				result := <-finished
				return result.duration, result.err, current == generation
			}
		}
	}
}
func (s *videoService) cleanDeleted(ctx context.Context, conn rowExecutor) error {
	rows, err := conn.Query(ctx, `SELECT id,storage_key FROM videos WHERE deleted_at IS NOT NULL AND storage_key IS NOT NULL AND status<>'processing' LIMIT 50`)
	if err != nil {
		return err
	}
	type item struct {
		id  int64
		key string
	}
	items := []item{}
	for rows.Next() {
		var v item
		if err = rows.Scan(&v.id, &v.key); err != nil {
			break
		}
		items = append(items, v)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return err
	}
	for _, v := range items {
		if storageKeyPattern.MatchString(v.key) && os.RemoveAll(filepath.Join(s.root, v.key)) == nil {
			if _, err = conn.Exec(ctx, `UPDATE videos SET storage_key=NULL WHERE id=$1 AND deleted_at IS NOT NULL AND status<>'processing'`, v.id); err != nil {
				return err
			}
			if _, err = conn.Exec(ctx, `DELETE FROM media_reservations WHERE storage_key=$1`, v.key); err != nil {
				return err
			}
		}
	}
	return nil
}
func (s *videoService) transcodeFiles(ctx context.Context, key string) (int, error) {
	return s.transcodeFilesGeneration(ctx, key, 0)
}
func (s *videoService) transcodeFilesGeneration(ctx context.Context, key string, generation int64) (int, error) {
	if !storageKeyPattern.MatchString(key) {
		return 0, fmt.Errorf("invalid storage key")
	}
	dir := filepath.Join(s.root, key)
	source := filepath.Join(dir, "source")
	probe, stop := context.WithTimeout(ctx, 30*time.Second)
	data, err := exec.CommandContext(probe, s.ffprobe, "-v", "error", "-protocol_whitelist", "file,pipe", "-format_whitelist", "mov,matroska,webm", "-select_streams", "v:0", "-show_entries", "format=duration:stream=codec_type,width,height:stream_side_data=rotation", "-of", "json", source).Output()
	stop()
	if err != nil {
		return 0, fmt.Errorf("invalid source")
	}
	var metadata struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
		Streams []struct {
			SideData []struct {
				Rotation float64 `json:"rotation"`
			} `json:"side_data_list"`
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
	if len(metadata.Streams) == 0 {
		return 0, fmt.Errorf("no supported video stream")
	}
	stream := metadata.Streams[0]
	if stream.Type != "video" || stream.Width < 2 || stream.Height < 2 || stream.Width > 7680 || stream.Height > 4320 {
		return 0, fmt.Errorf("no supported video stream")
	}
	width, height := stream.Width, stream.Height
	for _, side := range stream.SideData {
		if int(math.Abs(side.Rotation))%180 == 90 {
			width, height = height, width
			break
		}
	}
	levels := videoRenditions(width, height)
	for _, kind := range []string{"full", "preview"} {
		output := filepath.Join(s.root, hlsDirectory(key, generation), kind)
		if err = ctx.Err(); err != nil {
			return 0, err
		}
		if err = os.RemoveAll(output); err != nil {
			return 0, err
		}
		if err = os.Mkdir(output, 0700); err != nil {
			return 0, err
		}
		limit := seconds
		if kind == "preview" && limit > 20 {
			limit = 20
		}
		for _, level := range levels {
			args := []string{"-nostdin", "-hide_banner", "-loglevel", "error", "-y", "-protocol_whitelist", "file,pipe", "-format_whitelist", "mov,matroska,webm", "-threads", strconv.Itoa(s.threads), "-filter_threads", "1", "-i", source, "-map", "0:v:0", "-map", "0:a:0?", "-vf", fmt.Sprintf("scale=%d:%d,setsar=1", level.Width, level.Height), "-c:v", "libx264", "-preset", "veryfast", "-tune", "zerolatency", "-crf", "23", "-r", "30", "-maxrate", strconv.Itoa(level.Bitrate), "-bufsize", strconv.Itoa(level.Bitrate * 2), "-pix_fmt", "yuv420p", "-threads", strconv.Itoa(s.threads), "-force_key_frames", "expr:gte(t,n_forced*4)", "-c:a", "aac", "-b:a", "128k", "-ac", "2", "-t", strconv.FormatFloat(limit, 'f', 3, 64), "-f", "hls", "-hls_time", "4", "-hls_playlist_type", "vod", "-hls_flags", "independent_segments", "-hls_segment_filename", filepath.Join(output, fmt.Sprintf("segment_%dp_%%06d.ts", level.Height)), filepath.Join(output, fmt.Sprintf("%dp.m3u8", level.Height))}
			if err = exec.CommandContext(ctx, s.ffmpeg, args...).Run(); err != nil {
				return 0, fmt.Errorf("transcoding failed: %w", err)
			}
			playlist, err := os.ReadFile(filepath.Join(output, fmt.Sprintf("%dp.m3u8", level.Height)))
			if err != nil || !strings.Contains(string(playlist), "#EXT-X-ENDLIST") {
				return 0, fmt.Errorf("incomplete HLS")
			}
		}
		if err = os.WriteFile(filepath.Join(output, "index.m3u8"), []byte(masterPlaylist(levels)), 0600); err != nil {
			return 0, err
		}
	}
	return int(math.Ceil(seconds)), nil
}
