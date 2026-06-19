# Zip Mod Loading Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let BMO load a mod directly from a `mods/<id>.zip` archive at runtime, in addition to the existing `mods/<id>/` directory form, with the directory winning when both exist.

**Architecture:** Route every runtime mod read through `io/fs.FS`. `internal/mod` owns the dir-vs-zip decision: `os.DirFS` for directories, `archive/zip` (`*zip.ReadCloser`, which implements `fs.FS`) + `fs.Sub` for archives. `mod.Mod` gains an `FS fs.FS` field (populated by `Open`) and a `Close` method; the existing path accessors (`Root`, `PersonaPath`, …) stay for identity, logging, and the directory-only `cmd/generate-audio` build tool. Consumers (`mod.LoadManifest`, `internal/face`, `internal/config`, `internal/clips`) are refactored from path strings to `fs.FS`.

**Tech Stack:** Go, `io/fs`, `archive/zip`, `testing/fstest`. Pure-Go packages (`mod`, `face`, `config`, `clips`) test under `CGO_ENABLED=0`; the final `cmd/bmo-pak` integration build needs `CGO_ENABLED=1` (SDL).

---

## Background context for the implementer

- A mod is a folder (or, after this change, a `.zip`) under `<dataRoot>/BMO/mods/`. `mods/default` is a special overlay mod (per-asset fallback to embedded BMO assets) and is always a directory.
- "Self-contained" mod = not `default` AND ships at least one `faces/*.svg`. Self-contained mods do NOT inherit embedded faces; overlay mods do.
- A mod's contents are: `mod.json` (optional manifest), `persona.txt`, `voice.txt`, `quotes.txt` (all optional overrides), `faces/*.svg`, `audio/*.pcm`.
- "FS rooted at mod contents" means: from the returned `fs.FS`, `mod.json` is at path `"mod.json"`, faces at `"faces/<name>.svg"`, audio at `"audio/<name>.pcm"`, prompts at `"persona.txt"` etc.
- `release.sh` already produces `dist/mods/<id>.zip` whose contents live under a top-level `<id>/` folder (because it runs `zip -qr ../../dist/mods/<id>.zip <id>` from `examples/mods/`). Unzipping yields `<id>/`. This feature lets BMO read that same archive without unzipping.

### File map (what changes and why)

| File | Change |
|------|--------|
| `internal/mod/mod.go` | Add `FS fs.FS` field + `Open(logf)` / `Close()`; `FacesHasSVG` reads from `FS`. Keep path accessors. |
| `internal/mod/manifest.go` | `LoadManifest(fsys fs.FS)` reads `"mod.json"`. |
| `internal/mod/discover.go` | `Discover(modsRoot, logf)`: list dir + zip mods, dir-wins precedence + warning; read each manifest via a transient `Open`/`Close`. |
| `internal/face/library.go` | `NewLibrary`/`NewLibraryMode` take `fs.FS`; internal reads use `fs.ReadFile`/`fs.Stat`. |
| `internal/face/emotion.go` | `FaceNamesInFS(fsys fs.FS)` replaces `FaceNamesInDir(dir string)`. |
| `internal/config/prompts.go` | `CheckOverrides(fsys fs.FS)` reads via fs; add `LoadPromptFS(fsys, name, def)`. Keep `LoadPromptFile(path, def)` for generate-audio. |
| `cmd/bmo-pak/main.go` | Open active mod's FS, build face/clips/prompt/CheckOverrides from it, `Close` on mod-switch + exit; sub-FS for faces/audio. |
| `cmd/bmo-pak/idle_faces.go` | `modIdleFaces` uses `FaceNamesInFS`. |
| `internal/examplemods/evilbmo_test.go` | Adapt to fs.FS APIs; add zip regression test. |
| `docs/MODDING.md` | New "Distributing as a `.zip`" section. |

---

## Task 1: `LoadManifest` reads from an `fs.FS`

**Files:**
- Modify: `internal/mod/manifest.go`
- Test: `internal/mod/manifest_test.go` (create if absent)

- [ ] **Step 1: Write the failing test**

Add to `internal/mod/manifest_test.go`:

```go
package mod

import (
	"testing"
	"testing/fstest"
)

func TestLoadManifestFromFS(t *testing.T) {
	fsys := fstest.MapFS{
		"mod.json": {Data: []byte(`{"name":"Evil BMO","apiVersion":1}`)},
	}
	m := LoadManifest(fsys)
	if m.Name != "Evil BMO" {
		t.Errorf("Name = %q, want %q", m.Name, "Evil BMO")
	}
	if m.EffectiveAPIVersion() != 1 {
		t.Errorf("EffectiveAPIVersion = %d, want 1", m.EffectiveAPIVersion())
	}
}

func TestLoadManifestMissingReturnsZero(t *testing.T) {
	m := LoadManifest(fstest.MapFS{})
	if m.Name != "" {
		t.Errorf("Name = %q, want empty", m.Name)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `CGO_ENABLED=0 go test ./internal/mod/ -run TestLoadManifest -v`
Expected: FAIL — `LoadManifest` currently takes a `string` (compile error: cannot use `fstest.MapFS` as `string`).

- [ ] **Step 3: Change `LoadManifest` to take `fs.FS`**

In `internal/mod/manifest.go`, replace the imports and `LoadManifest`:

```go
import (
	"encoding/json"
	"io/fs"
)

