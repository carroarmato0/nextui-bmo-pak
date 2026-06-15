# BMO Mod Emotion Vocabulary Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Derive the LLM emotion vocabulary from the active mod's faces (with optional `mod.json` descriptions), let a self-contained mod render brand-new custom-named expressions, and update both live on mod switch.

**Architecture:** `face` gains name-classification helpers and a `Library.Resolve` that lets the cache key/render a custom-named SVG; the renderer passes the raw expression to the cache so custom names survive. `assistant` replaces its hardcoded 28-word vocabulary with `EmotionEntry`/`BuildEmotionVocabulary`, a dynamic protocol prompt, and a `ParseEmotion` that takes the active name set; the pipeline gets a per-utterance vocabulary source. `cmd/bmo-pak` builds that source from the active mod.

**Tech Stack:** Go (CGO disabled for tests; SDL/renderer needs CGO), `go:embed`, standard library. Module `github.com/carroarmato0/nextui-bmo`.

**Verification (run after every task):**
- `CGO_ENABLED=0 go test ./...` (SDL packages `internal/renderer` + `cmd/bmo-pak` fail to *build* under CGO_ENABLED=0 — pre-existing, SDL needs CGO; use `CGO_ENABLED=1 go test ./...` to include them)
- `golangci-lint run ./...` (new code must add no findings)

---

## File Structure

- **Create** `internal/face/emotion.go` — `FunctionalNames`, `EmotionNames()`, `EmotionFaceNamesInDir()`.
- **Create** `internal/face/emotion_test.go`
- **Modify** `internal/face/library.go` — add `Resolve`.
- **Modify** `internal/face/library_test.go` — `Resolve` tests.
- **Modify** `internal/face/cache.go` — `resolved` map; `Frame` keys by `Resolve`; `Warm` warms disk emotion faces.
- **Modify** `internal/face/cache_test.go` — custom-name render test.
- **Modify** `internal/renderer/bmo.go` — pass raw expression to `Frame`.
- **Modify** `internal/mod/manifest.go` — add `Emotions` field.
- **Modify** `internal/mod/manifest_test.go` — emotions parse test.
- **Modify** `internal/assistant/emotion.go` — `EmotionEntry`, `BuildEmotionVocabulary`, dynamic prompt, `ParseEmotion(reply, valid)`; remove the global.
- **Modify** `internal/assistant/emotion_test.go` — rewrite for the new API.
- **Modify** `internal/assistant/voice.go` — vocabulary source + wiring.
- **Modify** `internal/assistant/voice_test.go` — vocabulary source test.
- **Modify** `internal/assistant/state.go` — fix a stale comment referencing `EmotionVocabulary`.
- **Modify** `cmd/bmo-pak/main.go` — install the vocabulary source from the active mod.

---

## Task 1: `face` name classification

**Files:**
- Create: `internal/face/emotion.go`
- Test: `internal/face/emotion_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/face/emotion_test.go`:

```go
package face

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEmotionNamesExcludeFunctional(t *testing.T) {
	names := EmotionNames()
	if want := len(CanonicalNames) - len(FunctionalNames); len(names) != want {
		t.Fatalf("EmotionNames len = %d, want %d", len(names), want)
	}
	in := map[string]bool{}
	for _, n := range names {
		in[n] = true
	}
	for _, f := range FunctionalNames {
		if in[f] {
			t.Errorf("EmotionNames must not contain functional face %q", f)
		}
	}
	// Every emotion name must resolve to itself, or the model would be told
	// about a face BMO cannot show.
	for _, n := range names {
		if got := Canonical(n); got != n {
			t.Errorf("Canonical(%q) = %q, want self-resolving", n, got)
		}
	}
}

func TestEmotionFaceNamesInDir(t *testing.T) {
	dir := t.TempDir()
	write := func(name string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("<svg/>"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("happy.svg")
	write("grumpy.svg")
	write("neutral.svg")
	write("speaking.svg") // functional — excluded
	write("notes.txt")    // not an svg — excluded

	got := EmotionFaceNamesInDir(dir)
	want := []string{"grumpy", "happy", "neutral"} // sorted, functional/non-svg dropped
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	if EmotionFaceNamesInDir(filepath.Join(dir, "missing")) != nil {
		t.Error("missing dir should yield nil")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `CGO_ENABLED=0 go test ./internal/face/ -run 'TestEmotionNames|TestEmotionFaceNames' -v`
Expected: FAIL — `undefined: EmotionNames`, `undefined: FunctionalNames`, `undefined: EmotionFaceNamesInDir`.

- [ ] **Step 3: Write the implementation**

Create `internal/face/emotion.go`:

```go
package face

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// FunctionalNames are the state-driven faces the assistant never requests as an
// emotion. They remain overridable as art, but are excluded from the emotion
// vocabulary advertised to the chat model.
var FunctionalNames = []string{ExprBlink, ExprListening, ExprThinking, ExprSpeaking, ExprSleeping}

