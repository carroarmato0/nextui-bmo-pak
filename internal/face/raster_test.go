package face

import (
	"bytes"
	"fmt"
	"runtime"
	"sync"
	"testing"
)

// altSVG is intentionally different from testSVG (different colours and a
// smaller shape) so a reused, un-zeroed destination or un-cleared scanner
// scratch would visibly bleed across calls.
const altSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210">
  <rect x="0" y="0" width="280" height="210" fill="#00ff00"/>
  <circle cx="70" cy="52" r="15" fill="#ffffff"/>
</svg>`

const testSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210">
  <rect x="0" y="0" width="280" height="210" fill="#ff0000"/>
  <circle cx="140" cy="105" r="30" fill="#0000ff"/>
</svg>`

// argbAt maps a viewBox coordinate to a pixel in a w×h buffer, sampling at
// the stretched position. Returns R, G, B.
func argbAt(buf []uint32, w, h int, vx, vy float64) (uint8, uint8, uint8) {
	x := int(vx / 280.0 * float64(w))
	y := int(vy / 210.0 * float64(h))
	if x >= w {
		x = w - 1
	}
	if y >= h {
		y = h - 1
	}
	px := buf[y*w+x]
	return uint8(px >> 16), uint8(px >> 8), uint8(px)
}

func near(a, b uint8) bool {
	d := int(a) - int(b)
	return d >= -8 && d <= 8
}

func assertColor(t *testing.T, buf []uint32, w, h int, vx, vy float64, wR, wG, wB uint8, label string) {
	t.Helper()
	r, g, b := argbAt(buf, w, h, vx, vy)
	if !near(r, wR) || !near(g, wG) || !near(b, wB) {
		t.Errorf("%s at (%.0f,%.0f): got (%d,%d,%d) want (%d,%d,%d)", label, vx, vy, r, g, b, wR, wG, wB)
	}
}

func TestRasterizeStretchesToFill(t *testing.T) {
	// 1280x720 is 16:9 against the 4:3 viewBox: circle must land at the
	// stretched position, proving non-uniform scaling (no letterboxing).
	for _, size := range [][2]int{{1024, 768}, {1280, 720}} {
		w, h := size[0], size[1]
		buf, err := Rasterize([]byte(testSVG), w, h)
		if err != nil {
			t.Fatalf("Rasterize(%dx%d): %v", w, h, err)
		}
		if len(buf) != w*h {
			t.Fatalf("buffer length %d, want %d", len(buf), w*h)
		}
		// viewBox centre (140,105) should be blue (circle)
		assertColor(t, buf, w, h, 140, 105, 0, 0, 0xff, "circle centre")
		// corner (5,5) should be red (background)
		assertColor(t, buf, w, h, 5, 5, 0xff, 0, 0, "background corner")
	}
}

func TestRasterizeInvalidSize(t *testing.T) {
	_, err := Rasterize([]byte(testSVG), 0, 480)
	if err == nil {
		t.Fatal("expected error for zero width")
	}
}

func TestRasterizeGarbageInput(t *testing.T) {
	// oksvg silently parses non-XML as an empty icon; blank-output guard must catch it.
	_, err := Rasterize([]byte("not an svg at all"), 100, 100)
	if err == nil {
		t.Fatal("expected error for garbage input")
	}
}

// TestRasterizeReuseNoBleed guards the pooling invariant: rasterizing a second,
// different SVG between two renders of the same SVG must not change the result.
// A reused destination that isn't zeroed, or scanner scratch that isn't
// cleared, would let altSVG's pixels bleed into the second render of testSVG.
func TestRasterizeReuseNoBleed(t *testing.T) {
	const w, h = 256, 192
	first, err := Rasterize([]byte(testSVG), w, h)
	if err != nil {
		t.Fatalf("first Rasterize: %v", err)
	}
	if _, err := Rasterize([]byte(altSVG), w, h); err != nil {
		t.Fatalf("interleaved Rasterize: %v", err)
	}
	again, err := Rasterize([]byte(testSVG), w, h)
	if err != nil {
		t.Fatalf("repeat Rasterize: %v", err)
	}
	if len(first) != len(again) {
		t.Fatalf("length drift: %d vs %d", len(first), len(again))
	}
	for i := range first {
		if first[i] != again[i] {
			t.Fatalf("pixel %d differs after interleaved render: %08x vs %08x", i, first[i], again[i])
		}
	}
}

