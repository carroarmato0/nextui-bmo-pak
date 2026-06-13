package devctx

import (
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/carroarmato0/nextui-bmo/internal/config"
)

// Builder runs the enabled collectors and assembles the DEVICE AWARENESS
// block appended to BMO's system prompt. Results are cached for a short TTL
// so back-to-back utterances do not rescan the SD card. Safe for concurrent
// use (the voice pipeline goroutine and the proactive scheduler both read
// it).
type Builder struct {
	mu         sync.Mutex
	collectors []Collector
	enabled    map[string]bool
	detail     string // propagated to detailAware collectors on each Collect call
	ttl        time.Duration
	now        func() time.Time
	rng        *rand.Rand
	reminisce  func(now time.Time) (memory, subject string, ok bool)
	memory     *Memory
	quotes     func() []string

	cachedAt       time.Time
	cachedSections []Section
}

// detailAware is implemented by collectors whose verbosity depends on the
// LibraryDetail config setting (LibraryCollector, PlayLogCollector).
type detailAware interface {
	withDetail(string) Collector
}

func NewBuilder(collectors []Collector, ttl time.Duration, seed int64) *Builder {
	return &Builder{
		collectors: collectors,
		enabled:    map[string]bool{},
		ttl:        ttl,
		now:        time.Now,
		rng:        rand.New(rand.NewSource(seed)),
	}
}

// SetLibraryDetail sets the detail mode ("full" or "random") forwarded to
// detailAware collectors and invalidates the cache.
func (b *Builder) SetLibraryDetail(d string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.detail = d
	b.cachedAt = time.Time{}
}

// SetEnabled maps the config toggles onto collector keys and invalidates
// the cache so settings changes take effect immediately.
func (b *Builder) SetEnabled(dc config.DeviceContext) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.enabled = map[string]bool{
		KeyLibrary:      dc.Library,
		KeySaves:        dc.Saves,
		KeyPlayLog:      dc.PlayLog,
		KeySystem:       dc.System,
		KeyAchievements: dc.Achievements,
	}
	b.cachedAt = time.Time{}
}

// SetReminisce installs the reminisce source used by ProactiveNudge
// (wired to AchievementsCollector.RandomPastUnlock). fn is invoked while
// the Builder's lock is held; it must not call any Builder method.
func (b *Builder) SetReminisce(fn func(time.Time) (string, string, bool)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.reminisce = fn
}

// SetMemory installs the memory consulted for cooldown dedup.
// A nil memory disables dedup (every candidate is always eligible).
func (b *Builder) SetMemory(m *Memory) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.memory = m
}

// SetClock overrides the time source (tests).
func (b *Builder) SetClock(now func() time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.now = now
}

// sectionsLocked refreshes the cache when stale and returns it along with
// the current time. Caller must hold b.mu.
func (b *Builder) sectionsLocked() ([]Section, time.Time) {
	now := b.now()
	if !b.cachedAt.IsZero() && now.Sub(b.cachedAt) < b.ttl {
		return b.cachedSections, now
	}
	var sections []Section
	for _, c := range b.collectors {
		if c == nil || !b.enabled[c.Key()] {
			continue
		}
		if da, ok := c.(detailAware); ok {
			c = da.withDetail(b.detail)
		}
		s, err := c.Collect(now)
		if err != nil || strings.TrimSpace(s.Body) == "" {
			continue // best-effort: a failing collector just disappears
		}
		sections = append(sections, s)
	}
	b.cachedSections = sections
	b.cachedAt = now
	return sections, now
}

// Snapshot returns the formatted DEVICE AWARENESS block. Worst case (all
// collectors failing or disabled) it is just the clock anchor, which keeps
// BMO time-aware for free.
func (b *Builder) Snapshot() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	sections, now := b.sectionsLocked()
	var sb strings.Builder
	sb.WriteString("DEVICE AWARENESS (real, current facts about the handheld you live in; weave them in naturally, never recite them as a list):\n")
	sb.WriteString("It is " + now.Format("Monday, 2006-01-02 15:04") + ".\n")
	for _, s := range sections {
		sb.WriteString("\n" + s.Title + ": " + s.Body + "\n")
	}
	return strings.TrimSpace(sb.String())
}
