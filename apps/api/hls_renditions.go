package main

import (
	"fmt"
	"math"
	"strings"
)

type rendition struct{ Width, Height, Bitrate int }

// Bound both dimensions, keep the source aspect ratio, and never upscale.
func videoRenditions(width, height int) []rendition {
	levels := []rendition{}
	for _, target := range []rendition{{640, 360, 800000}, {854, 480, 1400000}, {1280, 720, 2500000}, {1920, 1080, 5000000}, {2560, 1440, 9000000}, {3840, 2160, 16000000}} {
		if height < target.Height && len(levels) > 0 {
			continue
		}
		scale := math.Min(1, math.Min(float64(target.Width)/float64(width), float64(target.Height)/float64(height)))
		w, h := max(2, int(float64(width)*scale)/2*2), max(2, int(float64(height)*scale)/2*2)
		duplicate := false
		for _, level := range levels {
			if level.Height == h {
				duplicate = true
			}
		}
		if !duplicate {
			levels = append(levels, rendition{w, h, target.Bitrate})
		}
	}
	return levels
}
func masterPlaylist(levels []rendition) string {
	var result strings.Builder
	result.WriteString("#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-INDEPENDENT-SEGMENTS\n")
	for _, level := range levels {
		fmt.Fprintf(&result, "#EXT-X-STREAM-INF:BANDWIDTH=%d,RESOLUTION=%dx%d\n%dp.m3u8\n", level.Bitrate+128000, level.Width, level.Height, level.Height)
	}
	return result.String()
}
