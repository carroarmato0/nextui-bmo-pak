package face

import "testing"

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
