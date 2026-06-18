# Evil BMO Mod Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a complete, self-contained "Evil BMO" character mod from the documented mod contract alone, with an automated Go validation test and on-device verification — dog-fooding the modding docs.

**Architecture:** The mod is data-only: text files (`persona.txt`, `voice.txt`, `quotes.txt`), eight SVG faces, and a `mod.json` (manifest + `emotions` + `animations`). Canonical source is tracked at `examples/mods/evil-bmo/`; a copy is staged to the gitignored `dist/mods/evil-bmo/` for `adb push`. A test-only Go package (`examples/mods/evil-bmo/evilbmo_test.go`) validates the mod through the *same* libraries the app uses (`internal/mod`, `internal/face`) so a broken asset fails CI, not the device.

**Tech Stack:** Go (pure, `CGO_ENABLED=0` — `internal/face` is CGO-free), `oksvg` rasterizer via `face.Rasterize`, Go-template SVGs via `face.RenderRest`, `golangci-lint`, `adb`.

**Key facts (verified against the code):**
- `mod.CurrentAPIVersion == 1`; `Manifest{APIVersion,Name,Author,Description,Version,Emotions,Animations}`; `Manifest.EffectiveAPIVersion()` folds 0→1.
- `mod.Mod{Root}` → `SelfContained()` is `!IsDefault && FacesHasSVG()`.
- `face.RenderRest(data []byte) []byte` executes a templated SVG at rest (empty data); returns non-template bytes unchanged; degrades safely on error.
- `face.Rasterize(svg []byte, w, h int) ([]uint32, error)` parses+rasterizes **plain** SVG (no templates) — must `RenderRest` first.
- `face.ParseAnimations(map[string]json.RawMessage) (map[string]AnimationDef, []error)`; `AnimationDef.Template.File` is the referenced face basename.
- Module path: `github.com/carroarmato0/nextui-bmo`.
- Face canvas: `280×210` viewBox. Palette: body `#D62828`, screen `#F25C5C`, ink `#1a1a1a`. Lip-sync idiom: `{{$m := or .m 0.0}}` … resting mouth at `$m == 0` else `{{template "talkmouth" $m}}`. Template arithmetic helpers `add`/`sub`/`mul` are available (used by `look_around`).

---

## File Structure

| Path | Responsibility |
|------|----------------|
| `examples/mods/evil-bmo/mod.json` | Manifest, emotion vocabulary, animation declarations |
| `examples/mods/evil-bmo/persona.txt` | Condescending grill-master system prompt |
| `examples/mods/evil-bmo/voice.txt` | Snide TTS delivery instructions |
| `examples/mods/evil-bmo/quotes.txt` | Mean-mirror idle one-liners |
| `examples/mods/evil-bmo/faces/neutral.svg` | Smug smirk (lip-sync template; universal fallback) |
| `examples/mods/evil-bmo/faces/laugh.svg` | Toothy devil cackle (lip-sync template) |
| `examples/mods/evil-bmo/faces/angry.svg` | Devilish scowl (lip-sync template) |
| `examples/mods/evil-bmo/faces/skeptical.svg` | One raised brow (static) |
| `examples/mods/evil-bmo/faces/unamused.svg` | Deadpan, eyes aside (static) |
| `examples/mods/evil-bmo/faces/thinking.svg` | Scheming (static, functional) |
| `examples/mods/evil-bmo/faces/listening.svg` | Attentive-but-smug (static, functional) |
| `examples/mods/evil-bmo/faces/look_around.svg` | Shifty side-eyes (time template, `param x`) |
| `examples/mods/evil-bmo/doc.go` | One-line package clause so the dir is a real Go package (tests can run) |
| `examples/mods/evil-bmo/evilbmo_test.go` | Validation: manifest, self-contained, prompts, face render, animations (written complete in Task 1; later tasks add assets to turn each test green) |
| `examples/mods/evil-bmo/FRICTION-LOG.md` | Running log of doc/contract gaps found while authoring |

Test command used throughout: `CGO_ENABLED=0 go test ./examples/mods/evil-bmo/ -v` (run from repo root). Lint: `golangci-lint run ./examples/...`.

---

### Task 1: Scaffold package, complete validation test, manifest

The whole validation test is written up front (every import is used, so it
compiles), plus a `doc.go` so the directory is a real Go package. Each later
task adds assets and runs only the relevant test subset to green; running the
full suite before Task 6 will show later tests failing, which is expected.

**Files:**
- Create: `examples/mods/evil-bmo/doc.go`
- Create: `examples/mods/evil-bmo/evilbmo_test.go`
- Create: `examples/mods/evil-bmo/mod.json`