// TestRasterizeSizeChangeInterleave exercises the rebuild path: a render at a
// different size between two same-size renders must still produce correct,
// identical output for the repeated size.
func TestRasterizeSizeChangeInterleave(t *testing.T) {
	const w, h = 200, 150
	first, err := Rasterize([]byte(testSVG), w, h)
	if err != nil {
		t.Fatalf("first Rasterize: %v", err)
	}
	if _, err := Rasterize([]byte(testSVG), 320, 240); err != nil {
		t.Fatalf("different-size Rasterize: %v", err)
	}
	again, err := Rasterize([]byte(testSVG), w, h)
	if err != nil {
		t.Fatalf("repeat Rasterize: %v", err)
	}
	for i := range first {
		if first[i] != again[i] {
			t.Fatalf("pixel %d differs after size-change render: %08x vs %08x", i, first[i], again[i])
		}
	}
	// Spot-check correctness survived the rebuild path.
	assertColor(t, again, w, h, 140, 105, 0, 0, 0xff, "circle centre")
	assertColor(t, again, w, h, 5, 5, 0xff, 0, 0, "background corner")
}

// TestRasterizeConcurrent runs many rasterizations across goroutines (with
// pooled scratch shared via sync.Pool) and asserts each matches its sequential
// reference. Run under -race to catch pool misuse / shared-state bugs.
func TestRasterizeConcurrent(t *testing.T) {
	const w, h = 192, 144
	svgs := [][]byte{[]byte(testSVG), []byte(altSVG)}
	refs := make([][]uint32, len(svgs))
	for i, s := range svgs {
		buf, err := Rasterize(s, w, h)
		if err != nil {
			t.Fatalf("reference Rasterize %d: %v", i, err)
		}
		refs[i] = buf
	}

	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for g := 0; g < 32; g++ {
		wg.Add(1)
		go func(sel int) {
			defer wg.Done()
			buf, err := Rasterize(svgs[sel], w, h)
			if err != nil {
				errs <- fmt.Errorf("goroutine Rasterize: %w", err)
				return
			}
			if !bytes.Equal(uint32sAsBytes(buf), uint32sAsBytes(refs[sel])) {
				errs <- fmt.Errorf("goroutine result for svg %d differs from reference", sel)
			}
		}(g % len(svgs))
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

// TestRasterizeAllocationBounded is the optimization-driving test: at a stable
// output size, repeated rasterizations must not re-allocate the destination
// image and scanner scratch on every call. With per-call scratch reuse, only
// the retained result buffer (w*h*4 bytes) plus small SVG-parse overhead is
// allocated, well under the unpooled ~3× that.
func TestRasterizeAllocationBounded(t *testing.T) {
	const w, h = 1024, 768
	const iters = 8
	// Warm the pool so the first (allocating) call isn't counted.
	if _, err := Rasterize([]byte(testSVG), w, h); err != nil {
		t.Fatalf("warmup Rasterize: %v", err)
	}

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	for i := 0; i < iters; i++ {
		if _, err := Rasterize([]byte(testSVG), w, h); err != nil {
			t.Fatalf("Rasterize iter %d: %v", i, err)
		}
	}
	runtime.ReadMemStats(&after)

	perOp := (after.TotalAlloc - before.TotalAlloc) / iters
	resultBytes := uint64(w * h * 4) // the unavoidable retained out buffer
	// Allow result buffer + generous parse/overhead headroom, but far below
	// the unpooled footprint (result + dest image + scanner scratch ≈ 3×).
	limit := resultBytes * 2
	if perOp > limit {
		t.Errorf("per-call allocation %d bytes exceeds %d (result buffer %d); scratch not being reused",
			perOp, limit, resultBytes)
	}
	t.Logf("per-call allocation: %d bytes (result buffer %d)", perOp, resultBytes)
}

// uint32sAsBytes reinterprets a uint32 slice as bytes for a fast equality
// check; it is only used in tests.
func uint32sAsBytes(s []uint32) []byte {
	b := make([]byte, len(s)*4)
	for i, v := range s {
		b[i*4] = byte(v)
		b[i*4+1] = byte(v >> 8)
		b[i*4+2] = byte(v >> 16)
		b[i*4+3] = byte(v >> 24)
	}
	return b
}
