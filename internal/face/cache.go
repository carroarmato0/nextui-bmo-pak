package face

import (
	"sync"
)

// Strip is a rectangular sub-region of a frame, extracted for partial blitting.
type Strip struct {
	X, Y, W, H int
	Pix        []uint32
}

// speakSet holds rendered speaking faces for one resolution: either a static
// frame (plain SVG mod), or a base frame plus one mouth-band strip per level
// above zero.
type speakSet struct {
	static []uint32
	base   []uint32
	strips []*Strip
}

// Cache rasterizes and caches face frames at the current output resolution.
type Cache struct {
	lib         *Library
	mu          sync.Mutex
	w, h        int
	frames      map[string][]uint32
	failed      map[string]bool
	speak       *speakSet
	speakFailed bool
}

// NewCache returns a Cache backed by lib.
func NewCache(lib *Library) *Cache {
	return &Cache{lib: lib}
}

// Warm pre-rasterizes every canonical expression at w×h. Call in a goroutine.
func (c *Cache) Warm(w, h int) {
	for _, name := range CanonicalNames {
		if name == ExprSpeaking {
			continue
		}
		c.Frame(name, w, h)
	}
	c.Speak(1, w, h)
}

// Frame returns the cached ARGB buffer for expr at w×h, rasterizing on first
// access. Returns nil only if both the override and embedded default fail.
func (c *Cache) Frame(expr string, w, h int) []uint32 {
	canonical := Canonical(expr)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.resizeLocked(w, h)
	if buf, ok := c.frames[canonical]; ok {
		return buf
	}
	if c.failed[canonical] {
		return nil
	}
	buf := c.renderLocked(canonical, w, h)
	if buf != nil {
		c.frames[canonical] = buf
	} else {
		c.failed[canonical] = true
	}
	return buf
}

// Speak returns the base frame and optional mouth-strip for openness t ∈ [0,1].
// Level 0 returns (base, nil); levels 1-11 return (base, strip).
// Returns (nil, nil) if speaking could not be rendered.
func (c *Cache) Speak(t float64, w, h int) ([]uint32, *Strip) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.resizeLocked(w, h)
	if c.speak == nil && !c.speakFailed {
		c.speak = c.buildSpeakLocked(w, h)
		c.speakFailed = c.speak == nil
	}
	s := c.speak
	if s == nil {
		return nil, nil
	}
	if s.static != nil {
		return s.static, nil
	}
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	lvl := int(t*float64(speakLevels-1) + 0.5)
	if lvl == 0 {
		return s.base, nil
	}
	return s.base, s.strips[lvl-1]
}

// resizeLocked clears the cache if the target size has changed.
func (c *Cache) resizeLocked(w, h int) {
	if c.w == w && c.h == h {
		return
	}
	c.w, c.h = w, h
	c.frames = make(map[string][]uint32)
	c.failed = make(map[string]bool)
	c.speak = nil
	c.speakFailed = false
}

// renderLocked rasterizes canonical at w×h, falling back from override to
// embedded default on failure. Must be called with mu held.
func (c *Cache) renderLocked(canonical string, w, h int) []uint32 {
	data, fromDisk := c.lib.Bytes(canonical)
	if data == nil {
		return nil
	}
	buf, err := Rasterize(data, w, h)
	if err == nil {
		return buf
	}
	if fromDisk {
		c.lib.logf("face: override %s.svg failed to rasterize (%v); using default", canonical, err)
		if def, ok := defaultBytes(canonical); ok {
			buf, err = Rasterize(def, w, h)
			if err == nil {
				return buf
			}
		}
	}
	return nil
}

// buildSpeakLocked renders all 12 speaking levels and returns the speakSet.
// Returns nil if the template (or fallback default) cannot be rendered.
func (c *Cache) buildSpeakLocked(w, h int) *speakSet {
	data, fromDisk := c.lib.Bytes(ExprSpeaking)
	if data == nil {
		return nil
	}

	// Detect whether it is a template or a plain static SVG.
	if !IsSpeakTemplate(data) {
		buf, err := Rasterize(data, w, h)
		if err != nil {
			if fromDisk {
				c.lib.logf("face: override speaking.svg failed to rasterize (%v); using default template", err)
				return c.buildSpeakFromDefault(w, h)
			}
			return nil
		}
		return &speakSet{static: buf}
	}

	set := c.renderSpeakLevelsLocked(data, w, h)
	if set == nil && fromDisk {
		c.lib.logf("face: override speaking.svg template failed; using default template")
		return c.buildSpeakFromDefault(w, h)
	}
	return set
}

// buildSpeakFromDefault renders the embedded default speaking template.
func (c *Cache) buildSpeakFromDefault(w, h int) *speakSet {
	def, ok := defaultBytes(ExprSpeaking)
	if !ok {
		return nil
	}
	return c.renderSpeakLevelsLocked(def, w, h)
}

// renderSpeakLevelsLocked rasterizes all 12 openness levels from a template
// and extracts mouth-band strips for levels above zero.
func (c *Cache) renderSpeakLevelsLocked(tmplData []byte, w, h int) *speakSet {
	bx0, by0, bx1, by1 := speakBand(w, h)
	set := &speakSet{}
	for lvl := 0; lvl < speakLevels; lvl++ {
		t := float64(lvl) / float64(speakLevels-1)
		svg, err := renderSpeakSVG(tmplData, t)
		if err != nil {
			return nil
		}
		buf, err := Rasterize(svg, w, h)
		if err != nil {
			return nil
		}
		if lvl == 0 {
			set.base = buf
			continue
		}
		sw, sh := bx1-bx0, by1-by0
		strip := &Strip{X: bx0, Y: by0, W: sw, H: sh, Pix: make([]uint32, sw*sh)}
		for row := 0; row < sh; row++ {
			copy(strip.Pix[row*sw:(row+1)*sw], buf[(by0+row)*w+bx0:(by0+row)*w+bx1])
		}
		set.strips = append(set.strips, strip)
	}
	return set
}