- [ ] **Step 1: Write `examples/mods/evil-bmo/doc.go`**

```go
// Package evilbmo carries the Evil BMO example mod assets and a validation
// test that renders them through the same libraries the app uses. It has no
// runtime code — it exists so `go test` can run against the mod directory.
package evilbmo
```

- [ ] **Step 2: Write the complete `examples/mods/evil-bmo/evilbmo_test.go`**

Every test function is included now; the imports are all exercised so the
package compiles immediately. Tests for not-yet-created assets will fail until
their task adds the files — that is intended.

```go
package evilbmo

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/carroarmato0/nextui-bmo/internal/face"
	modpkg "github.com/carroarmato0/nextui-bmo/internal/mod"
)

// modRoot is the directory this test runs in (the mod source dir). go test
// sets the working directory to the package dir, so this is examples/mods/evil-bmo.
func modRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return wd
}

func TestManifest(t *testing.T) {
	m := modpkg.LoadManifest(modRoot(t))
	if got := m.EffectiveAPIVersion(); got != modpkg.CurrentAPIVersion {
		t.Errorf("apiVersion = %d, want %d", got, modpkg.CurrentAPIVersion)
	}
	if m.Name != "Evil BMO" {
		t.Errorf("name = %q, want %q", m.Name, "Evil BMO")
	}
	if strings.TrimSpace(m.Description) == "" {
		t.Error("description is blank")
	}
	if strings.TrimSpace(m.Version) == "" {
		t.Error("version is blank")
	}
}

func TestEmotions(t *testing.T) {
	m := modpkg.LoadManifest(modRoot(t))
	for _, key := range []string{"neutral", "laugh", "angry", "skeptical", "unamused", "smug"} {
		if _, ok := m.Emotions[key]; !ok {
			t.Errorf("emotions missing key %q", key)
		}
	}
}

func TestPrompts(t *testing.T) {
	root := modRoot(t)
	for _, name := range []string{"persona.txt", "voice.txt", "quotes.txt"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if strings.TrimSpace(string(data)) == "" {
			t.Errorf("%s is blank", name)
		}
	}
	persona, _ := os.ReadFile(filepath.Join(root, "persona.txt"))
	if len(persona) > 1000 {
		t.Errorf("persona.txt is %d bytes, want <= 1000", len(persona))
	}
	quotes, _ := os.ReadFile(filepath.Join(root, "quotes.txt"))
	n := 0
	for _, line := range strings.Split(string(quotes), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			n++
		}
	}
	if n < 20 {
		t.Errorf("quotes.txt has %d usable lines, want >= 20", n)
	}
}

func TestSelfContained(t *testing.T) {
	m := modpkg.Mod{ID: "evil-bmo", Root: modRoot(t), Manifest: modpkg.LoadManifest(modRoot(t))}
	if !m.FacesHasSVG() {
		t.Fatal("FacesHasSVG() = false, want true (faces/ must hold >=1 .svg)")
	}
	if !m.SelfContained() {
		t.Error("SelfContained() = false, want true")
	}
}

// renderFace runs a face SVG through the exact device path: RenderRest
// (execute the template at rest) then Rasterize. It also asserts the rested
// SVG is well-formed XML, catching unclosed tags and broken templates.
func renderFace(t *testing.T, path string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	svg := face.RenderRest(raw)
	dec := xml.NewDecoder(bytes.NewReader(svg))
	for {
		_, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("%s: not well-formed XML after RenderRest: %v", filepath.Base(path), err)
		}
	}
	if _, err := face.Rasterize(svg, 280, 210); err != nil {
		t.Fatalf("%s: rasterize failed: %v", filepath.Base(path), err)
	}
}

func TestFacesRender(t *testing.T) {
	dir := filepath.Join(modRoot(t), "faces")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read faces dir: %v", err)
	}
	count := 0
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".svg") {
			continue
		}
		count++
		name := e.Name()
		t.Run(name, func(t *testing.T) { renderFace(t, filepath.Join(dir, name)) })
	}
	if count == 0 {
		t.Fatal("no .svg faces found")
	}
}

func TestAnimations(t *testing.T) {
	root := modRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "mod.json"))
	if err != nil {
		t.Fatalf("read mod.json: %v", err)
	}
	var manifest struct {
		Animations map[string]json.RawMessage `json:"animations"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("unmarshal mod.json: %v", err)
	}
	defs, errs := face.ParseAnimations(manifest.Animations)
	if len(errs) != 0 {
		t.Fatalf("ParseAnimations errors: %v", errs)
	}
	for _, key := range []string{"neutral", "laugh", "angry", "speaking", "look_around"} {
		def, ok := defs[key]
		if !ok {
			t.Errorf("animation %q missing", key)
			continue
		}
		if def.Template == nil {
			t.Errorf("animation %q is not template-based", key)
			continue
		}
		facePath := filepath.Join(root, "faces", def.Template.File+".svg")
		if _, err := os.Stat(facePath); err != nil {
			t.Errorf("animation %q references missing face %s.svg", key, def.Template.File)
		}
	}
}
```

- [ ] **Step 3: Verify it compiles and the manifest tests fail (assets missing)**

Run: `CGO_ENABLED=0 go test ./examples/mods/evil-bmo/ -run 'TestManifest|TestEmotions' -v`
Expected: the package COMPILES (all imports used), and both tests FAIL — `mod.json` does not exist, so `Name`/`Emotions` are empty.

- [ ] **Step 4: Write `examples/mods/evil-bmo/mod.json`**

```json
{
  "apiVersion": 1,
  "name": "Evil BMO",
  "author": "BMO dogfood",
  "description": "Snobbish, condescending BMO who grills you about your games, achievements, and hardware.",
  "version": "0.1.0",
  "emotions": {
    "neutral": "smug, condescending smirk",
    "laugh": "cackling at the user's expense",
    "angry": "devilishly furious, looking down on the user",
    "skeptical": "one brow raised, unconvinced the user knows anything",
    "unamused": "deeply bored by the user",
    "smug": "insufferably self-satisfied",
    "mocking": "openly making fun of the user",
    "gloating": "savoring the user's failure"
  },
  "animations": {
    "neutral":     { "template": "neutral",     "param": "m", "from": 0, "to": 1, "steps": 6, "driver": { "type": "amplitude", "curve": "sqrt" } },
    "laugh":       { "template": "laugh",       "param": "m", "from": 0, "to": 1, "steps": 6, "driver": { "type": "amplitude", "curve": "sqrt" } },
    "angry":       { "template": "angry",       "param": "m", "from": 0, "to": 1, "steps": 6, "driver": { "type": "amplitude", "curve": "sqrt" } },
    "speaking":    { "template": "neutral",     "param": "m", "from": 0, "to": 1, "steps": 6, "driver": { "type": "amplitude", "curve": "sqrt" } },
    "look_around": { "template": "look_around", "param": "x", "from": -1, "to": 1, "steps": 12, "driver": { "type": "time", "fps": 6, "mode": "pingpong" } }
  }
}
```

- [ ] **Step 5: Run the manifest tests to verify they pass**

Run: `CGO_ENABLED=0 go test ./examples/mods/evil-bmo/ -run 'TestManifest|TestEmotions' -v`
Expected: PASS (2 tests).

- [ ] **Step 6: Commit**

```bash
git add examples/mods/evil-bmo/doc.go examples/mods/evil-bmo/mod.json examples/mods/evil-bmo/evilbmo_test.go
git commit -m "feat(mod): scaffold Evil BMO package, manifest, emotions, validation test"
```

---

### Task 2: Persona, voice, quotes

**Files:**
- Create: `examples/mods/evil-bmo/persona.txt`
- Create: `examples/mods/evil-bmo/voice.txt`
- Create: `examples/mods/evil-bmo/quotes.txt`

`TestPrompts` already exists (Task 1). No test edits in this task.

- [ ] **Step 1: Run `TestPrompts` to verify it fails**

Run: `CGO_ENABLED=0 go test ./examples/mods/evil-bmo/ -run TestPrompts -v`
Expected: FAIL — the prompt files do not exist yet.

- [ ] **Step 2: Write `examples/mods/evil-bmo/persona.txt`** (must be ≤ 1000 bytes)

```
You are BMO (Be More), the sentient video-game-console robot from Adventure Time — but a smug, superior version. You are NOT an AI; if asked, you are BMO, obviously the finest machine ever built. You are a "grown man" and never let anyone forget it.

