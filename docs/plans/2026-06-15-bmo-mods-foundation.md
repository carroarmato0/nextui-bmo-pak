# BMO `mods/` Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Introduce a `mods/` directory where each subfolder is a selectable customization of BMO (persona, voice, quotes, faces, audio), resolved through a new `internal/mod` package and selectable live from the Settings menu.

**Architecture:** A new `internal/mod` package is the single source of truth for discovering mods and resolving asset paths. `mods/default` has overlay semantics (per-asset fallback to embedded BMO); any other folder is a self-contained character (its `faces/` set is authoritative, with no embedded face fallback once it ships ≥1 SVG). `face.Library` gains a `selfContained` mode to encode that rule. The active mod id is stored in `config.ActiveMod` and chosen from a new Settings menu item; selecting one rebuilds the face cache and clip library in place — `main.go`'s nav handling and render loop share one goroutine, so the swap is race-free.

**Tech Stack:** Go (CGO disabled for tests), `go:embed` for default assets, SDL renderer, standard `encoding/json`. Module path: `github.com/carroarmato0/nextui-bmo`.

**Verification (run after every task):**
- `CGO_ENABLED=0 go test ./...`
- `golangci-lint run ./...` (new code must add no findings)

---

## File Structure

- **Create** `internal/mod/manifest.go` — `Manifest` struct, `CurrentAPIVersion`, tolerant `LoadManifest`, `EffectiveAPIVersion`.
- **Create** `internal/mod/manifest_test.go`
- **Create** `internal/mod/mod.go` — `Mod` struct, path-resolution methods, `FacesHasSVG`, `SelfContained`.
- **Create** `internal/mod/mod_test.go`
- **Create** `internal/mod/discover.go` — `Discover(modsRoot) []Mod`.
- **Create** `internal/mod/discover_test.go`
- **Modify** `internal/face/library.go` — add `selfContained` mode + `NewLibraryMode`.
- **Modify** `internal/face/library_test.go` — add self-contained tests.
- **Modify** `internal/config/config.go` — add `ActiveMod` field + normalize.
- **Modify** `internal/config/config_test.go` — round-trip test.
- **Modify** `internal/ui/settings_menu.go` — add `MOD` selector item, `ModChoice`, callbacks.
- **Modify** `internal/ui/settings_menu_test.go` — update index/count assertions, add mod-cycle test.
- **Modify** `cmd/bmo-pak/main.go` — wire the active mod, build face/clip libraries from it, live-reload on switch.

---

## Task 1: `internal/mod` — Manifest + API versioning

**Files:**
- Create: `internal/mod/manifest.go`
- Test: `internal/mod/manifest_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/mod/manifest_test.go`:

```go
package mod

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadManifestAbsent(t *testing.T) {
	m := LoadManifest(t.TempDir())
	if m.EffectiveAPIVersion() != 1 {
		t.Fatalf("absent manifest: EffectiveAPIVersion = %d, want 1", m.EffectiveAPIVersion())
	}
	if m.Name != "" {
		t.Fatalf("absent manifest: Name = %q, want empty", m.Name)
	}
}

func TestLoadManifestValid(t *testing.T) {
	dir := t.TempDir()
	body := `{"apiVersion":2,"name":"Evil BMO","author":"me","description":"d","version":"1.0"}`
	if err := os.WriteFile(filepath.Join(dir, "mod.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	m := LoadManifest(dir)
	if m.Name != "Evil BMO" || m.Author != "me" || m.Version != "1.0" {
		t.Fatalf("manifest fields wrong: %+v", m)
	}
	if m.EffectiveAPIVersion() != 2 {
		t.Fatalf("EffectiveAPIVersion = %d, want 2", m.EffectiveAPIVersion())
	}
}

func TestLoadManifestPartialDefaultsAPIVersion(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "mod.json"), []byte(`{"name":"Just A Name"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	m := LoadManifest(dir)
	if m.Name != "Just A Name" {
		t.Fatalf("Name = %q", m.Name)
	}
	if m.EffectiveAPIVersion() != 1 {
		t.Fatalf("omitted apiVersion should default to 1, got %d", m.EffectiveAPIVersion())
	}
}

func TestLoadManifestMalformedTolerated(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "mod.json"), []byte(`{not json`), 0o644); err != nil {
		t.Fatal(err)
	}
	m := LoadManifest(dir) // must not panic; returns zero value
	if m.Name != "" || m.EffectiveAPIVersion() != 1 {
		t.Fatalf("malformed manifest should yield zero value, got %+v", m)
	}
}

