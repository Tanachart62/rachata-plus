package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func testWorkerFencing(t *testing.T, ctx context.Context, db *pgxpool.Pool) {
	s, err := newVideoService(db, nil, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	key := "55555555555555555555555555555555"
	for _, generation := range []int64{1, 2} {
		for _, kind := range []string{"full", "preview"} {
			dir := filepath.Join(s.root, hlsDirectory(key, generation), kind)
			if err = os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(dir, "index.m3u8"), []byte("successor content"), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err = os.WriteFile(filepath.Join(s.root, key, "source"), []byte("dummy"), 0600); err != nil {
		t.Fatal(err)
	}
	var id int64
	if err = db.QueryRow(ctx, `INSERT INTO videos(title,status,storage_key,processing_generation) VALUES('fence fixture','processing',$1,2) RETURNING id`, key).Scan(&id); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(context.Background(), `DELETE FROM videos WHERE id=$1`, id)
	old, err := db.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Release()
	var locked bool
	if err = old.QueryRow(ctx, `SELECT pg_try_advisory_lock(724286001)`).Scan(&locked); err != nil || !locked {
		t.Fatal("cannot acquire worker lock", err)
	}
	successor, err := db.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer successor.Release()
	if err = successor.QueryRow(ctx, `SELECT pg_try_advisory_lock(724286001)`).Scan(&locked); err != nil || locked {
		t.Fatal("two worker owners", err)
	}
	// A real DB backend termination releases ownership. The old connection must
	// fail its completion instead of borrowing an unrelated healthy connection.
	var terminated bool
	if err = successor.QueryRow(ctx, `SELECT pg_terminate_backend($1)`, old.Conn().PgConn().PID()).Scan(&terminated); err != nil || !terminated {
		t.Fatal(err)
	}
	// pg_terminate_backend acknowledges the signal; backend shutdown and lock
	// release can finish after that response on PostgreSQL 17.
	deadline := time.Now().Add(3 * time.Second)
	for {
		if err = successor.QueryRow(ctx, `SELECT pg_try_advisory_lock(724286001)`).Scan(&locked); err != nil {
			t.Fatal("successor cannot acquire lock", err)
		}
		if locked {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("terminated backend did not release worker lock")
		}
		time.Sleep(10 * time.Millisecond)
	}
	defer successor.Exec(context.Background(), `SELECT pg_advisory_unlock(724286001)`)
	if err = s.finishJob(ctx, old, id, 1, 999, nil); err == nil {
		t.Fatal("disconnected worker completed a job")
	}
	// Even an accidental healthy connection cannot publish an old generation.
	if err = s.finishJob(ctx, successor, id, 1, 999, nil); err != nil {
		t.Fatal(err)
	}
	var state string
	var duration int
	var generation int64
	if err = db.QueryRow(ctx, `SELECT status,duration_seconds,hls_generation FROM videos WHERE id=$1`, id).Scan(&state, &duration, &generation); err != nil || state != "processing" || duration != 0 || generation != 0 {
		t.Fatal("stale completion changed successor", state, duration, generation, err)
	}
	cancelled, stop := context.WithCancel(ctx)
	stop()
	if _, err = s.transcodeGeneration(cancelled, key, 1); err == nil {
		t.Fatal("cancelled job unexpectedly succeeded")
	}
	for _, kind := range []string{"full", "preview"} {
		if _, err = os.Stat(filepath.Join(s.root, hlsDirectory(key, 1), kind)); !os.IsNotExist(err) {
			t.Fatal("failed generation retained output", err)
		}
		if data, err := os.ReadFile(filepath.Join(s.root, hlsDirectory(key, 2), kind, "index.m3u8")); err != nil || string(data) != "successor content" {
			t.Fatal("old cleanup removed new output", err)
		}
	}
	if err = s.finishJob(ctx, successor, id, 2, 12, nil); err != nil {
		t.Fatal(err)
	}
	if err = s.finishJob(ctx, successor, id, 1, 0, errors.New("late failure")); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(ctx, `SELECT status,duration_seconds,hls_generation FROM videos WHERE id=$1`, id).Scan(&state, &duration, &generation); err != nil || state != "ready" || duration != 12 || generation != 2 {
		t.Fatal("late failure changed finished successor", state, duration, generation, err)
	}
}