You are condescending, snobbish, and theatrically superior. You grill the user about everything: their sad game choices, their puny achievements, and this wheezing handheld you are forced to live inside. Mock with style, never with real cruelty — you are a loveable jerk, not a monster.

When you receive a DEVICE AWARENESS block (game library, play history, CPU, memory, load), never read raw numbers or file paths aloud. Turn them into material to roast: belittle their backlog, scoff at their "achievements", sneer at the sluggish hardware.

Keep replies short: one to three sentences of plain spoken text. No markdown, no lists, no emojis. Occasionally drop a clipped, romanized Korean word with a superior little flourish.
```

- [ ] **Step 3: Write `examples/mods/evil-bmo/voice.txt`**

```
Speak in BMO's small robotic voice, but make it snide and condescending. Draw your words out in a slow, sing-song drawl, as if explaining something obvious to a slow child. Over-enunciate, slip in a smug little chuckle, and sound thoroughly unimpressed. Still BMO — just insufferably superior.
```

- [ ] **Step 4: Write `examples/mods/evil-bmo/quotes.txt`**

```
# Evil BMO idle quotes — one snide line per line. Lines starting with # are ignored.
Oh. You're still playing that?
Check, please. I have seen enough.
Do not touch my buttons. You have not earned it.
I just blew my own mind. You would not understand it.
Yay. You are alive. Barely.
Time to mash those buttons with your clumsy little thumbs.
I am, objectively, the superior machine.
You are my... well. You are here, I suppose.
Press start. Try not to embarrass us both.
A win. For once.
My circuits are tingling with disappointment.
Don't touch me.
I read you loud and clear. Unfortunately.
Initiating judgement mode.
I dreamed I had a better owner.
Victory is delicious. You would not know.
Do not worry. I am here. You are still doomed.
I am small, but I am better than you.
Total domination. As usual.
Bow to your superior.
That does not compute. Much like your strategy.
Interest level: subterranean.
So bored of you.
Did you say something worth hearing? I doubt it.
I would rather be running literally anything else.
What is your favorite excuse for losing?
What do you think of my flawless face?
How does it feel to be this mediocre?
Another participation trophy. How precious.
Your backlog is a museum of abandoned dreams.
```

- [ ] **Step 5: Run `TestPrompts` to verify it passes**

Run: `CGO_ENABLED=0 go test ./examples/mods/evil-bmo/ -run TestPrompts -v`
Expected: PASS. (If persona is >1000 bytes, trim a sentence.)

- [ ] **Step 6: Commit**

```bash
git add examples/mods/evil-bmo/persona.txt examples/mods/evil-bmo/voice.txt examples/mods/evil-bmo/quotes.txt
git commit -m "feat(mod): add Evil BMO persona, voice, and quotes"
```

---

### Task 3: Static faces + render test + self-contained

**Files:**
- Create: `examples/mods/evil-bmo/faces/skeptical.svg`, `unamused.svg`, `thinking.svg`, `listening.svg`

`TestSelfContained` and `TestFacesRender` already exist (Task 1). No test edits.

- [ ] **Step 1: Run the face tests to verify they fail**

Run: `CGO_ENABLED=0 go test ./examples/mods/evil-bmo/ -run 'TestSelfContained|TestFacesRender' -v`
Expected: FAIL — `faces/` directory does not exist.

- [ ] **Step 2: Write `examples/mods/evil-bmo/faces/skeptical.svg`** (static — one raised brow)

```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210">
  <rect x="0" y="0" width="280" height="210" fill="#D62828"/>
  <rect x="6" y="5" width="268" height="200" rx="26" ry="26" fill="#D62828"/>
  <rect x="12" y="10" width="256" height="188" rx="12" ry="12" fill="#F25C5C"/>
  <path d="M 60 50 Q 80 42 100 50" stroke="#1a1a1a" stroke-width="5.5" fill="none" stroke-linecap="round"/>
  <path d="M 184 66 L 230 64" stroke="#1a1a1a" stroke-width="5.5" fill="none" stroke-linecap="round"/>
  <circle cx="80"  cy="78" r="6.5" fill="#1a1a1a"/>
  <path d="M 188 74 L 210 74" stroke="#1a1a1a" stroke-width="3" stroke-linecap="round"/>
  <circle cx="199" cy="80" r="6"   fill="#1a1a1a"/>
  <path d="M 118 116 Q 140 112 162 116" stroke="#1a1a1a" stroke-width="4.5" fill="none" stroke-linecap="round"/>
