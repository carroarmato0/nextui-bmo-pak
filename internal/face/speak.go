package face

import (
	"bytes"
	"fmt"
	"math"
	"text/template"
)

// speakLevels is the number of mouth-openness frames in the talking animation
// (level 0 = closed, plus speakLevels-1 open levels). Each level is a full SVG
// rasterization during warm-up, so this directly sets the warm cost; 6 keeps
// the speaking face ready within ~1s on the slow Mali GPU while still reading
// as smooth lip movement.
const speakLevels = 6

// SpeakParams holds the computed mouth geometry for a single openness level.
type SpeakParams struct {
	MouthH       float64 // mouth rect height in viewBox units (6..36)
	MouthRx      float64 // mouth rect corner radius
	TeethPath    string
	InteriorPath string
	TonguePath   string
}

// IsSpeakTemplate reports whether data is a Go template (contains "{{").
func IsSpeakTemplate(data []byte) bool {
	return bytes.Contains(data, []byte("{{"))
}

// speakParams computes mouth geometry for openness t ∈ [0,1].
func speakParams(t float64) SpeakParams {
	t = math.Max(0, math.Min(1, t))
	const left, right, top = 106.0, 174.0, 106.0
	h := 6 + 30*t
	r := math.Min(16, h/2)
	bottom := top + h

	// Teeth band: top arc intersects the rounded corners.
	th := 0.28 * h
	dy := r - th
	dx := math.Sqrt(r*r - dy*dy)
	tlx, trx := left+r-dx, right-r+dx
	tby := top + th

	teeth := fmt.Sprintf(
		"M %.2f %.2f A %.2f %.2f 0 0 1 %.2f %.2f L %.2f %.2f A %.2f %.2f 0 0 1 %.2f %.2f Z",
		tlx, tby, r, r, left+r, top, right-r, top, r, r, trx, tby)

	interior := fmt.Sprintf(
		"M %.2f %.2f L %.2f %.2f "+
			"A %.2f %.2f 0 0 1 %.2f %.2f L %.2f %.2f "+
			"A %.2f %.2f 0 0 1 %.2f %.2f L %.2f %.2f "+
			"A %.2f %.2f 0 0 1 %.2f %.2f L %.2f %.2f "+
			"A %.2f %.2f 0 0 1 %.2f %.2f Z",
		tlx, tby, trx, tby,
		r, r, right, top+r, right, bottom-r,
		r, r, right-r, bottom, left+r, bottom,
		r, r, left, bottom-r, left, top+r,
		r, r, tlx, tby)

	// Tongue: a bump sitting on the bottom edge, bulging up into the mouth.
	// A quadratic Bézier is used instead of an elliptical arc because the arc's
	// rx equals exactly half its chord (the degenerate half-ellipse case), and
	// rasterizers disagree on which way such an arc sweeps — oksvg (the device
	// renderer) bulged it *downward*, so the tongue stuck out below the mouth.
	// The Bézier control point above the edge makes the bulge direction
	// unambiguous and identical across rasterizers; it peaks ty above the edge.
	tr := 19.0 * h / 36.0
	ty := 0.18 * h
	tongue := fmt.Sprintf("M %.2f %.2f Q %.2f %.2f %.2f %.2f Z",
		140-tr, bottom, 140.0, bottom-2*ty, 140+tr, bottom)

	return SpeakParams{
		MouthH:       h,
		MouthRx:      r,
		TeethPath:    teeth,
		InteriorPath: interior,
		TonguePath:   tongue,
	}
}

// renderSpeakSVG executes the template at openness t ∈ [0,1].
func renderSpeakSVG(tmplData []byte, t float64) ([]byte, error) {
	tmpl, err := template.New("speaking").Parse(string(tmplData))
	if err != nil {
		return nil, fmt.Errorf("parse speaking template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, speakParams(t)); err != nil {
		return nil, fmt.Errorf("execute speaking template: %w", err)
	}
	return buf.Bytes(), nil
}

// speakBand returns the pixel bounding box of the mouth animation strip
// (viewBox x 92..188, y 96..150 — mouth at 106..174 with anti-aliasing margin).
func speakBand(w, h int) (x0, y0, x1, y1 int) {
	x0 = int(92.0 / 280.0 * float64(w))
	y0 = int(96.0 / 210.0 * float64(h))
	x1 = int(188.0/280.0*float64(w)) + 1
	y1 = int(150.0/210.0*float64(h)) + 1
	if x1 > w {
		x1 = w
	}
	if y1 > h {
		y1 = h
	}
	return
}