func isFunctional(name string) bool {
	for _, f := range FunctionalNames {
		if f == name {
			return true
		}
	}
	return false
}

// EmotionNames returns the built-in emotion faces: every canonical name that is
// not a functional, state-driven face. This is the default vocabulary for the
// embedded BMO and any mod that inherits embedded faces.
func EmotionNames() []string {
	out := make([]string, 0, len(CanonicalNames))
	for _, n := range CanonicalNames {
		if !isFunctional(n) {
			out = append(out, n)
		}
	}
	return out
}

// EmotionFaceNamesInDir lists the emotion faces a mod ships on disk: the base
// names of *.svg files in dir, excluding functional faces and unsafe names,
// sorted. A missing or unreadable dir yields nil.
func EmotionFaceNamesInDir(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.EqualFold(filepath.Ext(e.Name()), ".svg") {
			continue
		}
		base := strings.ToLower(strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())))
		if !fileNameRe.MatchString(base) || isFunctional(base) {
			continue
		}
		out = append(out, base)
	}
	sort.Strings(out)
	return out
}
```

(`fileNameRe` is defined in `library.go`, same package.)

- [ ] **Step 4: Run the test to verify it passes**

Run: `CGO_ENABLED=0 go test ./internal/face/ -run 'TestEmotionNames|TestEmotionFaceNames' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/face/emotion.go internal/face/emotion_test.go
git commit -m "feat(face): emotion name classification helpers"
```

(Project rule: no `Co-Authored-By` trailer on any commit.)

---

## Task 2: `face.Library.Resolve` + cache + renderer pass-through

**Files:**
- Modify: `internal/face/library.go`
- Modify: `internal/face/library_test.go`
- Modify: `internal/face/cache.go`
- Modify: `internal/face/cache_test.go`
- Modify: `internal/renderer/bmo.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/face/library_test.go`:

```go
func TestResolveRawWhenFileExists(t *testing.T) {
	dir := t.TempDir()
	svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210"><rect width="280" height="210" fill="#0f0"/></svg>`
	if err := os.WriteFile(filepath.Join(dir, "grumpy.svg"), []byte(svg), 0o644); err != nil {
		t.Fatal(err)
	}
	lib := NewLibraryMode(dir, true)
	if got := lib.Resolve("grumpy"); got != "grumpy" {
		t.Fatalf("Resolve(grumpy) = %q, want grumpy", got)
	}
	// No happy.svg on disk: falls back to the canonical name.
	if got := lib.Resolve("happy"); got != "happy" {
		t.Fatalf("Resolve(happy) = %q, want happy", got)
	}
	// Alias with no disk file resolves through Canonical.
	if got := lib.Resolve("shocked"); got != ExprSurprised {
		t.Fatalf("Resolve(shocked) = %q, want %q", got, ExprSurprised)
	}
}

func TestResolveCanonicalWhenNoDir(t *testing.T) {
	lib := NewLibrary(filepath.Join(t.TempDir(), "missing"))
	if got := lib.Resolve("cry"); got != ExprCrying {
		t.Fatalf("Resolve(cry) = %q, want %q", got, ExprCrying)
	}
	// Unsafe names never hit disk and fold to neutral via Canonical.
	if got := lib.Resolve("../etc/passwd"); got != ExprNeutral {
		t.Fatalf("Resolve(traversal) = %q, want %q", got, ExprNeutral)
	}
}
```

Append to `internal/face/cache_test.go`:

```go
func TestFrameRendersCustomName(t *testing.T) {
	dir := t.TempDir()
	neutral := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210"><rect width="280" height="210" fill="#0000ff"/></svg>`
	grumpy := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210"><rect width="280" height="210" fill="#00ff00"/></svg>`
	if err := os.WriteFile(filepath.Join(dir, "neutral.svg"), []byte(neutral), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "grumpy.svg"), []byte(grumpy), 0o644); err != nil {
		t.Fatal(err)
	}
	c := NewCache(NewLibraryMode(dir, true))

	g := c.Frame("grumpy", 28, 21)
	if g == nil {
		t.Fatal("custom expression grumpy did not render")
	}
	n := c.Frame("neutral", 28, 21)
	if n == nil {
		t.Fatal("neutral did not render")
	}
	// Different fills must produce different buffers (custom face is not folded
	// to neutral).
	same := true
	for i := range g {
		if g[i] != n[i] {
			same = false
			break
		}
	}
	if same {
		t.Fatal("grumpy frame equals neutral frame; custom name was folded to neutral")
	}
}
```

(`cache_test.go` already imports `os`, `path/filepath`, `testing`. If `library_test.go` lacks an import, add it — it already uses `os`/`path/filepath`.)

- [ ] **Step 2: Run the tests to verify they fail**

Run: `CGO_ENABLED=0 go test ./internal/face/ -run 'TestResolve|TestFrameRendersCustomName' -v`
Expected: FAIL — `lib.Resolve undefined` (and the Frame test still folds to neutral until the cache change).

- [ ] **Step 3a: Add `Library.Resolve`**

In `internal/face/library.go`, after the `Bytes` method (end of file), add:

```go

