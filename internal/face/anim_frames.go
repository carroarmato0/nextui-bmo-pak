package face

import (
	"bytes"
	"fmt"
	"text/template"
)

// animFuncs provides minimal float arithmetic for templated SVG mouths, since
// Go's text/template has no built-in add/sub/mul.
var animFuncs = template.FuncMap{
	"add": func(a, b float64) float64 { return a + b },
	"sub": func(a, b float64) float64 { return a - b },
	"mul": func(a, b float64) float64 { return a * b },
}

// talkMouthPartial is the shared open-mouth ladder every talking expression
// uses. Each emotion template renders its own resting mouth at m==0 and includes
// this partial for every open level, so the teeth/tongue mouth that tracks voice
// amplitude has a single source of truth: {{template "talkmouth" $m}}. The five
// branches match the original per-emotion ladders byte-for-byte, so the refactor
// is frame-identical.
const talkMouthPartial = `{{define "talkmouth"}}{{$m := .}}{{if lt $m 0.3}}
  <rect x="106" y="106" width="68" height="12" rx="6" ry="6" fill="#1a1a1a"/>
  <path d="M 106.61 109.36 A 6.00 6.00 0 0 1 112.00 106.00 L 168.00 106.00 A 6.00 6.00 0 0 1 173.39 109.36 Z" fill="#e4e4e4"/>
  <path d="M 106.61 109.36 L 173.39 109.36 A 6.00 6.00 0 0 1 174.00 112.00 L 174.00 112.00 A 6.00 6.00 0 0 1 168.00 118.00 L 112.00 118.00 A 6.00 6.00 0 0 1 106.00 112.00 L 106.00 112.00 A 6.00 6.00 0 0 1 106.61 109.36 Z" fill="#1a7848"/>
  {{else if lt $m 0.5}}
  <rect x="106" y="106" width="68" height="18" rx="9" ry="9" fill="#1a1a1a"/>
  <path d="M 106.92 111.04 A 9.00 9.00 0 0 1 115.00 106.00 L 165.00 106.00 A 9.00 9.00 0 0 1 173.08 111.04 Z" fill="#e4e4e4"/>
  <path d="M 106.92 111.04 L 173.08 111.04 A 9.00 9.00 0 0 1 174.00 115.00 L 174.00 115.00 A 9.00 9.00 0 0 1 165.00 124.00 L 115.00 124.00 A 9.00 9.00 0 0 1 106.00 115.00 L 106.00 115.00 A 9.00 9.00 0 0 1 106.92 111.04 Z" fill="#1a7848"/>
  {{else if lt $m 0.7}}
  <rect x="106" y="106" width="68" height="24" rx="12" ry="12" fill="#1a1a1a"/>
  <path d="M 107.22 112.72 A 12.00 12.00 0 0 1 118.00 106.00 L 162.00 106.00 A 12.00 12.00 0 0 1 172.78 112.72 Z" fill="#e4e4e4"/>
  <path d="M 107.22 112.72 L 172.78 112.72 A 12.00 12.00 0 0 1 174.00 118.00 L 174.00 118.00 A 12.00 12.00 0 0 1 162.00 130.00 L 118.00 130.00 A 12.00 12.00 0 0 1 106.00 118.00 L 106.00 118.00 A 12.00 12.00 0 0 1 107.22 112.72 Z" fill="#1a7848"/>
  <path d="M 127.33 130.00 Q 140.00 121.36 152.67 130.00 Z" fill="#16ae81"/>
  {{else if lt $m 0.9}}
  <rect x="106" y="106" width="68" height="30" rx="15" ry="15" fill="#1a1a1a"/>
  <path d="M 107.53 114.40 A 15.00 15.00 0 0 1 121.00 106.00 L 159.00 106.00 A 15.00 15.00 0 0 1 172.47 114.40 Z" fill="#e4e4e4"/>
  <path d="M 107.53 114.40 L 172.47 114.40 A 15.00 15.00 0 0 1 174.00 121.00 L 174.00 121.00 A 15.00 15.00 0 0 1 159.00 136.00 L 121.00 136.00 A 15.00 15.00 0 0 1 106.00 121.00 L 106.00 121.00 A 15.00 15.00 0 0 1 107.53 114.40 Z" fill="#1a7848"/>
  <path d="M 124.17 136.00 Q 140.00 125.20 155.83 136.00 Z" fill="#16ae81"/>
  {{else}}
  <rect x="106" y="106" width="68" height="36" rx="16" ry="16" fill="#1a1a1a"/>
  <path d="M 107.14 116.08 A 16.00 16.00 0 0 1 122.00 106.00 L 158.00 106.00 A 16.00 16.00 0 0 1 172.86 116.08 Z" fill="#e4e4e4"/>
  <path d="M 107.14 116.08 L 172.86 116.08 A 16.00 16.00 0 0 1 174.00 122.00 L 174.00 126.00 A 16.00 16.00 0 0 1 158.00 142.00 L 122.00 142.00 A 16.00 16.00 0 0 1 106.00 126.00 L 106.00 122.00 A 16.00 16.00 0 0 1 107.14 116.08 Z" fill="#1a7848"/>
  <path d="M 121.00 142.00 Q 140.00 129.04 159.00 142.00 Z" fill="#16ae81"/>
  {{end}}{{end}}`