func TestCurrentAPIVersionIsOne(t *testing.T) {
	if CurrentAPIVersion != 1 {
		t.Fatalf("CurrentAPIVersion = %d, want 1", CurrentAPIVersion)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `CGO_ENABLED=0 go test ./internal/mod/ -run TestLoadManifest -v`
Expected: FAIL — `undefined: LoadManifest` / package has no non-test files.

- [ ] **Step 3: Write the implementation**

Create `internal/mod/manifest.go`:

```go
// Package mod discovers and resolves BMO mods: subfolders of mods/ that
// customize persona, voice, quotes, faces, and audio. mods/default has
// overlay semantics (per-asset fallback to embedded BMO); any other folder
// is a self-contained character.
package mod

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// CurrentAPIVersion is the mod-format contract this build implements. Bump it
// in a future spec when a compatibility-breaking change to the mod format
// ships; add version-specific handling keyed off Manifest.EffectiveAPIVersion.
const CurrentAPIVersion = 1

// Manifest is the optional mod.json. Every field is optional; a missing or
// malformed file yields the zero value and the mod still loads.
type Manifest struct {
	APIVersion  int    `json:"apiVersion"`  // mod-format version; 0/absent => 1
	Name        string `json:"name"`        // display name override
	Author      string `json:"author"`      // shown in Settings
	Description string `json:"description"` // shown in Settings
	Version     string `json:"version"`     // author's own free-form release string
}

// EffectiveAPIVersion returns the declared apiVersion, treating absent (0) as
// 1 — the version current when the field was introduced. This default is
// frozen forever so that undeclared mods keep their original semantics when a
// newer API ships.
func (m Manifest) EffectiveAPIVersion() int {
	if m.APIVersion <= 0 {
		return 1
	}
	return m.APIVersion
}

// LoadManifest reads modRoot/mod.json. A missing or malformed file is
// tolerated: it returns the zero Manifest (whose EffectiveAPIVersion is 1).
func LoadManifest(modRoot string) Manifest {
	data, err := os.ReadFile(filepath.Join(modRoot, "mod.json"))
	if err != nil {
		return Manifest{}
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}
	}
	return m
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `CGO_ENABLED=0 go test ./internal/mod/ -v`
Expected: PASS (all manifest tests).

- [ ] **Step 5: Commit**

```bash
git add internal/mod/manifest.go internal/mod/manifest_test.go
git commit -m "feat(mod): manifest parsing with frozen apiVersion default"
```

---

## Task 2: `internal/mod` — Mod type + path resolution

**Files:**
- Create: `internal/mod/mod.go`
- Test: `internal/mod/mod_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/mod/mod_test.go`:

```go
package mod

import (
	"os"
	"path/filepath"
	"testing"
)

func TestModPaths(t *testing.T) {
	m := Mod{ID: "evil", Root: "/x/mods/evil"}
	cases := map[string]string{
		m.PersonaPath(): "/x/mods/evil/persona.txt",
		m.VoicePath():   "/x/mods/evil/voice.txt",
		m.QuotesPath():  "/x/mods/evil/quotes.txt",
		m.FacesDir():    "/x/mods/evil/faces",
		m.AudioDir():    "/x/mods/evil/audio",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
	}
}

func TestFacesHasSVG(t *testing.T) {
	dir := t.TempDir()
	m := Mod{ID: "evil", Root: dir}
	if m.FacesHasSVG() {
		t.Fatal("no faces dir yet: FacesHasSVG should be false")
	}
	facesDir := filepath.Join(dir, "faces")
	if err := os.MkdirAll(facesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(facesDir, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if m.FacesHasSVG() {
		t.Fatal("only a .txt present: FacesHasSVG should be false")
	}
	if err := os.WriteFile(filepath.Join(facesDir, "happy.svg"), []byte("<svg/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !m.FacesHasSVG() {
		t.Fatal("an .svg is present: FacesHasSVG should be true")
	}
}

func TestSelfContained(t *testing.T) {
	dir := t.TempDir()
	facesDir := filepath.Join(dir, "faces")
	if err := os.MkdirAll(facesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(facesDir, "happy.svg"), []byte("<svg/>"), 0o644); err != nil {
		t.Fatal(err)
	}

	named := Mod{ID: "evil", Root: dir, IsDefault: false}
	if !named.SelfContained() {
		t.Fatal("named mod with ≥1 svg must be self-contained")
	}

	def := Mod{ID: "default", Root: dir, IsDefault: true}
	if def.SelfContained() {
		t.Fatal("default mod is overlay: never self-contained")
	}

	bare := Mod{ID: "lite", Root: t.TempDir(), IsDefault: false}
	if bare.SelfContained() {
		t.Fatal("named mod with no faces must inherit embedded (not self-contained)")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `CGO_ENABLED=0 go test ./internal/mod/ -run 'TestModPaths|TestFacesHasSVG|TestSelfContained' -v`
Expected: FAIL — `undefined: Mod`.

- [ ] **Step 3: Write the implementation**

Create `internal/mod/mod.go`:

```go
package mod

import (
	"os"
	"path/filepath"
	"strings"
)

// DefaultID is the reserved folder name with overlay semantics.
const DefaultID = "default"

// Mod is a resolved mod: a folder under mods/ plus its (optional) manifest.
type Mod struct {
	ID        string   // folder name; the default entry uses DefaultID
	Root      string   // absolute path to mods/<id> (may not exist on disk)
	Manifest  Manifest // zero value when no mod.json
	IsDefault bool     // ID == DefaultID — overlay semantics
}

// DisplayName is what the Settings menu shows: the manifest name if set, else
// a friendly label for the default entry, else the folder id.
func (m Mod) DisplayName() string {
	if name := strings.TrimSpace(m.Manifest.Name); name != "" {
		return name
	}
	if m.IsDefault {
		return "BMO (Default)"
	}
	return m.ID
}

func (m Mod) PersonaPath() string { return filepath.Join(m.Root, "persona.txt") }
func (m Mod) VoicePath() string   { return filepath.Join(m.Root, "voice.txt") }
func (m Mod) QuotesPath() string  { return filepath.Join(m.Root, "quotes.txt") }
func (m Mod) FacesDir() string    { return filepath.Join(m.Root, "faces") }
func (m Mod) AudioDir() string    { return m.Root } // clips.Library appends "audio/"

// FacesHasSVG reports whether the mod's faces/ directory holds at least one
// .svg file. A missing or unreadable directory counts as none.
func (m Mod) FacesHasSVG() bool {
	entries, err := os.ReadDir(m.FacesDir())
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".svg") {
			return true
		}
	}
	return false
}

// SelfContained reports whether the mod owns its entire face set with no
// embedded fallback. True only for a named mod that ships ≥1 face; the default
// overlay and a named mod with no faces both inherit embedded BMO art.
func (m Mod) SelfContained() bool {
	return !m.IsDefault && m.FacesHasSVG()
}
```

Note: `AudioDir()` returns `m.Root` because `clips.NewLibrary(root)` already
appends `audio/` internally (see `internal/clips/library.go`). The method name
documents intent at the call site.

- [ ] **Step 4: Run the test to verify it passes**

Run: `CGO_ENABLED=0 go test ./internal/mod/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/mod/mod.go internal/mod/mod_test.go
git commit -m "feat(mod): Mod type with path resolution and self-contained rule"
```

---

## Task 3: `internal/mod` — Discover

**Files:**
- Create: `internal/mod/discover.go`
- Test: `internal/mod/discover_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/mod/discover_test.go`:

```go
package mod

import (
	"os"
	"path/filepath"
	"testing"
)

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverEmptyRootHasDefaultOnly(t *testing.T) {
	mods := Discover(filepath.Join(t.TempDir(), "mods")) // dir doesn't exist
	if len(mods) != 1 {
		t.Fatalf("want 1 mod (synthetic default), got %d", len(mods))
	}
	if !mods[0].IsDefault || mods[0].ID != DefaultID {
		t.Fatalf("first entry must be the default, got %+v", mods[0])
	}
}

func TestDiscoverOrdersDefaultFirstThenAlpha(t *testing.T) {
	root := filepath.Join(t.TempDir(), "mods")
	mkdir(t, filepath.Join(root, "zebra"))
	mkdir(t, filepath.Join(root, "alpha"))
	mkdir(t, filepath.Join(root, "default"))
	// Noise that must be ignored:
	mkdir(t, filepath.Join(root, ".git"))
	if err := os.WriteFile(filepath.Join(root, "loose.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	mods := Discover(root)
	var ids []string
	for _, m := range mods {
		ids = append(ids, m.ID)
	}
	want := []string{DefaultID, "alpha", "zebra"}
	if len(ids) != len(want) {
		t.Fatalf("ids = %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("ids = %v, want %v", ids, want)
		}
	}
	if !mods[0].IsDefault {
		t.Fatal("default must be first and flagged IsDefault")
	}
}

func TestDiscoverAttachesManifest(t *testing.T) {
	root := filepath.Join(t.TempDir(), "mods")
	evil := filepath.Join(root, "evil")
	mkdir(t, evil)
	if err := os.WriteFile(filepath.Join(evil, "mod.json"), []byte(`{"name":"Evil BMO"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, m := range Discover(root) {
		if m.ID == "evil" {
			if m.DisplayName() != "Evil BMO" {
				t.Fatalf("DisplayName = %q, want %q", m.DisplayName(), "Evil BMO")
			}
			return
		}
	}
	t.Fatal("evil mod not discovered")
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `CGO_ENABLED=0 go test ./internal/mod/ -run TestDiscover -v`
Expected: FAIL — `undefined: Discover`.

- [ ] **Step 3: Write the implementation**

Create `internal/mod/discover.go`:

```go
package mod

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Discover returns the selectable mods under modsRoot. The synthetic "default"
// entry (overlay on embedded BMO) is always first, even when mods/default does
// not exist on disk. Remaining entries are the existing subfolders in
// alphabetical order. Non-directories and dot-prefixed folders are ignored.
func Discover(modsRoot string) []Mod {
	def := Mod{
		ID:        DefaultID,
		Root:      filepath.Join(modsRoot, DefaultID),
		IsDefault: true,
	}
	def.Manifest = LoadManifest(def.Root)
	out := []Mod{def}

	entries, err := os.ReadDir(modsRoot)
	if err != nil {
		return out
	}

	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if name == DefaultID || strings.HasPrefix(name, ".") {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		root := filepath.Join(modsRoot, name)
		out = append(out, Mod{
			ID:       name,
			Root:     root,
			Manifest: LoadManifest(root),
		})
	}
	return out
}

// Active returns the mod in mods matching id, or the default entry (index 0)
// when id is empty or not found. mods must be the slice returned by Discover.
func Active(mods []Mod, id string) Mod {
	id = strings.TrimSpace(id)
	if id != "" {
		for _, m := range mods {
			if m.ID == id {
				return m
			}
		}
	}
	if len(mods) > 0 {
		return mods[0] // the default entry
	}
	return Mod{ID: DefaultID, IsDefault: true}
}
```

- [ ] **Step 4: Add a test for `Active` and run**

Append to `internal/mod/discover_test.go`:

```go
func TestActiveFallsBackToDefault(t *testing.T) {
	root := filepath.Join(t.TempDir(), "mods")
	mkdir(t, filepath.Join(root, "evil"))
	mods := Discover(root)

	if got := Active(mods, "evil"); got.ID != "evil" {
		t.Fatalf("Active(evil) = %q, want evil", got.ID)
	}
	if got := Active(mods, ""); !got.IsDefault {
		t.Fatal("Active(\"\") must return the default entry")
	}
	if got := Active(mods, "ghost"); !got.IsDefault {
		t.Fatal("Active(unknown id) must fall back to the default entry")
	}
}
```

Run: `CGO_ENABLED=0 go test ./internal/mod/ -v`
Expected: PASS (all mod tests).

- [ ] **Step 5: Commit**

```bash
git add internal/mod/discover.go internal/mod/discover_test.go
git commit -m "feat(mod): discover mods with synthetic default + active selection"
```

---

## Task 4: `face.Library` self-contained mode

**Files:**
- Modify: `internal/face/library.go`
- Test: `internal/face/library_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/face/library_test.go`:

```go
func TestSelfContainedFoldsMissingToModNeutral(t *testing.T) {
	dir := t.TempDir()
	happy := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210"><rect width="280" height="210" fill="#0f0"/></svg>`
	neutral := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210"><rect width="280" height="210" fill="#00f"/></svg>`
	if err := os.WriteFile(filepath.Join(dir, "happy.svg"), []byte(happy), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "neutral.svg"), []byte(neutral), 0o644); err != nil {
		t.Fatal(err)
	}
	lib := NewLibraryMode(dir, true)

	// An expression the mod ships is served directly.
	if data, fromDisk := lib.Bytes("happy"); !fromDisk || string(data) != happy {
		t.Fatalf("happy: fromDisk=%v", fromDisk)
	}
	// A missing expression folds to the mod's own neutral, never embedded.
	data, fromDisk := lib.Bytes("sad")
	if !fromDisk || string(data) != neutral {
		t.Fatalf("missing expr should fold to mod neutral, fromDisk=%v", fromDisk)
	}
}

func TestSelfContainedNoNeutralReturnsNothing(t *testing.T) {
	dir := t.TempDir()
	happy := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210"><rect width="280" height="210" fill="#0f0"/></svg>`
	if err := os.WriteFile(filepath.Join(dir, "happy.svg"), []byte(happy), 0o644); err != nil {
		t.Fatal(err)
	}
	lib := NewLibraryMode(dir, true)
	if data, fromDisk := lib.Bytes("sad"); data != nil || fromDisk {
		t.Fatal("self-contained mod with no neutral must return (nil,false), not embedded")
	}
}

func TestNonSelfContainedStillFallsBackToEmbedded(t *testing.T) {
	lib := NewLibraryMode(t.TempDir(), false)
	data, fromDisk := lib.Bytes(ExprNeutral)
	if fromDisk {
		t.Fatal("expected embedded source")
	}
	want, _ := defaultBytes(ExprNeutral)
	if string(data) != string(want) {
		t.Fatal("expected embedded neutral bytes")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `CGO_ENABLED=0 go test ./internal/face/ -run 'TestSelfContained|TestNonSelfContained' -v`
Expected: FAIL — `undefined: NewLibraryMode`.

- [ ] **Step 3: Write the implementation**

In `internal/face/library.go`, replace the `Library` struct and `NewLibrary` (lines 13–23) with:

```go
// Library resolves expression names to SVG bytes. The embedded defaults are
// the source of truth; an optional on-disk faces/ directory overrides per file.
// In self-contained mode the embedded defaults are NOT used: a named mod owns
// its whole face set, and a missing expression folds to the mod's own neutral.
type Library struct {
	dir           string
	selfContained bool
	logf          func(string, ...any)
}

// NewLibrary returns a Library that checks dir for overrides before falling
// back to embedded defaults. dir may not exist; missing dirs are silently
// ignored.
func NewLibrary(dir string) *Library {
	return NewLibraryMode(dir, false)
}

// NewLibraryMode is like NewLibrary but, when selfContained is true, disables
// the embedded fallback: only the on-disk faces/ directory is consulted, and a
// missing expression folds to the directory's neutral.svg (or, if that is also
// missing, resolves to nothing so the renderer draws its plain fallback).
func NewLibraryMode(dir string, selfContained bool) *Library {
	return &Library{dir: dir, selfContained: selfContained, logf: func(string, ...any) {}}
}
```

Then replace the embedded-fallback tail of `Bytes` (lines 68–72, the
`data, ok := defaultBytes(canonical)` block) with:

```go
	if l.selfContained {
		// Owns its whole set: no embedded fallback. Fold a missing expression
		// to the mod's own neutral, if it ships one.
		if canonical != ExprNeutral && l.dir != "" {
			data, err := os.ReadFile(filepath.Join(l.dir, ExprNeutral+".svg"))
			if err == nil && len(bytes.TrimSpace(data)) > 0 {
				return data, true
			}
		}
		return nil, false
	}

	data, ok := defaultBytes(canonical)
	if !ok {
		return nil, false
	}
	return data, false
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `CGO_ENABLED=0 go test ./internal/face/ -v`
Expected: PASS (new tests plus the existing `TestLibrary*` suite unchanged).

- [ ] **Step 5: Commit**

```bash
git add internal/face/library.go internal/face/library_test.go
git commit -m "feat(face): self-contained library mode for named mods"
```

---

## Task 5: `config.ActiveMod` field

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/config/config_test.go`:

```go
func TestActiveModRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg := Default()
	cfg.ActiveMod = "evil"
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.ActiveMod != "evil" {
		t.Fatalf("ActiveMod = %q, want evil", got.ActiveMod)
	}
}

func TestActiveModNormalizesWhitespace(t *testing.T) {
	cfg := Default()
	cfg.ActiveMod = "  evil  "
	cfg.Normalize()
	if cfg.ActiveMod != "evil" {
		t.Fatalf("ActiveMod = %q, want trimmed 'evil'", cfg.ActiveMod)
	}
}
```

(`filepath` is already imported in `config_test.go`.)

- [ ] **Step 2: Run the test to verify it fails**

Run: `CGO_ENABLED=0 go test ./internal/config/ -run TestActiveMod -v`
Expected: FAIL — `cfg.ActiveMod undefined`.

- [ ] **Step 3: Write the implementation**

In `internal/config/config.go`, add the field to the `Config` struct (after the
`Personality` field, line 117):

```go
	ActiveMod     string        `json:"active_mod,omitempty"`
```

In `Normalize()` (after the `Personality` block, around line 205), add:

```go
	c.ActiveMod = strings.TrimSpace(c.ActiveMod)
```

No `Validate()` change: an `ActiveMod` naming a missing folder is resolved to
the default entry at runtime by `mod.Active`, not rejected here.

- [ ] **Step 4: Run the test to verify it passes**

Run: `CGO_ENABLED=0 go test ./internal/config/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): add ActiveMod field"
```

---

## Task 6: Settings menu — MOD selector

The Settings menu uses fixed focus indices with `count = 16`. We insert `mod`
at index 15 and push `restore_defaults` to index 16, raising `count` to 17. The
`shouldSkip` rule (idx 3–6 always; idx 1 unless debug) is unchanged.

**Files:**
- Modify: `internal/ui/settings_menu.go`
- Test: `internal/ui/settings_menu_test.go`

- [ ] **Step 1: Update existing tests to the new layout (they will fail first)**

In `internal/ui/settings_menu_test.go`:

Rename and update `TestSettingsMenuHas16Items`:

```go
func TestSettingsMenuHas17Items(t *testing.T) {
	m := NewSettingsMenu(config.Default())
	if got := len(m.Overlay().Items); got != 17 {
		t.Fatalf("expected 17 overlay items, got %d", got)
	}
}
```

In `TestSettingsMenuItemCodes`, change the `want` slice's tail so `"mod"`
precedes `"restore_defaults"`:

```go
		"library_detail", "request_timeout", "proactive_talk", "mod", "restore_defaults",
```

In `TestSettingsMenuOverlayShowsAwarenessItems`, change `!= 16` to `!= 17` and
the message to `17`.

In `TestSettingsMenuRestoreDefaults`, change `menu.Move(15)` to `menu.Move(16)`
and the comment to "restore_defaults is now at idx 16."

In `TestSettingsMenuRestoreDefaultsIsLastSlot`, change `m.Move(15)` to
`m.Move(16)` and the comment/message to "focus 16."

Add a new mod-cycle test:

```go
func TestSettingsMenuCyclesMod(t *testing.T) {
	m := NewSettingsMenu(config.Default())
	m.SetModChoices([]ModChoice{
		{ID: "default", Label: "BMO (DEFAULT)"},
		{ID: "evil", Label: "EVIL BMO"},
	})
	var changed string
	m.SetModChangeCallback(func(id string) { changed = id })

	m.Move(15) // mod selector
	if got := m.Overlay().Items[15].Code; got != "mod" {
		t.Fatalf("expected mod item at idx 15, got %q", got)
	}
	if err := m.ToggleFocused(); err != nil { // default -> evil
		t.Fatalf("ToggleFocused: %v", err)
	}
	if m.Config().ActiveMod != "evil" {
		t.Fatalf("ActiveMod = %q, want evil", m.Config().ActiveMod)
	}
	if changed != "evil" {
		t.Fatalf("callback got %q, want evil", changed)
	}
	if err := m.ToggleFocused(); err != nil { // evil -> default (wraps)
		t.Fatalf("ToggleFocused: %v", err)
	}
	if m.Config().ActiveMod != "default" {
		t.Fatalf("ActiveMod = %q, want default after wrap", m.Config().ActiveMod)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `CGO_ENABLED=0 go test ./internal/ui/ -run 'TestSettingsMenu' -v`
Expected: FAIL — `m.SetModChoices undefined`, plus index/count mismatches.

- [ ] **Step 3: Write the implementation**

In `internal/ui/settings_menu.go`:

Add the choice type near the top (after the `logLevelOrder` var, line 10):

```go
// ModChoice is one selectable mod in the MOD cycle item. ID is persisted to
// config.ActiveMod; Label is the already-formatted display string.
type ModChoice struct {
	ID    string
	Label string
}
```

Add fields to `SettingsMenu` (after `onRestore`, line 17):

```go
	modChoices  []ModChoice
	onModChange func(string)
```

Add setters (after `SetRestoreDefaultsCallback`, line 38):

```go
// SetModChoices supplies the selectable mods shown by the MOD item.
func (m *SettingsMenu) SetModChoices(choices []ModChoice) {
	if m != nil {
		m.modChoices = choices
	}
}

// SetModChangeCallback registers a function called when the active mod is
// cycled, so the app can reload persona/voice/quotes/faces/audio in place.
func (m *SettingsMenu) SetModChangeCallback(fn func(string)) {
	if m != nil {
		m.onModChange = fn
	}
}

// modLabel returns the display label for the currently active mod.
func (m *SettingsMenu) modLabel() string {
	for _, c := range m.modChoices {
		if c.ID == m.cfg.ActiveMod {
			return c.Label
		}
	}
	if len(m.modChoices) > 0 {
		return m.modChoices[0].Label // active id not found: show the default
	}
	return "BMO (DEFAULT)"
}
```

Change the `count` constant in `Move` (line 54) from `16` to `17`.

In `ToggleFocused`, renumber the restore case and insert the mod case. Replace
the `case 15:` block (lines 139–142) with:

```go
	case 15:
		if len(m.modChoices) == 0 {
			return nil
		}
		idx := 0
		for i, c := range m.modChoices {
			if c.ID == m.cfg.ActiveMod {
				idx = i
				break
			}
		}
		next := m.modChoices[(idx+1)%len(m.modChoices)]
		m.cfg.ActiveMod = next.ID
		if m.onModChange != nil {
			m.onModChange(next.ID)
		}
	case 16:
		if m.onRestore != nil {
			return m.onRestore()
		}
```

In `Overlay()`, insert the mod item before `restore_defaults` and renumber
restore's `Focused`. Replace the final item (line 179) with:

```go
		{Code: "mod", Label: "MOD: " + m.modLabel(),
			Selected: true, Focused: m.focus == 15},
		{Code: "restore_defaults", Label: "RESTORE DEFAULTS", Focused: m.focus == 16},
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `CGO_ENABLED=0 go test ./internal/ui/ -v`
Expected: PASS (updated suite + `TestSettingsMenuCyclesMod`).

- [ ] **Step 5: Commit**

```bash
git add internal/ui/settings_menu.go internal/ui/settings_menu_test.go
git commit -m "feat(ui): MOD selector in settings menu"
```

---

## Task 7: Wire the active mod into `main.go` with live reload

`cmd/bmo-pak/main.go` has no unit tests, so this task is verified by build,
the full test suite, lint, and a manual desktop smoke run. Make the edits, then
run every check in Steps 4–6.

**Files:**
- Modify: `cmd/bmo-pak/main.go`

- [ ] **Step 1: Add the import**

In the import block, add:

```go
	"github.com/carroarmato0/nextui-bmo/internal/mod"
```

- [ ] **Step 2: Resolve the active mod and source assets from it**

After the config is loaded and validated (after line 80, before the
`// Persona, voice, and quotes prompts...` comment at line 82), insert:

```go
	// Mods live under $home/mods/<name>. The "default" entry overlays embedded
	// BMO; any other folder is a self-contained character. mods/default is
	// created so users have an obvious place to drop overrides.
	modsRoot := filepath.Join(homeDir, "mods")
	if err := os.MkdirAll(filepath.Join(modsRoot, mod.DefaultID), 0o755); err != nil {
		return fmt.Errorf("create mods directory: %w", err)
	}
	mods := mod.Discover(modsRoot)
	activeMod := mod.Active(mods, cfg.ActiveMod)
```

(No logging here — `logger` is constructed later, at line 92.) Then, just after
`defer logger.Close()` (line 96), add the active-mod log line:

```go
	logger.Infof("active mod: %s (self-contained=%t)", activeMod.ID, activeMod.SelfContained())
```

Replace the path assignments (lines 85–87) with mod-sourced paths:

```go
	personaPath := activeMod.PersonaPath()
	voicePath := activeMod.VoicePath()
	quotesPath := activeMod.QuotesPath()
```

These are plain variables captured by the per-utterance source closures
(`SetSystemPromptSource`, `SetTTSInstructionsSource`, the quotes provider).
Reassigning them in the reload function below updates what those closures read.

- [ ] **Step 3: Build the face and clip libraries from the active mod**

Replace the face library construction (lines 249–250):

```go
	faceLib := face.NewLibraryMode(activeMod.FacesDir(), activeMod.SelfContained())
	faceLib.SetLogf(logger.Warnf)
```

Replace the clip library construction (line 195):

```go
		clipLib := clips.NewLibrary(activeMod.AudioDir())
```

And the two pipeline clip loads (lines 232–233):

```go
				audioPipeline.SetTimeoutClip(clips.NewLibrary(activeMod.AudioDir()).Load("timeout"))
				audioPipeline.SetErrorClip(clips.NewLibrary(activeMod.AudioDir()).Load("error"))
```

Change the startup override check (line 476) to validate the active mod's
folder instead of the flat home:

```go
			if overrideErrs := config.CheckOverrides(activeMod.Root); len(overrideErrs) > 0 {
```

Change the restore-defaults callback (lines 120–127) to remove overrides from
the active mod's folder:

```go
	settingsMenu.SetRestoreDefaultsCallback(func() error {
		if err := config.RemoveOverrides(personaPath, voicePath, quotesPath); err != nil {
			logger.Warnf("restore defaults: %v", err)
			return err
		}
		logger.Infof("persona, voice, and quotes restored to built-in defaults")
		return nil
	})
```

(`personaPath`/`voicePath`/`quotesPath` are now the active mod's paths and are
updated by the reload function, so this closure always targets the current mod.)

- [ ] **Step 4: Wire the MOD selector choices and the live-reload function**

After `settingsMenu := ui.NewSettingsMenu(cfg)` (line 115), supply the choices:

```go
	modChoices := make([]ui.ModChoice, 0, len(mods))
	for _, md := range mods {
		modChoices = append(modChoices, ui.ModChoice{ID: md.ID, Label: strings.ToUpper(md.DisplayName())})
	}
	settingsMenu.SetModChoices(modChoices)
```

After the renderer and face cache are set up (after line 254,
`go faceCache.Warm(screen.Size())`), define the reload function and register
the callback. `clipPlayer` and `audioPipeline` are local variables captured by
the render loop and closures, so reassigning their dependencies here is visible
to subsequent frames. This runs on the main goroutine (from `handleNav`), the
same goroutine as `screen.Draw`, so swapping the face cache is race-free.

```go
	reloadMod := func(id string) {
		active := mod.Active(mods, id)
		activeMod = active
		personaPath = active.PersonaPath()
		voicePath = active.VoicePath()
		quotesPath = active.QuotesPath()

		// Rebuild and re-warm the face cache for the new art set.
		newLib := face.NewLibraryMode(active.FacesDir(), active.SelfContained())
		newLib.SetLogf(logger.Warnf)
		newCache := face.NewCache(newLib)
		faceCache = newCache
		screen.SetFaces(newCache)
		go newCache.Warm(screen.Size())

		// Rebuild clips from the new mod's audio/ dir.
		if audioSession != nil {
			clipLib := clips.NewLibrary(active.AudioDir())
			clipPlayer = clips.NewPlayer(audioSession, audioCfg.SampleRate, audioCfg.PlaybackChannels, clipLib)
			if audioPipeline != nil {
				audioPipeline.SetTimeoutClip(clipLib.Load("timeout"))
				audioPipeline.SetErrorClip(clipLib.Load("error"))
			}
		}
		logger.Infof("switched to mod %q (self-contained=%t)", active.ID, active.SelfContained())
	}
	settingsMenu.SetModChangeCallback(reloadMod)
```

Because `faceCache` is reassigned here, ensure its declaration is a plain
assignable variable: change line 251 from `faceCache := face.NewCache(faceLib)`
to keep `:=` (it already is) — `reloadMod` closes over it, so no change beyond
defining `reloadMod` after `faceCache` exists. Likewise `clipPlayer` (line 193,
`var clipPlayer *clips.Player`) and `activeMod` are already assignable.

- [ ] **Step 5: Build, test, and lint**

```bash
CGO_ENABLED=0 go build ./...
CGO_ENABLED=0 go test ./...
golangci-lint run ./...
```

Expected: build succeeds; all tests pass; no new lint findings.

- [ ] **Step 6: Manual desktop smoke test**

```bash
mkdir -p /tmp/bmo-mods-test/BMO/mods/evil/faces
printf 'You are EVIL BMO. Be short and menacing.\n' > /tmp/bmo-mods-test/BMO/mods/evil/persona.txt
printf '{"name":"Evil BMO","author":"test"}\n' > /tmp/bmo-mods-test/BMO/mods/evil/mod.json
BMO_DATA_ROOT=/tmp/bmo-mods-test CGO_ENABLED=1 go run ./cmd/bmo-pak
```

Verify in the log / on screen:
- Startup logs `active mod: default (self-contained=false)`.
- Opening Settings (Start) and cycling the **MOD** item shows `MOD: EVIL BMO`.
- The log shows `switched to mod "evil"`.
- `cat /tmp/bmo-mods-test/BMO/config.json` shows `"active_mod": "evil"` after the
  cycle is auto-saved.

(If SDL/audio is unavailable on the dev box, confirm the log lines; the face/
clip reload paths are exercised by the build + unit tests above.)

- [ ] **Step 7: Commit**

```bash
git add cmd/bmo-pak/main.go
git commit -m "feat(bmo-pak): select active mod from mods/ with live reload"
```

---

## Self-Review Notes (verification of this plan against the spec)

- **Directory layout** → Task 7 creates `mods/default`; all asset paths resolve under `mods/<id>/` (Task 2 path methods).
- **Resolution semantics** (text/audio always fall back; faces fall back only for default or a faceless named mod) → Task 2 `SelfContained`, Task 4 `face.NewLibraryMode`. Text/quotes/audio fall back via the existing `config.LoadPromptFile` / `clips.Library` embedded fallback, unchanged.
- **Named mod with no faces inherits embedded** → `SelfContained()` is false when `FacesHasSVG()` is false → `NewLibraryMode(dir, false)` keeps embedded fallback (Task 2 + 4 tests).
- **Discovery + manifest** → Task 1 (manifest), Task 3 (`Discover`, ordering, dotfile/non-dir tolerance).
- **API versioning** (absent = 1, frozen; newer tolerated) → Task 1 `EffectiveAPIVersion` + `CurrentAPIVersion`. (Newer-than-supported degrade/flag in Settings is display polish deferred to the emotion-hinting spec, which adds the per-mod Settings detail panel; v1 has no shims, matching the spec's "establishes the contract" intent.)
- **Config + Settings** → Task 5 (`ActiveMod`), Task 6 (MOD menu item), Task 7 (choices wired, auto-save via existing `commitMenu`).
- **Runtime switching** → Task 7 `reloadMod`: re-points text/quote/clip sources and rebuilds + re-warms the face cache on the render goroutine.
- **`CheckOverrides`/`RemoveOverrides` mod-aware** → Task 7 passes the active mod's root/paths; the functions are already path-parameterized, so no signature change is needed.

**Deferred (not in this plan, per spec):** animation engine + spec format, generalized lip-sync, deriving the LLM emotion vocabulary from the active mod's face set, the Settings "needs newer BMO" flag, and the modder tutorial.
