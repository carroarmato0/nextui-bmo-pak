package face

import (
	"bytes"
	"fmt"
	"image"
	"sync"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
)

// rasterCtx bundles the reusable scratch for one rasterization: the destination
// RGBA image and the rasterx dasher/scanner. The scanner embeds a
// vector.Rasterizer whose float/uint mask is the bulk of a rasterization's
// allocation (~3 MiB at 1024×768), and the destination image is another ~3 MiB.
// Reusing both across calls at a stable output size turns ~12 MB/call of
// transient churn into just the retained result buffer plus small parse
// overhead. See docs/profiling-findings-2026-06-18.md item #3.
type rasterCtx struct {
	img     *image.RGBA
	scanner *rasterx.ScannerGV
	dasher  *rasterx.Dasher
	w, h    int
}

// prepare readies the context for a w×h target. When the size is unchanged
// (the common case — BMO renders at one resolution) it reuses the existing
// buffers: the destination is zeroed so the scanner composites onto a clean
// transparent frame, and SetBounds resets the scanner's vector scratch in place
// (golang.org/x/image/vector.Rasterizer.Reset reuses capacity). On a size
// change it rebuilds; the old buffers are released to GC.
func (c *rasterCtx) prepare(w, h int) {
	if c.img == nil || c.w != w || c.h != h {
		c.img = image.NewRGBA(image.Rect(0, 0, w, h))
		c.scanner = rasterx.NewScannerGV(w, h, c.img, c.img.Bounds())
		c.dasher = rasterx.NewDasher(w, h, c.scanner)
		c.w, c.h = w, h
		return
	}
	clear(c.img.Pix) // fresh transparent destination; scanner composites over it
	c.scanner.Dest = c.img
	c.scanner.Targ = c.img.Bounds()
	c.dasher.SetBounds(w, h) // resets the vector scratch, reusing its capacity
}

// rasterPool hands out per-goroutine rasterCtx scratch. Rasterize is called
// concurrently (render loop, warm goroutine, animation frame builder), so the
// pool — not a single shared context — is what keeps the reuse race-free.
var rasterPool = sync.Pool{New: func() any { return &rasterCtx{} }}

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

	ctx := rasterPool.Get().(*rasterCtx)
	ctx.prepare(w, h)
	icon.Draw(ctx.dasher, 1.0)

	out := make([]uint32, w*h)
	pix := ctx.img.Pix
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
	// Recycle the scratch (result is already copied into out, which is the only
	// buffer that escapes). Safe even on the blank-render error path below.
	rasterPool.Put(ctx)

	if !anyOpaque {
		return nil, fmt.Errorf("SVG rendered blank (no opaque pixels)")
	}
	return out, nil
}
