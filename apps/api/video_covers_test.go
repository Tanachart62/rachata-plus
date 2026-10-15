package main

import (
	"bytes"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveCover(t *testing.T) {
	for _, format := range []string{"png", "jpeg"} {
		t.Run(format, func(t *testing.T) {
			var data bytes.Buffer
			picture := image.NewRGBA(image.Rect(0, 0, 32, 18))
			if format == "png" {
				png.Encode(&data, picture)
			} else {
				jpeg.Encode(&data, picture, nil)
			}
			path := filepath.Join(t.TempDir(), "cover.jpg")
			if err := saveCover(&data, path); err != nil {
				t.Fatal(err)
			}
			file, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			config, got, err := image.DecodeConfig(file)
			if err != nil || got != "jpeg" || config.Width != 32 || config.Height != 18 {
				t.Fatal("invalid normalized cover")
			}
		})
	}
	var excessive bytes.Buffer
	png.Encode(&excessive, image.NewGray(image.Rect(0, 0, 4097, 1)))
	for _, data := range [][]byte{nil, []byte("<svg onload='alert(1)'></svg>"), bytes.Repeat([]byte{'x'}, int(maxCoverBytes+1)), excessive.Bytes()} {
		path := filepath.Join(t.TempDir(), "cover.jpg")
		if saveCover(bytes.NewReader(data), path) == nil {
			t.Fatal("invalid cover accepted")
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("invalid cover retained")
		}
	}
}