// Resolve returns the cache key / load name for expr: the raw (sanitized) name
// when a matching on-disk face exists, so a self-contained mod's custom-named
// SVG renders under its own name; otherwise the canonical name.
func (l *Library) Resolve(expr string) string {
	raw := strings.ToLower(strings.TrimSpace(expr))
	if l.dir != "" && fileNameRe.MatchString(raw) {
		if _, err := os.Stat(filepath.Join(l.dir, raw+".svg")); err == nil {
			return raw
		}
	}
	return Canonical(raw)
}
```

- [ ] **Step 3b: Key the cache by `Resolve`, cached per-expr**

In `internal/face/cache.go`, add a field to the `Cache` struct (after `failed map[string]bool`):

```go
	resolved    map[string]string
```

Replace the whole `Frame` method:

```go
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
```

with:

```go
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
		return buf
	}
	if c.failed[key] {
		return nil
	}
	buf := c.renderLocked(key, w, h)
	if buf != nil {
		c.frames[key] = buf
	} else {
		c.failed[key] = true
	}
	return buf
}
```

- [ ] **Step 3c: Warm the mod's disk emotion faces**

In `internal/face/cache.go`, in `Warm`, after the `for _, name := range CanonicalNames { ... }` loop, add:

```go

	// Pre-rasterize the active mod's custom emotion faces so a custom name does
	// not stutter on first use. warmFrame is idempotent for names already warmed.
	for _, name := range EmotionFaceNamesInDir(c.lib.dir) {
		c.warmFrame(name, w, h)
	}
```

- [ ] **Step 3d: Renderer passes the raw expression to `Frame`**

In `internal/renderer/bmo.go`, replace this block (lines ~289-292):

```go
	canonical := face.Canonical(frame.Expression)
	if !r.blitFace(canonical, frame, phase) {
		r.drawPlainFace(layout)
	}
```

with:

```go
	canonical := face.Canonical(frame.Expression)
	if !r.blitFace(frame.Expression, frame, phase) {
		r.drawPlainFace(layout)
	}