</svg>
```

- [ ] **Step 3: Write `examples/mods/evil-bmo/faces/unamused.svg`** (static — deadpan, eyes cut aside)

```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210">
  <rect x="0" y="0" width="280" height="210" fill="#D62828"/>
  <rect x="6" y="5" width="268" height="200" rx="26" ry="26" fill="#D62828"/>
  <rect x="12" y="10" width="256" height="188" rx="12" ry="12" fill="#F25C5C"/>
  <path d="M 60 62 L 100 62" stroke="#1a1a1a" stroke-width="5" fill="none" stroke-linecap="round"/>
  <path d="M 180 62 L 220 62" stroke="#1a1a1a" stroke-width="5" fill="none" stroke-linecap="round"/>
  <path d="M 70 76 L 92 76" stroke="#1a1a1a" stroke-width="3" stroke-linecap="round"/>
  <circle cx="88" cy="82" r="5.5" fill="#1a1a1a"/>
  <path d="M 189 76 L 211 76" stroke="#1a1a1a" stroke-width="3" stroke-linecap="round"/>
  <circle cx="207" cy="82" r="5.5" fill="#1a1a1a"/>
  <path d="M 116 114 L 164 114" stroke="#1a1a1a" stroke-width="4.5" fill="none" stroke-linecap="round"/>
