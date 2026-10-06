package main

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"path/filepath"
	"testing"
)

func TestMediaBudgetBounds(t *testing.T) {
	t.Setenv("MEDIA_JOB_BYTES", "100")
	if _, err := readMediaBudget(); err == nil {
		t.Fatal("job must fit a source")
	}
}
func TestDirectoryBytes(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "one"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "one", "source"), make([]byte, 17), 0600); err != nil {
		t.Fatal(err)
	}
	bytes, err := directoryBytes(context.Background(), root, nil)
	if err != nil || bytes != 17 {
		t.Fatal(bytes, err)
	}
	bytes, err = directoryBytes(context.Background(), root, map[string]int64{"one": 100})
	if err != nil || bytes != 0 {
		t.Fatal(bytes, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = directoryBytes(ctx, root, nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
func testMediaReservations(t *testing.T, ctx context.Context, db *pgxpool.Pool) {
	t.Helper()
	s, err := newVideoService(db, nil, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s.budget = mediaBudget{Total: 1500, Job: 1000, MinFree: 1}
	key1, key2 := "11111111111111111111111111111111", "22222222222222222222222222222222"
	if err = s.reserveMedia(ctx, key1, true); err != nil {
		t.Fatal(err)
	}
	if err = s.reserveMedia(ctx, key2, true); !errors.Is(err, errMediaBudget) {
		t.Fatal("must reject overcommitted jobs", err)
	}
	s.releaseMedia(key1)
	if err = s.reserveMedia(ctx, key2, true); err != nil {
		t.Fatal("released reservation must be reusable", err)
	}
	if err = os.Mkdir(filepath.Join(s.root, key2), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(s.root, key2, "source"), make([]byte, 20), 0600); err != nil {
		t.Fatal(err)
	}
	s.settleMedia(key2)
	var bytes int64
	if err = db.QueryRow(ctx, `SELECT reserved_bytes FROM media_reservations WHERE storage_key=$1`, key2).Scan(&bytes); err != nil || bytes != 20 {
		t.Fatal(bytes, err)
	}
}