```

(`canonical` is still used just below for the `ExprSleeping` check — leave that line.)

Then change `blitFace` to canonicalize internally only for the speaking check, and pass the raw expr to `Frame`. Replace the signature and the two relevant lines:

```go
func (r *Renderer) blitFace(canonical string, frame FrameState, phase float64) bool {
	if r.faces == nil {
		return false
	}
	w, h := int(r.W), int(r.H)
	if canonical == face.ExprSpeaking {
```

becomes:

```go
func (r *Renderer) blitFace(expr string, frame FrameState, phase float64) bool {
	if r.faces == nil {
		return false
	}
	w, h := int(r.W), int(r.H)
	if face.Canonical(expr) == face.ExprSpeaking {
```

and:

```go
	buf := r.faces.Frame(canonical, w, h)
```

becomes:

```go
	buf := r.faces.Frame(expr, w, h)
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `CGO_ENABLED=0 go test ./internal/face/ -v` (face tests, incl. new ones).
Run: `CGO_ENABLED=1 go test ./internal/face/ ./internal/renderer/ 2>&1 | tail -5` (renderer builds + passes).
Run: `golangci-lint run ./internal/face/... ./internal/renderer/...`
Expected: all PASS; lint 0 findings.

- [ ] **Step 5: Commit**

```bash
git add internal/face/library.go internal/face/library_test.go internal/face/cache.go internal/face/cache_test.go internal/renderer/bmo.go
git commit -m "feat(face): render custom-named mod expressions via Library.Resolve"
```

---

## Task 3: `mod.Manifest.Emotions`

**Files:**
- Modify: `internal/mod/manifest.go`
- Modify: `internal/mod/manifest_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/mod/manifest_test.go`:

```go
func TestLoadManifestEmotions(t *testing.T) {
	dir := t.TempDir()
	body := `{"name":"Evil BMO","emotions":{"grumpy":"sulky and irritable","ecstatic":"overjoyed"}}`
	if err := os.WriteFile(filepath.Join(dir, "mod.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	m := LoadManifest(dir)
	if got := m.Emotions["grumpy"]; got != "sulky and irritable" {
		t.Fatalf("Emotions[grumpy] = %q", got)
	}
	if got := m.Emotions["ecstatic"]; got != "overjoyed" {
		t.Fatalf("Emotions[ecstatic] = %q", got)
	}
}

func TestLoadManifestNoEmotions(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "mod.json"), []byte(`{"name":"x"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if m := LoadManifest(dir); m.Emotions != nil {
		t.Fatalf("Emotions should be nil when absent, got %v", m.Emotions)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `CGO_ENABLED=0 go test ./internal/mod/ -run TestLoadManifestEmotions -v`
Expected: FAIL — `m.Emotions undefined`.

- [ ] **Step 3: Add the field**

In `internal/mod/manifest.go`, add to the `Manifest` struct after the `Version` field:

```go
	Emotions    map[string]string `json:"emotions,omitempty"` // emotion name -> LLM description
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `CGO_ENABLED=0 go test ./internal/mod/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/mod/manifest.go internal/mod/manifest_test.go
git commit -m "feat(mod): optional per-emotion descriptions in manifest"
```

---

## Task 4: rewrite `assistant/emotion.go` for dynamic vocabulary

**Files:**
- Modify: `internal/assistant/emotion.go`
- Modify: `internal/assistant/emotion_test.go`
- Modify: `internal/assistant/state.go` (stale comment)

- [ ] **Step 1: Rewrite the tests first**

Replace the entire contents of `internal/assistant/emotion_test.go` with:

```go
package assistant

import (
	"strings"
	"testing"

	"github.com/carroarmato0/nextui-bmo/internal/face"
)

func builtinVocab() []EmotionEntry {
	return BuildEmotionVocabulary(face.EmotionNames(), nil, nil)
}

func TestBuildEmotionVocabularyOverlay(t *testing.T) {
	v := BuildEmotionVocabulary([]string{"happy", "sad"}, []string{"sad", "grumpy"}, map[string]string{"grumpy": "sulky"})
	var names []string
	for _, e := range v {
		names = append(names, e.Name)
	}
	want := []string{"happy", "sad", "grumpy"} // builtin first, then new disk names, deduped
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("names = %v, want %v", names, want)
	}
	if v[2].Description != "sulky" {
		t.Fatalf("grumpy description = %q, want sulky", v[2].Description)
	}
}

func TestBuildEmotionVocabularySelfContained(t *testing.T) {
	v := BuildEmotionVocabulary(nil, []string{"grumpy", "happy"}, nil)
	if len(v) != 2 || v[0].Name != "grumpy" || v[1].Name != "happy" {
		t.Fatalf("self-contained vocab = %+v, want [grumpy happy]", v)
	}
}

func TestParseEmotion(t *testing.T) {
	valid := emotionNameSet(builtinVocab())
	tests := []struct {
		name      string
		in        string
		wantClean string
		wantEmo   Expression
	}{
		{"no directive", "Hello there!", "Hello there!", ""},
		{"no directive preserves double space", "Hello  there", "Hello  there", ""},
		{"leading directive", "[happy] Hello there!", "Hello there!", ExpressionHappy},
		{"leading no space", "[happy]Hello", "Hello", ExpressionHappy},
		{"embedded directive", "Oh [excited] I love it", "Oh I love it", ExpressionExcited},
		{"trailing directive", "Goodbye [sad]", "Goodbye", ExpressionSad},
		{"case insensitive", "[HAPPY] hi", "hi", ExpressionHappy},
		{"unknown bracket kept", "Wait [pauses] then go", "Wait [pauses] then go", ""},
		{"numeric bracket kept", "See note [1] here", "See note [1] here", ""},
		{"multiple first wins all stripped", "[sad] no [happy] yes", "no yes", ExpressionSad},
		{"only a directive", "[happy]", "", ExpressionHappy},
		{"directive with surrounding spaces tidy", "hi  [happy]  there", "hi there", ExpressionHappy},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clean, emo := ParseEmotion(tt.in, valid)
			if clean != tt.wantClean {
				t.Errorf("clean = %q, want %q", clean, tt.wantClean)
			}
			if emo != tt.wantEmo {
				t.Errorf("emotion = %q, want %q", emo, tt.wantEmo)
			}
		})
	}
}

func TestParseEmotionCustomName(t *testing.T) {
	valid := emotionNameSet(BuildEmotionVocabulary(nil, []string{"grumpy"}, nil))
	clean, emo := ParseEmotion("[grumpy] go away", valid)
	if clean != "go away" || emo != Expression("grumpy") {
		t.Fatalf("clean=%q emo=%q, want %q/grumpy", clean, emo, "go away")
	}
	// A hyphenated custom name matches the widened token regex.
	valid2 := emotionNameSet(BuildEmotionVocabulary(nil, []string{"side-eye"}, nil))
	if _, e := ParseEmotion("[side-eye] hmm", valid2); e != Expression("side-eye") {
		t.Fatalf("hyphenated custom name not parsed: %q", e)
	}
	// A name not in the active vocabulary passes through untouched.
	if c, e := ParseEmotion("[grumpy] hi", emotionNameSet(builtinVocab())); c != "[grumpy] hi" || e != "" {
		t.Fatalf("unknown custom name should pass through: clean=%q emo=%q", c, e)
	}
}

func TestEmotionProtocolPrompt(t *testing.T) {
	p := emotionProtocolPrompt(builtinVocab())
	if !strings.Contains(p, "[happy]") {
		t.Errorf("protocol missing [happy] example: %q", p)
	}
	for _, e := range builtinVocab() {
		if !strings.Contains(p, e.Name) {
			t.Errorf("protocol missing vocabulary word %q", e.Name)
		}
	}
	if !strings.Contains(strings.ToLower(p), "never spoken") {
		t.Errorf("protocol must say the directive is never spoken: %q", p)
	}
}

func TestEmotionProtocolPromptDescriptions(t *testing.T) {
	p := emotionProtocolPrompt([]EmotionEntry{
		{Name: "grumpy", Description: "sulky and irritable"},
		{Name: "happy"},
	})
	if !strings.Contains(p, "grumpy — sulky and irritable") {
		t.Errorf("protocol missing described entry: %q", p)
	}
	if !strings.Contains(p, "happy") {
		t.Errorf("protocol missing bare entry: %q", p)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `CGO_ENABLED=0 go test ./internal/assistant/ -run 'TestBuildEmotion|TestParseEmotion|TestEmotionProtocol' -v`
Expected: FAIL — `undefined: EmotionEntry`, `BuildEmotionVocabulary`, `emotionNameSet`, signature mismatch on `ParseEmotion`/`emotionProtocolPrompt`.

- [ ] **Step 3: Rewrite `emotion.go`**

Replace the entire contents of `internal/assistant/emotion.go` with:

```go
package assistant

import (
	"regexp"
	"strings"
)

// EmotionEntry is one advertised emotion: a face name plus an optional human
// description used to help the chat model choose it.
type EmotionEntry struct {
	Name        string
	Description string
}

// BuildEmotionVocabulary combines the built-in emotion names (empty for a
// self-contained mod) with the emotion faces the active mod ships on disk,
// de-duplicating by name (first occurrence wins, built-ins first) and attaching
// any description from the mod manifest. It is the single source of truth for
// both the system-prompt advertising and the parser whitelist, so they cannot
// drift apart.
func BuildEmotionVocabulary(builtin, disk []string, descriptions map[string]string) []EmotionEntry {
	seen := make(map[string]bool, len(builtin)+len(disk))
	entries := make([]EmotionEntry, 0, len(builtin)+len(disk))
	add := func(name string) {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		entries = append(entries, EmotionEntry{Name: name, Description: strings.TrimSpace(descriptions[name])})
	}
	for _, n := range builtin {
		add(n)
	}
	for _, n := range disk {
		add(n)
	}
	return entries
}

// emotionNameSet builds the parser whitelist from a vocabulary: lower-cased name
// -> Expression (the name itself, which is what the renderer resolves).
func emotionNameSet(entries []EmotionEntry) map[string]Expression {
	m := make(map[string]Expression, len(entries))
	for _, e := range entries {
		m[e.Name] = Expression(e.Name)
	}
	return m
}

// emotionTokenRe matches a bracketed single token of the face-filename charset,
// e.g. "[happy]" or "[side-eye]". Only tokens whose word is in the active
// vocabulary are treated as directives; anything else is left untouched.
var emotionTokenRe = regexp.MustCompile(`\[([A-Za-z0-9_-]+)\]`)

// extraSpaceRe collapses runs of spaces/tabs left behind after removing a
// directive. Newlines are preserved.
var extraSpaceRe = regexp.MustCompile(`[ \t]{2,}`)

// emotionProtocolPrompt is appended to the chat persona so the model knows how
// to drive BMO's face. Built from the supplied vocabulary so it can never
// advertise a word the parser would not accept. Entries with a description are
// rendered as "name — description"; others as the bare name.
func emotionProtocolPrompt(entries []EmotionEntry) string {
	parts := make([]string, len(entries))
	for i, e := range entries {
		if e.Description != "" {
			parts[i] = e.Name + " — " + e.Description
		} else {
			parts[i] = e.Name
		}
	}
	return "You have an animated face. You may begin your reply with exactly one " +
		"directive in square brackets to set your facial expression, for example " +
		"[happy]. The bracketed word is silent — it is never spoken aloud, only " +
		"used to choose your face. Include it only when an emotion clearly fits; " +
		"otherwise leave it out. Valid expressions: " + strings.Join(parts, ", ") + "."
}

// ParseEmotion extracts the chat model's facial directive. It removes every
// recognised [emotion] token (those whose lower-cased word is a key in valid)
// from reply, tidies the whitespace the removal leaves behind, and returns the
// spoken text plus the FIRST recognised emotion (empty Expression if none).
// Bracketed words not in valid pass through unchanged.
func ParseEmotion(reply string, valid map[string]Expression) (string, Expression) {
	var first Expression
	clean := emotionTokenRe.ReplaceAllStringFunc(reply, func(tok string) string {
		name := strings.ToLower(tok[1 : len(tok)-1]) // strip surrounding [ and ]
		if emo, ok := valid[name]; ok {
			if first == "" {
				first = emo
			}
			return ""
		}
		return tok
	})
	if first != "" {
		clean = strings.TrimSpace(extraSpaceRe.ReplaceAllString(clean, " "))
	}
	return clean, first
}
```

- [ ] **Step 4: Fix the stale comment in `state.go`**

In `internal/assistant/state.go`, the comment near line 68 references `EmotionVocabulary`. Replace the phrase `(see EmotionVocabulary)` with `(see face.EmotionNames)`. (Comment only — if the exact text differs, just update the reference to point at `face.EmotionNames` instead of the removed global.)

- [ ] **Step 5: Run the tests to verify they pass**

Run: `CGO_ENABLED=0 go test ./internal/assistant/ -run 'TestBuildEmotion|TestParseEmotion|TestEmotionProtocol' -v`
Expected: PASS. (The pipeline still references the old `ParseEmotion`/`emotionProtocolPrompt` signatures — `go test` of the whole package will not compile yet; that is fixed in Task 5. Running just these tests compiles the test file against the new `emotion.go`. If the package fails to build due to `voice.go`, proceed to Task 5 and run the full package suite there.)

- [ ] **Step 6: Commit**

```bash
git add internal/assistant/emotion.go internal/assistant/emotion_test.go internal/assistant/state.go
git commit -m "feat(assistant): dynamic emotion vocabulary and parser"
```

---

## Task 5: pipeline vocabulary source

**Files:**
- Modify: `internal/assistant/voice.go`
- Modify: `internal/assistant/voice_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/assistant/voice_test.go`:

```go
func TestEmotionVocabularySourceDrivesPromptAndParse(t *testing.T) {
	pipe := &VoicePipeline{}
	pipe.SetEmotionVocabularySource(func() []EmotionEntry {
		return BuildEmotionVocabulary(nil, []string{"grumpy"}, map[string]string{"grumpy": "sulky"})
	})

	prompt := pipe.currentSystemPrompt()
	if !strings.Contains(prompt, "grumpy — sulky") {
		t.Fatalf("system prompt should advertise the mod vocabulary: %q", prompt)
	}
	if strings.Contains(prompt, "excited") {
		t.Fatalf("self-contained vocab must not include built-in names: %q", prompt)
	}

	vocab := pipe.currentEmotionVocab()
	clean, emo := ParseEmotion("[grumpy] hi", emotionNameSet(vocab))
	if clean != "hi" || emo != Expression("grumpy") {
		t.Fatalf("clean=%q emo=%q, want hi/grumpy", clean, emo)
	}
}

func TestEmotionVocabularyDefaultsToBuiltin(t *testing.T) {
	pipe := &VoicePipeline{}
	if got := len(pipe.currentEmotionVocab()); got != len(face.EmotionNames()) {
		t.Fatalf("default vocab len = %d, want %d", got, len(face.EmotionNames()))
	}
}
```

Ensure `voice_test.go` imports `"github.com/carroarmato0/nextui-bmo/internal/face"` (add it to the import block if absent) and `"strings"` (already used in the file).

- [ ] **Step 2: Run the test to verify it fails**

Run: `CGO_ENABLED=0 go test ./internal/assistant/ -run TestEmotionVocabulary -v`
Expected: FAIL — `pipe.SetEmotionVocabularySource undefined`, `pipe.currentEmotionVocab undefined`.

- [ ] **Step 3: Implement the source + wiring**

In `internal/assistant/voice.go`, add `"github.com/carroarmato0/nextui-bmo/internal/face"` to the import block.

Add a field to the `VoicePipeline` struct, right after `systemPromptSource    func() string`:

```go
	emotionVocabSource    func() []EmotionEntry
```

Add the setter and resolver after `SetSystemPromptSource` (after its closing brace, ~line 189):

```go

// SetEmotionVocabularySource installs a function consulted before each utterance
// for the active emotion vocabulary, so a mod switch updates what the model is
// told. An empty result falls back to the built-in emotion set.
func (p *VoicePipeline) SetEmotionVocabularySource(source func() []EmotionEntry) {
	if p != nil {
		p.emotionVocabSource = source
	}
}

// currentEmotionVocab resolves the emotion vocabulary for the next utterance,
// falling back to the built-in emotion faces when no source is installed.
func (p *VoicePipeline) currentEmotionVocab() []EmotionEntry {
	if p.emotionVocabSource != nil {
		if v := p.emotionVocabSource(); len(v) > 0 {
			return v
		}
	}
	return BuildEmotionVocabulary(face.EmotionNames(), nil, nil)
}
```

Replace `currentSystemPrompt` (the body using `emotionProtocolPrompt()`):

```go
func (p *VoicePipeline) currentSystemPrompt() string {
	persona := p.systemPrompt
	if p.systemPromptSource != nil {
		if prompt := strings.TrimSpace(p.systemPromptSource()); prompt != "" {
			persona = prompt
		}
	}
	if strings.TrimSpace(persona) == "" {
		return emotionProtocolPrompt()
	}
	return persona + "\n\n" + emotionProtocolPrompt()
}
```

with:

```go
func (p *VoicePipeline) currentSystemPrompt() string {
	persona := p.systemPrompt
	if p.systemPromptSource != nil {
		if prompt := strings.TrimSpace(p.systemPromptSource()); prompt != "" {
			persona = prompt
		}
	}
	proto := emotionProtocolPrompt(p.currentEmotionVocab())
	if strings.TrimSpace(persona) == "" {
		return proto
	}
	return persona + "\n\n" + proto
}
```

In `resolveSpeech`, replace:

```go
	spoken, emotion := ParseEmotion(reply)
```

with:

```go
	spoken, emotion := ParseEmotion(reply, emotionNameSet(p.currentEmotionVocab()))
```

- [ ] **Step 4: Run the full assistant suite**

Run: `CGO_ENABLED=0 go test ./internal/assistant/ -v 2>&1 | tail -30`
Expected: PASS — new tests plus every pre-existing assistant test (the package now compiles against the new `emotion.go` API).
Run: `golangci-lint run ./internal/assistant/...`
Expected: 0 findings.

- [ ] **Step 5: Commit**

```bash
git add internal/assistant/voice.go internal/assistant/voice_test.go
git commit -m "feat(assistant): per-utterance emotion vocabulary source"
```

---

## Task 6: wire the active mod's vocabulary in `main.go`

`cmd/bmo-pak` has no unit tests; verify by build + full suite + lint + a manual check.

**Files:**
- Modify: `cmd/bmo-pak/main.go`

- [ ] **Step 1: Install the vocabulary source**

In `cmd/bmo-pak/main.go`, find the existing source-installation block inside the `if cfg.UsesAI() && audioSession != nil` section:

```go
			audioPipeline.SetSystemPromptSource(func() string {
				return systemPromptWithContext(
					config.LoadPromptFile(personaPath, config.DefaultSystemPrompt),
					deviceCtx.Snapshot(),
					memory.PromptBlock(time.Now().UTC()),
				)
			})
```

Immediately after that closing `})`, add:

```go
			// Advertise the active mod's emotions to the chat model. A
			// self-contained mod owns its set (no built-ins); otherwise the
			// embedded emotion faces are the base. activeMod is reassigned by
			// reloadMod, so a mod switch updates this on the next utterance.
			audioPipeline.SetEmotionVocabularySource(func() []assistant.EmotionEntry {
				var builtin []string
				if !activeMod.SelfContained() {
					builtin = face.EmotionNames()
				}
				disk := face.EmotionFaceNamesInDir(activeMod.FacesDir())
				return assistant.BuildEmotionVocabulary(builtin, disk, activeMod.Manifest.Emotions)
			})
```

(`assistant`, `face`, and `activeMod` are all already in scope in this function. No `reloadMod` change is needed — the closure reads the current `activeMod`.)

- [ ] **Step 2: Build, test, lint**

```bash
CGO_ENABLED=1 go build ./...
CGO_ENABLED=1 go test ./...
golangci-lint run ./...
```

Expected: build succeeds; all packages pass; 0 lint findings.

- [ ] **Step 3: Manual mod-switch check (best-effort; needs CGO+SDL)**

```bash
mkdir -p /tmp/bmo-emo-test/BMO/mods/evil/faces
printf 'You are EVIL BMO.\n' > /tmp/bmo-emo-test/BMO/mods/evil/persona.txt
printf '{"name":"Evil BMO","emotions":{"grumpy":"sulky and irritable"}}\n' > /tmp/bmo-emo-test/BMO/mods/evil/mod.json
# A self-contained face set including a custom emotion:
printf '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210"><rect width="280" height="210" fill="#0a0"/></svg>' > /tmp/bmo-emo-test/BMO/mods/evil/faces/neutral.svg
printf '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210"><rect width="280" height="210" fill="#a00"/></svg>' > /tmp/bmo-emo-test/BMO/mods/evil/faces/grumpy.svg
CGO_ENABLED=1 go build -o /tmp/bmo-emo-smoke ./cmd/bmo-pak && echo "CGO build OK"
```

If a device/desktop run is available: select the **evil** mod in Settings and confirm the log shows the switch; with debug logging + `log_system_prompt`, confirm the system prompt advertises `grumpy — sulky and irritable` and not the built-in list. Otherwise rely on the build + unit tests (the vocabulary-source logic is covered in Task 5, the custom-face render in Task 2). Clean up `/tmp/bmo-emo-*` afterward.

- [ ] **Step 4: Commit**

```bash
git add cmd/bmo-pak/main.go
git commit -m "feat(bmo-pak): advertise the active mod's emotion vocabulary"
```

---

## Self-Review Notes (plan vs. spec)

- **Render pipeline** → Task 2: `Library.Resolve` (raw-when-on-disk, `fileNameRe`-guarded, `os.Stat`), `Cache.Frame` keyed by resolved name with a per-expr `resolved` cache (no per-frame stat), `Warm` warms disk emotion faces, and the renderer passes the raw expression to `Frame` while still canonicalizing for the speaking/sleeping state checks. Behavior-preserving for the default mod (known names/aliases still flow through `Canonical`).
- **Name classification / kill the hardcoded 28-list** → Task 1: `FunctionalNames`, `EmotionNames()` (derived = canonical − functional), `EmotionFaceNamesInDir`.
- **Vocabulary model (one rule)** → Task 4 `BuildEmotionVocabulary` (builtin empty for self-contained, dedupe, ordering) + Task 6 (main.go passes `builtin=nil` when `activeMod.SelfContained()`).
- **Manifest hints** → Task 3 `Manifest.Emotions`; applied in Task 4 and read in Task 6.
- **Dynamic protocol + parser** → Task 4 `emotionProtocolPrompt(entries)`, `ParseEmotion(reply, valid)`, widened regex `[A-Za-z0-9_-]+`, global removed; Task 5 wires them per-utterance.
- **Live switch** → Task 5 per-utterance source + Task 6 closure reading the reassigned `activeMod` (no `reloadMod` edit needed; it already reassigns `activeMod`).
- **Testing** → covered per task; `cmd/bmo-pak` by build/lint/manual as the spec states.
- **Out of scope** (animation engine, mod-controlled TTS voice/provider, description sanitization) → untouched, per spec.

**Type consistency check:** `ParseEmotion(reply string, valid map[string]Expression)`, `emotionProtocolPrompt(entries []EmotionEntry)`, `BuildEmotionVocabulary(builtin, disk []string, descriptions map[string]string) []EmotionEntry`, `emotionNameSet(entries []EmotionEntry) map[string]Expression`, `(*VoicePipeline).SetEmotionVocabularySource(func() []EmotionEntry)`, `(*VoicePipeline).currentEmotionVocab() []EmotionEntry`, `face.EmotionNames() []string`, `face.EmotionFaceNamesInDir(string) []string`, `face.FunctionalNames []string`, `(*Library).Resolve(string) string` — used consistently across Tasks 1–6.