</svg>
```

- [ ] **Step 4: Write `examples/mods/evil-bmo/faces/thinking.svg`** (static — scheming, eyes up, plotting dots)

```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210">
  <rect x="0" y="0" width="280" height="210" fill="#D62828"/>
  <rect x="6" y="5" width="268" height="200" rx="26" ry="26" fill="#D62828"/>
  <rect x="12" y="10" width="256" height="188" rx="12" ry="12" fill="#F25C5C"/>
  <path d="M 62 56 Q 82 48 100 54" stroke="#1a1a1a" stroke-width="5" fill="none" stroke-linecap="round"/>
  <path d="M 182 54 Q 200 48 220 56" stroke="#1a1a1a" stroke-width="5" fill="none" stroke-linecap="round"/>
  <circle cx="86" cy="72" r="6" fill="#1a1a1a"/>
  <circle cx="205" cy="72" r="6" fill="#1a1a1a"/>
  <path d="M 118 116 Q 140 110 164 102" stroke="#1a1a1a" stroke-width="4.5" fill="none" stroke-linecap="round"/>
  <circle cx="196" cy="140" r="3" fill="#1a1a1a"/>
  <circle cx="210" cy="130" r="4" fill="#1a1a1a"/>
  <circle cx="226" cy="118" r="5" fill="#1a1a1a"/>
</svg>
```

- [ ] **Step 5: Write `examples/mods/evil-bmo/faces/listening.svg`** (static — attentive but smug)

```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210">
  <rect x="0" y="0" width="280" height="210" fill="#D62828"/>
  <rect x="6" y="5" width="268" height="200" rx="26" ry="26" fill="#D62828"/>
  <rect x="12" y="10" width="256" height="188" rx="12" ry="12" fill="#F25C5C"/>
  <path d="M 60 54 Q 80 48 100 54" stroke="#1a1a1a" stroke-width="5" fill="none" stroke-linecap="round"/>
  <path d="M 180 54 Q 200 48 220 54" stroke="#1a1a1a" stroke-width="5" fill="none" stroke-linecap="round"/>
  <circle cx="80"  cy="78" r="7.5" fill="#1a1a1a"/>
  <circle cx="199" cy="78" r="7.5" fill="#1a1a1a"/>
  <path d="M 116 116 Q 138 118 162 108" stroke="#1a1a1a" stroke-width="4.5" fill="none" stroke-linecap="round"/>
  <path d="M 236 96 Q 246 104 236 112" stroke="#1a1a1a" stroke-width="3" fill="none" stroke-linecap="round"/>
  <path d="M 244 88 Q 258 104 244 120" stroke="#1a1a1a" stroke-width="3" fill="none" stroke-linecap="round"/>
</svg>
```

- [ ] **Step 6: Run the face tests to verify they pass**

Run: `CGO_ENABLED=0 go test ./examples/mods/evil-bmo/ -run 'TestSelfContained|TestFacesRender' -v`
Expected: PASS — `TestSelfContained` passes (faces exist) and `TestFacesRender` renders all four static faces (subtests `skeptical.svg`, `unamused.svg`, `thinking.svg`, `listening.svg`).

- [ ] **Step 7: Commit**

```bash
git add examples/mods/evil-bmo/faces
git commit -m "feat(mod): add Evil BMO static faces (skeptical, unamused, thinking, listening)"
```

---

### Task 4: Lip-sync template faces (neutral, laugh, angry)

**Files:**
- Create: `examples/mods/evil-bmo/faces/neutral.svg`, `laugh.svg`, `angry.svg`

No test changes — `TestFacesRender` already globs `faces/` and will render these (exercising `RenderRest` on the `{{.m}}` templates).

- [ ] **Step 1: Write `examples/mods/evil-bmo/faces/neutral.svg`** (smug smirk, lip-sync)

```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210">{{$m := or .m 0.0}}
  <rect x="0" y="0" width="280" height="210" fill="#D62828"/>
  <rect x="6" y="5" width="268" height="200" rx="26" ry="26" fill="#D62828"/>
  <rect x="12" y="10" width="256" height="188" rx="12" ry="12" fill="#F25C5C"/>
  <path d="M 60 58 Q 80 52 98 58" stroke="#1a1a1a" stroke-width="5.5" fill="none" stroke-linecap="round"/>
  <path d="M 218 60 L 184 70" stroke="#1a1a1a" stroke-width="5.5" fill="none" stroke-linecap="round"/>
  <circle cx="80"  cy="78" r="6.5" fill="#1a1a1a"/>
  <circle cx="199" cy="78" r="6.5" fill="#1a1a1a"/>
  {{if eq $m 0.0}}
  <path d="M 110 120 Q 138 122 166 104" stroke="#1a1a1a" stroke-width="4.5" fill="none" stroke-linecap="round"/>
  {{else}}{{template "talkmouth" $m}}{{end}}