// LoadManifest reads and parses mod.json from the root of fsys. A missing or
// malformed file yields the zero Manifest (mods need no manifest).
func LoadManifest(fsys fs.FS) Manifest {
	data, err := fs.ReadFile(fsys, "mod.json")
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

Remove the now-unused `os`/`path/filepath` imports from `manifest.go` if present.

- [ ] **Step 4: Run test to verify it passes**

Run: `CGO_ENABLED=0 go test ./internal/mod/ -run TestLoadManifest -v`
Expected: PASS. (Other `internal/mod` tests/build may still fail — fixed in Task 2/3.)

- [ ] **Step 5: Commit**

```bash
git add internal/mod/manifest.go internal/mod/manifest_test.go
git commit -m "refactor(mod): LoadManifest reads from fs.FS"
```

---

## Task 2: `Mod` gains `FS`, `Open`, `Close`; `FacesHasSVG` reads from `FS`

**Files:**
- Modify: `internal/mod/mod.go`
- Test: `internal/mod/mod_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `internal/mod/mod_test.go`:

```go
import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

// writeZip writes a .zip at path whose entries are keyed by their in-archive
// path (e.g. "evil/mod.json"). Returns path for convenience.
func writeZip(t *testing.T, path string, files map[string]string) string {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create %s: %v", name, err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatalf("zip write %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return path
}

func TestOpenDirectoryMod(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "faces"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "faces", "neutral.svg"), []byte("<svg/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := Mod{ID: "evil", Root: dir}
	if err := m.Open(nil); err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer m.Close()
	if !m.FacesHasSVG() {
		t.Error("FacesHasSVG() = false, want true")
	}
}

func TestOpenZipModWithWrappingFolder(t *testing.T) {
	dir := t.TempDir()
	zipPath := writeZip(t, filepath.Join(dir, "evil.zip"), map[string]string{
		"evil/mod.json":          `{"name":"Evil BMO"}`,
		"evil/faces/neutral.svg": "<svg/>",
	})
	m := Mod{ID: "evil", Root: zipPath}
	if err := m.Open(nil); err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer m.Close()
	if !m.FacesHasSVG() {
		t.Error("FacesHasSVG() = false, want true (faces under evil/)")
	}
	if got := LoadManifest(m.FS).Name; got != "Evil BMO" {
		t.Errorf("manifest Name = %q, want %q", got, "Evil BMO")
	}
}

func TestOpenZipModRootFallback(t *testing.T) {
	dir := t.TempDir()
	var warned bool
	zipPath := writeZip(t, filepath.Join(dir, "evil.zip"), map[string]string{
		"mod.json":          `{"name":"Evil BMO"}`,
		"faces/neutral.svg": "<svg/>",
	})
	m := Mod{ID: "evil", Root: zipPath}
	if err := m.Open(func(string, ...any) { warned = true }); err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer m.Close()
	if !m.FacesHasSVG() {
		t.Error("FacesHasSVG() = false, want true (faces at zip root)")
	}
	if !warned {
		t.Error("expected a warning for missing wrapping folder")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `CGO_ENABLED=0 go test ./internal/mod/ -run TestOpen -v`
Expected: FAIL — `m.Open` undefined, `m.FS` undefined.

- [ ] **Step 3: Implement `FS`, `Open`, `Close`, and FS-based `FacesHasSVG`**

Replace the contents of `internal/mod/mod.go` with:

```go
package mod

import (
	"archive/zip"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// DefaultID is the overlay mod that falls back per-asset to embedded BMO.
const DefaultID = "default"

// Mod is a selectable character under mods/: a directory or a .zip archive.
type Mod struct {
	ID        string
	Root      string // mods/<id> or mods/<id>.zip — identity, logging, dir-only tools
	Manifest  Manifest
	IsDefault bool

	// FS is rooted at the mod's contents (mod.json at root, faces/ and audio/
	// as subtrees). Populated by Open; nil until then.
	FS fs.FS

	closer func() error
}

// DisplayName is the manifest name if set, else a friendly default.
func (m Mod) DisplayName() string {
	if name := strings.TrimSpace(m.Manifest.Name); name != "" {
		return name
	}
	if m.IsDefault {
		return "BMO (Default)"
	}
	return m.ID
}

// Path accessors operate on Root. They are valid only for directory mods and
// are used by cmd/generate-audio (a directory-only build tool) and for logging.
func (m Mod) PersonaPath() string { return filepath.Join(m.Root, "persona.txt") }
func (m Mod) VoicePath() string   { return filepath.Join(m.Root, "voice.txt") }
func (m Mod) QuotesPath() string  { return filepath.Join(m.Root, "quotes.txt") }
func (m Mod) FacesDir() string    { return filepath.Join(m.Root, "faces") }
func (m Mod) AudioDir() string    { return filepath.Join(m.Root, "audio") }

// Open populates m.FS from Root. Directories use os.DirFS; .zip archives are
// opened and rooted at their top-level <id>/ folder when present, otherwise at
// the archive root (logging a warning via logf, which may be nil). Callers must
// Close the mod when done so zip file descriptors are released.
func (m *Mod) Open(logf func(format string, args ...any)) error {
	if strings.HasSuffix(m.Root, ".zip") {
		zr, err := zip.OpenReader(m.Root)
		if err != nil {
			return err
		}
		var fsys fs.FS = zr
		if info, err := fs.Stat(zr, m.ID); err == nil && info.IsDir() {
			if sub, err := fs.Sub(zr, m.ID); err == nil {
				fsys = sub
			}
		} else if logf != nil {
			logf("mod %q: zip has no top-level %q/ folder; reading from archive root", m.ID, m.ID)
		}
		m.FS = fsys
		m.closer = zr.Close
		return nil
	}
	m.FS = os.DirFS(m.Root)
	m.closer = nil
	return nil
}

// Close releases the mod's source (a no-op for directories).
func (m *Mod) Close() error {
	if m.closer != nil {
		err := m.closer()
		m.closer = nil
		return err
	}
	return nil
}

// FacesHasSVG reports whether the mod ships at least one faces/*.svg. Requires
// Open to have populated FS.
func (m Mod) FacesHasSVG() bool {
	if m.FS == nil {
		return false
	}
	entries, err := fs.ReadDir(m.FS, "faces")
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

// SelfContained reports whether the mod ships its own faces and is not the
// overlay default (so it must not inherit embedded faces/animations).
func (m Mod) SelfContained() bool {
	return !m.IsDefault && m.FacesHasSVG()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `CGO_ENABLED=0 go test ./internal/mod/ -run 'TestOpen|TestLoadManifest' -v`
Expected: PASS for the new tests. (Existing `Discover` tests may fail to compile — fixed in Task 3.)

- [ ] **Step 5: Commit**

```bash
git add internal/mod/mod.go internal/mod/mod_test.go
git commit -m "feat(mod): Mod.FS via Open/Close; FacesHasSVG reads from FS"
```

---

## Task 3: `Discover` lists directory + zip mods with dir-wins precedence

**Files:**
- Modify: `internal/mod/discover.go`
- Test: `internal/mod/discover_test.go` (create if absent)

- [ ] **Step 1: Write the failing tests**

Add to `internal/mod/discover_test.go` (reuse `writeZip` from `mod_test.go`, same package):

```go
package mod

import (
	"os"
	"path/filepath"
	"testing"
)

func ids(mods []Mod) []string {
	out := make([]string, len(mods))
	for i, m := range mods {
		out[i] = m.ID
	}
	return out
}

func TestDiscoverFindsZipMod(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "default"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeZip(t, filepath.Join(root, "evil.zip"), map[string]string{
		"evil/mod.json": `{"name":"Evil BMO"}`,
	})
	mods := Discover(root, nil)
	if got := ids(mods); len(got) != 2 || got[0] != "default" || got[1] != "evil" {
		t.Fatalf("ids = %v, want [default evil]", got)
	}
	if mods[1].Manifest.Name != "Evil BMO" {
		t.Errorf("zip manifest Name = %q, want %q", mods[1].Manifest.Name, "Evil BMO")
	}
	if mods[1].Root != filepath.Join(root, "evil.zip") {
		t.Errorf("Root = %q, want the .zip path", mods[1].Root)
	}
}

func TestDiscoverDirectoryWinsOverZip(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "default"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "evil"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeZip(t, filepath.Join(root, "evil.zip"), map[string]string{"evil/mod.json": `{}`})

	var warned bool
	mods := Discover(root, func(string, ...any) { warned = true })

	var evil *Mod
	for i := range mods {
		if mods[i].ID == "evil" {
			evil = &mods[i]
		}
	}
	if evil == nil {
		t.Fatal("evil mod not found")
	}
	if evil.Root != filepath.Join(root, "evil") {
		t.Errorf("Root = %q, want the directory (dir wins)", evil.Root)
	}
	if !warned {
		t.Error("expected a warning when both directory and .zip exist")
	}
	// id must appear only once.
	count := 0
	for _, id := range ids(mods) {
		if id == "evil" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("evil appears %d times, want 1", count)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `CGO_ENABLED=0 go test ./internal/mod/ -run TestDiscover -v`
Expected: FAIL — `Discover` takes one arg today (compile error: too many arguments).

- [ ] **Step 3: Rewrite `Discover` to add `logf`, zip support, and precedence**

Replace `internal/mod/discover.go` with:

```go
// Package mod discovers mods: subfolders or .zip archives of mods/ that
// customize persona, voice, quotes, faces, and audio. mods/default has overlay
// semantics (per-asset fallback to BMO); other entries are self-contained
// characters.
package mod

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Discover returns selectable mods under modsRoot. The synthetic "default" is
// always first. Remaining entries are directories and <id>.zip archives, by id.
// When both a directory and a .zip exist for the same id, the directory wins
// and a warning is logged via logf (which may be nil). Non-directory,
// dot-prefixed, and non-.zip files are ignored.
func Discover(modsRoot string, logf func(format string, args ...any)) []Mod {
	def := Mod{
		ID:        DefaultID,
		Root:      filepath.Join(modsRoot, DefaultID),
		IsDefault: true,
	}
	def.Manifest = manifestFor(def.Root, def.ID, def.IsDefault)
	out := []Mod{def}

	entries, err := os.ReadDir(modsRoot)
	if err != nil {
		return out
	}

	// roots maps id -> Root path; directories take precedence over .zip files.
	roots := map[string]string{}
	isDir := map[string]bool{}
	var ids []string

	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if e.IsDir() {
			if name == DefaultID {
				continue
			}
			if _, seen := roots[name]; !seen {
				ids = append(ids, name)
			}
			roots[name] = filepath.Join(modsRoot, name)
			isDir[name] = true
			continue
		}
		if !strings.EqualFold(filepath.Ext(name), ".zip") {
			continue
		}
		id := strings.TrimSuffix(name, filepath.Ext(name))
		if id == DefaultID || id == "" {
			continue
		}
		if isDir[id] {
			if logf != nil {
				logf("mod %q: both directory and .zip present; using directory", id)
			}
			continue
		}
		if _, seen := roots[id]; !seen {
			ids = append(ids, id)
		}
		roots[id] = filepath.Join(modsRoot, name)
	}
	// Note: os.ReadDir returns entries sorted by name, and "<id>" always sorts
	// before "<id>.zip", so a directory is always seen before its same-named
	// archive — the isDir check above is sufficient for dir-wins precedence.

	sort.Strings(ids)
	for _, id := range ids {
		root := roots[id]
		out = append(out, Mod{
			ID:       id,
			Root:     root,
			Manifest: manifestFor(root, id, false),
		})
	}
	return out
}

// manifestFor reads a mod's manifest by transiently opening its source (so zip
// file descriptors are not held open across discovery). Returns the zero
// Manifest on any error.
func manifestFor(root, id string, isDefault bool) Manifest {
	tmp := Mod{Root: root, ID: id, IsDefault: isDefault}
	if err := tmp.Open(nil); err != nil {
		return Manifest{}
	}
	defer tmp.Close()
	return LoadManifest(tmp.FS)
}

// Active returns the mod in mods matching id, or the default entry (index 0).
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
		return mods[0]
	}
	return Mod{ID: DefaultID, IsDefault: true}
}
```

> Note: the directory `Open` on a default `mods/default` that doesn't exist still yields a usable `os.DirFS`; `LoadManifest` returns the zero manifest when `mod.json` is absent, matching today's behavior.

- [ ] **Step 4: Run tests to verify they pass**

Run: `CGO_ENABLED=0 go test ./internal/mod/ -v`
Expected: PASS for all `internal/mod` tests. If pre-existing `discover_test.go` cases called `Discover(root)` with one arg, update them to `Discover(root, nil)`.

- [ ] **Step 5: Commit**

```bash
git add internal/mod/discover.go internal/mod/discover_test.go
git commit -m "feat(mod): Discover lists zip mods; directory wins over .zip"
```

---

## Task 4: `face` library and `FaceNamesInFS` read from `fs.FS`

**Files:**
- Modify: `internal/face/library.go`, `internal/face/emotion.go`, `internal/face/cache.go`
- Test: `internal/face/library_test.go` (add cases)

- [ ] **Step 1: Write the failing test**

Add to `internal/face/library_test.go`:

```go
import (
	"testing"
	"testing/fstest"
)

func TestLibraryReadsOverrideFromFS(t *testing.T) {
	fsys := fstest.MapFS{
		"neutral.svg": {Data: []byte("<svg id=\"override\"/>")},
	}
	lib := NewLibraryMode(fsys, true)
	data, fromDisk := lib.Bytes("neutral")
	if !fromDisk {
		t.Fatal("expected override to come from fsys")
	}
	if string(data) != "<svg id=\"override\"/>" {
		t.Errorf("got %q", data)
	}
}

func TestFaceNamesInFS(t *testing.T) {
	fsys := fstest.MapFS{
		"neutral.svg": {Data: []byte("<svg/>")},
		"angry.svg":   {Data: []byte("<svg/>")},
		"notes.txt":   {Data: []byte("ignore me")},
	}
	got := FaceNamesInFS(fsys)
	if len(got) != 2 || got[0] != "angry" || got[1] != "neutral" {
		t.Errorf("FaceNamesInFS = %v, want [angry neutral]", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `CGO_ENABLED=0 go test ./internal/face/ -run 'TestLibraryReadsOverrideFromFS|TestFaceNamesInFS' -v`
Expected: FAIL — `NewLibraryMode` takes a `string`; `FaceNamesInFS` undefined.

- [ ] **Step 3: Convert the library to `fs.FS`**

In `internal/face/library.go`: change the import block and the `Library` struct + constructors + read sites. The `fsys` field replaces `dir`; a nil `fsys` means "no overrides, embedded only".

Replace the import block:

```go
import (
	"bytes"
	"io/fs"
	"regexp"
	"strings"
)
```

Replace the struct + constructors:

```go
type Library struct {
	fsys          fs.FS // rooted at the mod's faces/; nil = embedded only
	selfContained bool
	logf          func(string, ...any)
}

// NewLibrary builds an overlay library: on-disk faces override embedded
// defaults per file. fsys may be nil (embedded only).
func NewLibrary(fsys fs.FS) *Library {
	return NewLibraryMode(fsys, false)
}

// NewLibraryMode builds a library. When selfContained is true, embedded
// fallback is disabled: only fsys is consulted and a missing expression folds
// to neutral.svg.
func NewLibraryMode(fsys fs.FS, selfContained bool) *Library {
	return &Library{fsys: fsys, selfContained: selfContained, logf: func(string, ...any) {}}
}
```

In `Bytes`, replace each `if l.dir != "" { ... os.ReadFile(filepath.Join(l.dir, name+".svg")) ... }` block with the `fsys` equivalent. The override-read section becomes:

```go
	if l.fsys != nil {
		for _, name := range names {
			if !fileNameRe.MatchString(name) {
				continue
			}
			data, err := fs.ReadFile(l.fsys, name+".svg")
			if err != nil {
				continue
			}
			if len(bytes.TrimSpace(data)) == 0 {
				l.logf("face: override %s.svg is blank; using default", name)
				continue
			}
			return data, true
		}
	}
```

And the self-contained neutral fallback block becomes:

```go
	if l.selfContained {
		if canonical != ExprNeutral && l.fsys != nil {
			data, err := fs.ReadFile(l.fsys, ExprNeutral+".svg")
			if err == nil && len(bytes.TrimSpace(data)) > 0 {
				return data, true
			}
		}
		return nil, false
	}
```

In `Resolve`, replace the on-disk probe:

```go
func (l *Library) Resolve(expr string) string {
	raw := strings.ToLower(strings.TrimSpace(expr))
	if l.fsys != nil && fileNameRe.MatchString(raw) {
		if _, err := fs.Stat(l.fsys, raw+".svg"); err == nil {
			return raw
		}
	}
	return Canonical(raw)
}
```

In `rawBytes`, replace the on-disk read:

```go
	if l.fsys != nil {
		if data, err := fs.ReadFile(l.fsys, name+".svg"); err == nil && len(bytes.TrimSpace(data)) > 0 {
			return data, true
		}
	}
```

- [ ] **Step 4: Convert `FaceNamesInFS` in `emotion.go`**

In `internal/face/emotion.go`, change the import block to use `io/fs` (drop `os`, `path/filepath` if now unused — keep `sort`, `strings`):

```go
import (
	"io/fs"
	"sort"
	"strings"
)
```

Replace `FaceNamesInDir` and `faceNamesInDir`:

```go
// FaceNamesInFS lists every face a mod ships: base names of *.svg files at the
// root of fsys (which is rooted at the mod's faces/), sorted, INCLUDING
// functional/idle faces.
func FaceNamesInFS(fsys fs.FS) []string {
	return faceNamesInFS(fsys, false)
}

// EmotionFaceNamesInFS lists only the emotion faces (functional/idle faces
// excluded). Replaces EmotionFaceNamesInDir.
func EmotionFaceNamesInFS(fsys fs.FS) []string {
	return faceNamesInFS(fsys, true)
}

func faceNamesInFS(fsys fs.FS, excludeFunctional bool) []string {
	if fsys == nil {
		return nil
	}
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		dot := strings.LastIndex(name, ".")
		if dot < 0 || !strings.EqualFold(name[dot:], ".svg") {
			continue
		}
		base := strings.ToLower(name[:dot])
		if !fileNameRe.MatchString(base) {
			continue
		}
		if excludeFunctional && isFunctional(base) {
			continue
		}
		out = append(out, base)
	}
	sort.Strings(out)
	return out
}
```

Also update the one in-package consumer that reaches into the library's field: `internal/face/cache.go:86` currently does `for _, name := range EmotionFaceNamesInDir(c.lib.dir)`. Change it to read the library's new `fsys` field:

```go
	for _, name := range EmotionFaceNamesInFS(c.lib.fsys) {
```

> Confirm there are no other `*InDir`/`c.lib.dir` references left in the `face` package before compiling: `grep -rn "InDir\|\.dir\b" internal/face/*.go`. All face-package references to the old `dir` string and `*InDir` funcs must be gone.

- [ ] **Step 5: Run tests to verify they pass**

Run: `CGO_ENABLED=0 go test ./internal/face/ -v`
Expected: PASS. Update any existing face tests that constructed `NewLibrary("dir")` / `NewLibraryMode("dir", ...)` to pass an `fs.FS` — `os.DirFS(dir)` for a temp dir, or `fstest.MapFS{...}` for inline SVGs. For the old "no dir" case use `NewLibrary(nil)`.

- [ ] **Step 6: Commit**

```bash
git add internal/face/library.go internal/face/emotion.go internal/face/cache.go internal/face/library_test.go
git commit -m "refactor(face): Library and face listing read from fs.FS"
```

---

## Task 5: `config.CheckOverrides` and `LoadPromptFS` read from `fs.FS`

**Files:**
- Modify: `internal/config/prompts.go`
- Test: `internal/config/prompts_test.go` (adapt + add)

- [ ] **Step 1: Write the failing tests**

Add to `internal/config/prompts_test.go`:

```go
import (
	"testing"
	"testing/fstest"
)

func TestCheckOverridesFSValid(t *testing.T) {
	fsys := fstest.MapFS{
		"persona.txt":      {Data: []byte("be evil")},
		"faces/neutral.svg": {Data: []byte("<svg></svg>")},
	}
	if errs := CheckOverrides(fsys); len(errs) != 0 {
		t.Errorf("CheckOverrides = %v, want none", errs)
	}
}

func TestCheckOverridesFSBlankPersona(t *testing.T) {
	fsys := fstest.MapFS{"persona.txt": {Data: []byte("   ")}}
	if errs := CheckOverrides(fsys); len(errs) == 0 {
		t.Error("expected an error for blank persona.txt")
	}
}

func TestCheckOverridesFSInvalidSVG(t *testing.T) {
	fsys := fstest.MapFS{"faces/x.svg": {Data: []byte("<svg><unclosed>")}}
	if errs := CheckOverrides(fsys); len(errs) == 0 {
		t.Error("expected an error for invalid SVG")
	}
}

func TestLoadPromptFSOverride(t *testing.T) {
	fsys := fstest.MapFS{"persona.txt": {Data: []byte("custom")}}
	if got := LoadPromptFS(fsys, "persona.txt", "default"); got != "custom" {
		t.Errorf("LoadPromptFS = %q, want %q", got, "custom")
	}
}

func TestLoadPromptFSFallback(t *testing.T) {
	if got := LoadPromptFS(fstest.MapFS{}, "persona.txt", "default"); got != "default" {
		t.Errorf("LoadPromptFS = %q, want %q", got, "default")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `CGO_ENABLED=0 go test ./internal/config/ -run 'TestCheckOverridesFS|TestLoadPromptFS' -v`
Expected: FAIL — `CheckOverrides` takes a `string`; `LoadPromptFS` undefined.

- [ ] **Step 3: Add `LoadPromptFS` and convert `CheckOverrides`**

In `internal/config/prompts.go`, add `io/fs` to imports. Keep `LoadPromptFile(path, def string)` (used by `cmd/generate-audio`). Add:

```go
// LoadPromptFS returns the trimmed content of name within fsys if present and
// non-blank, else the built-in def. Mirrors LoadPromptFile for fs.FS sources.
func LoadPromptFS(fsys fs.FS, name, def string) string {
	if fsys != nil {
		if data, err := fs.ReadFile(fsys, name); err == nil {
			if content := strings.TrimSpace(string(data)); content != "" {
				return content
			}
		}
	}
	return strings.TrimSpace(def)
}
```

Replace `CheckOverrides` (keep `isValidXML` unchanged):

```go
// CheckOverrides validates a mod's override files in fsys, returning any
// problems: persona.txt / voice.txt / quotes.txt present-but-blank, and any
// faces/*.svg that is not valid XML. A missing file is not an error.
func CheckOverrides(fsys fs.FS) []error {
	var errs []error
	checkText := func(name, label string) {
		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			return // absent is fine
		}
		if strings.TrimSpace(string(data)) == "" {
			errs = append(errs, fmt.Errorf("%s exists but is blank", label))
		}
	}
	checkText("persona.txt", "persona.txt")
	checkText("voice.txt", "voice.txt")
	checkText("quotes.txt", "quotes.txt")

	entries, err := fs.ReadDir(fsys, "faces")
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, fmt.Errorf("faces dir: %w", err))
		}
		return errs
	}
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".svg") {
			continue
		}
		data, err := fs.ReadFile(fsys, "faces/"+e.Name())
		if err != nil {
			errs = append(errs, fmt.Errorf("faces/%s: %w", e.Name(), err))
			continue
		}
		if !isValidXML(data) {
			errs = append(errs, fmt.Errorf("faces/%s: not valid XML", e.Name()))
		}
	}
	return errs
}
```

> Imports: `CheckOverrides` now needs `io/fs` and still uses `errors`, `fmt`, `filepath`, `strings`. The `os` import remains required by `LoadPromptFile`/`RemoveOverrides`. `bytes`/`encoding/xml`/`io` remain for `isValidXML`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `CGO_ENABLED=0 go test ./internal/config/ -v`
Expected: PASS. Adapt any pre-existing `CheckOverrides`-based tests (the ones in `prompts_test.go` that wrote files to a temp dir): wrap the temp dir with `os.DirFS(dir)` when calling `CheckOverrides`.

- [ ] **Step 5: Commit**

```bash
git add internal/config/prompts.go internal/config/prompts_test.go
git commit -m "refactor(config): CheckOverrides + LoadPromptFS read from fs.FS"
```

---

## Task 6: `clips.Library` reads from `fs.FS`

**Files:**
- Modify: `internal/clips/library.go`
- Test: `internal/clips/library_test.go` (adapt)

- [ ] **Step 1: Write the failing test**

Add to `internal/clips/library_test.go`:

```go
import (
	"testing"
	"testing/fstest"
)

func TestLibraryLoadsOverrideFromFS(t *testing.T) {
	fsys := fstest.MapFS{
		"hello.pcm": {Data: []byte("PCMDATA-1234")},
	}
	lib := NewLibrary(fsys)
	if got := lib.Load("hello"); string(got) != "PCMDATA-1234" {
		t.Errorf("Load = %q, want override bytes", got)
	}
}

func TestLibraryNilFSFallsBackToEmbedded(t *testing.T) {
	lib := NewLibrary(nil)
	if got := lib.Load("hello"); got == nil {
		t.Error("Load(hello) = nil, want embedded default")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `CGO_ENABLED=0 go test ./internal/clips/ -run TestLibrary -v`
Expected: FAIL — `NewLibrary` takes a `string`.

- [ ] **Step 3: Convert `clips.Library` to `fs.FS`**

Replace `internal/clips/library.go`:

```go
package clips

import "io/fs"

// Library loads pre-recorded PCM clips, preferring on-disk overrides in the
// mod's audio/ filesystem over the embedded defaults.
type Library struct {
	fsys fs.FS // rooted at the mod's audio/; nil = embedded only
}

func NewLibrary(fsys fs.FS) *Library {
	return &Library{fsys: fsys}
}

// Load returns the PCM bytes for the named clip. It checks <name>.pcm in fsys
// first; if that file is absent or empty it falls back to the embedded asset.
// Returns nil if the clip is not found in either location.
func (l *Library) Load(name string) []byte {
	if l != nil && l.fsys != nil {
		if data, err := fs.ReadFile(l.fsys, name+".pcm"); err == nil && len(data) > 0 {
			return data
		}
	}
	data, err := embeddedAssets.ReadFile("assets/audio/" + name + ".pcm")
	if err != nil || len(data) == 0 {
		return nil
	}
	return data
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `CGO_ENABLED=0 go test ./internal/clips/ -v`
Expected: PASS. Adapt pre-existing tests that called `NewLibrary(tmpDir)` to `NewLibrary(os.DirFS(filepath.Join(tmpDir, "audio")))`, or build the override via `fstest.MapFS`. (The old code joined `homeDir/audio/<name>.pcm`; the new `fsys` is rooted at `audio/`.)

- [ ] **Step 5: Commit**

```bash
git add internal/clips/library.go internal/clips/library_test.go
git commit -m "refactor(clips): Library reads from fs.FS rooted at audio/"
```

---

## Task 7: Wire the mod `FS` through `cmd/bmo-pak`

**Files:**
- Modify: `cmd/bmo-pak/main.go`, `cmd/bmo-pak/idle_faces.go`, `cmd/generate-audio/main.go`

This task has no new unit tests (it is integration glue verified by the build + the full suite + Task 9's example regression). Compile and run the suite after each sub-change.

> **AUTHORITATIVE (verified against the real `main.go` 2026-06-19) — follow this; it supersedes the sub-steps below where they differ.**
>
> The prompt/emotion closures **already capture `activeMod` by reference** and `reloadMod` **already reassigns `activeMod` and rebuilds objects** (`face.NewCache`, `clips.NewPlayer`, `buildAnimationEngine`) — there are NO `SetLibrary`/`SetPersona` setters; do not invent any. The change is: open `activeMod.FS`, and make every reader use the mod's `fs.FS` (compute `fs.Sub(activeMod.FS, "faces")` / `"audio"` at each site). Keep `personaPath`/`voicePath`/`quotesPath` — they're still used by `config.RemoveOverrides` (settings reset) and reassigned in `reloadMod`. `FacesDir()`/`AudioDir()` accessors become unused at runtime but may stay (harmless).
>
> **`main.go` edits (exact sites):**
> 1. Add `"io/fs"` to imports.
> 2. `mods := mod.Discover(modsRoot, nil)` (line ~100; logger isn't ready yet, so `nil` logf — the dir-vs-zip precedence warning is silent on first boot, which is acceptable).
> 3. Right after `activeMod := mod.Active(mods, cfg.ActiveMod)` (line ~105), open it:
>    ```go
>    if err := activeMod.Open(nil); err != nil {
>        activeMod = mod.Active(mods, mod.DefaultID)
>        _ = activeMod.Open(nil)
>    }
>    defer func() { _ = activeMod.Close() }()
>    ```
>    (Open before the prompt loads at 114-115, since they now read from `activeMod.FS`.)
> 4. Lines 114-115 — initial prompt strings via FS:
>    ```go
>    personaPrompt := config.LoadPromptFS(activeMod.FS, "persona.txt", config.DefaultSystemPrompt)
>    voicePrompt := config.LoadPromptFS(activeMod.FS, "voice.txt", config.DefaultTTSInstructions)
>    ```
>    Keep `personaPath`/`voicePath`/`quotesPath` (lines 111-113) as-is — needed by `RemoveOverrides` (line 162) and `reloadMod`.
> 5. quotesFn (line 258): `config.LoadPromptFS(activeMod.FS, "quotes.txt", config.DefaultQuotes)`.
> 6. TTS-instructions source (line 306): `return config.LoadPromptFS(activeMod.FS, "voice.txt", config.DefaultTTSInstructions)`.
> 7. System-prompt source (lines 308-310): `config.LoadPromptFS(activeMod.FS, "persona.txt", config.DefaultSystemPrompt)`.
> 8. Emotion-vocab source (line 325): `disk := face.EmotionFaceNamesInFS(modFacesSub(activeMod))` where `modFacesSub` is a tiny local helper `func(m mod.Mod) fs.FS { s, _ := fs.Sub(m.FS, "faces"); return s }` (define once near the top of `run`, or inline `fs.Sub`).
> 9. clipLib (line 276): `clips.NewLibrary(modAudioSub(activeMod))` (analogous `"audio"` sub helper).
> 10. timeout/error preload (lines 329-330): build one `clips.NewLibrary(modAudioSub(activeMod))` and call `.Load("timeout")`/`.Load("error")` on it (don't construct two).
> 11. faceLib (line 346): `face.NewLibraryMode(modFacesSub(activeMod), activeMod.SelfContained())`.
> 12. gallery enumerator (line 526): `for _, n := range face.FaceNamesInFS(modFacesSub(activeMod))`.
> 13. `reloadMod` (lines 377-413): keep the path reassignments (380-382); after `activeMod = active`, do `_ = activeMod.Close()` on the PREVIOUS mod BEFORE reassigning — i.e. close the old, then `active.Open(logger.Warnf)` (fallback to default on error), then `activeMod = active`. Replace `active.FacesDir()`→`modFacesSub(active)` (lines 384, 391 via newLib) and `active.AudioDir()`→`modAudioSub(active)` (line 402). `modIdleFaces(active)` stays (helper computes its own sub — see idle_faces.go below).
>     - Lifecycle ordering in reloadMod: `_ = activeMod.Close()` (old) → `if err := active.Open(logger.Warnf); err != nil { active = mod.Active(mods, mod.DefaultID); _ = active.Open(logger.Warnf) }` → `activeMod = active` → recompute subs/rebuild.
>
> **`idle_faces.go`:** change `modIdleFaces(m mod.Mod)` to compute its own faces sub-FS (keeps both call sites unchanged):
> ```go
> func modIdleFaces(m mod.Mod) map[assistant.Expression]bool {
>     if !m.SelfContained() { return nil }
>     facesFS, err := fs.Sub(m.FS, "faces")
>     if err != nil { return nil }
>     names := face.FaceNamesInFS(facesFS)
>     if len(names) == 0 { return nil }
>     set := make(map[assistant.Expression]bool, len(names))
>     for _, n := range names { set[assistant.Expression(n)] = true }
>     return set
> }
> ```
> (Add `"io/fs"` import; drop `face` only if unused — it's still used.)
>
> **`cmd/generate-audio/main.go`:** update the `mod.Discover(...)` call to pass the new `logf` arg: `mod.Discover(modsRoot, nil)`. It keeps using `activeMod.VoicePath()`/`PersonaPath()` with `config.LoadPromptFile` (directory-only tool) — unchanged otherwise.
>
> **Verify after:** `CGO_ENABLED=1 go build ./...` && `CGO_ENABLED=1 go test ./...` && `golangci-lint run ./...`.

The sub-steps below are the original (pre-verification) sketch; defer to the AUTHORITATIVE block above wherever they conflict (especially the `reloadMod` setters, which do not exist).

- [ ] **Step 1: Add imports and open the active mod's FS at startup**

In `cmd/bmo-pak/main.go`, add `"io/fs"` to the import block.

Find the discovery block (around line 100-115):

```go
	mods := mod.Discover(modsRoot)
	activeMod := mod.Active(mods, cfg.ActiveMod)

	personaPath := activeMod.PersonaPath()
	voicePath := activeMod.VoicePath()
	quotesPath := activeMod.QuotesPath()
	personaPrompt := config.LoadPromptFile(personaPath, config.DefaultSystemPrompt)
	voicePrompt := config.LoadPromptFile(voicePath, config.DefaultTTSInstructions)
```

Replace with (note `Discover` now takes `logger`-style `logf`, but the logger isn't built yet here — pass `nil` at discovery, then open with a deferred warning via `log.Printf` since logger init follows; simplest: open after logger is ready). Restructure as:

```go
	mods := mod.Discover(modsRoot, nil)
	activeMod := mod.Active(mods, cfg.ActiveMod)
```

Then, immediately AFTER the logger is constructed (after `defer logger.Close()` / `log.SetOutput(...)` around line 122), insert:

```go
	// Open the active mod's filesystem (directory or .zip). Re-opened by
	// reloadMod on mod-switch; closed on exit and before each re-open.
	if err := activeMod.Open(logger.Warnf); err != nil {
		logger.Warnf("open mod %q: %v; falling back to default", activeMod.ID, err)
		activeMod = mod.Active(mods, mod.DefaultID)
		_ = activeMod.Open(logger.Warnf)
	}
	defer func() { _ = activeMod.Close() }()

	modFacesFS, _ := fs.Sub(activeMod.FS, "faces")
	modAudioFS, _ := fs.Sub(activeMod.FS, "audio")

	personaPrompt := config.LoadPromptFS(activeMod.FS, "persona.txt", config.DefaultSystemPrompt)
	voicePrompt := config.LoadPromptFS(activeMod.FS, "voice.txt", config.DefaultTTSInstructions)
```

Delete the now-unused `personaPath`/`voicePath`/`quotesPath` locals here (they were only inputs to `LoadPromptFile`). If `quotesPath`/`personaPath`/`voicePath` are referenced later (e.g. proactive quotes, reloadMod), convert those sites too — see Steps 3-4.

> `fs.Sub` only errors on an invalid path; `"faces"`/`"audio"` are always valid, so the ignored error is safe. For a directory mod whose `faces/` is absent, the sub-FS simply errors on read and the face library falls back to embedded — matching today's behavior.

- [ ] **Step 2: Build the clip and face libraries from the sub-filesystems**

Around line 276, replace:

```go
		clipLib := clips.NewLibrary(activeMod.AudioDir())
```
with:
```go
		clipLib := clips.NewLibrary(modAudioFS)
```

Around line 346, replace:

```go
	faceLib := face.NewLibraryMode(activeMod.FacesDir(), activeMod.SelfContained())
```
with:
```go
	faceLib := face.NewLibraryMode(modFacesFS, activeMod.SelfContained())
```

- [ ] **Step 3: Convert the proactive-quotes load**

Find where quotes are loaded (the `quotesPath` consumer; search serena `find_referencing_symbols` on `QuotesPath`, or grep `quotesPath` / `DefaultQuotes` in `main.go`). Replace the `config.LoadPromptFile(quotesPath, config.DefaultQuotes)` (or equivalent) with:

```go
	quotesText := config.LoadPromptFS(activeMod.FS, "quotes.txt", config.DefaultQuotes)
```

Keep the variable name the downstream code already uses; only the source changes from path to FS.

- [ ] **Step 4: Update `reloadMod` to re-open the FS and rebuild sub-filesystems**

In `reloadMod` (around line 368-413), after `active := mod.Active(mods, id)` and `activeMod = active`, close the previous FS and open the new one, then rebuild the prompt strings and sub-filesystems and re-point the dependent libraries. Replace the body that set `personaPath`/`voicePath` with:

```go
	reloadMod := func(id string) {
		_ = activeMod.Close()
		active := mod.Active(mods, id)
		if err := active.Open(logger.Warnf); err != nil {
			logger.Warnf("open mod %q: %v; keeping default", id, err)
			active = mod.Active(mods, mod.DefaultID)
			_ = active.Open(logger.Warnf)
		}
		activeMod = active

		modFacesFS, _ = fs.Sub(activeMod.FS, "faces")
		modAudioFS, _ = fs.Sub(activeMod.FS, "audio")

		personaPrompt = config.LoadPromptFS(activeMod.FS, "persona.txt", config.DefaultSystemPrompt)
		voicePrompt = config.LoadPromptFS(activeMod.FS, "voice.txt", config.DefaultTTSInstructions)

		// Re-point the prompt-consuming pipeline.
		if audioPipeline != nil {
			audioPipeline.SetPersona(personaPrompt)       // existing setter; keep current name
			audioPipeline.SetTTSInstructions(voicePrompt)
		}

		// Rebuild face + clip libraries from the new filesystems.
		faceLib = face.NewLibraryMode(modFacesFS, activeMod.SelfContained())
		faceLib.SetLogf(logger.Warnf)
		faceCache.SetLibrary(faceLib)                     // existing rebuild path; keep current call
		animEngine = buildAnimationEngine(faceLib, activeMod, logger.Warnf)
		screen.SetAnimations(animEngine)
		go faceCache.Warm(screen.Size())

		if clipPlayer != nil {
			clipPlayer.SetLibrary(clips.NewLibrary(modAudioFS)) // existing rebuild path; keep current call
		}
		if scheduler != nil {
			scheduler.SetAvailable(modIdleFaces(active))
		}
		logger.Infof("switched to mod %q (self-contained=%t)", active.ID, active.SelfContained())
	}
```

> IMPORTANT: the existing `reloadMod` already rebuilds the face library / cache / engine and clip library somehow (it re-points prompt paths and re-warms today). Preserve whatever rebuild mechanism it currently uses — only swap the *sources* from `activeMod.FacesDir()` / `activeMod.AudioDir()` / path-based prompts to `modFacesFS` / `modAudioFS` / `LoadPromptFS`. Read the current `reloadMod` body first (serena `find_symbol reloadMod`) and adapt the exact setter names; the setter names above (`SetPersona`, `SetLibrary`) are placeholders for whatever the current code calls. Do NOT invent new setters — if the current code rebuilds objects rather than mutating them, follow that pattern.

- [ ] **Step 5: Update `modIdleFaces` and the gallery enumerator to use the FS**

In `cmd/bmo-pak/idle_faces.go`, change `modIdleFaces` to take the faces FS instead of reading `m.FacesDir()`:

```go
package main

import (
	"io/fs"

	"github.com/carroarmato0/nextui-bmo/internal/assistant"
	"github.com/carroarmato0/nextui-bmo/internal/face"
	"github.com/carroarmato0/nextui-bmo/internal/mod"
)

// modIdleFaces returns the set of expressions the idle scheduler may use for a
// self-contained mod. facesFS is the mod's faces/ filesystem. Returns nil for
// overlay mods (which use the embedded idle set).
func modIdleFaces(m mod.Mod, facesFS fs.FS) map[assistant.Expression]bool {
	if !m.SelfContained() {
		return nil
	}
	names := face.FaceNamesInFS(facesFS)
	if len(names) == 0 {
		return nil
	}
	set := make(map[assistant.Expression]bool, len(names))
	for _, n := range names {
		set[assistant.Expression(n)] = true
	}
	return set
}
```

Update both `modIdleFaces(activeMod)` call sites in `main.go` (initial scheduler setup + inside `reloadMod`) to `modIdleFaces(activeMod, modFacesFS)`.

In the gallery enumerator (around line 525), replace:

```go
		for _, n := range face.FaceNamesInDir(activeMod.FacesDir()) {
```
with:
```go
		for _, n := range face.FaceNamesInFS(modFacesFS) {
```

- [ ] **Step 6: Update the startup `CheckOverrides` call**

Around line 775, replace:

```go
		if overrideErrs := config.CheckOverrides(activeMod.Root); len(overrideErrs) > 0 {
```
with:
```go
		if overrideErrs := config.CheckOverrides(activeMod.FS); len(overrideErrs) > 0 {
```

- [ ] **Step 7: Update `cmd/generate-audio` for the new `Discover` signature**

`cmd/generate-audio/main.go` calls `mod.Discover(...)`. Update it to pass `nil` (or a `log.Printf`-style adapter) for the new `logf` parameter:

```go
	mods := mod.Discover(modsRoot, nil)
```

It keeps using `activeMod.VoicePath()` / `activeMod.PersonaPath()` with `config.LoadPromptFile` — unchanged (directory-only tool).

- [ ] **Step 8: Build and run the full suite**

Run:
```bash
CGO_ENABLED=1 go build ./...
CGO_ENABLED=1 go test ./...
golangci-lint run ./...
```
Expected: build succeeds; all tests pass; no new lint findings. Fix compile errors by reconciling setter/rebuild names against the actual current `reloadMod` body (Step 4 note).

- [ ] **Step 9: Commit**

```bash
git add cmd/bmo-pak/main.go cmd/bmo-pak/idle_faces.go cmd/generate-audio/main.go
git commit -m "feat(bmo-pak): load active mod via fs.FS (directory or .zip)"
```

---

## Task 8: Corrupt-zip resilience

**Files:**
- Test: `internal/mod/discover_test.go`

A corrupt/unreadable `.zip` must be discoverable (it still appears as a mod by id) but `Open` returns an error, which `cmd/bmo-pak` already handles by falling back to default (Task 7 Step 1). Verify `Open` surfaces the error rather than panicking.

- [ ] **Step 1: Write the failing test**

Add to `internal/mod/discover_test.go`:

```go
func TestOpenCorruptZipReturnsError(t *testing.T) {
	root := t.TempDir()
	zipPath := filepath.Join(root, "broken.zip")
	if err := os.WriteFile(zipPath, []byte("not a real zip"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := Mod{ID: "broken", Root: zipPath}
	if err := m.Open(nil); err == nil {
		m.Close()
		t.Fatal("Open(corrupt zip) = nil error, want error")
	}
}
```

- [ ] **Step 2: Run test to verify it passes (behavior already implemented in Task 2)**

Run: `CGO_ENABLED=0 go test ./internal/mod/ -run TestOpenCorruptZip -v`
Expected: PASS — `zip.OpenReader` returns an error for non-zip bytes, which `Open` propagates. If it fails, ensure `Open` returns the `zip.OpenReader` error before touching `m.FS`.

- [ ] **Step 3: Verify discovery tolerates the corrupt zip**

Add:

```go
func TestDiscoverListsCorruptZipByID(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "default"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "broken.zip"), []byte("garbage"), 0o644); err != nil {
		t.Fatal(err)
	}
	mods := Discover(root, nil) // manifestFor swallows the open error → zero manifest
	found := false
	for _, m := range mods {
		if m.ID == "broken" {
			found = true
		}
	}
	if !found {
		t.Error("corrupt zip should still be listed by id")
	}
}
```

Run: `CGO_ENABLED=0 go test ./internal/mod/ -run TestDiscoverListsCorruptZip -v`
Expected: PASS (`manifestFor` returns the zero manifest on `Open` error, so discovery does not crash).

- [ ] **Step 4: Commit**

```bash
git add internal/mod/discover_test.go
git commit -m "test(mod): corrupt zip is listed but Open errors cleanly"
```

---

## Task 9: Example-mod zip regression test

**Files:**
- Modify: `internal/examplemods/evilbmo_test.go`

The existing `internal/examplemods` tests validate the `evil-bmo` directory through the device path. After this change the device path is `fs.FS`-based, so:
1. adapt the existing tests to the new APIs (`os.DirFS` over the dir), and
2. add a regression that zips `evil-bmo` in memory→on-disk and asserts it validates identically.

- [ ] **Step 1: Adapt existing tests to `fs.FS`**

In `internal/examplemods/evilbmo_test.go`, add a helper returning the dir FS, and update the API call sites:

```go
import "io/fs"

func modFS(t *testing.T) fs.FS {
	t.Helper()
	return os.DirFS(modRoot(t))
}
```

- `TestDeviceValidation`: `config.CheckOverrides(modFS(t))`.
- `TestManifest`: `modpkg.LoadManifest(modFS(t))`.
- `TestEmotions`: `modpkg.LoadManifest(modFS(t))` (the `os.Stat` face-existence checks on `modRoot` paths can stay — `evil-bmo` is a directory).
- `TestSelfContained`: build the mod and open it:

```go
func TestSelfContained(t *testing.T) {
	m := modpkg.Mod{ID: "evil-bmo", Root: modRoot(t)}
	if err := m.Open(nil); err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer m.Close()
	m.Manifest = modpkg.LoadManifest(m.FS)
	if !m.FacesHasSVG() {
		t.Fatal("FacesHasSVG() = false, want true")
	}
	if !m.SelfContained() {
		t.Error("SelfContained() = false, want true")
	}
}
```

- [ ] **Step 2: Write the failing zip-regression test**

Add:

```go
import (
	"archive/zip"
	"os"
	"path/filepath"
)

// zipExampleMod packages examples/mods/evil-bmo into <tmp>/evil-bmo.zip with a
// top-level evil-bmo/ folder, exactly like scripts/release.sh, and returns the
// archive path.
func zipExampleMod(t *testing.T) string {
	t.Helper()
	src := modRoot(t)
	dst := filepath.Join(t.TempDir(), "evil-bmo.zip")
	f, err := os.Create(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	err = filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		w, err := zw.Create(filepath.ToSlash(filepath.Join("evil-bmo", rel)))
		if err != nil {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	})
	if err != nil {
		t.Fatalf("walk/zip: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return dst
}

func TestZippedExampleModValidates(t *testing.T) {
	m := modpkg.Mod{ID: "evil-bmo", Root: zipExampleMod(t)}
	if err := m.Open(nil); err != nil {
		t.Fatalf("Open zip: %v", err)
	}
	defer m.Close()

	if errs := config.CheckOverrides(m.FS); len(errs) != 0 {
		t.Errorf("CheckOverrides on zip: %v", errs)
	}
	if got := modpkg.LoadManifest(m.FS).Name; got != "Evil BMO" {
		t.Errorf("zip manifest Name = %q, want %q", got, "Evil BMO")
	}
	if !m.FacesHasSVG() || !m.SelfContained() {
		t.Error("zipped evil-bmo should be self-contained")
	}
}
```

- [ ] **Step 3: Run the example tests**

Run: `CGO_ENABLED=0 go test ./internal/examplemods/ -v`
Expected: PASS for the adapted tests and the new `TestZippedExampleModValidates`.

- [ ] **Step 4: Commit**

```bash
git add internal/examplemods/evilbmo_test.go
git commit -m "test(examplemods): validate evil-bmo loaded from a .zip"
```

---

## Task 10: Document zip distribution in `MODDING.md`

**Files:**
- Modify: `docs/MODDING.md`

- [ ] **Step 1: Read the current install/distribution section**

Run: `grep -n "unzip\|mods/\|Install\|Distribut" docs/MODDING.md`
Locate the install-flow section so the new section sits beside it and matches tone.

- [ ] **Step 2: Add a "Distributing as a `.zip`" section**

Insert (adjust heading level to match surrounding sections):

```markdown
## Distributing as a `.zip`

A mod can be installed either as a folder or as a single `.zip` archive — BMO
reads both directly, so users no longer have to unzip.

- **Filename is the mod id.** `evil-bmo.zip` loads as the mod `evil-bmo`.
- **Wrap the mod in a top-level folder inside the archive.** The archive should
  contain `evil-bmo/mod.json`, `evil-bmo/faces/…`, etc. — exactly what you get
  from `zip -r evil-bmo.zip evil-bmo`. (Files at the archive root are tolerated
  with a warning, but the wrapping folder is the supported layout.)
- **Drop the `.zip` into `mods/`** on the device, alongside any folder mods.
- **A directory wins over a same-named `.zip`.** If both `mods/evil-bmo/` and
  `mods/evil-bmo.zip` exist, BMO uses the directory (so an extracted, edited
  copy overrides the archive) and logs a warning. This keeps the edit/iterate
  workflow intact.

`scripts/release.sh` produces ready-to-ship archives at `dist/mods/<id>.zip`,
and `scripts/deploy-mods.sh` pushes the example mods to a connected device.
```

- [ ] **Step 3: Verify internal doc links still resolve**

Run:
```bash
cd /home/carroarmato0/Applications/Development/NextUI/Paks/BMO
fail=0
for tgt in $(grep -oE '\]\(([^)]+)\)' docs/MODDING.md | sed -E 's/.*\(([^)]+)\)/\1/' | grep -vE '^https?://'); do
  rel=$(printf '%s' "$tgt" | sed 's/#.*//'); [ -z "$rel" ] && continue
  [ -e "docs/$rel" ] || [ -e "$rel" ] || { echo "BROKEN: $tgt"; fail=1; }
done
echo "link check exit=$fail"
```
Expected: `link check exit=0`.

- [ ] **Step 4: Commit**

```bash
git add docs/MODDING.md
git commit -m "docs(mods): document .zip distribution and directory-wins precedence"
```

---

## Final verification

- [ ] Run the full suite with CGO (renderer/bmo-pak need it):

```bash
CGO_ENABLED=1 go test ./...
```
Expected: all green.

- [ ] Lint:

```bash
golangci-lint run ./...
```
Expected: no new findings.

- [ ] Cross-compile + package (confirms `release.sh` still builds and the `dist/mods/*.zip` artifacts are produced):

```bash
./scripts/release.sh
```
Expected: `RELEASE_EXIT=0`; `dist/mods/evil-bmo.zip` listed.

- [ ] (Manual, optional) Deploy and smoke-test on device: push `dist/mods/evil-bmo.zip` to `/mnt/SDCARD/.userdata/tg5040/BMO/mods/`, remove the `evil-bmo/` directory if present, launch BMO, select Evil BMO under Settings → MOD, confirm the face renders and a spoken reaction animates the mouth (proves faces + audio resolve from the zip).

---

## Self-review notes (for the executor)

- **Path accessors stay** (`Root`, `PersonaPath`, `VoicePath`, `QuotesPath`, `FacesDir`, `AudioDir`) — required by `cmd/generate-audio` and logging. Only the runtime *read* path moved to `fs.FS`.
- **Signature consistency:** `LoadManifest(fs.FS)`, `Discover(string, func(string,...any))`, `Mod.Open(func(string,...any)) error`, `Mod.Close() error`, `face.NewLibraryMode(fs.FS, bool)`, `face.NewLibrary(fs.FS)`, `face.FaceNamesInFS(fs.FS)`, `config.CheckOverrides(fs.FS)`, `config.LoadPromptFS(fs.FS, string, string)`, `clips.NewLibrary(fs.FS)`, `modIdleFaces(mod.Mod, fs.FS)`.
- **`reloadMod` rebuild names are placeholders** — Task 7 Step 4 explicitly says to read the current body and reuse its actual setter/rebuild mechanism rather than inventing setters.
- **fs.Sub on "faces"/"audio" never errors** (constant valid paths); ignoring the error is intentional and safe.
- **Out of scope (per spec):** signing/encryption, writing into zip mods, zip hot-reload, `default` as a zip.
