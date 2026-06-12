package face

import (
	"bytes"
	"fmt"
	"image"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
)

// Rasterize renders svg into a w×h ARGB8888 pixel buffer (row-major,
// stride == w), stretching the viewBox non-uniformly to fill the full target.
func Rasterize(svg []byte, w, h int) ([]uint32, error) {
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("invalid raster size %dx%d", w, h)
	}
	icon, err := oksvg.ReadIconStream(bytes.NewReader(svg), oksvg.WarnErrorMode)
	if err != nil {
		return nil, fmt.Errorf("parse SVG: %w", err)
	}
	icon.SetTarget(0, 0, float64(w), float64(h))
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	scanner := rasterx.NewScannerGV(w, h, img, img.Bounds())
	icon.Draw(rasterx.NewDasher(w, h, scanner), 1.0)

	out := make([]uint32, w*h)
	pix := img.Pix
	anyOpaque := false
	for i := 0; i < w*h; i++ {
		r := uint32(pix[i*4])
		g := uint32(pix[i*4+1])
		b := uint32(pix[i*4+2])
		a := uint32(pix[i*4+3])
		out[i] = a<<24 | r<<16 | g<<8 | b
		if a > 0 {
			anyOpaque = true
		}
	}
	if !anyOpaque {
		return nil, fmt.Errorf("SVG rendered blank (no opaque pixels)")
	}
	return out, nil
}