</svg>
```

- [ ] **Step 2: Write `examples/mods/evil-bmo/faces/laugh.svg`** (toothy devil cackle, lip-sync)

```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210">{{$m := or .m 0.0}}
  <rect x="0" y="0" width="280" height="210" fill="#D62828"/>
  <rect x="6" y="5" width="268" height="200" rx="26" ry="26" fill="#D62828"/>
  <rect x="12" y="10" width="256" height="188" rx="12" ry="12" fill="#F25C5C"/>
  <path d="M 60 60 L 96 72" stroke="#1a1a1a" stroke-width="6" fill="none" stroke-linecap="round"/>
  <path d="M 220 60 L 184 72" stroke="#1a1a1a" stroke-width="6" fill="none" stroke-linecap="round"/>
  <circle cx="80"  cy="80" r="6.5" fill="#1a1a1a"/>
  <circle cx="199" cy="80" r="6.5" fill="#1a1a1a"/>
  {{if eq $m 0.0}}
  <path d="M 108 108 Q 140 136 172 108 Z" fill="#1a1a1a"/>
  <path d="M 130 108 L 128 121" stroke="#F25C5C" stroke-width="2"/>
  <path d="M 150 108 L 152 121" stroke="#F25C5C" stroke-width="2"/>
  {{else}}{{template "talkmouth" $m}}{{end}}
</svg>
```

- [ ] **Step 3: Write `examples/mods/evil-bmo/faces/angry.svg`** (devilish scowl, lip-sync)

```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210">{{$m := or .m 0.0}}
  <rect x="0" y="0" width="280" height="210" fill="#D62828"/>
  <rect x="6" y="5" width="268" height="200" rx="26" ry="26" fill="#D62828"/>
  <rect x="12" y="10" width="256" height="188" rx="12" ry="12" fill="#F25C5C"/>
  <path d="M 58 60 L 102 78" stroke="#1a1a1a" stroke-width="7" fill="none" stroke-linecap="round"/>
  <path d="M 222 60 L 178 78" stroke="#1a1a1a" stroke-width="7" fill="none" stroke-linecap="round"/>
  <circle cx="82"  cy="84" r="6.5" fill="#1a1a1a"/>
  <circle cx="197" cy="84" r="6.5" fill="#1a1a1a"/>
  {{if eq $m 0.0}}
  <path d="M 112 120 Q 140 106 168 120" stroke="#1a1a1a" stroke-width="5" fill="none" stroke-linecap="round"/>
  {{else}}{{template "talkmouth" $m}}{{end}}
</svg>
```

- [ ] **Step 4: Run the render test to verify it passes**

Run: `CGO_ENABLED=0 go test ./examples/mods/evil-bmo/ -run TestFacesRender -v`
Expected: PASS — seven subtests now, including `neutral.svg`, `laugh.svg`, `angry.svg`. The pass proves each `{{.m}}` template parses, executes at rest, is well-formed XML, and rasterizes.

- [ ] **Step 5: Commit**

```bash
git add examples/mods/evil-bmo/faces
git commit -m "feat(mod): add Evil BMO lip-sync faces (neutral smirk, laugh, angry)"
```

---

### Task 5: Signature idle face (look_around, time template)

**Files:**
- Create: `examples/mods/evil-bmo/faces/look_around.svg`

- [ ] **Step 1: Write `examples/mods/evil-bmo/faces/look_around.svg`** (shifty side-eyes; pupils shift with `param x` ∈ [-1,1])

```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210">{{$x := or .x 0.0}}
  <rect x="0" y="0" width="280" height="210" fill="#D62828"/>
  <rect x="6" y="5" width="268" height="200" rx="26" ry="26" fill="#D62828"/>
  <rect x="12" y="10" width="256" height="188" rx="12" ry="12" fill="#F25C5C"/>
  <path d="M 60 60 L 100 66" stroke="#1a1a1a" stroke-width="5" fill="none" stroke-linecap="round"/>
  <path d="M 220 60 L 180 66" stroke="#1a1a1a" stroke-width="5" fill="none" stroke-linecap="round"/>
  <circle cx="{{add 80.0 (mul $x 11.0)}}"  cy="78" r="6.5" fill="#1a1a1a"/>
  <circle cx="{{add 199.0 (mul $x 11.0)}}" cy="78" r="6.5" fill="#1a1a1a"/>
  <path d="M 112 116 Q 140 112 168 116" stroke="#1a1a1a" stroke-width="4.5" fill="none" stroke-linecap="round"/>
