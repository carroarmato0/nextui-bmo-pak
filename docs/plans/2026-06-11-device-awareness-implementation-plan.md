# BMO Device Awareness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give BMO read-only awareness of the device (game library, saves, play history, RetroAchievements, system health) injected into the chat system prompt, plus freshness-weighted proactive idle remarks — all per-category toggleable in Settings.

**Architecture:** New `internal/devctx` package holds five best-effort Collectors plus a TTL-cached snapshot Builder that appends a `DEVICE AWARENESS` block (clock anchor + sections) to the persona prompt via the existing `SetSystemPromptSource` hook. A new `ProactiveScheduler` in `internal/assistant` fires jittered, gated timers; nudge content comes from `Builder.ProactiveNudge()`; speech goes through a new `VoicePipeline.SpeakRemark` that reuses the existing chat→TTS→speak path.

**Tech Stack:** Go 1.22, stdlib, `modernc.org/sqlite` (pure-Go, CGO_ENABLED=0 compatible). Spec: `docs/specs/2026-06-11-device-awareness-design.md`.

**Conventions:** Existing tests use plain `testing` with no assertion libs. Run tests per-package as you go: `go test ./internal/<pkg>/`. Commit after every task. NO Co-Authored-By trailers in commits.

**File map:**
- Create: `internal/devctx/devctx.go` (types), `reltime.go`, `library.go`, `saves.go`, `playlog.go`, `system.go`, `achievements.go`, `snapshot.go`, `nudge.go` + matching `_test.go` files
- Create: `internal/assistant/proactive.go` + `proactive_test.go`
- Modify: `internal/config/config.go` (DeviceContext, ProactiveTalk), `internal/assistant/state.go` (idle→thinking), `internal/assistant/voice.go` (SpeakRemark), `internal/ui/settings_menu.go` (6 new items), `cmd/bmo-pak/main_fb.go`, `cmd/bmo-pak/main_sdl.go`, `cmd/bmo-pak/ptt_shared.go` (wiring)

---

### Task 1: Config — DeviceContext toggles + ProactiveTalk level

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/config/config_test.go`:

```go
func TestDefaultDeviceContextAllEnabled(t *testing.T) {
	cfg := Default()
	dc := cfg.DeviceContext
	if !dc.Library || !dc.Saves || !dc.PlayLog || !dc.System || !dc.Achievements {
		t.Fatalf("expected all device context categories enabled by default, got %+v", dc)
	}
	if cfg.ProactiveTalk != ProactiveOff {
		t.Fatalf("expected proactive talk off by default, got %q", cfg.ProactiveTalk)
	}
}

func TestLoadConfigWithoutDeviceContextDefaultsEnabled(t *testing.T) {
	// Configs written before this feature have no device_context key; the
	// Load-over-Default merge must leave every category enabled.
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"mode":"idle","log_level":"info","personality":"bmo","stt":{},"chat":{},"tts":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	dc := cfg.DeviceContext
	if !dc.Library || !dc.Saves || !dc.PlayLog || !dc.System || !dc.Achievements {
		t.Fatalf("expected legacy config to default all categories on, got %+v", dc)
	}
}

func TestNormalizeProactiveTalk(t *testing.T) {
	cfg := Config{ProactiveTalk: "  CHATTY "}
	cfg.Normalize()
	if cfg.ProactiveTalk != ProactiveChatty {
		t.Fatalf("expected normalized chatty, got %q", cfg.ProactiveTalk)
	}
	cfg = Config{}
	cfg.Normalize()
	if cfg.ProactiveTalk != ProactiveOff {
		t.Fatalf("expected empty level normalized to off, got %q", cfg.ProactiveTalk)
	}
}

func TestValidateRejectsUnknownProactiveTalk(t *testing.T) {
	cfg := Default()
	cfg.ProactiveTalk = "constantly"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for unknown proactive talk level")
	}
}

func TestProactiveInterval(t *testing.T) {
	cases := map[string]time.Duration{
		ProactiveOff:        0,
		ProactiveChatty:     7 * time.Minute,
		ProactiveRegular:    30 * time.Minute,
		ProactiveOccasional: time.Hour,
		ProactiveRare:       3 * time.Hour,
		"bogus":             0,
		"":                  0,
	}
	for level, want := range cases {
		if got := ProactiveInterval(level); got != want {
			t.Errorf("ProactiveInterval(%q) = %v, want %v", level, got, want)
		}
	}
}
```

Add `"os"`, `"path/filepath"`, `"time"` to the test file imports if not present.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/config/ -run 'DeviceContext|ProactiveTalk|ProactiveInterval' -v`
Expected: FAIL — `undefined: ProactiveOff`, `cfg.DeviceContext undefined`, etc.

- [ ] **Step 3: Implement in `internal/config/config.go`**

Add `"time"` to imports. Add constants near the existing `ModeIdle`/`ModeAI` const block:

```go
	// Proactive talk levels: how often BMO may make a spontaneous idle
	// remark. Off is the default — it is the only feature that spends API
	// money unprompted.
	ProactiveOff        = "off"
	ProactiveChatty     = "chatty"
	ProactiveRegular    = "regular"
	ProactiveOccasional = "occasional"
	ProactiveRare       = "rare"
```

Add the struct above `type Config struct`:

```go
// DeviceContext gates which read-only device facts are collected into the
// chat system prompt's DEVICE AWARENESS block. All categories default to
// enabled; they are harmless reads.
type DeviceContext struct {
	Library      bool `json:"library"`
	Saves        bool `json:"saves"`
	PlayLog      bool `json:"play_log"`
	System       bool `json:"system"`
	Achievements bool `json:"achievements"`
}

func DefaultDeviceContext() DeviceContext {
	return DeviceContext{Library: true, Saves: true, PlayLog: true, System: true, Achievements: true}
}
```

Add two fields to `Config` (after `ReducedMotion`):

```go
	DeviceContext DeviceContext `json:"device_context"`
	ProactiveTalk string        `json:"proactive_talk"`
```

In `Default()` add:

```go
		DeviceContext: DefaultDeviceContext(),
		ProactiveTalk: ProactiveOff,
```

NOTE: `Load()` unmarshals over `Default()`, so legacy configs without the
`device_context` key keep all-true. Do NOT add DeviceContext handling to
`Normalize()` — plain bools cannot distinguish "absent" from "false".

In `Normalize()` add:

```go
	c.ProactiveTalk = strings.ToLower(strings.TrimSpace(c.ProactiveTalk))
	if c.ProactiveTalk == "" {
		c.ProactiveTalk = ProactiveOff
	}
```

In `Validate()` (after the mode check, following its style):

```go
	switch cfg.ProactiveTalk {
	case ProactiveOff, ProactiveChatty, ProactiveRegular, ProactiveOccasional, ProactiveRare:
	default:
		return fmt.Errorf("%w: unknown proactive_talk %q", ErrInvalid, cfg.ProactiveTalk)
	}
```

Add at the bottom of the file:

```go
// SupportedProactiveTalkLevels returns the cycle order used by the settings
// menu.
func SupportedProactiveTalkLevels() []string {
	return []string{ProactiveOff, ProactiveChatty, ProactiveRegular, ProactiveOccasional, ProactiveRare}
}

// ProactiveInterval returns the base interval between proactive remarks for
// a level, or 0 when proactive talk is off (or the level is unknown).
func ProactiveInterval(level string) time.Duration {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case ProactiveChatty:
		return 7 * time.Minute
	case ProactiveRegular:
		return 30 * time.Minute
	case ProactiveOccasional:
		return time.Hour
	case ProactiveRare:
		return 3 * time.Hour
	default:
		return 0
	}
}
```

- [ ] **Step 4: Run the package tests**

Run: `go test ./internal/config/`
Expected: PASS (all, including pre-existing tests)

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: device-context toggles and proactive-talk level in config"
```

---

### Task 2: devctx package — Section/Collector types + relative time helper

**Files:**
- Create: `internal/devctx/devctx.go`
- Create: `internal/devctx/reltime.go`
- Test: `internal/devctx/reltime_test.go`

- [ ] **Step 1: Create `internal/devctx/devctx.go`**

```go
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
	Freshest time.Time // most recent event in the category; zero = evergreen
}

// Collector produces one Section. Implementations must be read-only and
// fast (they run at most once per Builder TTL window).
type Collector interface {
	Key() string
	Collect(now time.Time) (Section, error)
}
```

- [ ] **Step 2: Write the failing test `internal/devctx/reltime_test.go`**

```go
package devctx

import (
	"testing"
	"time"
)

