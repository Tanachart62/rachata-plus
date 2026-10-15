package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHighResolutionHLS(t *testing.T) {
	if os.Getenv("LOCAL_VIDEO_INTEGRATION") != "1" {
		t.Skip("set LOCAL_VIDEO_INTEGRATION=1 with FFmpeg")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	s := &videoService{root: t.TempDir()}
	if err := s.initTranscoder(2); err != nil {
		t.Fatal(err)
	}
	key := "44444444444444444444444444444444"
	dir := filepath.Join(s.root, key)
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	args := []string{"-nostdin", "-v", "error", "-f", "lavfi", "-i", "color=c=green:s=3840x2160:r=24", "-t", "2", "-c:v", "libx264", "-threads", "2", "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-f", "mp4", filepath.Join(dir, "source")}
	if data, err := exec.CommandContext(ctx, s.ffmpeg, args...).CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v %s", err, data)
	}
	if _, err := s.transcodeFiles(ctx, key); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"full", "preview"} {
		master, err := os.ReadFile(filepath.Join(dir, kind, "index.m3u8"))
		if err != nil || strings.Count(string(master), "#EXT-X-STREAM-INF:") != 6 {
			t.Fatal("missing high resolution variants", err)
		}
		for _, height := range []int{360, 480, 720, 1080, 1440, 2160} {
			playlist, err := os.ReadFile(filepath.Join(dir, kind, fmt.Sprintf("%dp.m3u8", height)))
			if err != nil || !strings.Contains(string(playlist), "#EXT-X-ENDLIST") {
				t.Fatal("invalid variant", height, err)
			}
			data, err := exec.CommandContext(ctx, s.ffprobe, "-v", "error", "-select_streams", "v:0", "-show_entries", "stream=height", "-of", "json", filepath.Join(dir, kind, fmt.Sprintf("segment_%dp_000000.ts", height))).Output()
			var info struct {
				Streams []struct {
					Height int `json:"height"`
				} `json:"streams"`
			}
			if err != nil || json.Unmarshal(data, &info) != nil || len(info.Streams) == 0 || info.Streams[0].Height != height {
				t.Fatal("encoded height mismatch", height, err)
			}
		}
	}
}