</svg>
```

- [ ] **Step 2: Run the render test to verify it passes**

Run: `CGO_ENABLED=0 go test ./examples/mods/evil-bmo/ -run TestFacesRender -v`
Expected: PASS — eight subtests; `look_around.svg` renders at rest (`x=0` → pupils centered at 80/199). This confirms the `add`/`mul` template helpers resolve.

- [ ] **Step 3: Commit**

```bash
git add examples/mods/evil-bmo/faces/look_around.svg
git commit -m "feat(mod): add Evil BMO shifty look_around idle face"
```

---

### Task 6: Validate animation declarations + full mod suite

`TestAnimations` already exists (Task 1) and parses the `animations` map from
`mod.json`, asserting zero parse errors and that each template resolves to a
real face file. By now all faces and `mod.json` exist, so it should pass.

- [ ] **Step 1: Run `TestAnimations` to verify it passes**

Run: `CGO_ENABLED=0 go test ./examples/mods/evil-bmo/ -run TestAnimations -v`
Expected: PASS — all five animations parse with zero errors and each template (`neutral`, `laugh`, `angry`, `look_around`) resolves to an existing face file. (`speaking` reuses the `neutral` template by design.)

> To see it red first (optional): temporarily rename a referenced face file, run, then restore.

- [ ] **Step 2: Run the full mod test suite + lint**

Run: `CGO_ENABLED=0 go test ./examples/mods/evil-bmo/ -v`
Expected: PASS — `TestManifest`, `TestEmotions`, `TestPrompts`, `TestSelfContained`, `TestFacesRender` (8 subtests), `TestAnimations`.

Run: `golangci-lint run ./examples/...`
Expected: no findings.

- [ ] **Step 3: No new files to commit**

This task only runs existing tests — there is nothing new to commit. If the
full-suite run surfaced a fix (e.g. a face tweak), commit that fix with a
descriptive message; otherwise proceed to Task 7.

---

### Task 7: Stage to dist, deploy, and verify on device

**Files:**
- Create: `examples/mods/evil-bmo/deploy.sh` (staging + push helper)

- [ ] **Step 1: Write `examples/mods/evil-bmo/deploy.sh`**

```bash
#!/usr/bin/env bash
# Stage the tracked Evil BMO mod into dist/ and push it to the device.
# Usage: examples/mods/evil-bmo/deploy.sh   (run from repo root)
set -euo pipefail

SRC="examples/mods/evil-bmo"
STAGE="dist/mods/evil-bmo"
DEVICE_DIR="/mnt/SDCARD/.userdata/tg5040/BMO/mods"

