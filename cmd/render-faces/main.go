// Command render-faces rasterizes the embedded BMO face SVGs to PNGs for the
// documentation gallery, using the same oksvg path the device uses so the
// images match on-device rendering. Run from the repo root: go run ./cmd/render-faces
package main

import (
	"fmt"
	"image"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/carroarmato0/nextui-bmo/internal/face"
)

const (
	srcDir = "internal/face/assets"
	outDir = "docs/images/faces"
	width  = 480
	height = 360
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	matches, err := filepath.Glob(filepath.Join(srcDir, "*.svg"))
	if err != nil {
		return err
	}
	if len(matches) == 0 {
		return fmt.Errorf("no SVGs found in %s", srcDir)
	}
	for _, src := range matches {
		name := strings.TrimSuffix(filepath.Base(src), ".svg")
		dst := filepath.Join(outDir, name+".png")
		if err := renderOne(src, dst); err != nil {
			return fmt.Errorf("render %s: %w", name, err)
		}
		fmt.Printf("rendered %s\n", dst)
	}
	return nil
}

func renderOne(src, dst string) error {
	svg, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	buf, err := face.Rasterize(svg, width, height)
	if err != nil {
		return err
	}
	// Rasterize returns row-major ARGB8888 (a<<24|r<<16|g<<8|b) from an
	// alpha-premultiplied image.RGBA; reverse it back into image.RGBA bytes.
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for i, px := range buf {
		img.Pix[i*4] = byte(px >> 16)   // R
		img.Pix[i*4+1] = byte(px >> 8)  // G
		img.Pix[i*4+2] = byte(px)       // B
		img.Pix[i*4+3] = byte(px >> 24) // A
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
