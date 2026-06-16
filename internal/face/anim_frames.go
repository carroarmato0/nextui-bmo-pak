package face

import (
	"bytes"
	"fmt"
	"text/template"
)

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
func renderAnimTemplate(data []byte, param string, val float64) ([]byte, error) {
	tmpl, err := template.New("anim").Parse(string(data))
	if err != nil {
		return nil, fmt.Errorf("parse animation template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, map[string]any{param: val}); err != nil {
		return nil, fmt.Errorf("execute animation template: %w", err)
	}
	return buf.Bytes(), nil
}
