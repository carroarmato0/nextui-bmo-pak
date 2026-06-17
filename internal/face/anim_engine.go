package face

import (
	"strings"
	"sync"
)

// defaultEngineCap bounds how many distinct expression animations stay resident
// at once. Each entry is up to 6 full-screen frames, so this trades memory for
// avoiding rebuild gaps when the rendered expression switches (e.g. idle emotion
// → speaking → emotion). The hot set in practice is {speaking, current emotion},
// so a small cap keeps re-entry instant while bounding memory.
const defaultEngineCap = 4

// animState holds one expression's built frames at a given size.
type animState struct {
	w, h     int
	ready    bool
	building bool
	frames   [][]uint32
}

// Engine renders declarative animations. Multiple expressions stay resident in a
// small LRU cache so switching between them (and re-entering one) does not force
// a rebuild — the old single-slot engine evicted on every switch, which made the
// mouth visibly lag the audio whenever the expression changed. Builds run on a
// background goroutine so the render loop never blocks; AnimFrame returns
// (nil,false) until the requested animation is built.
type Engine struct {
	lib  *Library
	defs map[string]AnimationDef

	mu     sync.Mutex
	cache  map[string]*animState
	lru    []string        // unpinned expression keys, oldest first
	pinned map[string]bool // keys exempt from eviction
	cap    int
}

// NewEngine returns an Engine over lib with the given effective animation set
// (keyed by lowercase expression name).
func NewEngine(lib *Library, defs map[string]AnimationDef) *Engine {
	return &Engine{lib: lib, defs: defs, cache: map[string]*animState{}, pinned: map[string]bool{}, cap: defaultEngineCap}
}

// Pin marks expr's animation exempt from LRU eviction so it stays resident once
// built. Used for the canonical talking face, which clips (hello/goodbye) show
// at unpredictable times — without pinning a long session evicts it and the
// mouth lags the clip audio while it rebuilds on demand.
func (e *Engine) Pin(expr string) {
	key := normExpr(expr)
	e.mu.Lock()
	defer e.mu.Unlock()
	e.pinned[key] = true
	for i, k := range e.lru { // a pinned key never sits in the LRU list
		if k == key {
			e.lru = append(e.lru[:i], e.lru[i+1:]...)
			break
		}
	}
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
// is static or its animation is not yet built at this size.
func (e *Engine) AnimFrame(expr string, w, h int, clock, epoch float64, signal float32) ([]uint32, bool) {
	key := normExpr(expr)
	def, ok := e.defs[key]
	if !ok {
		return nil, false
	}
	e.mu.Lock()
	e.ensureLocked(key, def, w, h)
	st := e.cache[key]
	if st == nil || !st.ready || st.w != w || st.h != h {
		e.mu.Unlock()
		return nil, false
	}
	frames := st.frames
	e.mu.Unlock()

	step := def.Driver.Step(clock, epoch, signal, len(frames))
	if step < 0 || step >= len(frames) {
		return nil, false
	}
	return frames[step], true
}

// ensureLocked starts a background build for (key,w,h) unless a matching ready or
// in-flight build already exists, and marks the key most-recently-used. Caller
// holds e.mu.
func (e *Engine) ensureLocked(key string, def AnimationDef, w, h int) {
	if st := e.cache[key]; st != nil && st.w == w && st.h == h && (st.ready || st.building) {
		e.touchLocked(key)
		return
	}
	st := &animState{w: w, h: h, building: true}
	e.cache[key] = st
	e.touchLocked(key)
	e.evictLocked()

	lib := e.lib
	go func() {
		frames, err := buildFrames(lib, def, w, h)
		e.mu.Lock()
		defer e.mu.Unlock()
		if e.cache[key] != st { // evicted or superseded while building
			return
		}
		st.building = false
		if err == nil {
			st.frames = frames
			st.ready = true
		}
	}()
}

// touchLocked moves key to the most-recently-used end of the LRU list. Pinned
// keys are never tracked in the LRU, so they are never selected for eviction.
func (e *Engine) touchLocked(key string) {
	if e.pinned[key] {
		return
	}
	for i, k := range e.lru {
		if k == key {
			e.lru = append(e.lru[:i], e.lru[i+1:]...)
			break
		}
	}
	e.lru = append(e.lru, key)
}

// evictLocked drops least-recently-used entries until the cache fits e.cap.
func (e *Engine) evictLocked() {
	for len(e.lru) > e.cap {
		oldest := e.lru[0]
		e.lru = e.lru[1:]
		delete(e.cache, oldest)
	}
}

// Ready reports whether expr's animation is built and resident at the current
// size, i.e. AnimFrame will return frames. False for static or not-yet-built.
func (e *Engine) Ready(expr string) bool {
	key := normExpr(expr)
	if _, ok := e.defs[key]; !ok {
		return false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	st := e.cache[key]
	return st != nil && st.ready
}

func normExpr(expr string) string {
	return strings.ToLower(strings.TrimSpace(expr))
}
