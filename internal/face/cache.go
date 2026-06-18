package face

import (
	"sync"
)

// staticCacheModBudget bounds how many *non-canonical* (mod-supplied) face
// frames stay resident. The built-in canonical set is pinned and excluded from
// this budget, so the idle rotation never thrashes; only a mod's own custom
// emotion faces — an unbounded axis — are LRU-evicted past this limit. At the
// device's ~1024x768 / 1280x720 output each frame is ~3 MiB, so a 12-frame
// mod budget caps custom-face residency at ~36 MiB on top of the pinned
// canonical set. Beyond the budget, the least-recently-used custom face is
// dropped and re-rasterized on demand (one static Rasterize, cheap vs an OOM).
const staticCacheModBudget = 12

// canonicalKeySet is the set of cache keys that are pinned (never evicted).
// CanonicalNames are already canonical lowercase strings, which is exactly the
// key form Library.Resolve yields for a canonical expression (and for a mod
// override of one), so membership identifies a pinned frame.
var canonicalKeySet = func() map[string]bool {
	m := make(map[string]bool, len(CanonicalNames))
	for _, n := range CanonicalNames {
		m[n] = true
	}
	return m
}()

// Cache rasterizes and caches face frames at the current output resolution.
type Cache struct {
	lib      *Library
	mu       sync.Mutex
	w, h     int
	frames   map[string][]uint32
	failed   map[string]bool
	resolved map[string]string
	lru      []string // unpinned (mod) keys, least-recently-used first
}

// NewCache returns a Cache backed by lib.
func NewCache(lib *Library) *Cache {
	return &Cache{lib: lib}
}

// noteUseLocked records a use of key for LRU ordering. Pinned (canonical) keys
// are not tracked — they are never evicted. Caller holds c.mu.
func (c *Cache) noteUseLocked(key string) {
	if canonicalKeySet[key] {
		return
	}
	for i, k := range c.lru {
		if k == key {
			c.lru = append(c.lru[:i], c.lru[i+1:]...)
			break
		}
	}
	c.lru = append(c.lru, key)
}

// evictLocked drops least-recently-used unpinned frames until the unpinned
// resident count fits staticCacheModBudget. Caller holds c.mu.
func (c *Cache) evictLocked() {
	for len(c.lru) > staticCacheModBudget {
		oldest := c.lru[0]
		c.lru = c.lru[1:]
		delete(c.frames, oldest)
	}
}

// Warm pre-rasterizes every canonical expression at w×h without holding the
// mutex during the expensive Rasterize calls, so the render loop is never
// blocked for more than the brief store-write at the end of each expression.
// Call in a goroutine.
func (c *Cache) Warm(w, h int) {
	// Initialise the size maps once so warmFrame can store results immediately.
	c.mu.Lock()
	c.resizeLocked(w, h)
	c.mu.Unlock()

	for _, name := range CanonicalNames {
		c.warmFrame(name, w, h)
	}

	// Pre-rasterize the active mod's custom emotion faces so a custom name does
	// not stutter on first use. warmFrame is idempotent for names already warmed.
	for _, name := range EmotionFaceNamesInDir(c.lib.dir) {
		c.warmFrame(name, w, h)
	}
}

// warmFrame rasterizes one expression outside the mutex, then stores the
// result with a brief lock.  If the cache was resized while rasterizing, the
// result is silently discarded (the render loop will re-render on demand).
func (c *Cache) warmFrame(name string, w, h int) {
	data, fromDisk := c.lib.Bytes(name)
	if data == nil {
		return
	}
	buf, err := Rasterize(renderRestSVG(data), w, h) // expensive – NO lock held
	if err != nil {
		if !fromDisk {
			return
		}
		def, ok := defaultBytes(name)
		if !ok {
			return
		}
		buf, err = Rasterize(renderRestSVG(def), w, h)
		if err != nil {
			return
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.w != w || c.h != h || c.frames == nil {
		return // size changed while rasterizing; render loop will redo
	}
	if !c.failed[name] {
		if _, ok := c.frames[name]; !ok {
			c.frames[name] = buf
			c.noteUseLocked(name)
			c.evictLocked()
		}
	}
}

// Frame returns the cached ARGB buffer for expr at w×h, rasterizing on first
// access. Returns nil only if both the override and embedded default fail.
func (c *Cache) Frame(expr string, w, h int) []uint32 {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.resizeLocked(w, h)
	if c.resolved == nil {
		c.resolved = make(map[string]string)
	}
	key, ok := c.resolved[expr]
	if !ok {
		// Resolve performs one os.Stat; cached per distinct expr so the render
		// loop never re-stats on a hit. resolved is size-independent, so it is
		// intentionally not cleared on resize.
		key = c.lib.Resolve(expr)
		c.resolved[expr] = key
	}
	if buf, ok := c.frames[key]; ok {
		c.noteUseLocked(key)
		return buf
	}
	if c.failed[key] {
		return nil
	}
	buf := c.renderLocked(key, w, h)
	if buf != nil {
		c.frames[key] = buf
		c.noteUseLocked(key)
		c.evictLocked()
	} else {
		c.failed[key] = true
	}
	return buf
}

// Source reports the origin of expr's rendered bytes — "mod-override",
// "embedded-default", or "none" — by delegating to the backing Library. It
// does no logging and is safe to call from the render loop on the same
// goroutine that calls Frame.
func (c *Cache) Source(expr string) string {
	return c.lib.Source(expr)
}

// resizeLocked clears the cache if the target size has changed.
func (c *Cache) resizeLocked(w, h int) {
	if c.w == w && c.h == h && c.frames != nil {
		return
	}
	c.w, c.h = w, h
	c.frames = make(map[string][]uint32)
	c.failed = make(map[string]bool)
	c.lru = nil
}

// renderLocked rasterizes canonical at w×h, falling back from override to
// embedded default on failure. Must be called with mu held.
func (c *Cache) renderLocked(canonical string, w, h int) []uint32 {
	data, fromDisk := c.lib.Bytes(canonical)
	if data == nil {
		return nil
	}
	buf, err := Rasterize(renderRestSVG(data), w, h)
	if err == nil {
		return buf
	}
	if fromDisk {
		c.lib.logf("face: override %s.svg failed to rasterize (%v); using default", canonical, err)
		if def, ok := defaultBytes(canonical); ok {
			buf, err = Rasterize(renderRestSVG(def), w, h)
			if err == nil {
				return buf
			}
		}
	}
	return nil
}
