// Package devctx assembles a compact, read-only "device awareness" context
// block from the handheld: game library, save files, play history,
// RetroAchievements unlocks, and system health. Everything is best-effort —
// a failing collector simply omits its section, never breaking the voice
// pipeline. No write operations of any kind.
package devctx

import "time"

// Category keys; must match the config.DeviceContext toggles wired in
// Builder.SetEnabled.
const (
	KeyLibrary      = "library"
	KeySaves        = "saves"
	KeyPlayLog      = "playlog"
	KeySystem       = "system"
	KeyAchievements = "achievements"
)

// Section is one category's contribution to the context block.
type Section struct {
	Key      string
	Title    string    // uppercase header, e.g. "GAME LIBRARY"
	Body     string    // formatted plain-text, no markdown
	Subject  string    // the specific news item, e.g. the newest unlock; "" = the category itself
	Freshest time.Time // most recent event in the category; zero = evergreen
}

// Collector produces one Section. Implementations must be read-only and
// fast (they run at most once per Builder TTL window).
type Collector interface {
	Key() string
	Collect(now time.Time) (Section, error)
}
