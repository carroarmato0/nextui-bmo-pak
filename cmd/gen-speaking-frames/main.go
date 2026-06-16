// Command gen-speaking-frames writes the BMO speaking animation assets:
// speaking_0.svg (closed) .. speaking_5.svg (open), plus speaking.svg (= step 0,
// the static fallback). Run from the repo root:
//
//	go run ./cmd/gen-speaking-frames
//
// It writes to internal/face/assets relative to the working directory, so it
// must be run from the repo root (no //go:generate directive, which would run
// from this file's own directory and write the assets to the wrong place).
package main

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"text/template"
)

const speakLevels = 6

const speakTemplate = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210">
  <rect x="0" y="0" width="280" height="210" fill="#4ECBA8"/>
  <rect x="6" y="5" width="268" height="200" rx="26" ry="26" fill="#4ECBA8"/>
  <rect x="12" y="10" width="256" height="188" rx="12" ry="12" fill="#90e5c8"/>
  <rect x="76"  y="68" width="7" height="20" rx="3" ry="3" fill="#1a1a1a"/>
  <rect x="195" y="68" width="7" height="20" rx="3" ry="3" fill="#1a1a1a"/>
  <rect x="106" y="106" width="68" height="{{.MouthH}}" rx="{{.MouthRx}}" ry="{{.MouthRx}}" fill="#1a1a1a"/>
  <path d="{{.TeethPath}}" fill="#e4e4e4"/>
  <path d="{{.InteriorPath}}" fill="#1a7848"/>
  <path d="{{.TonguePath}}" fill="#16ae81"/>
</svg>
`

type speakParams struct {
	MouthH       float64
	MouthRx      float64
	TeethPath    string
	InteriorPath string
	TonguePath   string
}

func computeParams(t float64) speakParams {
	t = math.Max(0, math.Min(1, t))
	const left, right, top = 106.0, 174.0, 106.0
	h := 6 + 30*t
	r := math.Min(16, h/2)
	bottom := top + h

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

	tr := 19.0 * h / 36.0
	ty := 0.18 * h
	tongue := fmt.Sprintf("M %.2f %.2f Q %.2f %.2f %.2f %.2f Z",
		140-tr, bottom, 140.0, bottom-2*ty, 140+tr, bottom)

	return speakParams{MouthH: h, MouthRx: r, TeethPath: teeth, InteriorPath: interior, TonguePath: tongue}
}

func main() {
	outDir := filepath.Join("internal", "face", "assets")
	tmpl := template.Must(template.New("speak").Parse(speakTemplate))
	for lvl := 0; lvl < speakLevels; lvl++ {
		t := float64(lvl) / float64(speakLevels-1)
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, computeParams(t)); err != nil {
			panic(err)
		}
		name := fmt.Sprintf("speaking_%d.svg", lvl)
		if err := os.WriteFile(filepath.Join(outDir, name), buf.Bytes(), 0o644); err != nil {
			panic(err)
		}
		if lvl == 0 {
			// Static fallback = closed mouth.
			if err := os.WriteFile(filepath.Join(outDir, "speaking.svg"), buf.Bytes(), 0o644); err != nil {
				panic(err)
			}
		}
		fmt.Println("wrote", name)
	}
}