// buildFrames rasterizes every step of def into a full w×h ARGB buffer.
func buildFrames(lib *Library, def AnimationDef, w, h int) ([][]uint32, error) {
	n := def.Steps()
	if n < 1 {
		return nil, fmt.Errorf("animation has no steps")
	}
	out := make([][]uint32, n)
	for i := 0; i < n; i++ {
		svg, err := frameSVG(lib, def, i, n)
		if err != nil {
			return nil, err
		}
		buf, err := Rasterize(svg, w, h)
		if err != nil {
			return nil, fmt.Errorf("rasterize step %d: %w", i, err)
		}
		out[i] = buf
	}
	return out, nil
}

// frameSVG returns the SVG bytes for step i of an n-step animation.
func frameSVG(lib *Library, def AnimationDef, i, n int) ([]byte, error) {
	if def.Template != nil {
		data, ok := lib.rawBytes(def.Template.File)
		if !ok {
			return nil, fmt.Errorf("template %q not found", def.Template.File)
		}
		val := def.Template.From
		if n > 1 {
			val = def.Template.From + (def.Template.To-def.Template.From)*float64(i)/float64(n-1)
		}
		return renderAnimTemplate(data, def.Template.Param, val)
	}
	name := def.Frames[i]
	data, ok := lib.rawBytes(name)
	if !ok {
		return nil, fmt.Errorf("frame %q not found", name)
	}
	return data, nil
}

// renderAnimTemplate executes a Go-template SVG with a single named parameter.
// The shared "talkmouth" partial is registered first so any emotion template may
// open the common voice-driven mouth via {{template "talkmouth" $m}}.
func renderAnimTemplate(data []byte, param string, val float64) ([]byte, error) {
	tmpl, err := template.New("anim").Funcs(animFuncs).Parse(talkMouthPartial)
	if err != nil {
		return nil, fmt.Errorf("parse talkmouth partial: %w", err)
	}
	if _, err := tmpl.Parse(string(data)); err != nil {
		return nil, fmt.Errorf("parse animation template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, map[string]any{param: val}); err != nil {
		return nil, fmt.Errorf("execute animation template: %w", err)
	}
	return buf.Bytes(), nil
}

// renderRestSVG executes a templated SVG with empty data (its resting state).
// If the bytes contain no template syntax, they are returned unchanged. On any
// parse/execute error the input is returned unchanged so callers degrade safely.
func renderRestSVG(data []byte) []byte {
	if !bytes.Contains(data, []byte("{{")) {
		return data
	}
	tmpl, err := template.New("rest").Funcs(animFuncs).Parse(talkMouthPartial)
	if err != nil {
		return data
	}
	if _, err := tmpl.Parse(string(data)); err != nil {
		return data
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, map[string]any{}); err != nil {
		return data
	}
	return buf.Bytes()
}
