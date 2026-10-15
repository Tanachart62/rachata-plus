package main

import (
	"strings"
	"testing"
)

func TestVideoRenditions(t *testing.T) {
	for _, test := range []struct{ width, height, count int }{{3840, 2160, 6}, {2560, 1440, 5}, {1920, 1080, 4}, {1280, 720, 3}, {854, 480, 2}, {320, 180, 1}, {720, 1280, 4}, {2560, 1080, 4}} {
		levels := videoRenditions(test.width, test.height)
		if len(levels) != test.count {
			t.Fatalf("%dx%d: got %v", test.width, test.height, levels)
		}
		for _, level := range levels {
			if level.Width > test.width || level.Height > test.height || level.Width > 3840 || level.Height > 2160 || level.Width%2 != 0 || level.Height%2 != 0 {
				t.Fatal("invalid output size", level)
			}
		}
		master := masterPlaylist(levels)
		if strings.Count(master, "#EXT-X-STREAM-INF:") != test.count || strings.Contains(master, "#EXT-X-ENDLIST") {
			t.Fatal("invalid master playlist")
		}
	}
}
func TestMediaFileAllowlist(t *testing.T) {
	for _, name := range []string{"index.m3u8", "360p.m3u8", "720p.m3u8", "1080p.m3u8", "1440p.m3u8", "2160p.m3u8", "segment_2160p_000001.ts", "segment_480p_000001.ts", "segment_000001.ts"} {
		if !mediaFilePattern.MatchString(name) {
			t.Fatal("valid file rejected", name)
		}
	}
	for _, name := range []string{"source", "../360p.m3u8", "cover.jpg", "360p/source", "segment_720p_000001.ts/../source"} {
		if mediaFilePattern.MatchString(name) {
			t.Fatal("unsafe file accepted", name)
		}
	}
}
