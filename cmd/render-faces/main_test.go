package main

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestRenderOneProducesPNG(t *testing.T) {
	src := filepath.Join("..", "..", "internal", "face", "assets", "neutral.svg")
	dst := filepath.Join(t.TempDir(), "neutral.png")

	if err := renderOne(src, dst); err != nil {
		t.Fatalf("renderOne: %v", err)
	}

	f, err := os.Open(dst)
	if err != nil {
		t.Fatalf("open output: %v", err)
	}
	defer f.Close()

	cfg, err := png.DecodeConfig(f)
	if err != nil {
		t.Fatalf("decode png: %v", err)
	}
	if cfg.Width != width || cfg.Height != height {
		t.Fatalf("got %dx%d, want %dx%d", cfg.Width, cfg.Height, width, height)
	}
}
