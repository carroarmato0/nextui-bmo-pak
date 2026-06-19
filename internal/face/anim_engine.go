package face

import (
	"strings"
	"sync"
)

const (
	// minEngineCap is the smallest the adaptive residency cap will shrink to:
	// the currently-spoken emotion plus one recent one stay warm even on a tiny
	// budget or before the per-frame cost is known. Kept low because at the
	// device's real 1280×720 output each 6-frame animation is ~22 MiB, and the
	// idle face-cache + GL textures already sit at ~550 MiB on the ~1 GiB device
	// — an over-generous floor pushed the speaking-time peak into the OOM-killer.
	minEngineCap = 2
	// animMemoryBudget bounds the TOTAL bytes of resident animation frames
	// (pinned + unpinned). At 1280×720 a 6-frame animation is ~22 MiB, so holding
	// every face resident (~500 MiB for the built-in set) would be unsafe on the
	// ~1 GiB device. The engine derives its cap from this budget and the real
	// frame size measured at the first build, so it self-adapts to the render
	// resolution and to how many faces a mod actually declares — small/low-res
	// mods keep more resident, large/high-res ones fewer — never exceeding the
	// budget. 128 MiB ⇒ 4 pinned + ~2 unpinned ≈ 132 MiB at 1280×720, leaving
	// headroom for the static cache, textures and TTS buffers. Note: amplitude
	// faces are NOT built at idle (the renderer serves them from the static
	// cache), so the unpinned slots only fill while BMO is actively speaking.
	animMemoryBudget = 128 << 20 // 128 MiB
)

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

	mu        sync.Mutex
	cache     map[string]*animState
	lru       []string        // unpinned expression keys, oldest first
	pinned    map[string]bool // keys exempt from eviction
	cap       int             // max unpinned residents; adaptive unless manualCap
	budget    int             // total byte budget for resident frames (0 = unbounded)
	perEntry  int             // bytes per resident animation, learned at first build
	manualCap bool            // test override: skip adaptive recomputation
}

// NewEngine returns an Engine over lib with the given effective animation set
// (keyed by lowercase expression name). The residency cap adapts to the number
// of declared animations and the live frame size, bounded by animMemoryBudget;
// see adaptiveCapLocked.
func NewEngine(lib *Library, defs map[string]AnimationDef) *Engine {
	e := &Engine{lib: lib, defs: defs, cache: map[string]*animState{}, pinned: map[string]bool{}, budget: animMemoryBudget}
	e.cap = e.adaptiveCapLocked() // provisional until the first build measures perEntry
	return e
}

// adaptiveCapLocked computes how many unpinned animations may stay resident: as
// many as the engine declares, bounded by the memory budget (after reserving
// space for the pinned set) once the per-animation byte cost is known, and never
// below minEngineCap. Until the first build measures perEntry it stays
// conservative. Caller holds e.mu.
func (e *Engine) adaptiveCapLocked() int {
	if e.perEntry <= 0 {
		return minEngineCap
	}
	want := len(e.defs)
	if e.budget > 0 {
		byBudget := e.budget/e.perEntry - len(e.pinned)
		if byBudget < minEngineCap {
			byBudget = minEngineCap
		}
		if want > byBudget {
			want = byBudget
		}
	}
	if want < minEngineCap {
		want = minEngineCap
	}
	return want
}

// setCapForTest fixes the unpinned residency cap and disables adaptive
// recomputation, so eviction tests stay deterministic regardless of frame size.
func (e *Engine) setCapForTest(n int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.manualCap = true
	e.cap = n
	e.evictLocked()
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
	// Pinned frames count against the budget, so the unpinned cap shrinks as more
	// faces are pinned.
	if !e.manualCap {
		e.cap = e.adaptiveCapLocked()
		e.evictLocked()
	}
}

// Has reports whether expr has a declared animation.
func (e *Engine) Has(expr string) bool {
	_, ok := e.defs[normExpr(expr)]
	return ok
}

// IsTimeDriven reports whether expr is a self-animating (time-driven) face such
// as whistle, look_around or sleeping. Amplitude-driven faces return false:
// they rest at frame 0 in silence and only need the engine while speaking, so
// the renderer can serve them from the cheap static cache when idle and avoid
// building (and churning) their full frame set just to show a still pose.
func (e *Engine) IsTimeDriven(expr string) bool {
	def, ok := e.defs[normExpr(expr)]
	return ok && def.Driver.Kind == DriverTime
}

// IsAmplitude reports whether expr is a voice-amplitude-driven (lip-sync) face.
// The render loop uses this to apply the inter-syllable mouth floor to every
// such face uniformly — so no emotion's rest mouth flashes during silence gaps
// while speaking — instead of a hardcoded per-name list.
func (e *Engine) IsAmplitude(expr string) bool {
	def, ok := e.defs[normExpr(expr)]
	return ok && def.Driver.Kind == DriverAmplitude
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

// FrameStep reports the frame index AnimFrame would return for expr under the
// given driver inputs, without copying any pixels. ok is false when expr is
// static or its animation is not yet built at this size — the same readiness
// condition as AnimFrame, so a caller can treat "not ready" as "no stable step,
// rebuild". Unlike AnimFrame it does NOT start a build (that stays AnimFrame's
// job); it is a pure read used by the renderer to fold a time-driven
// animation's current step into its frame signature, so a frame being held
// between steps is skipped instead of re-rendered every tick.
func (e *Engine) FrameStep(expr string, w, h int, clock, epoch float64, signal float32) (int, bool) {
	key := normExpr(expr)
	def, ok := e.defs[key]
	if !ok {
		return 0, false
	}
	e.mu.Lock()
	st := e.cache[key]
	if st == nil || !st.ready || st.w != w || st.h != h {
		e.mu.Unlock()
		return 0, false
	}
	n := len(st.frames)
	e.mu.Unlock()

	step := def.Driver.Step(clock, epoch, signal, n)
	if step < 0 || step >= n {
		return 0, false
	}
	return step, true
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
			// Learn the real per-animation cost from the first successful build
			// and size the cap to the memory budget at the live resolution.
			if e.perEntry == 0 && len(frames) > 0 {
				e.perEntry = st.w * st.h * 4 * len(frames)
				if !e.manualCap {
					e.cap = e.adaptiveCapLocked()
					e.evictLocked()
				}
			}
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
