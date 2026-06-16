package face

import (
	"strings"
	"sync"
)

// Engine renders declarative animations. Only the active expression's frames
// are resident; builds run on a background goroutine so the render loop never
// blocks. AnimFrame returns (nil,false) until the active animation is ready.
type Engine struct {
	lib  *Library
	defs map[string]AnimationDef

	mu       sync.Mutex
	expr     string // resident/in-flight expression key
	w, h     int
	ready    bool
	building bool
	frames   [][]uint32
}

// NewEngine returns an Engine over lib with the given effective animation set
// (keyed by lowercase expression name).
func NewEngine(lib *Library, defs map[string]AnimationDef) *Engine {
	return &Engine{lib: lib, defs: defs}
}

// Has reports whether expr has a declared animation.
func (e *Engine) Has(expr string) bool {
	_, ok := e.defs[normExpr(expr)]
	return ok
}

// Prewarm asynchronously builds expr's frames at w×h so the first display is
// smooth. Safe to call from any goroutine; a no-op for static expressions.
func (e *Engine) Prewarm(expr string, w, h int) {
	key := normExpr(expr)
	def, ok := e.defs[key]
	if !ok {
		return
	}
	e.mu.Lock()
	e.ensureLocked(key, def, w, h)
	e.mu.Unlock()
}

// AnimFrame returns the current frame for expr at w×h, or (nil,false) when expr
// is static or its animation is not yet built.
func (e *Engine) AnimFrame(expr string, w, h int, clock, epoch float64, signal float32) ([]uint32, bool) {
	key := normExpr(expr)
	def, ok := e.defs[key]
	if !ok {
		return nil, false
	}
	e.mu.Lock()
	e.ensureLocked(key, def, w, h)
	if !e.ready || e.expr != key || e.w != w || e.h != h {
		e.mu.Unlock()
		return nil, false
	}
	frames := e.frames
	e.mu.Unlock()

	step := def.Driver.Step(clock, epoch, signal, len(frames))
	if step < 0 || step >= len(frames) {
		return nil, false
	}
	return frames[step], true
}

// ensureLocked starts a background build if the resident state does not match
// (key,w,h) and no matching build is already in flight. Caller holds e.mu.
func (e *Engine) ensureLocked(key string, def AnimationDef, w, h int) {
	if e.expr == key && e.w == w && e.h == h && (e.ready || e.building) {
		return
	}
	e.expr, e.w, e.h = key, w, h
	e.ready, e.building, e.frames = false, true, nil
	lib := e.lib
	go func() {
		frames, err := buildFrames(lib, def, w, h)
		e.mu.Lock()
		defer e.mu.Unlock()
		if e.expr != key || e.w != w || e.h != h {
			return // superseded by a newer request
		}
		e.building = false
		if err == nil {
			e.frames = frames
			e.ready = true
		}
	}()
}

func normExpr(expr string) string {
	return strings.ToLower(strings.TrimSpace(expr))
}