func TestRelTime(t *testing.T) {
	now := time.Date(2026, 6, 11, 16, 0, 0, 0, time.UTC)
	cases := []struct {
		ago  time.Duration
		want string
	}{
		{30 * time.Second, "just now"},
		{90 * time.Second, "a minute ago"},
		{20 * time.Minute, "20 minutes ago"},
		{90 * time.Minute, "an hour ago"},
		{5 * time.Hour, "5 hours ago"},
		{30 * time.Hour, "yesterday"},
		{3 * 24 * time.Hour, "3 days ago"},
		{10 * 24 * time.Hour, "a week ago"},
		{20 * 24 * time.Hour, "2 weeks ago"},
		{45 * 24 * time.Hour, "a month ago"},
		{100 * 24 * time.Hour, "3 months ago"},
		{400 * 24 * time.Hour, "over a year ago"},
		{-time.Minute, "just now"}, // clock skew: never claim the future
	}
	for _, c := range cases {
		if got := RelTime(now.Add(-c.ago), now); got != c.want {
			t.Errorf("RelTime(now-%v) = %q, want %q", c.ago, got, c.want)
		}
	}
	if got := RelTime(time.Time{}, now); got != "" {
		t.Errorf("RelTime(zero) = %q, want empty", got)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/devctx/ -v`
Expected: FAIL — `undefined: RelTime`

- [ ] **Step 4: Create `internal/devctx/reltime.go`**

```go
package devctx

import (
	"fmt"
	"time"
)

// RelTime renders t relative to now ("just now", "20 minutes ago",
// "yesterday", "2 weeks ago"). Chat models reason far better about
// pre-digested relative phrases than raw timestamps, so collectors must
// never emit epochs. Zero time renders as "".
func RelTime(t, now time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < 2*time.Minute:
		return "a minute ago"
	case d < time.Hour:
		return fmt.Sprintf("%d minutes ago", int(d.Minutes()))
	case d < 2*time.Hour:
		return "an hour ago"
	case d < 24*time.Hour:
		return fmt.Sprintf("%d hours ago", int(d.Hours()))
	case d < 48*time.Hour:
		return "yesterday"
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%d days ago", int(d.Hours()/24))
	case d < 14*24*time.Hour:
		return "a week ago"
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%d weeks ago", int(d.Hours()/(24*7)))
	case d < 60*24*time.Hour:
		return "a month ago"
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%d months ago", int(d.Hours()/(24*30)))
	default:
		return "over a year ago"
	}
}
```

(The `d < time.Minute` arm also catches negative durations from clock skew.)

- [ ] **Step 5: Run tests**

Run: `go test ./internal/devctx/`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/devctx/
git commit -m "feat: devctx package skeleton with relative-time helper"
```

---

### Task 3: Library collector

**Files:**
- Create: `internal/devctx/library.go`
- Test: `internal/devctx/library_test.go`

- [ ] **Step 1: Write the failing test `internal/devctx/library_test.go`**

```go
package devctx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeFiles(t *testing.T, dir string, names ...string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLibraryCollectorCountsPerSystem(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, filepath.Join(root, "Game Boy (GB)"), "a.gb", "b.gb", "c.gb")
	writeFiles(t, filepath.Join(root, "Sega Genesis (MD)"), "x.md")
	writeFiles(t, filepath.Join(root, "Virtual Boy (VB)")) // empty: skipped
	writeFiles(t, filepath.Join(root, ".res"), "hidden.png") // hidden dir: skipped
	writeFiles(t, filepath.Join(root, "Game Boy (GB)"), ".hidden") // hidden file: not counted
	if err := os.WriteFile(filepath.Join(root, "recentlist.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err) // stray file at root: ignored
	}

	s, err := LibraryCollector{Root: root}.Collect(time.Now())
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if s.Key != KeyLibrary || s.Title != "GAME LIBRARY" {
		t.Fatalf("unexpected section identity: %+v", s)
	}
	if !strings.Contains(s.Body, "4 games across 2 systems") {
		t.Errorf("missing totals in body: %q", s.Body)
	}
	// Sorted by count descending: GB before MD.
	gb := strings.Index(s.Body, "Game Boy (GB): 3")
	md := strings.Index(s.Body, "Sega Genesis (MD): 1")
	if gb == -1 || md == -1 || gb > md {
		t.Errorf("expected count-sorted systems in body: %q", s.Body)
	}
	if strings.Contains(s.Body, "Virtual Boy") || strings.Contains(s.Body, ".res") {
		t.Errorf("empty/hidden dirs leaked into body: %q", s.Body)
	}
	if !s.Freshest.IsZero() {
		t.Errorf("library is evergreen; Freshest must be zero, got %v", s.Freshest)
	}
}

func TestLibraryCollectorMissingRoot(t *testing.T) {
	if _, err := (LibraryCollector{Root: "/nonexistent/roms"}).Collect(time.Now()); err == nil {
		t.Fatal("expected error for missing root")
	}
}

func TestLibraryCollectorEmptyLibrary(t *testing.T) {
	if _, err := (LibraryCollector{Root: t.TempDir()}).Collect(time.Now()); err == nil {
		t.Fatal("expected error for empty library")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/devctx/ -run Library -v`
Expected: FAIL — `undefined: LibraryCollector`

- [ ] **Step 3: Create `internal/devctx/library.go`**

```go
package devctx

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// LibraryCollector summarizes the ROM library as per-system game counts.
// Game names are deliberately not listed (a full listing is ~6K tokens);
// specific titles reach BMO through saves, play history, and achievements.
type LibraryCollector struct {
	Root string // e.g. /mnt/SDCARD/Roms
}

func (LibraryCollector) Key() string { return KeyLibrary }

func (c LibraryCollector) Collect(now time.Time) (Section, error) {
	systems, err := os.ReadDir(c.Root)
	if err != nil {
		return Section{}, fmt.Errorf("read roms dir: %w", err)
	}
	type sysCount struct {
		name  string
		count int
	}
	var counts []sysCount
	total := 0
	for _, sys := range systems {
		if !sys.IsDir() || strings.HasPrefix(sys.Name(), ".") {
			continue
		}
		entries, err := os.ReadDir(filepath.Join(c.Root, sys.Name()))
		if err != nil {
			continue
		}
		n := 0
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".") {
				continue
			}
			n++
		}
		if n == 0 {
			continue
		}
		counts = append(counts, sysCount{sys.Name(), n})
		total += n
	}
	if total == 0 {
		return Section{}, fmt.Errorf("no games found under %s", c.Root)
	}
	sort.Slice(counts, func(i, j int) bool { return counts[i].count > counts[j].count })
	parts := make([]string, 0, len(counts))
	for _, sc := range counts {
		parts = append(parts, fmt.Sprintf("%s: %d", sc.name, sc.count))
	}
	body := fmt.Sprintf("%d games across %d systems. %s.", total, len(counts), strings.Join(parts, "; "))
	return Section{Key: KeyLibrary, Title: "GAME LIBRARY", Body: body}, nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/devctx/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/devctx/library.go internal/devctx/library_test.go
git commit -m "feat: devctx library collector with per-system game counts"
```

---

### Task 4: Saves collector

**Files:**
- Create: `internal/devctx/saves.go`
- Test: `internal/devctx/saves_test.go`

- [ ] **Step 1: Write the failing test `internal/devctx/saves_test.go`**

```go
package devctx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeSave(t *testing.T, path string, mtime time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

func TestSavesCollector(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 6, 11, 16, 0, 0, 0, time.UTC)
	writeSave(t, filepath.Join(root, "GB", "Deadeus.gb.sav"), now.Add(-2*time.Hour))
	writeSave(t, filepath.Join(root, "GB", "Pokemon - Red Version (USA, Europe).zip.sav"), now.Add(-72*time.Hour))
	writeSave(t, filepath.Join(root, "PS", "Crash Bandicoot (USA).cue.sav"), now.Add(-30*time.Hour))

	s, err := SavesCollector{Root: root}.Collect(now)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if s.Key != KeySaves || s.Title != "SAVE FILES" {
		t.Fatalf("unexpected section identity: %+v", s)
	}
	if !strings.Contains(s.Body, "3 save files") {
		t.Errorf("missing total: %q", s.Body)
	}
	if !strings.Contains(s.Body, "GB: 2") || !strings.Contains(s.Body, "PS: 1") {
		t.Errorf("missing per-system counts: %q", s.Body)
	}
	// Double extensions stripped so real game titles surface.
	if !strings.Contains(s.Body, "Deadeus (GB, 2 hours ago)") {
		t.Errorf("missing recent save with relative time: %q", s.Body)
	}
	if strings.Contains(s.Body, ".sav") || strings.Contains(s.Body, ".zip") {
		t.Errorf("extensions leaked into body: %q", s.Body)
	}
	// Recency order: Deadeus (2h) before Crash (30h) before Pokemon (72h).
	d := strings.Index(s.Body, "Deadeus")
	cr := strings.Index(s.Body, "Crash Bandicoot")
	p := strings.Index(s.Body, "Pokemon")
	if !(d < cr && cr < p) {
		t.Errorf("recent saves not sorted by mtime desc: %q", s.Body)
	}
	if !s.Freshest.Equal(now.Add(-2 * time.Hour)) {
		t.Errorf("Freshest = %v, want newest save mtime", s.Freshest)
	}
}

func TestSavesCollectorEmpty(t *testing.T) {
	if _, err := (SavesCollector{Root: t.TempDir()}).Collect(time.Now()); err == nil {
		t.Fatal("expected error for empty saves dir")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/devctx/ -run Saves -v`
Expected: FAIL — `undefined: SavesCollector`

- [ ] **Step 3: Create `internal/devctx/saves.go`**

```go
package devctx

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// SavesCollector summarizes save files: counts per system plus the most
// recently touched save names. Save file names carry real game titles, so
// they are BMO's main source of "games with progress".
type SavesCollector struct {
	Root string // e.g. /mnt/SDCARD/Saves
}

func (SavesCollector) Key() string { return KeySaves }

func (c SavesCollector) Collect(now time.Time) (Section, error) {
	systems, err := os.ReadDir(c.Root)
	if err != nil {
		return Section{}, fmt.Errorf("read saves dir: %w", err)
	}
	type saveFile struct {
		game   string
		system string
		mtime  time.Time
	}
	var files []saveFile
	counts := map[string]int{}
	var order []string
	for _, sys := range systems {
		if !sys.IsDir() || strings.HasPrefix(sys.Name(), ".") {
			continue
		}
		entries, err := os.ReadDir(filepath.Join(c.Root, sys.Name()))
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			if counts[sys.Name()] == 0 {
				order = append(order, sys.Name())
			}
			counts[sys.Name()]++
			files = append(files, saveFile{gameName(e.Name()), sys.Name(), info.ModTime()})
		}
	}
	if len(files) == 0 {
		return Section{}, fmt.Errorf("no save files under %s", c.Root)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mtime.After(files[j].mtime) })
	sort.Slice(order, func(i, j int) bool { return counts[order[i]] > counts[order[j]] })

	countParts := make([]string, 0, len(order))
	for _, sys := range order {
		countParts = append(countParts, fmt.Sprintf("%s: %d", sys, counts[sys]))
	}
	recent := files
	if len(recent) > 5 {
		recent = recent[:5]
	}
	recentParts := make([]string, 0, len(recent))
	for _, f := range recent {
		recentParts = append(recentParts, fmt.Sprintf("%s (%s, %s)", f.game, f.system, RelTime(f.mtime, now)))
	}
	body := fmt.Sprintf("%d save files (%s). Most recently touched: %s.",
		len(files), strings.Join(countParts, ", "), strings.Join(recentParts, "; "))
	return Section{Key: KeySaves, Title: "SAVE FILES", Body: body, Freshest: files[0].mtime}, nil
}

// gameName strips up to two trailing extensions from a save file name:
// "Pokemon Red (USA).zip.sav" → "Pokemon Red (USA)". Parenthesized region
// tags are kept — they are part of how players know their ROMs.
func gameName(file string) string {
	name := file
	for i := 0; i < 2; i++ {
		ext := filepath.Ext(name)
		if ext == "" || len(ext) == len(name) {
			break
		}
		name = strings.TrimSuffix(name, ext)
	}
	return name
}
```

NOTE: `filepath.Ext("Pokemon - Red Version (USA, Europe).zip.sav")` returns
`.sav` — but watch out: `Ext` on `"Crash Bandicoot (USA).cue"` after first
strip returns `.cue`; the loop handles both. A name like `"v1.1.2"` inside
parens is safe because `Ext` only looks after the last dot — `"SOLASTRA
v1.1.2.gb.sav"` becomes `"SOLASTRA v1.1"` which is acceptable noise; do not
over-engineer this.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/devctx/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/devctx/saves.go internal/devctx/saves_test.go
git commit -m "feat: devctx saves collector with recent save names"
```

---

### Task 5: Play log collector (sqlite)

**Files:**
- Modify: `go.mod` / `go.sum` (new dependency)
- Create: `internal/devctx/playlog.go`
- Test: `internal/devctx/playlog_test.go`

- [ ] **Step 1: Add the pure-Go sqlite driver**

```bash
go get modernc.org/sqlite@latest && go mod tidy
```

Expected: go.mod gains `modernc.org/sqlite` plus several `modernc.org/*`
indirects. If `go get` complains the module requires a newer Go than the
`go 1.22` directive, pin instead: `go get modernc.org/sqlite@v1.34.5`.
This driver is pure Go — it works with the device's `CGO_ENABLED=0` arm64
build (verified in the final task). Registers driver name `"sqlite"`.

- [ ] **Step 2: Write the failing test `internal/devctx/playlog_test.go`**

The fixture mirrors NextUI's real schema (pulled from the device):
`rom(id, type, name, file_path, image_path, created_at, updated_at)` and
`play_activity(rom_id, play_time, created_at, updated_at)`; `play_time` is
seconds, `created_at` unix epoch. The same game can appear as multiple rom
rows (e.g. two "Deadeus" entries on the real device), so queries must group
by name.

```go
package devctx

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixtureDB(t *testing.T, now time.Time) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "game_logs.sqlite")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	stmts := []string{
		`CREATE TABLE rom(id INTEGER PRIMARY KEY, type TEXT, name TEXT, file_path TEXT, image_path TEXT, created_at INTEGER, updated_at INTEGER)`,
		`CREATE TABLE play_activity(rom_id INTEGER, play_time INTEGER, created_at INTEGER, updated_at INTEGER)`,
		`INSERT INTO rom(id, name) VALUES (1, 'Deadeus'), (2, 'Deadeus'), (3, 'Splore'), (4, 'Crash Bandicoot (USA)')`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatal(err)
		}
	}
	insert := func(romID int, playSeconds int64, at time.Time) {
		if _, err := db.Exec(`INSERT INTO play_activity(rom_id, play_time, created_at) VALUES (?, ?, ?)`,
			romID, playSeconds, at.Unix()); err != nil {
			t.Fatal(err)
		}
	}
	insert(1, 4634, now.Add(-2*time.Hour))           // Deadeus rom A
	insert(2, 286, now.Add(-26*time.Hour))           // Deadeus rom B (same name)
	insert(3, 13597, now.Add(-3*24*time.Hour))       // Splore, most played
	insert(4, 164, now.Add(-27*time.Hour))           // Crash
	return path
}

func TestPlayLogCollector(t *testing.T) {
	now := time.Date(2026, 6, 11, 16, 0, 0, 0, time.UTC)
	s, err := PlayLogCollector{DBPath: fixtureDB(t, now)}.Collect(now)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if s.Key != KeyPlayLog || s.Title != "PLAY HISTORY" {
		t.Fatalf("unexpected section identity: %+v", s)
	}
	if !strings.Contains(s.Body, "Last play session was 2 hours ago") {
		t.Errorf("missing reunion gap line: %q", s.Body)
	}
	// Same-name rom rows merged: 4634+286 = 4920s = 1h22m total.
	if !strings.Contains(s.Body, "Deadeus (last played 2 hours ago, 1h22m total)") {
		t.Errorf("missing merged recent entry: %q", s.Body)
	}
	// Most played ordering: Splore (3h46m) before Deadeus (1h22m).
	sp := strings.Index(s.Body, "Splore (3h46m total)")
	de := strings.LastIndex(s.Body, "Deadeus (1h22m total)")
	if sp == -1 || de == -1 || sp > de {
		t.Errorf("most-played list wrong: %q", s.Body)
	}
	if !s.Freshest.Equal(now.Add(-2 * time.Hour)) {
		t.Errorf("Freshest = %v, want most recent session", s.Freshest)
	}
}

func TestPlayLogCollectorMissingDB(t *testing.T) {
	if _, err := (PlayLogCollector{DBPath: "/nonexistent/db.sqlite"}).Collect(time.Now()); err == nil {
		t.Fatal("expected error for missing db")
	}
}

func TestPlayLogCollectorEmptyDB(t *testing.T) {
	now := time.Now()
	path := filepath.Join(t.TempDir(), "empty.sqlite")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`CREATE TABLE rom(id INTEGER PRIMARY KEY, name TEXT)`,
		`CREATE TABLE play_activity(rom_id INTEGER, play_time INTEGER, created_at INTEGER)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()
	if _, err := (PlayLogCollector{DBPath: path}).Collect(now); err == nil {
		t.Fatal("expected error for empty play log")
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/devctx/ -run PlayLog -v`
Expected: FAIL — `undefined: PlayLogCollector`

- [ ] **Step 4: Create `internal/devctx/playlog.go`**

```go
package devctx

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure-Go driver; works with CGO_ENABLED=0
)

// PlayLogCollector reads NextUI's game tracking database (read-only) and
// summarizes recently played and most-played games. NextUI may hold the DB
// open while a game runs; any error here just omits the section.
type PlayLogCollector struct {
	DBPath string // e.g. /mnt/SDCARD/.userdata/shared/game_logs.sqlite
}

func (PlayLogCollector) Key() string { return KeyPlayLog }

type playRow struct {
	name    string
	seconds int64
	last    int64 // unix epoch of most recent session
}

func (c PlayLogCollector) Collect(now time.Time) (Section, error) {
	if _, err := os.Stat(c.DBPath); err != nil {
		return Section{}, fmt.Errorf("play log db: %w", err)
	}
	db, err := sql.Open("sqlite", "file:"+c.DBPath+"?mode=ro")
	if err != nil {
		return Section{}, fmt.Errorf("open play log db: %w", err)
	}
	defer db.Close()

	// The same game can exist as several rom rows; group by name.
	recent, err := queryPlayRows(db, `
		SELECT r.name, SUM(p.play_time), MAX(p.created_at)
		FROM play_activity p JOIN rom r ON r.id = p.rom_id
		GROUP BY r.name ORDER BY MAX(p.created_at) DESC LIMIT 5`)
	if err != nil {
		return Section{}, err
	}
	if len(recent) == 0 {
		return Section{}, fmt.Errorf("play log is empty")
	}
	top, err := queryPlayRows(db, `
		SELECT r.name, SUM(p.play_time), MAX(p.created_at)
		FROM play_activity p JOIN rom r ON r.id = p.rom_id
		GROUP BY r.name ORDER BY SUM(p.play_time) DESC LIMIT 5`)
	if err != nil {
		return Section{}, err
	}

	freshest := time.Unix(recent[0].last, 0).UTC()
	recentParts := make([]string, 0, len(recent))
	for _, r := range recent {
		recentParts = append(recentParts, fmt.Sprintf("%s (last played %s, %s total)",
			r.name, RelTime(time.Unix(r.last, 0).UTC(), now), playDuration(r.seconds)))
	}
	topParts := make([]string, 0, len(top))
	for _, r := range top {
		topParts = append(topParts, fmt.Sprintf("%s (%s total)", r.name, playDuration(r.seconds)))
	}
	body := fmt.Sprintf("Last play session was %s. Recently played: %s. Most played ever: %s.",
		RelTime(freshest, now), strings.Join(recentParts, "; "), strings.Join(topParts, "; "))
	return Section{Key: KeyPlayLog, Title: "PLAY HISTORY", Body: body, Freshest: freshest}, nil
}

func queryPlayRows(db *sql.DB, query string) ([]playRow, error) {
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("query play log: %w", err)
	}
	defer rows.Close()
	var out []playRow
	for rows.Next() {
		var r playRow
		if err := rows.Scan(&r.name, &r.seconds, &r.last); err != nil {
			return nil, fmt.Errorf("scan play log: %w", err)
		}
		if strings.TrimSpace(r.name) == "" {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// playDuration formats seconds of play compactly: "5m", "1h22m".
func playDuration(seconds int64) string {
	if seconds < 60 {
		return "under a minute"
	}
	h := seconds / 3600
	m := (seconds % 3600) / 60
	if h == 0 {
		return fmt.Sprintf("%dm", m)
	}
	return fmt.Sprintf("%dh%02dm", h, m)
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/devctx/`
Expected: PASS (13597+0s Splore = 3h46m; 4920s Deadeus = 1h22m)

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/devctx/playlog.go internal/devctx/playlog_test.go
git commit -m "feat: devctx play log collector reading game_logs.sqlite"
```

---

### Task 6: System collector

**Files:**
- Create: `internal/devctx/system.go`
- Test: `internal/devctx/system_test.go`

- [ ] **Step 1: Write the failing test `internal/devctx/system_test.go`**

```go
package devctx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSystemCollector(t *testing.T) {
	dir := t.TempDir()
	uptime := filepath.Join(dir, "uptime")
	meminfo := filepath.Join(dir, "meminfo")
	powerDir := filepath.Join(dir, "power_supply")
	if err := os.WriteFile(uptime, []byte("242883.21 423310.42\n"), 0o600); err != nil { // 2d19h
		t.Fatal(err)
	}
	if err := os.WriteFile(meminfo, []byte("MemTotal:        998332 kB\nMemFree:         511784 kB\nMemAvailable:    680408 kB\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(powerDir, "axp2202-battery"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(powerDir, "axp2202-battery", "capacity"), []byte("84\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	c := SystemCollector{
		Model:       "TrimUI Brick",
		UptimePath:  uptime,
		MeminfoPath: meminfo,
		DiskPath:    dir, // real statfs on the temp dir
		PowerDir:    powerDir,
	}
	s, err := c.Collect(time.Now())
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if s.Key != KeySystem || s.Title != "YOUR BODY (THE DEVICE)" {
		t.Fatalf("unexpected section identity: %+v", s)
	}
	for _, want := range []string{
		"TrimUI Brick",
		"awake for 2 days and 19 hours",
		"Memory is 32% used", // 1 - 680408/998332 ≈ 31.8 → rounds to 32
		"SD card:",
		"Battery is at 84%",
	} {
		if !strings.Contains(s.Body, want) {
			t.Errorf("body missing %q: %q", want, s.Body)
		}
	}
	if !s.Freshest.IsZero() {
		t.Errorf("system section is evergreen; got Freshest %v", s.Freshest)
	}
}

func TestSystemCollectorPartialSources(t *testing.T) {
	// Only the model is known: still produces a section.
	s, err := (SystemCollector{Model: "TrimUI Brick"}).Collect(time.Now())
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if !strings.Contains(s.Body, "TrimUI Brick") {
		t.Errorf("missing model: %q", s.Body)
	}
}

func TestSystemCollectorNothingAvailable(t *testing.T) {
	if _, err := (SystemCollector{}).Collect(time.Now()); err == nil {
		t.Fatal("expected error when no system facts are available")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/devctx/ -run System -v`
Expected: FAIL — `undefined: SystemCollector`

- [ ] **Step 3: Create `internal/devctx/system.go`**

```go
package devctx

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// SystemCollector reports device health: model, uptime, memory, SD card
// space, battery. Every sub-reading is independently best-effort; the
// collector only fails when nothing at all is readable. Evergreen — it
// never claims to be news.
type SystemCollector struct {
	Model       string // human device name from the device tree, may be ""
	UptimePath  string // /proc/uptime
	MeminfoPath string // /proc/meminfo
	DiskPath    string // mount point to statfs, e.g. /mnt/SDCARD
	PowerDir    string // /sys/class/power_supply
}

func (SystemCollector) Key() string { return KeySystem }

func (c SystemCollector) Collect(now time.Time) (Section, error) {
	var parts []string
	if m := strings.TrimSpace(c.Model); m != "" {
		parts = append(parts, fmt.Sprintf("You live inside a %s handheld.", m))
	}
	if up, ok := readUptime(c.UptimePath); ok {
		parts = append(parts, fmt.Sprintf("You have been awake for %s.", up))
	}
	if mem, ok := readMemUsedPercent(c.MeminfoPath); ok {
		parts = append(parts, fmt.Sprintf("Memory is %d%% used.", mem))
	}
	if used, total, ok := diskUsage(c.DiskPath); ok {
		parts = append(parts, fmt.Sprintf("SD card: %.1fG used of %.1fG.", used, total))
	}
	if bat, ok := readBattery(c.PowerDir); ok {
		parts = append(parts, fmt.Sprintf("Battery is at %d%%.", bat))
	}
	if len(parts) == 0 {
		return Section{}, fmt.Errorf("no system facts available")
	}
	return Section{Key: KeySystem, Title: "YOUR BODY (THE DEVICE)", Body: strings.Join(parts, " ")}, nil
}

func readUptime(path string) (string, bool) {
	if path == "" {
		return "", false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return "", false
	}
	secs, err := strconv.ParseFloat(fields[0], 64)
	if err != nil || secs <= 0 {
		return "", false
	}
	d := time.Duration(secs) * time.Second
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	switch {
	case days > 0:
		return fmt.Sprintf("%d days and %d hours", days, hours), true
	case hours > 0:
		return fmt.Sprintf("%d hours and %d minutes", hours, mins), true
	default:
		return fmt.Sprintf("%d minutes", mins), true
	}
}

func readMemUsedPercent(path string) (int, bool) {
	if path == "" {
		return 0, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	var totalKB, availKB float64
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		v, err := strconv.ParseFloat(fields[1], 64)
		if err != nil {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			totalKB = v
		case "MemAvailable:":
			availKB = v
		}
	}
	if totalKB <= 0 || availKB < 0 || availKB > totalKB {
		return 0, false
	}
	return int(math.Round((1 - availKB/totalKB) * 100)), true
}

func diskUsage(path string) (usedG, totalG float64, ok bool) {
	if path == "" {
		return 0, 0, false
	}
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0, false
	}
	const g = 1 << 30
	total := float64(st.Blocks) * float64(st.Bsize) / g
	free := float64(st.Bavail) * float64(st.Bsize) / g
	if total <= 0 {
		return 0, 0, false
	}
	return total - free, total, true
}

func readBattery(dir string) (int, bool) {
	if dir == "" {
		return 0, false
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "*", "capacity"))
	for _, m := range matches {
		data, err := os.ReadFile(m)
		if err != nil {
			continue
		}
		if v, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && v >= 0 && v <= 100 {
			return v, true
		}
	}
	return 0, false
}
```

(`syscall.Statfs` is Linux-only; this project only targets Linux — both the
arm64 device build and the desktop SDL build.)

- [ ] **Step 4: Run tests**

Run: `go test ./internal/devctx/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/devctx/system.go internal/devctx/system_test.go
git commit -m "feat: devctx system collector — model, uptime, memory, disk, battery"
```

---

### Task 7: RetroAchievements cache parsing primitives

**Files:**
- Create: `internal/devctx/achievements.go` (parsing half)
- Test: `internal/devctx/achievements_test.go` (parsing half)

The rcheevos offline cache (`.ra/offline/cache/`) stores server JSON
responses as: 4-byte little-endian payload length, JSON payload, trailing
checksum bytes. Verified against the real device on 2026-06-11.

- [ ] **Step 1: Write the failing tests `internal/devctx/achievements_test.go`**

```go
package devctx

import (
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// writeCacheFile encodes v as an rcheevos offline cache file: 4-byte LE
// length prefix + JSON + fake trailing checksum.
func writeCacheFile(t *testing.T, path string, v any) {
	t.Helper()
	payload, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	data := make([]byte, 4, 4+len(payload)+8)
	binary.LittleEndian.PutUint32(data, uint32(len(payload)))
	data = append(data, payload...)
	data = append(data, 0xde, 0xad, 0xbe, 0xef, 0xde, 0xad, 0xbe, 0xef)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestReadCachePayload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "login.bin")
	writeCacheFile(t, path, map[string]any{"Success": true})
	payload, err := readCachePayload(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(payload) != `{"Success":true}` {
		t.Errorf("payload = %q", payload)
	}
}

func TestReadCachePayloadCorrupt(t *testing.T) {
	dir := t.TempDir()
	short := filepath.Join(dir, "short.bin")
	if err := os.WriteFile(short, []byte{1, 0}, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readCachePayload(short); err == nil {
		t.Error("expected error for short file")
	}
	oversize := filepath.Join(dir, "oversize.bin")
	if err := os.WriteFile(oversize, []byte{0xff, 0xff, 0xff, 0x7f, '{', '}'}, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readCachePayload(oversize); err == nil {
		t.Error("expected error for oversize length prefix")
	}
	if _, err := readCachePayload(filepath.Join(dir, "missing.bin")); err == nil {
		t.Error("expected error for missing file")
	}
}

func TestDifficultyTag(t *testing.T) {
	cases := []struct {
		points  int
		rarity  float64
		achType string
		want    string
	}{
		{10, 50, "win_condition", "beat the game!"},
		{10, 0, "", ""},                                            // missing data: no judgement
		{25, 3.2, "", "very rare — almost no players have done this"},
		{10, 12.5, "", "impressive — few players have this"},
		{5, 86.5, "", "easy — most players have this"},             // the Alleyway case
		{5, 30, "", "easy — most players have this"},               // low points alone
		{10, 70, "", "easy — most players have this"},              // high rarity alone
		{10, 40, "progression", "solid"},
	}
	for _, c := range cases {
		if got := difficultyTag(c.points, c.rarity, c.achType); got != c.want {
			t.Errorf("difficultyTag(%d, %v, %q) = %q, want %q", c.points, c.rarity, c.achType, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/devctx/ -run 'CachePayload|DifficultyTag' -v`
Expected: FAIL — `undefined: readCachePayload`, `undefined: difficultyTag`

- [ ] **Step 3: Create `internal/devctx/achievements.go` (parsing half)**

```go
package devctx

import (
	"encoding/binary"
	"fmt"
	"os"
)

// The rcheevos offline cache marks client-side pseudo-achievements (like
// "Warning: Unknown Emulator") with IDs at or above this floor; they are
// not real unlocks and must never reach BMO.
const syntheticAchievementIDFloor = 101_000_000

// readCachePayload extracts the JSON payload from an rcheevos offline cache
// file: 4-byte little-endian length prefix, JSON bytes, then a trailing
// checksum we ignore.
func readCachePayload(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) < 4 {
		return nil, fmt.Errorf("cache file too short: %s", path)
	}
	n := int(binary.LittleEndian.Uint32(data[:4]))
	if n <= 0 || n > len(data)-4 {
		return nil, fmt.Errorf("cache payload length %d out of range: %s", n, path)
	}
	return data[4 : 4+n], nil
}

// JSON shapes of the two cache responses we read. Field names match the
// RetroAchievements server payloads.
type raAchievement struct {
	ID          int     `json:"ID"`
	Title       string  `json:"Title"`
	Description string  `json:"Description"`
	Points      int     `json:"Points"`
	Rarity      float64 `json:"Rarity"` // % of players holding it; 0 = unknown
	Type        string  `json:"Type"`   // "progression", "win_condition", "missable", or null
}

type raSet struct {
	Achievements []raAchievement `json:"Achievements"`
}

type raGame struct {
	GameId int     `json:"GameId"`
	Title  string  `json:"Title"`
	Sets   []raSet `json:"Sets"`
}

type raUnlockStamp struct {
	ID   int   `json:"ID"`
	When int64 `json:"When"` // unix epoch
}

type raSession struct {
	Unlocks         []raUnlockStamp `json:"Unlocks"`
	HardcoreUnlocks []raUnlockStamp `json:"HardcoreUnlocks"`
}

// difficultyTag pre-digests rarity/points into a phrase BMO can react to
// proportionally: awe for rare unlocks, playful teasing for common ones.
// Beating the game always rates celebration.
func difficultyTag(points int, rarity float64, achType string) string {
	switch {
	case achType == "win_condition":
		return "beat the game!"
	case rarity == 0:
		return "" // missing data: stay neutral
	case rarity < 5:
		return "very rare — almost no players have done this"
	case rarity < 20:
		return "impressive — few players have this"
	case rarity >= 60 || points <= 5:
		return "easy — most players have this"
	default:
		return "solid"
	}
}
```

(Rarity checks run rare-first so a low-point but genuinely rare unlock is
still celebrated.)

- [ ] **Step 4: Run tests**

Run: `go test ./internal/devctx/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/devctx/achievements.go internal/devctx/achievements_test.go
git commit -m "feat: rcheevos offline-cache parsing and difficulty tags"
```

---

### Task 8: Achievements collector + RandomPastUnlock

**Files:**
- Modify: `internal/devctx/achievements.go` (collector half)
- Test: `internal/devctx/achievements_test.go` (append)

- [ ] **Step 1: Write the failing tests (append to `internal/devctx/achievements_test.go`)**

```go
// fixtureRA builds a cache dir with one game (two real achievements, one
// synthetic) where achievement 7869 is unlocked, plus a minuisettings file.
func fixtureRA(t *testing.T, now time.Time, raEnable string) AchievementsCollector {
	t.Helper()
	dir := t.TempDir()
	cache := filepath.Join(dir, "cache")
	const hash = "91128778a332495f77699eaf3a37fe30"
	writeCacheFile(t, filepath.Join(cache, "achievementsets_"+hash+".bin"), raGame{
		GameId: 682,
		Title:  "Alleyway",
		Sets: []raSet{{Achievements: []raAchievement{
			{ID: 101000001, Title: "Warning: Unknown Emulator", Points: 0},
			{ID: 7869, Title: "Reach Stage 7", Description: "Reach stage 7", Points: 5, Rarity: 86.52, Type: "progression"},
			{ID: 27252, Title: "Lucky Number Seven", Description: "Get 7 lives", Points: 5, Rarity: 28.41},
		}}},
	})
	writeCacheFile(t, filepath.Join(cache, "startsession_"+hash+".bin"), raSession{
		Unlocks:         []raUnlockStamp{{ID: 101000001, When: now.Add(-3 * time.Hour).Unix()}, {ID: 7869, When: now.Add(-2 * time.Hour).Unix()}},
		HardcoreUnlocks: []raUnlockStamp{{ID: 7869, When: now.Add(-2 * time.Hour).Unix()}},
	})
	settings := filepath.Join(dir, "minuisettings.txt")
	content := "radius=20\nraEnable=" + raEnable + "\nraUsername=tester\nraToken=SECRET\n"
	if err := os.WriteFile(settings, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return AchievementsCollector{
		CacheDir:     cache,
		SettingsPath: settings,
		Rng:          rand.New(rand.NewSource(1)),
	}
}

func TestAchievementsCollector(t *testing.T) {
	now := time.Date(2026, 6, 11, 16, 0, 0, 0, time.UTC)
	s, err := fixtureRA(t, now, "1").Collect(now)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if s.Key != KeyAchievements || s.Title != "RETROACHIEVEMENTS" {
		t.Fatalf("unexpected section identity: %+v", s)
	}
	for _, want := range []string{
		`"Reach Stage 7" in Alleyway`,
		"Reach stage 7",
		"2 hours ago",
		"easy — most players have this",
		"Alleyway: 1 of 2 unlocked", // synthetic excluded from total
	} {
		if !strings.Contains(s.Body, want) {
			t.Errorf("body missing %q: %q", want, s.Body)
		}
	}
	if strings.Contains(s.Body, "Unknown Emulator") {
		t.Errorf("synthetic achievement leaked: %q", s.Body)
	}
	if !s.Freshest.Equal(now.Add(-2 * time.Hour)) {
		t.Errorf("Freshest = %v, want unlock time", s.Freshest)
	}
}

func TestAchievementsCollectorDisabled(t *testing.T) {
	now := time.Now()
	if _, err := fixtureRA(t, now, "0").Collect(now); err == nil {
		t.Fatal("expected error when raEnable=0")
	}
}

func TestAchievementsCollectorMissingCache(t *testing.T) {
	c := fixtureRA(t, time.Now(), "1")
	c.CacheDir = filepath.Join(t.TempDir(), "nope")
	if _, err := c.Collect(time.Now()); err == nil {
		t.Fatal("expected error for missing cache dir")
	}
}

func TestRandomPastUnlock(t *testing.T) {
	now := time.Date(2026, 6, 11, 16, 0, 0, 0, time.UTC)
	c := fixtureRA(t, now, "1")
	memory, ok := c.RandomPastUnlock(now)
	if !ok {
		t.Fatal("expected a past unlock")
	}
	for _, want := range []string{`"Reach Stage 7"`, "Alleyway", "2 hours ago"} {
		if !strings.Contains(memory, want) {
			t.Errorf("memory missing %q: %q", want, memory)
		}
	}
	c2 := fixtureRA(t, now, "0")
	if _, ok := c2.RandomPastUnlock(now); ok {
		t.Fatal("expected no reminisce when RA disabled")
	}
}
```

Add `"math/rand"`, `"strings"`, `"time"` to the test file imports.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/devctx/ -run 'Achievements|PastUnlock' -v`
Expected: FAIL — `undefined: AchievementsCollector`

- [ ] **Step 3: Append the collector to `internal/devctx/achievements.go`**

Add `"encoding/json"`, `"math/rand"`, `"path/filepath"`, `"sort"`,
`"strings"`, `"time"` to the imports, then:

```go
// AchievementsCollector reads NextUI's local rcheevos offline cache. It
// never touches the network and never reads RA credentials — the only
// minuisettings.txt key consulted is raEnable, as a respect-the-user gate.
type AchievementsCollector struct {
	CacheDir     string // .../.ra/offline/cache
	SettingsPath string // .../minuisettings.txt
	Rng          *rand.Rand // for RandomPastUnlock; may be nil
}

func (AchievementsCollector) Key() string { return KeyAchievements }

// raUnlock is one real, resolved unlock joined across the two cache files.
type raUnlock struct {
	game        string
	title       string
	description string
	points      int
	rarity      float64
	achType     string
	when        time.Time
	unlockedIn  int // unlocks the player has in this game
	totalIn     int // real achievements in this game
}

func (c AchievementsCollector) raEnabled() bool {
	data, err := os.ReadFile(c.SettingsPath)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "raEnable=1" {
			return true
		}
	}
	return false
}

// load joins achievement sets with session unlocks per game hash and
// returns all real unlocks, newest first.
func (c AchievementsCollector) load() ([]raUnlock, error) {
	setPaths, err := filepath.Glob(filepath.Join(c.CacheDir, "achievementsets_*.bin"))
	if err != nil || len(setPaths) == 0 {
		return nil, fmt.Errorf("no cached achievement sets in %s", c.CacheDir)
	}
	var out []raUnlock
	for _, setPath := range setPaths {
		hash := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(setPath), "achievementsets_"), ".bin")
		payload, err := readCachePayload(setPath)
		if err != nil {
			continue
		}
		var game raGame
		if err := json.Unmarshal(payload, &game); err != nil || strings.TrimSpace(game.Title) == "" {
			continue
		}
		byID := map[int]raAchievement{}
		for _, set := range game.Sets {
			for _, a := range set.Achievements {
				if a.ID >= syntheticAchievementIDFloor {
					continue
				}
				byID[a.ID] = a
			}
		}
		sessPayload, err := readCachePayload(filepath.Join(c.CacheDir, "startsession_"+hash+".bin"))
		if err != nil {
			continue
		}
		var sess raSession
		if err := json.Unmarshal(sessPayload, &sess); err != nil {
			continue
		}
		// Union softcore+hardcore stamps, dedupe by ID keeping latest.
		stamps := map[int]int64{}
		for _, u := range sess.Unlocks {
			if u.ID < syntheticAchievementIDFloor && u.When > stamps[u.ID] {
				stamps[u.ID] = u.When
			}
		}
		for _, u := range sess.HardcoreUnlocks {
			if u.ID < syntheticAchievementIDFloor && u.When > stamps[u.ID] {
				stamps[u.ID] = u.When
			}
		}
		var gameUnlocks []raUnlock
		for id, when := range stamps {
			a, ok := byID[id]
			if !ok {
				continue
			}
			gameUnlocks = append(gameUnlocks, raUnlock{
				game:        game.Title,
				title:       a.Title,
				description: a.Description,
				points:      a.Points,
				rarity:      a.Rarity,
				achType:     a.Type,
				when:        time.Unix(when, 0).UTC(),
			})
		}
		for i := range gameUnlocks {
			gameUnlocks[i].unlockedIn = len(gameUnlocks)
			gameUnlocks[i].totalIn = len(byID)
		}
		out = append(out, gameUnlocks...)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].when.After(out[j].when) })
	return out, nil
}

func (c AchievementsCollector) Collect(now time.Time) (Section, error) {
	if !c.raEnabled() {
		return Section{}, fmt.Errorf("retroachievements disabled in minuisettings")
	}
	unlocks, err := c.load()
	if err != nil {
		return Section{}, err
	}
	if len(unlocks) == 0 {
		return Section{}, fmt.Errorf("no achievements unlocked yet")
	}
	recent := unlocks
	if len(recent) > 5 {
		recent = recent[:5]
	}
	parts := make([]string, 0, len(recent))
	for _, u := range recent {
		p := fmt.Sprintf("%q in %s — %s (%s)", u.title, u.game, u.description, RelTime(u.when, now))
		if tag := difficultyTag(u.points, u.rarity, u.achType); tag != "" {
			p += " [" + tag + "]"
		}
		parts = append(parts, p)
	}
	// Per-game progress, ordered by most recent unlock, deduped.
	seen := map[string]bool{}
	var progress []string
	for _, u := range unlocks {
		if seen[u.game] {
			continue
		}
		seen[u.game] = true
		progress = append(progress, fmt.Sprintf("%s: %d of %d unlocked", u.game, u.unlockedIn, u.totalIn))
	}
	body := fmt.Sprintf("Recent unlocks: %s. Progress: %s.",
		strings.Join(parts, "; "), strings.Join(progress, "; "))
	return Section{Key: KeyAchievements, Title: "RETROACHIEVEMENTS", Body: body, Freshest: unlocks[0].when}, nil
}

// RandomPastUnlock returns a one-line description of a randomly chosen past
// unlock for reminisce-style proactive remarks ("remember when..."), or
// false when RA is disabled or nothing is unlocked.
func (c AchievementsCollector) RandomPastUnlock(now time.Time) (string, bool) {
	if c.Rng == nil || !c.raEnabled() {
		return "", false
	}
	unlocks, err := c.load()
	if err != nil || len(unlocks) == 0 {
		return "", false
	}
	u := unlocks[c.Rng.Intn(len(unlocks))]
	memory := fmt.Sprintf("the time the player unlocked %q in %s (%s), %s",
		u.title, u.game, u.description, RelTime(u.when, now))
	if tag := difficultyTag(u.points, u.rarity, u.achType); tag != "" {
		memory += " — " + tag
	}
	return memory, true
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/devctx/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/devctx/achievements.go internal/devctx/achievements_test.go
git commit -m "feat: devctx achievements collector with reminisce support"
```

---

### Task 9: Snapshot builder — gating, TTL cache, clock anchor

**Files:**
- Create: `internal/devctx/snapshot.go`
- Test: `internal/devctx/snapshot_test.go`

- [ ] **Step 1: Write the failing tests `internal/devctx/snapshot_test.go`**

```go
package devctx

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/carroarmato0/nextui-bmo/internal/config"
)

// fakeCollector counts calls and returns a canned section or error.
type fakeCollector struct {
	key     string
	section Section
	err     error
	calls   int
}

func (f *fakeCollector) Key() string { return f.key }
func (f *fakeCollector) Collect(now time.Time) (Section, error) {
	f.calls++
	return f.section, f.err
}

func allEnabled() config.DeviceContext { return config.DefaultDeviceContext() }

func testBuilder(collectors ...Collector) (*Builder, *time.Time) {
	b := NewBuilder(collectors, 30*time.Second, 1)
	b.SetEnabled(allEnabled())
	now := time.Date(2026, 6, 11, 16, 45, 0, 0, time.UTC)
	b.SetClock(func() time.Time { return now })
	return b, &now
}

func TestSnapshotFormatsSectionsWithClockAnchor(t *testing.T) {
	lib := &fakeCollector{key: KeyLibrary, section: Section{Key: KeyLibrary, Title: "GAME LIBRARY", Body: "4 games."}}
	b, _ := testBuilder(lib)
	got := b.Snapshot()
	if !strings.Contains(got, "DEVICE AWARENESS") {
		t.Errorf("missing header: %q", got)
	}
	if !strings.Contains(got, "It is Thursday, 2026-06-11 16:45.") {
		t.Errorf("missing clock anchor: %q", got)
	}
	if !strings.Contains(got, "GAME LIBRARY: 4 games.") {
		t.Errorf("missing section: %q", got)
	}
}

func TestSnapshotOmitsDisabledAndFailingCollectors(t *testing.T) {
	lib := &fakeCollector{key: KeyLibrary, section: Section{Key: KeyLibrary, Title: "GAME LIBRARY", Body: "4 games."}}
	saves := &fakeCollector{key: KeySaves, err: fmt.Errorf("boom")}
	b, _ := testBuilder(lib, saves)
	dc := allEnabled()
	dc.Library = false
	b.SetEnabled(dc)
	got := b.Snapshot()
	if strings.Contains(got, "GAME LIBRARY") {
		t.Errorf("disabled section leaked: %q", got)
	}
	if lib.calls != 0 {
		t.Errorf("disabled collector was invoked %d times", lib.calls)
	}
	if !strings.Contains(got, "It is ") {
		t.Errorf("worst case must still carry the clock anchor: %q", got)
	}
}

func TestSnapshotCachesWithinTTL(t *testing.T) {
	lib := &fakeCollector{key: KeyLibrary, section: Section{Key: KeyLibrary, Title: "GAME LIBRARY", Body: "4 games."}}
	b, now := testBuilder(lib)
	b.Snapshot()
	b.Snapshot()
	if lib.calls != 1 {
		t.Fatalf("expected 1 collect within TTL, got %d", lib.calls)
	}
	*now = now.Add(31 * time.Second)
	b.Snapshot()
	if lib.calls != 2 {
		t.Fatalf("expected recollect after TTL, got %d", lib.calls)
	}
}

func TestSetEnabledInvalidatesCache(t *testing.T) {
	lib := &fakeCollector{key: KeyLibrary, section: Section{Key: KeyLibrary, Title: "GAME LIBRARY", Body: "4 games."}}
	b, _ := testBuilder(lib)
	b.Snapshot()
	b.SetEnabled(allEnabled()) // settings touched: cache must drop
	b.Snapshot()
	if lib.calls != 2 {
		t.Fatalf("expected recollect after SetEnabled, got %d", lib.calls)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/devctx/ -run Snapshot -v`
Expected: FAIL — `undefined: NewBuilder`

- [ ] **Step 3: Create `internal/devctx/snapshot.go`**

```go
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
	ttl        time.Duration
	now        func() time.Time
	rng        *rand.Rand
	reminisce  func(now time.Time) (string, bool)

	cachedAt       time.Time
	cachedSections []Section
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
// (wired to AchievementsCollector.RandomPastUnlock).
func (b *Builder) SetReminisce(fn func(time.Time) (string, bool)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.reminisce = fn
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
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/devctx/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/devctx/snapshot.go internal/devctx/snapshot_test.go
git commit -m "feat: devctx snapshot builder with toggles, TTL cache, clock anchor"
```

---

### Task 10: ProactiveNudge — freshness-weighted topic picking

**Files:**
- Create: `internal/devctx/nudge.go`
- Test: `internal/devctx/nudge_test.go`

- [ ] **Step 1: Write the failing tests `internal/devctx/nudge_test.go`**

```go
package devctx

import (
	"strings"
	"testing"
	"time"
)

func nudgeBuilder(t *testing.T, reminisce func(time.Time) (string, bool), sections ...Section) *Builder {
	t.Helper()
	collectors := make([]Collector, 0, len(sections))
	for i := range sections {
		collectors = append(collectors, &fakeCollector{key: sections[i].Key, section: sections[i]})
	}
	b, _ := testBuilder(collectors...)
	if reminisce != nil {
		b.SetReminisce(reminisce)
	}
	return b
}

func TestProactiveNudgePrefersFreshNews(t *testing.T) {
	now := time.Date(2026, 6, 11, 16, 45, 0, 0, time.UTC)
	fresh := Section{Key: KeyAchievements, Title: "RETROACHIEVEMENTS", Body: "x", Freshest: now.Add(-2 * time.Hour)}
	stale := Section{Key: KeyPlayLog, Title: "PLAY HISTORY", Body: "y", Freshest: now.Add(-10 * 24 * time.Hour)}
	evergreen := Section{Key: KeyLibrary, Title: "GAME LIBRARY", Body: "z"}
	b := nudgeBuilder(t, nil, fresh, stale, evergreen)
	// Fresh news must win every single time, not just often.
	for i := 0; i < 50; i++ {
		nudge, ok := b.ProactiveNudge()
		if !ok {
			t.Fatal("expected a nudge")
		}
		if !strings.Contains(nudge, "RetroAchievements unlocks") {
			t.Fatalf("iteration %d: fresh category not picked: %q", i, nudge)
		}
		if !strings.Contains(nudge, "react excitedly") {
			t.Fatalf("missing fresh tone hint: %q", nudge)
		}
	}
}

func TestProactiveNudgeStaleAndEvergreenTones(t *testing.T) {
	now := time.Date(2026, 6, 11, 16, 45, 0, 0, time.UTC)
	stale := Section{Key: KeyPlayLog, Title: "PLAY HISTORY", Body: "y", Freshest: now.Add(-10 * 24 * time.Hour)}
	evergreen := Section{Key: KeySystem, Title: "YOUR BODY (THE DEVICE)", Body: "z"}
	b := nudgeBuilder(t, nil, stale, evergreen)
	sawStale, sawEvergreen := false, false
	for i := 0; i < 200; i++ {
		nudge, ok := b.ProactiveNudge()
		if !ok {
			t.Fatal("expected a nudge")
		}
		if strings.Contains(nudge, "play activity") {
			sawStale = true
			if !strings.Contains(nudge, "a while ago") {
				t.Fatalf("stale topic missing reminisce tone: %q", nudge)
			}
		}
		if strings.Contains(nudge, "device itself") {
			sawEvergreen = true
		}
	}
	if !sawStale || !sawEvergreen {
		t.Fatalf("expected both stale and evergreen picks over 200 runs (stale=%v evergreen=%v)", sawStale, sawEvergreen)
	}
}

func TestProactiveNudgeReminiscesWhenNothingFresh(t *testing.T) {
	now := time.Date(2026, 6, 11, 16, 45, 0, 0, time.UTC)
	stale := Section{Key: KeyAchievements, Title: "RETROACHIEVEMENTS", Body: "x", Freshest: now.Add(-10 * 24 * time.Hour)}
	called := 0
	b := nudgeBuilder(t, func(at time.Time) (string, bool) {
		called++
		return `the time the player unlocked "Reach Stage 7" in Alleyway`, true
	}, stale)
	saw := false
	for i := 0; i < 200; i++ {
		nudge, ok := b.ProactiveNudge()
		if !ok {
			t.Fatal("expected a nudge")
		}
		if strings.Contains(nudge, "suddenly remembers") {
			saw = true
			if !strings.Contains(nudge, "Reach Stage 7") {
				t.Fatalf("reminisce nudge missing memory: %q", nudge)
			}
		}
	}
	if !saw || called == 0 {
		t.Fatalf("expected reminisce path over 200 runs (saw=%v called=%d)", saw, called)
	}
}

func TestProactiveNudgeNothingToSay(t *testing.T) {
	b, _ := testBuilder() // no collectors at all
	if _, ok := b.ProactiveNudge(); ok {
		t.Fatal("expected no nudge with no sections")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/devctx/ -run Nudge -v`
Expected: FAIL — `b.ProactiveNudge undefined`

- [ ] **Step 3: Create `internal/devctx/nudge.go`**

```go
package devctx

import "time"

// freshWindow separates "news" (react excitedly) from "old news"
// (reminisce). One day matches how players experience sessions.
const freshWindow = 24 * time.Hour

// nudgeTopics phrases each category as something BMO would notice on his
// own screen.
var nudgeTopics = map[string]string{
	KeyLibrary:      "the game collection stored on this device",
	KeySaves:        "the save files he can see on the SD card",
	KeyPlayLog:      "the player's recent play activity",
	KeySystem:       "how the device itself — his own body — is doing right now",
	KeyAchievements: "the player's recent RetroAchievements unlocks",
}

// ProactiveNudge picks the topic for a spontaneous idle remark, weighted by
// freshness: categories with events from the last 24h always win; with
// nothing fresh, BMO sometimes reminisces about a random past achievement,
// otherwise falls back to stale topics (framed as old news) or evergreen
// ones. Returns false when there is nothing at all to talk about.
func (b *Builder) ProactiveNudge() (string, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	sections, now := b.sectionsLocked()
	if len(sections) == 0 {
		return "", false
	}

	var fresh, rest []Section
	for _, s := range sections {
		if !s.Freshest.IsZero() && now.Sub(s.Freshest) < freshWindow {
			fresh = append(fresh, s)
		} else {
			rest = append(rest, s)
		}
	}
	if len(fresh) > 0 {
		s := fresh[b.rng.Intn(len(fresh))]
		return nudge(nudgeTopics[s.Key], "This news is fresh — react excitedly, like it just happened."), true
	}
	// Nothing fresh: one time in three, dig up an old achievement instead.
	if b.reminisce != nil && b.rng.Intn(3) == 0 {
		if memory, ok := b.reminisce(now); ok {
			return "(BMO suddenly remembers " + memory + ". He reminisces about it out loud in one or two short sentences, reacting proportionally to how hard it was: awed if it is rare, playfully teasing if it is easy. Do not greet the player; just make the remark.)", true
		}
	}
	s := rest[b.rng.Intn(len(rest))]
	tone := "Keep it playful and curious."
	if !s.Freshest.IsZero() {
		tone = "This happened a while ago — reminisce fondly or ask when they will play again."
	}
	return nudge(nudgeTopics[s.Key], tone), true
}

func nudge(topic, tone string) string {
	return "(BMO glances at his own screen and spontaneously says one or two short sentences about " + topic + ". " + tone + " Do not greet the player; just make the remark.)"
}
```

(`rest` cannot be empty on the fallback path: it is only reached when
`fresh` is empty, and `sections` is non-empty.)

- [ ] **Step 4: Run tests**

Run: `go test ./internal/devctx/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/devctx/nudge.go internal/devctx/nudge_test.go
git commit -m "feat: freshness-weighted proactive nudge picking with reminisce"
```

---

### Task 11: State machine — allow idle → thinking

Proactive remarks skip the listening phase (there is no user speech), so
`EventThink` must be a legal transition from `StateIdle`.

**Files:**
- Modify: `internal/assistant/state.go` (the `transitionState` function)
- Test: `internal/assistant/state_test.go`

- [ ] **Step 1: Write the failing test (append to `internal/assistant/state_test.go`)**

```go
func TestThinkFromIdleForProactiveRemarks(t *testing.T) {
	if got := Transition(StateIdle, EventThink); got != StateThinking {
		t.Fatalf("Transition(idle, think) = %v, want thinking", got)
	}
	// Sanity: still legal from listening, still ignored from sleeping.
	if got := Transition(StateListening, EventThink); got != StateThinking {
		t.Fatalf("Transition(listening, think) = %v, want thinking", got)
	}
	if got := Transition(StateSleeping, EventThink); got != StateSleeping {
		t.Fatalf("Transition(sleeping, think) = %v, want sleeping", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/assistant/ -run ThinkFromIdle -v`
Expected: FAIL — got `idle`, want `thinking`

- [ ] **Step 3: Modify `transitionState` in `internal/assistant/state.go`**

Change the `EventThink` case from:

```go
	case EventThink:
		if current == StateListening {
			return current, StateThinking
		}
```

to:

```go
	case EventThink:
		// Listening → thinking is the PTT path; idle → thinking is a
		// proactive remark (no user speech to listen to).
		if current == StateListening || current == StateIdle {
			return current, StateThinking
		}
```

- [ ] **Step 4: Run the package tests**

Run: `go test ./internal/assistant/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/assistant/state.go internal/assistant/state_test.go
git commit -m "feat: allow idle-to-thinking transition for proactive remarks"
```

---

### Task 12: VoicePipeline.SpeakRemark

**Files:**
- Modify: `internal/assistant/voice.go`
- Test: `internal/assistant/voice_test.go`

- [ ] **Step 1: Write the failing tests (append to `internal/assistant/voice_test.go`)**

These reuse the existing `fakeProvider`/`fakeWriter` helpers already defined
at the top of this file.

```go
func TestSpeakRemarkHappyPath(t *testing.T) {
	m := NewMachine()
	m.SetMode("ai")
	writer := &fakeWriter{}
	chat := &fakeProvider{reply: "You reached stage 7! Daebak!"}
	tts := &fakeProvider{speech: []byte{1, 2, 3, 4}}
	pipe := NewVoicePipeline(m, writer, &fakeProvider{}, chat, tts, "whisper-1", "gpt-4o-mini", "tts-1", "alloy", "be bmo", 16000, 1)
	pipe.SetSystemPromptSource(func() string { return "persona plus device context" })

	if err := pipe.SpeakRemark(context.Background(), "(BMO says something about achievements)"); err != nil {
		t.Fatalf("speak remark: %v", err)
	}
	if chat.lastChat.SystemPrompt != "persona plus device context" {
		t.Errorf("system prompt = %q", chat.lastChat.SystemPrompt)
	}
	if len(chat.lastChat.Messages) != 1 || chat.lastChat.Messages[0].Content != "(BMO says something about achievements)" {
		t.Errorf("nudge not sent as user message: %+v", chat.lastChat.Messages)
	}
	if tts.lastSpeech.Input != "You reached stage 7! Daebak!" {
		t.Errorf("tts input = %q", tts.lastSpeech.Input)
	}
	if writer.totalBytes() == 0 {
		t.Error("expected PCM written to playback")
	}
	if got := m.State(); got != StateIdle {
		t.Errorf("state after remark = %v, want idle", got)
	}
}

func TestSpeakRemarkSkippedOutsideAIMode(t *testing.T) {
	m := NewMachine() // idle mode
	chat := &fakeProvider{reply: "should never be called"}
	pipe := NewVoicePipeline(m, &fakeWriter{}, &fakeProvider{}, chat, &fakeProvider{}, "", "gpt-4o-mini", "", "", "", 16000, 1)
	if err := pipe.SpeakRemark(context.Background(), "(nudge)"); err != nil {
		t.Fatalf("speak remark: %v", err)
	}
	if chat.lastChat.Model != "" {
		t.Error("chat provider must not be called outside AI mode")
	}
}

func TestSpeakRemarkSkippedWhenNotIdle(t *testing.T) {
	m := NewMachine()
	m.SetMode("ai")
	m.Transition(EventListen) // user is mid-conversation
	chat := &fakeProvider{reply: "should never be called"}
	pipe := NewVoicePipeline(m, &fakeWriter{}, &fakeProvider{}, chat, &fakeProvider{}, "", "gpt-4o-mini", "", "", "", 16000, 1)
	if err := pipe.SpeakRemark(context.Background(), "(nudge)"); err != nil {
		t.Fatalf("speak remark: %v", err)
	}
	if chat.lastChat.Model != "" {
		t.Error("chat provider must not be called while not idle")
	}
}

func TestSpeakRemarkEmptyReplyReturnsToIdle(t *testing.T) {
	m := NewMachine()
	m.SetMode("ai")
	pipe := NewVoicePipeline(m, &fakeWriter{}, &fakeProvider{}, &fakeProvider{reply: "  "}, &fakeProvider{}, "", "gpt-4o-mini", "", "", "", 16000, 1)
	if err := pipe.SpeakRemark(context.Background(), "(nudge)"); err != nil {
		t.Fatalf("speak remark: %v", err)
	}
	if got := m.State(); got != StateIdle {
		t.Errorf("state = %v, want idle", got)
	}
}

func TestSpeakRemarkChatFailureEntersErrorState(t *testing.T) {
	m := NewMachine()
	m.SetMode("ai")
	chat := &fakeProvider{err: fmt.Errorf("boom")}
	pipe := NewVoicePipeline(m, &fakeWriter{}, &fakeProvider{}, chat, &fakeProvider{}, "", "gpt-4o-mini", "", "", "", 16000, 1)
	if err := pipe.SpeakRemark(context.Background(), "(nudge)"); err == nil {
		t.Fatal("expected error")
	}
	if got := m.State(); got != StateError {
		t.Errorf("state = %v, want error", got)
	}
}
```

Add `"fmt"` to the test file imports if not already present.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/assistant/ -run SpeakRemark -v`
Expected: FAIL — `pipe.SpeakRemark undefined`

- [ ] **Step 3: Add `SpeakRemark` to `internal/assistant/voice.go`**

Place it right after `ProcessUtterance`. It mirrors ProcessUtterance's
chat→TTS tail exactly (same request fields, same resample, same `p.speak`),
minus the STT stage:

```go
// SpeakRemark generates and speaks a spontaneous proactive remark. The
// nudge is a stage direction sent as the user message (there is no real
// user speech); the reply flows through the normal TTS → playback path, so
// PTT interruption, amplitude-driven mouth, and state transitions all
// behave exactly like a normal utterance. No-op outside AI mode or when
// BMO is not idle — a remark must never barge into a conversation.
func (p *VoicePipeline) SpeakRemark(ctx context.Context, nudge string) error {
	if p == nil || !p.aiModeEnabled() {
		return nil
	}
	nudge = strings.TrimSpace(nudge)
	if nudge == "" {
		return nil
	}
	if p.machine != nil && p.machine.State() != StateIdle {
		return nil
	}
	if p.machine != nil {
		p.machine.Transition(EventThink)
	}

	chatStart := time.Now()
	reply, err := p.chat.Reply(ctx, providers.ChatRequest{
		Model:        p.chatModel,
		Messages:     []providers.Message{{Role: "user", Content: nudge}},
		SystemPrompt: p.currentSystemPrompt(),
	})
	if err != nil {
		return p.fail(err, EventFail)
	}
	if p.logger != nil {
		p.logger.Infof("remark Chat: %dms", time.Since(chatStart).Milliseconds())
	}
	reply = strings.TrimSpace(reply)
	if reply == "" {
		if p.machine != nil {
			p.machine.Transition(EventRest)
		}
		return nil
	}
	if p.logger != nil {
		p.logger.Debugf("remark reply: %q", reply)
	}

	ttsStart := time.Now()
	speech, err := p.tts.Speak(ctx, providers.SpeechRequest{
		Model:        p.ttsModel,
		Voice:        p.ttsVoice,
		Input:        reply,
		Format:       "pcm",
		Instructions: p.currentTTSInstructions(),
	})
	if err != nil {
		return p.fail(err, EventFail)
	}
	if p.logger != nil {
		p.logger.Infof("remark TTS: %dms (%d bytes)", time.Since(ttsStart).Milliseconds(), len(speech))
	}
	speech = audio.ResampleS16LE(speech, ttsPCMSampleRate, p.sampleRate, p.channels)

	return p.speak(ctx, speech)
}
```

- [ ] **Step 4: Run the package tests**

Run: `go test ./internal/assistant/`
Expected: PASS (the empty-reply test works because Task 11 made
idle→thinking legal, and thinking→idle via EventRest already existed)

- [ ] **Step 5: Commit**

```bash
git add internal/assistant/voice.go internal/assistant/voice_test.go
git commit -m "feat: SpeakRemark — proactive remarks through the voice pipeline"
```

---

### Task 13: ProactiveScheduler — jittered timing and gating

**Files:**
- Create: `internal/assistant/proactive.go`
- Test: `internal/assistant/proactive_test.go`

- [ ] **Step 1: Write the failing tests `internal/assistant/proactive_test.go`**

```go
package assistant

import (
	"testing"
	"time"
)

func proactiveFixture() (*Machine, *ProactiveScheduler, time.Time) {
	m := NewMachine()
	m.SetMode("ai")
	t0 := time.Date(2026, 6, 11, 16, 0, 0, 0, time.UTC)
	m.RecordInteraction(t0.Add(-10 * time.Minute)) // long quiet
	return m, NewProactiveScheduler(m, 1), t0
}

func TestProactiveSchedulerDisabledByDefault(t *testing.T) {
	_, s, t0 := proactiveFixture()
	if s.Due(t0.Add(24 * time.Hour)) {
		t.Fatal("scheduler with no interval must never be due")
	}
}

func TestProactiveSchedulerFiresWithinJitterBounds(t *testing.T) {
	_, s, t0 := proactiveFixture()
	s.SetInterval(10 * time.Minute)
	// First Due() call arms the timer and reports not-due.
	if s.Due(t0) {
		t.Fatal("first tick must arm, not fire")
	}
	// ±40% jitter: never before 6m, always by 14m (plus a tick of slack).
	if s.Due(t0.Add(6*time.Minute - time.Second)) {
		t.Fatal("fired before jitter lower bound")
	}
	if !s.Due(t0.Add(14*time.Minute + time.Second)) {
		t.Fatal("not due after jitter upper bound")
	}
}

func TestProactiveSchedulerRescheduleSpreadsFires(t *testing.T) {
	_, s, t0 := proactiveFixture()
	s.SetInterval(10 * time.Minute)
	s.Due(t0) // arm
	fire1 := t0.Add(14*time.Minute + time.Second)
	if !s.Due(fire1) {
		t.Fatal("expected due at upper bound")
	}
	s.Reschedule(fire1)
	if s.Due(fire1.Add(6*time.Minute - time.Second)) {
		t.Fatal("fired again before lower bound after reschedule")
	}
	if !s.Due(fire1.Add(14*time.Minute + time.Second)) {
		t.Fatal("not due after upper bound after reschedule")
	}
}

func TestProactiveSchedulerGates(t *testing.T) {
	m, s, t0 := proactiveFixture()
	s.SetInterval(10 * time.Minute)
	s.Due(t0) // arm
	due := t0.Add(15 * time.Minute)

	m.SetMode("idle") // AI off
	if s.Due(due) {
		t.Fatal("must not fire outside AI mode")
	}
	m.SetMode("ai")

	m.Transition(EventListen) // mid-conversation
	if s.Due(due) {
		t.Fatal("must not fire while not idle")
	}
	m.Transition(EventRest) // back to idle, but lastInteraction just updated

	if s.Due(due) {
		t.Fatal("must not fire within the 2-minute quiet window")
	}
	m.RecordInteraction(due.Add(-3 * time.Minute))
	if !s.Due(due) {
		t.Fatal("expected due once idle, AI-on, and quiet")
	}
}

func TestProactiveSchedulerSetIntervalZeroDisables(t *testing.T) {
	_, s, t0 := proactiveFixture()
	s.SetInterval(10 * time.Minute)
	s.Due(t0) // arm
	s.SetInterval(0)
	if s.Due(t0.Add(24 * time.Hour)) {
		t.Fatal("interval 0 must disable the scheduler")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/assistant/ -run Proactive -v`
Expected: FAIL — `undefined: NewProactiveScheduler`

- [ ] **Step 3: Create `internal/assistant/proactive.go`**

```go
package assistant

import (
	"math/rand"
	"sync"
	"time"
)

// proactiveMinQuiet is how long BMO stays silent after any interaction
// before a proactive remark may fire — remarks must feel spontaneous, not
// like he is butting back into a conversation that just ended.
const proactiveMinQuiet = 2 * time.Minute

// ProactiveScheduler decides WHEN BMO may make a spontaneous remark. It is
// pure timing and gating; WHAT to say comes from devctx.Builder's
// ProactiveNudge, and saying it is VoicePipeline.SpeakRemark's job. Each
// fire is rescheduled at the base interval ±40% jitter so remarks never
// feel like clockwork. Safe for concurrent use.
type ProactiveScheduler struct {
	mu       sync.Mutex
	machine  *Machine
	rng      *rand.Rand
	interval time.Duration
	next     time.Time
}

func NewProactiveScheduler(machine *Machine, seed int64) *ProactiveScheduler {
	return &ProactiveScheduler{machine: machine, rng: rand.New(rand.NewSource(seed))}
}

// SetInterval sets the base interval between remarks; 0 disables. The next
// fire time is re-armed lazily on the next Due tick, so changing levels in
// the settings menu takes effect immediately without firing instantly.
func (s *ProactiveScheduler) SetInterval(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.interval == d {
		return
	}
	s.interval = d
	s.next = time.Time{}
}

// Due reports whether a proactive remark should fire now. The first call
// after enabling (or changing) the interval arms the timer and returns
// false. When the timer has elapsed but a gate blocks (not idle, AI off,
// too soon after an interaction), Due keeps returning false and fires as
// soon as the gates clear. Callers must Reschedule after acting on true.
func (s *ProactiveScheduler) Due(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.interval <= 0 {
		return false
	}
	if s.next.IsZero() {
		s.next = now.Add(s.jittered())
		return false
	}
	if now.Before(s.next) {
		return false
	}
	if s.machine == nil || !s.machine.AIEnabled() || s.machine.State() != StateIdle {
		return false
	}
	if now.Sub(s.machine.Snapshot().LastInteraction) < proactiveMinQuiet {
		return false
	}
	return true
}

// Reschedule arms the next fire after a remark was attempted (whether or
// not it produced speech — a failed remark should not retry immediately).
func (s *ProactiveScheduler) Reschedule(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.interval > 0 {
		s.next = now.Add(s.jittered())
	}
}

// jittered returns the base interval ±40%. Caller must hold s.mu.
func (s *ProactiveScheduler) jittered() time.Duration {
	f := 0.6 + 0.8*s.rng.Float64()
	return time.Duration(float64(s.interval) * f)
}
```

- [ ] **Step 4: Run the package tests**

Run: `go test ./internal/assistant/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/assistant/proactive.go internal/assistant/proactive_test.go
git commit -m "feat: proactive remark scheduler with jitter and idle gating"
```

---

### Task 14: Settings menu — 5 awareness toggles + proactive level

**Files:**
- Modify: `internal/ui/settings_menu.go`
- Test: `internal/ui/settings_menu_test.go`

Current menu has 6 items (focus 0–5): log level, mode, 3 API keys, restore
defaults. New layout has 12 (focus 0–11): the existing five stay at 0–4,
awareness toggles take 5–9, proactive talk is 10, and RESTORE DEFAULTS
moves to 11 (it stays last by convention).

- [ ] **Step 1: Write the failing tests (append to `internal/ui/settings_menu_test.go`)**

```go
func TestSettingsMenuTogglesAwarenessCategories(t *testing.T) {
	cfg := config.Default()
	m := NewSettingsMenu(cfg)
	// Focus 5 = Awareness: Library (defaults on → toggles off).
	m.Move(5)
	if err := m.ToggleFocused(); err != nil {
		t.Fatalf("toggle library: %v", err)
	}
	if m.Config().DeviceContext.Library {
		t.Fatal("library toggle did not flip off")
	}
	if err := m.ToggleFocused(); err != nil {
		t.Fatalf("toggle library back: %v", err)
	}
	if !m.Config().DeviceContext.Library {
		t.Fatal("library toggle did not flip back on")
	}
	// Focus 9 = Awareness: Achievements.
	m.Move(4)
	if err := m.ToggleFocused(); err != nil {
		t.Fatalf("toggle achievements: %v", err)
	}
	if m.Config().DeviceContext.Achievements {
		t.Fatal("achievements toggle did not flip off")
	}
}

func TestSettingsMenuCyclesProactiveTalk(t *testing.T) {
	m := NewSettingsMenu(config.Default())
	m.Move(10)
	want := []string{
		config.ProactiveChatty, config.ProactiveRegular,
		config.ProactiveOccasional, config.ProactiveRare, config.ProactiveOff,
	}
	for _, level := range want {
		if err := m.ToggleFocused(); err != nil {
			t.Fatalf("cycle proactive: %v", err)
		}
		if got := m.Config().ProactiveTalk; got != level {
			t.Fatalf("proactive talk = %q, want %q", got, level)
		}
	}
}

func TestSettingsMenuRestoreDefaultsMovedToLastSlot(t *testing.T) {
	m := NewSettingsMenu(config.Default())
	called := false
	m.SetRestoreDefaultsCallback(func() error { called = true; return nil })
	m.Move(11)
	if err := m.ToggleFocused(); err != nil {
		t.Fatalf("restore defaults: %v", err)
	}
	if !called {
		t.Fatal("restore defaults callback not invoked at focus 11")
	}
}

func TestSettingsMenuOverlayShowsAwarenessItems(t *testing.T) {
	m := NewSettingsMenu(config.Default())
	overlay := m.Overlay()
	if len(overlay.Items) != 12 {
		t.Fatalf("expected 12 overlay items, got %d", len(overlay.Items))
	}
	labels := map[string]string{}
	for _, item := range overlay.Items {
		labels[item.Code] = item.Label
	}
	for code, want := range map[string]string{
		"aware_library":      "AWARE LIBRARY: ON",
		"aware_saves":        "AWARE SAVES: ON",
		"aware_playlog":      "AWARE PLAY LOG: ON",
		"aware_system":       "AWARE SYSTEM: ON",
		"aware_achievements": "AWARE ACHIEVEMENTS: ON",
		"proactive_talk":     "PROACTIVE TALK: OFF",
	} {
		if labels[code] != want {
			t.Errorf("item %s label = %q, want %q", code, labels[code], want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui/ -run 'Awareness|ProactiveTalk|MovedToLast' -v`
Expected: FAIL — only 6 overlay items; focus 5 hits restore defaults

- [ ] **Step 3: Modify `internal/ui/settings_menu.go`**

In `Move`, change `const count = 6` to `const count = 12`.

In `ToggleFocused`, replace `case 5:` (restore defaults) with:

```go
		case 5:
			m.cfg.DeviceContext.Library = !m.cfg.DeviceContext.Library
		case 6:
			m.cfg.DeviceContext.Saves = !m.cfg.DeviceContext.Saves
		case 7:
			m.cfg.DeviceContext.PlayLog = !m.cfg.DeviceContext.PlayLog
		case 8:
			m.cfg.DeviceContext.System = !m.cfg.DeviceContext.System
		case 9:
			m.cfg.DeviceContext.Achievements = !m.cfg.DeviceContext.Achievements
		case 10: // proactive talk — cycle through supported levels
			levels := config.SupportedProactiveTalkLevels()
			curr := strings.ToLower(strings.TrimSpace(m.cfg.ProactiveTalk))
			next := levels[0]
			for i, l := range levels {
				if l == curr {
					next = levels[(i+1)%len(levels)]
					break
				}
			}
			m.cfg.ProactiveTalk = next
		case 11: // restore persona/voice prompt files to built-in defaults
			if m.onRestore != nil {
				return m.onRestore()
			}
```

In `Overlay`, replace the `restore_defaults` item with (keeping it last):

```go
		{Code: "aware_library", Label: "AWARE LIBRARY: " + onOff(m.cfg.DeviceContext.Library),
			Selected: m.cfg.DeviceContext.Library, Focused: m.focus == 5 && !m.editing},
		{Code: "aware_saves", Label: "AWARE SAVES: " + onOff(m.cfg.DeviceContext.Saves),
			Selected: m.cfg.DeviceContext.Saves, Focused: m.focus == 6 && !m.editing},
		{Code: "aware_playlog", Label: "AWARE PLAY LOG: " + onOff(m.cfg.DeviceContext.PlayLog),
			Selected: m.cfg.DeviceContext.PlayLog, Focused: m.focus == 7 && !m.editing},
		{Code: "aware_system", Label: "AWARE SYSTEM: " + onOff(m.cfg.DeviceContext.System),
			Selected: m.cfg.DeviceContext.System, Focused: m.focus == 8 && !m.editing},
		{Code: "aware_achievements", Label: "AWARE ACHIEVEMENTS: " + onOff(m.cfg.DeviceContext.Achievements),
			Selected: m.cfg.DeviceContext.Achievements, Focused: m.focus == 9 && !m.editing},
		{Code: "proactive_talk", Label: "PROACTIVE TALK: " + strings.ToUpper(m.cfg.ProactiveTalk),
			Selected: m.cfg.ProactiveTalk != config.ProactiveOff, Focused: m.focus == 10 && !m.editing},
		{Code: "restore_defaults", Label: "RESTORE DEFAULTS",
			Selected: true, Focused: m.focus == 11 && !m.editing},
```

Add the helper at the bottom of the file:

```go
func onOff(v bool) string {
	if v {
		return "ON"
	}
	return "OFF"
}
```

- [ ] **Step 4: Run the package tests**

Run: `go test ./internal/ui/`
Expected: PASS — if any pre-existing test asserts focus position 5 ==
restore defaults or item count == 6, update that test to the new layout
(focus 11, 12 items) as part of this step.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/settings_menu.go internal/ui/settings_menu_test.go
git commit -m "feat: settings menu items for device awareness and proactive talk"
```

---

### Task 15: Wiring — main_fb.go, main_sdl.go, ptt_shared.go

**Files:**
- Modify: `cmd/bmo-pak/ptt_shared.go`
- Modify: `cmd/bmo-pak/main_fb.go`
- Modify: `cmd/bmo-pak/main_sdl.go`

`main_fb.go` (`//go:build !cgo`, the device build) and `main_sdl.go`
(`//go:build cgo`, desktop) are parallel implementations — apply the SAME
changes to BOTH. Line references below are for main_fb.go; find the
equivalent spots in main_sdl.go by the same anchors (`hardware.Detect`,
`SetSystemPromptSource`, `commitMenu := func`, `case assistant.StateIdle:`).

There are no unit tests for main wiring (none exist today); correctness is
covered by the compile + the package tests + the verification task.

- [ ] **Step 1: Add the prompt-join helper to `cmd/bmo-pak/ptt_shared.go`**

```go
// systemPromptWithContext joins the persona prompt and the device-awareness
// block; either may be empty.
func systemPromptWithContext(persona, deviceCtx string) string {
	persona = strings.TrimSpace(persona)
	deviceCtx = strings.TrimSpace(deviceCtx)
	switch {
	case persona == "":
		return deviceCtx
	case deviceCtx == "":
		return persona
	default:
		return persona + "\n\n" + deviceCtx
	}
}
```

- [ ] **Step 2: Build the collectors and scheduler in `main_fb.go`**

Add `"math/rand"` and `"github.com/carroarmato0/nextui-bmo/internal/devctx"`
to the imports. Directly AFTER the `hardwareProfile := hardware.Detect(platform)`
block (around line 142, after the two `logger.Infof("hardware...")` lines),
insert:

```go
	// Device awareness: read-only collectors feeding the DEVICE AWARENESS
	// block of the system prompt. BMO_SDCARD_ROOT overrides the SD card
	// location for desktop testing against pulled fixtures.
	sdRoot := strings.TrimSpace(os.Getenv("BMO_SDCARD_ROOT"))
	if sdRoot == "" {
		sdRoot = "/mnt/SDCARD"
	}
	achievementsCollector := devctx.AchievementsCollector{
		CacheDir:     filepath.Join(sdRoot, ".userdata", "shared", ".ra", "offline", "cache"),
		SettingsPath: filepath.Join(sdRoot, ".userdata", "shared", "minuisettings.txt"),
		Rng:          rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	deviceCtx := devctx.NewBuilder([]devctx.Collector{
		devctx.LibraryCollector{Root: filepath.Join(sdRoot, "Roms")},
		devctx.SavesCollector{Root: filepath.Join(sdRoot, "Saves")},
		devctx.PlayLogCollector{DBPath: filepath.Join(sdRoot, ".userdata", "shared", "game_logs.sqlite")},
		devctx.SystemCollector{
			Model:       hardwareProfile.DeviceTreeModel,
			UptimePath:  "/proc/uptime",
			MeminfoPath: "/proc/meminfo",
			DiskPath:    sdRoot,
			PowerDir:    "/sys/class/power_supply",
		},
		achievementsCollector,
	}, 30*time.Second, time.Now().UnixNano())
	deviceCtx.SetEnabled(cfg.DeviceContext)
	deviceCtx.SetReminisce(achievementsCollector.RandomPastUnlock)
	proactive := assistant.NewProactiveScheduler(machine, time.Now().UnixNano())
	proactive.SetInterval(config.ProactiveInterval(cfg.ProactiveTalk))
```

- [ ] **Step 3: Feed the snapshot into the system prompt**

Replace (currently around line 176):

```go
			audioPipeline.SetSystemPromptSource(func() string { return readPromptFile(personaPath) })
```

with:

```go
			audioPipeline.SetSystemPromptSource(func() string {
				return systemPromptWithContext(readPromptFile(personaPath), deviceCtx.Snapshot())
			})
```

- [ ] **Step 4: Propagate settings changes in `commitMenu`**

Inside the `commitMenu := func(menu ui.Menu) error {` closure, directly
after `machine.SetMode(cfg.Mode)`, add:

```go
		// Apply awareness toggles and proactive level immediately too.
		deviceCtx.SetEnabled(cfg.DeviceContext)
		proactive.SetInterval(config.ProactiveInterval(cfg.ProactiveTalk))
```

- [ ] **Step 5: Fire proactive remarks from the face loop**

In the main loop's `case assistant.StateIdle:` block (after the
`nextIdleUpdate` expression-scheduling lines), add:

```go
			if audioPipeline != nil && proactive.Due(now) {
				proactive.Reschedule(now)
				if nudge, ok := deviceCtx.ProactiveNudge(); ok {
					remarkPipeline := audioPipeline
					go func() {
						if err := remarkPipeline.SpeakRemark(ctx, nudge); err != nil {
							logger.Warnf("proactive remark failed: %v", err)
						}
					}()
				}
			}
```

(`now` in that scope is already `time.Now().UTC()`. `SpeakRemark` re-checks
AI mode and idle state internally, so the goroutine racing a PTT press is
safe — worst case the remark is silently skipped.)

- [ ] **Step 6: Mirror Steps 2–5 in `main_sdl.go`**

Same code, same anchors: after `hardware.Detect`, the
`SetSystemPromptSource` call (line ~162), the `commitMenu` closure, and the
`case assistant.StateIdle:` block of its loop.

- [ ] **Step 7: Compile both variants and run everything**

```bash
gofmt -l ./cmd ./internal && go vet ./... && go test ./...
CGO_ENABLED=0 go build ./cmd/bmo-pak && rm -f bmo-pak
```

Expected: no gofmt output, vet clean, all tests pass, build succeeds.
(The cgo/SDL variant needs SDL headers; if `CGO_ENABLED=1 go build
./cmd/bmo-pak` fails for missing SDL on this machine, verify main_sdl.go
compiles with `go vet -tags cgo ./cmd/bmo-pak` instead.)

- [ ] **Step 8: Commit**

```bash
git add cmd/bmo-pak/
git commit -m "feat: wire device awareness and proactive remarks into the pak"
```

---

### Task 16: Final verification — full suite, device build, on-device smoke test

**Files:** none (verification only)

- [ ] **Step 1: Full test suite and lint**

```bash
go vet ./... && go test ./...
```

Expected: PASS across `internal/config`, `internal/devctx`,
`internal/assistant`, `internal/ui`, and the rest.

- [ ] **Step 2: Device (arm64, CGO-free) build — proves modernc.org/sqlite works there**

```bash
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -buildvcs=false -o /tmp/bmo-pak-arm64 ./cmd/bmo-pak
ls -la /tmp/bmo-pak-arm64
```

Expected: builds cleanly. Note the binary size (the sqlite driver adds a
few MB — that is accepted in the spec).

- [ ] **Step 3: Desktop smoke test against real device data (optional but recommended)**

Pull a fixture SD tree from the device and point the pak at it:

```bash
mkdir -p /tmp/bmo-sd/.userdata/shared
adb pull /mnt/SDCARD/.userdata/shared/game_logs.sqlite /tmp/bmo-sd/.userdata/shared/
adb pull /mnt/SDCARD/.userdata/shared/minuisettings.txt /tmp/bmo-sd/.userdata/shared/
adb pull /mnt/SDCARD/.userdata/shared/.ra /tmp/bmo-sd/.userdata/shared/.ra
adb pull /mnt/SDCARD/Roms /tmp/bmo-sd/Roms        # or a subset
adb pull /mnt/SDCARD/Saves /tmp/bmo-sd/Saves
```

Then run the pak with `BMO_SDCARD_ROOT=/tmp/bmo-sd` and confirm in the
debug log (`LOG: DEBUG` in settings; the pipeline logs the system prompt
content at debug level via the chat request) that the DEVICE AWARENESS
block contains all five sections, plausible relative times, and the
Alleyway achievement. Ask BMO "what did I play recently?" over PTT if a
desktop audio setup is available.

- [ ] **Step 4: On-device deploy (manual, with the user)**

Coordinate with the user before touching the device. Typical flow:

```bash
adb push /tmp/bmo-pak-arm64 /mnt/SDCARD/Tools/tg5040/BMO.pak/bmo-pak
```

Then on the device: launch BMO, open Settings, confirm the six new items
render and cycle, enable `PROACTIVE TALK: CHATTY`, leave BMO idle in AI
mode and wait ~5–10 minutes for a spontaneous remark. Confirm a remark
never fires while talking to BMO over PTT.

- [ ] **Step 5: Finish the branch**

Use the superpowers:finishing-a-development-branch skill (or ask the user)
to decide merge/PR/cleanup.

---

## Self-review notes (already applied)

- Spec coverage: 5 collectors (Tasks 3–8), time awareness (Tasks 2, 9, 10),
  proactive remarks with freshness weighting + reminisce + proportional
  difficulty reactions (Tasks 10, 12, 13), config + settings toggles with
  defaults on/off per spec (Tasks 1, 14), wiring + error handling (Task 15),
  read-only guarantee (no collector writes; achievements never reads
  credentials — enforced in Task 8 code and test fixture containing a
  raToken line that must never surface).
- Legacy-config migration risk covered by
  `TestLoadConfigWithoutDeviceContextDefaultsEnabled` (Task 1).
- Type consistency: `Section`/`Collector` (Task 2) used by Tasks 3–10;
  `config.DeviceContext`/`ProactiveInterval` (Task 1) used by Tasks 9, 13,
  14, 15; `SpeakRemark` (Task 12) called in Task 15; `ProactiveNudge`
  (Task 10) called in Task 15.