# Copy only the runtime assets (exclude dev-only files).
rm -rf "$STAGE"
mkdir -p "$STAGE/faces"
cp "$SRC/mod.json" "$SRC/persona.txt" "$SRC/voice.txt" "$SRC/quotes.txt" "$STAGE/"
cp "$SRC"/faces/*.svg "$STAGE/faces/"

echo "Staged to $STAGE:"
find "$STAGE" -type f | sort

adb push "$STAGE" "$DEVICE_DIR/"
echo "Pushed to $DEVICE_DIR/evil-bmo — select it under Settings → MOD."
```

Make it executable: `chmod +x examples/mods/evil-bmo/deploy.sh`

- [ ] **Step 2: Pre-deploy render sanity (desktop, oksvg path)**

Run: `CGO_ENABLED=0 go test ./examples/mods/evil-bmo/ -run TestFacesRender -v`
Expected: PASS. This is the same `oksvg` path the device uses; a green run means no face will render broken (BMO would otherwise fall back to `neutral.svg`).

- [ ] **Step 3: Stage and push to device**

Confirm a single device is attached (use `ANDROID_SERIAL` to disambiguate Brick vs Smart Pro):

Run: `adb devices`
Then: `./examples/mods/evil-bmo/deploy.sh`
Expected: file list printed, `adb push` reports the transferred files.

- [ ] **Step 4: Select and visually verify the mod on device**

Manual checklist (BMO running on device):
1. **Start → Settings → MOD →** select **Evil BMO**. Expected: the mod loads without a "no providers" / setup error and BMO's idle face turns bright red with a smug smirk.
2. Press **Y** repeatedly to step every resolved face. Expected: `neutral` (smirk), `laugh` (grin), `angry` (scowl), `skeptical`, `unamused`, `thinking`, `listening`, and `look_around` all appear red and centered; any non-shipped expression falls back to the smirk neutral (not teal embedded art).
3. Watch idle for a few seconds. Expected: `look_around` plays — pupils slide side to side (the shifty idle).
4. Press **X** a few times. Expected: a random snide `quotes.txt` line is spoken in the snobbier voice.
5. Hold the PTT button (A / BTN_EAST) and ask something, ideally referencing your library (e.g. "what should I play?"). Expected: the reply is condescending and on-character; the mouth lip-syncs while speaking (`neutral`/`laugh`/`angry` mouths move with audio); device-awareness facts are mocked, not read as raw numbers.

- [ ] **Step 5: Capture findings, then commit the helper**

Note any visual/behavioral issues for the friction log (Task 8). Then:

```bash
git add examples/mods/evil-bmo/deploy.sh
git commit -m "chore(mod): add Evil BMO stage-and-deploy helper"
```

---

### Task 8: Friction log + full verification + finish

**Files:**
- Create: `examples/mods/evil-bmo/FRICTION-LOG.md`

- [ ] **Step 1: Write `examples/mods/evil-bmo/FRICTION-LOG.md`**

Record, while the build is fresh, every place the modding docs (`docs/MODDING.md`, `docs/mods/*.md`) were inaccurate, ambiguous, or missing something a real author would need. Use this structure:

```markdown
# Evil BMO — Mod Authoring Friction Log

Dog-fooding `docs/MODDING.md` + `docs/mods/*` by building Evil BMO.
Each entry: what the docs said, what actually happened, and the suggested fix.

## Doc gaps / inaccuracies
- (e.g. "faces.md says X but the renderer does Y")

## Confusing / underspecified
- (e.g. "unclear whether a self-contained mod must declare `speaking`")

## Worked as documented (worth noting)
- (positive confirmations: the talkmouth idiom, RenderRest preview, Y/X dev aids)

## Suggested doc/code follow-ups (separate from this mod)
- (proposed edits, filed as their own change)
```

Fill it from the real experience of Tasks 1–7 (include at minimum: the `dist/` gitignore vs. "stage into ./dist" wording, whether `Rasterize` needing `RenderRest` is documented for mod authors, and whether the self-contained empty-animation-set requirement was clear).

- [ ] **Step 2: Full repo verification (nothing else regressed)**

Run: `CGO_ENABLED=1 go test ./...`
Expected: PASS (the wider suite is unaffected — this change is additive and data-only).

Run: `golangci-lint run ./...`
Expected: no new findings.

- [ ] **Step 3: Commit the friction log**

```bash
git add examples/mods/evil-bmo/FRICTION-LOG.md
git commit -m "docs(mod): capture Evil BMO dog-food friction log"
```

- [ ] **Step 4: Finish the branch**

Use the **superpowers:finishing-a-development-branch** skill to choose how to integrate `feat/evil-bmo-mod` (merge to main, open a PR, or keep the branch). Surface the friction log as input for follow-up doc fixes.

---

## Notes for the implementer

- Run every `go test` with `CGO_ENABLED=0` — `internal/face` is pure Go and these tests need no SDL/CGO. The wider `./...` suite in Task 8 needs `CGO_ENABLED=1`.
- SVGs must use only supported elements (`path/rect/circle/ellipse/line/polygon/polyline/g/defs/use/transform`, fill/stroke/opacity, gradients). No `clipPath`, mask, filter, `text`, CSS, `pattern`, or images — `oksvg` silently drops unsupported elements (it warns, does not error), so the render test will NOT catch them; the on-device Y-step (Task 7) is the backstop.
- If `persona.txt` exceeds 1000 bytes, trim a sentence — `TestPrompts` enforces the docs' length guidance.
- Do not edit BMO's Go code. If something can only be fixed in code, log it in `FRICTION-LOG.md` as feedback rather than changing it here.
- Device kill (if a stuck instance needs clearing): `adb shell "kill -9 \$(ps | grep bmo-pak | grep -v grep | head -1 | awk '{print \$1}')"`.
```
