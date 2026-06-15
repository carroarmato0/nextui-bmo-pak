# BMO New Expressions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add 25 new BMO face expressions (derived from the community Figma "BMO Face Templates" set, redrawn in our flat element-library style) as embedded SVG assets, wire them into the expression-resolution pipeline, and lock them down with geometry + browser-fidelity tests.

**Architecture:** Each expression is a self-contained `280×210` SVG in `internal/face/assets/`, auto-embedded by the existing `//go:embed assets/*.svg`. `internal/face/expr.go` gains a constant + `CanonicalNames` entry + `Canonical()` alias for each. Two test layers guard the result: a geometry test samples rendered pixels through the real oksvg path at both device resolutions, and a fidelity test asserts each shipped asset is byte-identical to a committed sha256 manifest of the browser-approved artifacts.

**Tech Stack:** Go (pure, no CGO in `internal/face`), `srwiley/oksvg`+`rasterx` renderer, Go `embed`, `crypto/sha256`, standard `testing`.

**Spec:** `docs/specs/2026-06-15-bmo-new-expressions-design.md`

**Canonical palette (do not change):** body `#4ECBA8`, screen `#90e5c8`, features `#1a1a1a`, teeth `#e4e4e4`, mouth interior `#1a7848`, tongue `#16ae81`. New colors: heart/tongue red `#e8443b`, star/sparkle gold `#f4c531`, tears blue `#5bc8e8`, blush green `#53AF66` @0.55.

**The 25 new names (stable order used everywhere):**
`sad, happy, laugh, content, angry, surprised, excited, love, shy` (Tier A);
`crying, teary, gloomy, dizzy, unamused, annoyed, skeptical, playful, kiss, grimace, shout` (Tier B);
`dead, glitch, dismayed, adoring, sparkle` (Tier C).

---

## File Structure

- `internal/face/assets/<name>.svg` — **Create 25.** The approved face artifacts. Full contents in **Appendix A**.
- `internal/face/expr.go` — **Modify.** Add 25 constants, append them to `CanonicalNames`, extend `Canonical()`.
- `internal/face/expr_test.go` — **Modify.** Re-point reassigned aliases, add new-name cases.
- `internal/face/assets_test.go` — **Modify.** Add `TestNewFacesGeometry`.
- `internal/face/fidelity_test.go` — **Create.** Byte-identity + non-blank-raster guards.
- `internal/face/testdata/approved_expressions.json` — **Create.** Frozen `name → sha256` manifest.
- `docs/FACES.md` — **Modify.** Extend the face-catalog table + alias note.

---

## Task 1: Add the 25 approved face assets

**Files:**
- Create: `internal/face/assets/{sad,happy,laugh,content,angry,surprised,excited,love,shy,crying,teary,gloomy,dizzy,unamused,annoyed,skeptical,playful,kiss,grimace,shout,dead,glitch,dismayed,adoring,sparkle}.svg`

- [ ] **Step 1: Create the asset files**

The approved artifacts are in `/tmp/bmo_cand/`. Copy them verbatim:

```bash
cp /tmp/bmo_cand/*.svg internal/face/assets/
```

If `/tmp/bmo_cand/` is unavailable, create each file from its exact contents in **Appendix A** of this plan (byte-for-byte — fidelity depends on it).

- [ ] **Step 2: Verify the asset set**

Run:
```bash
ls internal/face/assets/*.svg | wc -l
```
Expected: `33` (8 existing + 25 new).

- [ ] **Step 3: Verify the package still builds (embed picks up new files)**

Run:
```bash
CGO_ENABLED=0 go build ./...
```
Expected: no output, exit 0.

- [ ] **Step 4: Commit**

```bash
git add internal/face/assets/
git commit -m "feat(face): add 25 new expression SVG assets"
```

---

## Task 2: Wire the expressions into expr.go

**Files:**
- Modify: `internal/face/expr.go`

- [ ] **Step 1: Add the 25 constants**

In `internal/face/expr.go`, extend the `const` block (after `ExprSmile = "smile"`):

```go
const (
	ExprNeutral   = "neutral"
	ExprBlink     = "blink"
	ExprListening = "listening"
	ExprThinking  = "thinking"
	ExprSpeaking  = "speaking"
	ExprSleeping  = "sleeping"
	ExprConcerned = "concerned"
	ExprSmile     = "smile"

	// New expressions (Figma "BMO Face Templates" set).
	ExprSad       = "sad"
	ExprHappy     = "happy"
	ExprLaugh     = "laugh"
	ExprContent   = "content"
	ExprAngry     = "angry"
	ExprSurprised = "surprised"
	ExprExcited   = "excited"
	ExprLove      = "love"
	ExprShy       = "shy"
	ExprCrying    = "crying"
	ExprTeary     = "teary"
	ExprGloomy    = "gloomy"
	ExprDizzy     = "dizzy"
	ExprUnamused  = "unamused"
	ExprAnnoyed   = "annoyed"
	ExprSkeptical = "skeptical"
	ExprPlayful   = "playful"
	ExprKiss      = "kiss"
	ExprGrimace   = "grimace"
	ExprShout     = "shout"
	ExprDead      = "dead"
	ExprGlitch    = "glitch"
	ExprDismayed  = "dismayed"
	ExprAdoring   = "adoring"
	ExprSparkle   = "sparkle"
)
```

- [ ] **Step 2: Append the 25 names to CanonicalNames**

Replace the `CanonicalNames` var with:

```go
// CanonicalNames lists every canonical expression name in a stable order.
var CanonicalNames = []string{
	ExprNeutral, ExprBlink, ExprListening, ExprThinking,
	ExprSpeaking, ExprSleeping, ExprConcerned, ExprSmile,
	// New expressions.
	ExprSad, ExprHappy, ExprLaugh, ExprContent, ExprAngry, ExprSurprised,
	ExprExcited, ExprLove, ExprShy, ExprCrying, ExprTeary, ExprGloomy,
	ExprDizzy, ExprUnamused, ExprAnnoyed, ExprSkeptical, ExprPlayful,
	ExprKiss, ExprGrimace, ExprShout, ExprDead, ExprGlitch, ExprDismayed,
	ExprAdoring, ExprSparkle,
}
```

- [ ] **Step 3: Extend Canonical() with self-maps, reassigned aliases, and a small alias set**

Replace the `Canonical` function with:

```go
// Canonical maps any expression alias the assistant may emit to its canonical
// face name. Unknown inputs fall back to neutral.
func Canonical(expr string) string {
	switch strings.ToLower(strings.TrimSpace(expr)) {
	case "", "idle", ExprNeutral:
		return ExprNeutral
	case ExprBlink:
		return ExprBlink
	case "asleep", "sleep", ExprSleeping:
		return ExprSleeping
	// System states keep their meaning.
	case "error", "confused", ExprConcerned:
		return ExprConcerned
	case ExprListening:
		return ExprListening
	case ExprThinking:
		return ExprThinking
	case ExprSpeaking:
		return ExprSpeaking
	case ExprSmile:
		return ExprSmile
	// Expressions that used to alias onto smile/concerned now resolve to their
	// own assets.
	case ExprHappy:
		return ExprHappy
	case ExprLaugh:
		return ExprLaugh
	case ExprExcited:
		return ExprExcited
	case ExprSad:
		return ExprSad
	case ExprAngry:
		return ExprAngry
	case ExprContent:
		return ExprContent
	case "surprised", "shocked", "surprise":
		return ExprSurprised
	case ExprLove:
		return ExprLove
	case ExprShy:
		return ExprShy
	case "crying", "cry":
		return ExprCrying
	case ExprTeary:
		return ExprTeary
	case ExprGloomy:
		return ExprGloomy
	case ExprDizzy:
		return ExprDizzy
	case ExprUnamused:
		return ExprUnamused
	case ExprAnnoyed:
		return ExprAnnoyed
	case ExprSkeptical:
		return ExprSkeptical
	case "playful", "tongue":
		return ExprPlayful
	case "kiss", "kissing":
		return ExprKiss
	case ExprGrimace:
		return ExprGrimace
	case ExprShout:
		return ExprShout
	case ExprDead:
		return ExprDead
	case ExprGlitch:
		return ExprGlitch
	case ExprDismayed:
		return ExprDismayed
	case ExprAdoring:
		return ExprAdoring
	case "sparkle", "sparkles":
		return ExprSparkle
	default:
		return ExprNeutral
	}
}
```

- [ ] **Step 4: Verify it compiles**

Run:
```bash
CGO_ENABLED=0 go build ./...
```
Expected: no output, exit 0.

- [ ] **Step 5: Commit**

```bash
git add internal/face/expr.go
git commit -m "feat(face): register 25 new expressions and aliases"
```

---

## Task 3: Update expr_test.go for the reassigned aliases + new names

**Files:**
- Modify: `internal/face/expr_test.go`

- [ ] **Step 1: Replace the test table**

In `internal/face/expr_test.go`, replace the `tests` slice in `TestCanonical` with:

```go
	tests := []struct {
		in   string
		want string
	}{
		{"", ExprNeutral},
		{"idle", ExprNeutral},
		{"neutral", ExprNeutral},
		{" Neutral ", ExprNeutral},
		{"blink", ExprBlink},
		{"asleep", ExprSleeping},
		{"sleep", ExprSleeping},
		{"sleeping", ExprSleeping},
		{"error", ExprConcerned},
		{"confused", ExprConcerned},
		{"concerned", ExprConcerned},
		{"smile", ExprSmile},
		{"listening", ExprListening},
		{"thinking", ExprThinking},
		{"speaking", ExprSpeaking},
		{"look_around", ExprNeutral},
		// Reassigned: these no longer fold into smile/concerned.
		{"happy", ExprHappy},
		{"laugh", ExprLaugh},
		{"excited", ExprExcited},
		{"sad", ExprSad},
		{"angry", ExprAngry},
		// New canonical names map to themselves.
		{"content", ExprContent},
		{"surprised", ExprSurprised},
		{"love", ExprLove},
		{"shy", ExprShy},
		{"crying", ExprCrying},
		{"teary", ExprTeary},
		{"gloomy", ExprGloomy},
		{"dizzy", ExprDizzy},
		{"unamused", ExprUnamused},
		{"annoyed", ExprAnnoyed},
		{"skeptical", ExprSkeptical},
		{"playful", ExprPlayful},
		{"kiss", ExprKiss},
		{"grimace", ExprGrimace},
		{"shout", ExprShout},
		{"dead", ExprDead},
		{"glitch", ExprGlitch},
		{"dismayed", ExprDismayed},
		{"adoring", ExprAdoring},
		{"sparkle", ExprSparkle},
		// A few new aliases.
		{"shocked", ExprSurprised},
		{"cry", ExprCrying},
		{"tongue", ExprPlayful},
		{"kissing", ExprKiss},
		{"sparkles", ExprSparkle},
	}
```

- [ ] **Step 2: Run the test**

Run:
```bash
CGO_ENABLED=0 go test ./internal/face/ -run TestCanonical -v
```
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/face/expr_test.go
git commit -m "test(face): cover reassigned aliases and new expression names"
```

---

## Task 4: Add the geometry test

**Files:**
- Modify: `internal/face/assets_test.go`

This adds a second test function alongside the existing `TestEmbeddedFacesGeometry`, reusing the `assertColor` helper from `raster_test.go`. Each sample point lands on a solid feature (filled shape or stroke centerline) whose color we can predict from the SVG source; `assertColor` allows ±8 per channel.

- [ ] **Step 1: Append the geometry test**

Add to the end of `internal/face/assets_test.go`:

```go
func TestNewFacesGeometry(t *testing.T) {
	d := [3]uint8{0x1a, 0x1a, 0x1a}  // features
	mi := [3]uint8{0x1a, 0x78, 0x48} // mouth interior
	gd := [3]uint8{0xf4, 0xc5, 0x31} // star/sparkle gold
	rd := [3]uint8{0xe8, 0x44, 0x3b} // heart/tongue red
	th := [3]uint8{0xe4, 0xe4, 0xe4} // teeth

	type point struct {
		x, y  float64
		c     [3]uint8
		label string
	}
	cases := map[string][]point{
		ExprSad:       {{80, 78, d, "left dot eye"}, {199, 78, d, "right dot eye"}},
		ExprHappy:     {{80, 78, d, "left dot eye"}, {199, 78, d, "right dot eye"}},
		ExprLaugh:     {{80, 76, d, "left arc eye"}, {199, 76, d, "right arc eye"}, {140, 135, mi, "mouth interior"}},
		ExprContent:   {{80, 80, d, "left arc eye"}, {199, 80, d, "right arc eye"}},
		ExprAngry:     {{80, 78, d, "left dot eye"}, {199, 78, d, "right dot eye"}},
		ExprSurprised: {{80, 78, d, "left big eye"}, {199, 78, d, "right big eye"}, {140, 121, mi, "mouth interior"}},
		ExprExcited:   {{80, 78, gd, "left star eye"}, {199, 78, gd, "right star eye"}},
		ExprLove:      {{80, 80, rd, "left heart eye"}, {199, 80, rd, "right heart eye"}},
		ExprShy:       {{80, 78, d, "left dot eye"}, {199, 78, d, "right dot eye"}},
		ExprCrying:    {{80, 76, d, "left arc eye"}, {199, 76, d, "right arc eye"}, {140, 127, mi, "mouth interior"}},
		ExprTeary:     {{80, 78, d, "left big eye"}, {199, 78, d, "right big eye"}},
		ExprGloomy:    {{80, 73, d, "left arc eye"}, {199, 73, d, "right arc eye"}},
		ExprDizzy:     {{89, 76, d, "left spiral"}, {208, 76, d, "right spiral"}},
		ExprUnamused:  {{80, 81, d, "left eye dot"}, {199, 81, d, "right eye dot"}, {140, 118, d, "flat mouth"}},
		ExprAnnoyed:   {{80, 78, d, "left dash eye"}, {199, 78, d, "right dash eye"}, {140, 118, d, "dash mouth"}},
		ExprSkeptical: {{80, 78, d, "left dot eye"}, {199, 81, d, "right eye dot"}},
		ExprPlayful:   {{80, 78, d, "left dot eye"}, {140, 129, rd, "tongue"}},
		ExprKiss:      {{88, 78, d, "left > eye"}, {194, 78, d, "right < eye"}},
		ExprGrimace:   {{80, 78, d, "left dash eye"}, {199, 78, d, "right dash eye"}, {118, 116, th, "teeth"}},
		ExprShout:     {{80, 78, d, "left dot eye"}, {199, 78, d, "right dot eye"}, {140, 130, mi, "mouth interior"}},
		ExprDead:      {{80, 78, d, "left x eye"}, {199, 78, d, "right x eye"}, {140, 118, d, "flat mouth"}},
		ExprGlitch:    {{76, 74, d, "left pixel eye"}, {195, 74, d, "right pixel eye"}, {144, 131, d, "pixel mouth"}},
		ExprDismayed:  {{80, 78, d, "left big eye"}, {199, 78, d, "right big eye"}, {140, 124, mi, "gasp mouth interior"}},
		ExprAdoring:   {{80, 78, d, "left big eye"}, {199, 78, d, "right big eye"}},
		ExprSparkle:   {{80, 78, gd, "left sparkle eye"}, {199, 78, gd, "right sparkle eye"}},
	}
	for _, size := range [][2]int{{1024, 768}, {1280, 720}} {
		w, h := size[0], size[1]
		for name, points := range cases {
			data, ok := defaultBytes(name)
			if !ok {
				t.Fatalf("no embedded SVG for %q", name)
			}
			buf, err := Rasterize(data, w, h)
			if err != nil {
				t.Fatalf("rasterize %s at %dx%d: %v", name, w, h, err)
			}
			for _, p := range points {
				assertColor(t, buf, w, h, p.x, p.y, p.c[0], p.c[1], p.c[2], name+": "+p.label)
			}
		}
	}
}
```

- [ ] **Step 2: Run the test**

Run:
```bash
CGO_ENABLED=0 go test ./internal/face/ -run TestNewFacesGeometry -v
```
Expected: PASS.

If a single point fails because it landed on an anti-aliased edge, nudge it 1–2 viewBox units toward the center of that same feature (do **not** change the expected color, and do **not** weaken the test by deleting points). The fidelity test in Task 5 is the authoritative guarantee that the shipped bytes equal the approved artifact; this test verifies they rasterize at the expected place/color through oksvg.

- [ ] **Step 3: Commit**

```bash
git add internal/face/assets_test.go
git commit -m "test(face): geometry assertions for 25 new expressions"
```

---

## Task 5: Add the browser-fidelity test + frozen manifest

**Files:**
- Create: `internal/face/testdata/approved_expressions.json`
- Create: `internal/face/fidelity_test.go`

- [ ] **Step 1: Generate the frozen sha256 manifest from the embedded assets**

The assets in `internal/face/assets/` were copied verbatim from the browser-approved artifacts in Task 1, so their hashes capture the approved state. Generate the manifest:

```bash
mkdir -p internal/face/testdata
python3 - <<'EOF'
import hashlib, json
names = [
    "sad","happy","laugh","content","angry","surprised","excited","love","shy",
    "crying","teary","gloomy","dizzy","unamused","annoyed","skeptical","playful",
    "kiss","grimace","shout","dead","glitch","dismayed","adoring","sparkle",
]
out = {}
for n in names:
    with open(f"internal/face/assets/{n}.svg", "rb") as f:
        out[n] = hashlib.sha256(f.read()).hexdigest()
with open("internal/face/testdata/approved_expressions.json", "w") as f:
    json.dump(out, f, indent=2, sort_keys=True)
    f.write("\n")
print("wrote", len(out), "entries")
EOF
```
Expected: `wrote 25 entries`.

- [ ] **Step 2: Write the fidelity test**

Create `internal/face/fidelity_test.go`:

```go
package face

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

// newExpressions are the 25 faces added from the Figma "BMO Face Templates"
// set, in the same stable order as CanonicalNames. Each shipped asset must
// stay byte-identical to the artifact approved in the browser preview.
var newExpressions = []string{
	ExprSad, ExprHappy, ExprLaugh, ExprContent, ExprAngry, ExprSurprised,
	ExprExcited, ExprLove, ExprShy, ExprCrying, ExprTeary, ExprGloomy,
	ExprDizzy, ExprUnamused, ExprAnnoyed, ExprSkeptical, ExprPlayful,
	ExprKiss, ExprGrimace, ExprShout, ExprDead, ExprGlitch, ExprDismayed,
	ExprAdoring, ExprSparkle,
}

// TestNewExpressionFidelity guards that every shipped face is byte-identical to
// the frozen, browser-approved baseline. Byte-identity is used (instead of a
// golden render hash) so the check is deterministic across machines, Go, and
// oksvg versions; because the browser preview rendered these exact SVG bytes,
// byte-fidelity to them is fidelity to what was approved.
func TestNewExpressionFidelity(t *testing.T) {
	raw, err := os.ReadFile("testdata/approved_expressions.json")
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var approved map[string]string
	if err := json.Unmarshal(raw, &approved); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	if len(approved) != len(newExpressions) {
		t.Fatalf("manifest has %d entries, want %d", len(approved), len(newExpressions))
	}
	for _, name := range newExpressions {
		data, ok := defaultBytes(name)
		if !ok {
			t.Errorf("%s: no embedded SVG", name)
			continue
		}
		want, ok := approved[name]
		if !ok {
			t.Errorf("%s: missing from approved manifest", name)
			continue
		}
		sum := sha256.Sum256(data)
		got := hex.EncodeToString(sum[:])
		if got != want {
			t.Errorf("%s: sha256 %s != approved %s — asset no longer matches the browser-approved preview", name, got, want)
		}
	}
}

// TestNewExpressionsRasterize confirms every new asset renders non-blank through
// the real device path (oksvg) at both device resolutions. Rasterize returns an
// error on blank output, so a successful call proves visible pixels.
func TestNewExpressionsRasterize(t *testing.T) {
	for _, size := range [][2]int{{1024, 768}, {1280, 720}} {
		w, h := size[0], size[1]
		for _, name := range newExpressions {
			data, ok := defaultBytes(name)
			if !ok {
				t.Fatalf("no embedded SVG for %q", name)
			}
			if _, err := Rasterize(data, w, h); err != nil {
				t.Errorf("rasterize %s at %dx%d: %v", name, w, h, err)
			}
		}
	}
}
```

- [ ] **Step 3: Run the fidelity tests**

Run:
```bash
CGO_ENABLED=0 go test ./internal/face/ -run 'TestNewExpressionFidelity|TestNewExpressionsRasterize' -v
```
Expected: PASS (both).

- [ ] **Step 4: Commit**

```bash
git add internal/face/testdata/approved_expressions.json internal/face/fidelity_test.go
git commit -m "test(face): browser-fidelity (byte-identity) + raster guard for new faces"
```

---

## Task 6: Document the new faces

**Files:**
- Modify: `docs/FACES.md`

- [ ] **Step 1: Extend the face-catalog table**

In `docs/FACES.md`, replace the existing catalog table (the block starting `| File | Expression | When shown |`) with:

```markdown
| File | Expression | When shown |
|------|-----------|------------|
| `neutral.svg` | Idle / default | Waiting for input |
| `blink.svg` | Blink | Periodic eye blink |
| `listening.svg` | Listening | PTT recording active |
| `thinking.svg` | Thinking | AI processing |
| `speaking.svg` | Speaking | TTS playback |
| `sleeping.svg` | Sleeping | Quota exhausted |
| `concerned.svg` | Concerned | Error / setup required |
| `smile.svg` | Smile | Gentle smile |
| `happy.svg` | Happy | Wide grin |
| `laugh.svg` | Laughing | Squint eyes, open mouth |
| `content.svg` | Content | Calm, eyes closed |
| `sad.svg` | Sad | Downturned mouth |
| `angry.svg` | Angry | Furrowed brows |
| `surprised.svg` | Surprised | Wide eyes, small "o" mouth |
| `excited.svg` | Excited | Gold star eyes |
| `love.svg` | Love | Red heart eyes |
| `shy.svg` | Shy | Blush, wavy mouth |
| `crying.svg` | Crying | Tear streams, wail |
| `teary.svg` | Teary | Welling eyes, worried brows |
| `gloomy.svg` | Gloomy | Downcast eyes, sweat drop |
| `dizzy.svg` | Dizzy | Spiral eyes |
| `unamused.svg` | Unamused | Half-lidded eyes, flat mouth |
| `annoyed.svg` | Annoyed | `-_-` dash eyes/mouth |
| `skeptical.svg` | Skeptical | One raised brow, half-lid |
| `playful.svg` | Playful | Wink, tongue out |
| `kiss.svg` | Kiss | `>` `<` eyes, `3` mouth |
| `grimace.svg` | Grimace | Clenched teeth |
| `shout.svg` | Shout | Angry brows, big open mouth |
| `dead.svg` | Dead / KO | `x_x` eyes |
| `glitch.svg` | Glitch | 8-bit pixel face |
| `dismayed.svg` | Dismayed | Wide eyes, `D:` gasp |
| `adoring.svg` | Adoring | Shiny eyes, blush, sparkles |
| `sparkle.svg` | Sparkle | Gold 4-point sparkle eyes |
```

- [ ] **Step 2: Update the alias note**

In the `### Alias names` section, replace the example paragraph with:

```markdown
You can also use alias filenames. For example, `cry.svg` resolves to `crying`,
`shocked.svg` to `surprised`, and `tongue.svg` to `playful` when no exact
override exists. The lookup order is: exact filename → canonical name → built-in
default. (`happy`, `laugh`, `excited`, `sad`, and `angry` are now their own
expressions, not aliases of `smile`/`concerned`.)
```

- [ ] **Step 3: Commit**

```bash
git add docs/FACES.md
git commit -m "docs(face): catalog the 25 new expressions"
```

---

## Task 7: Full verification

**Files:** none (verification only).

- [ ] **Step 1: Run the full test suite**

Run:
```bash
CGO_ENABLED=0 go test ./...
```
Expected: all packages PASS (`ok` for `internal/face` and everything else; no FAIL).

- [ ] **Step 2: Run the race-enabled face tests**

Run:
```bash
CGO_ENABLED=1 go test -race ./internal/face/
```
Expected: PASS, no race warnings.

- [ ] **Step 3: Lint**

Run:
```bash
golangci-lint run ./...
```
Expected: no new findings.

- [ ] **Step 4: Final build**

Run:
```bash
CGO_ENABLED=0 go build ./...
```
Expected: clean.

- [ ] **Step 5: Commit (only if Steps 1–4 surfaced fixups)**

```bash
git add -A
git commit -m "chore(face): verification fixups for new expressions"
```

---

## Appendix A: Approved face SVG sources

Use these byte-for-byte if `/tmp/bmo_cand/` is unavailable. All share the same
body/screen header (palette preserved); only the feature elements differ.

### sad.svg
```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210">
  <rect x="0" y="0" width="280" height="210" fill="#4ECBA8"/>
  <rect x="6" y="5" width="268" height="200" rx="26" ry="26" fill="#4ECBA8"/>
  <rect x="12" y="10" width="256" height="188" rx="12" ry="12" fill="#90e5c8"/>
  <circle cx="80" cy="78" r="6.5" fill="#1a1a1a"/>
  <circle cx="199" cy="78" r="6.5" fill="#1a1a1a"/>
  <path d="M 116 113 Q 140 96 160 113" stroke="#1a1a1a" stroke-width="4" fill="none" stroke-linecap="round"/>
</svg>
```

### happy.svg
```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210">
  <rect x="0" y="0" width="280" height="210" fill="#4ECBA8"/>
  <rect x="6" y="5" width="268" height="200" rx="26" ry="26" fill="#4ECBA8"/>
  <rect x="12" y="10" width="256" height="188" rx="12" ry="12" fill="#90e5c8"/>
  <circle cx="80" cy="78" r="6.5" fill="#1a1a1a"/>
  <circle cx="199" cy="78" r="6.5" fill="#1a1a1a"/>
  <path d="M 108 111 Q 140 134 172 111" stroke="#1a1a1a" stroke-width="5" fill="none" stroke-linecap="round"/>
</svg>
```

### laugh.svg
```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210">
  <rect x="0" y="0" width="280" height="210" fill="#4ECBA8"/>
  <rect x="6" y="5" width="268" height="200" rx="26" ry="26" fill="#4ECBA8"/>
  <rect x="12" y="10" width="256" height="188" rx="12" ry="12" fill="#90e5c8"/>
  <path d="M 61 85 Q 80 66 99 85" stroke="#1a1a1a" stroke-width="7" fill="none" stroke-linecap="round"/>
  <path d="M 180 85 Q 199 66 218 85" stroke="#1a1a1a" stroke-width="7" fill="none" stroke-linecap="round"/>
  <rect x="98" y="108" width="84" height="42" rx="20" ry="20" fill="#1a1a1a"/>
  <path d="M 99.67 120 A 20 20 0 0 1 118 108 L 162 108 A 20 20 0 0 1 180.33 120 Z" fill="#e4e4e4"/>
  <path d="M 99.67 120 L 180.33 120 A 20 20 0 0 1 182 128 L 182 130 A 20 20 0 0 1 162 150 L 118 150 A 20 20 0 0 1 98 130 L 98 128 A 20 20 0 0 1 99.67 120 Z" fill="#1a7848"/>
  <path d="M 116 150 A 24 8 0 0 1 164 150 Z" fill="#16ae81"/>
</svg>
```

### content.svg
```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210">
  <rect x="0" y="0" width="280" height="210" fill="#4ECBA8"/>
  <rect x="6" y="5" width="268" height="200" rx="26" ry="26" fill="#4ECBA8"/>
  <rect x="12" y="10" width="256" height="188" rx="12" ry="12" fill="#90e5c8"/>
  <path d="M 61 73 Q 80 88 99 73" stroke="#1a1a1a" stroke-width="7" fill="none" stroke-linecap="round"/>
  <path d="M 180 73 Q 199 88 218 73" stroke="#1a1a1a" stroke-width="7" fill="none" stroke-linecap="round"/>
  <path d="M 116 111 Q 140 125 160 111" stroke="#1a1a1a" stroke-width="4" fill="none" stroke-linecap="round"/>
</svg>
```

### angry.svg
```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210">
  <rect x="0" y="0" width="280" height="210" fill="#4ECBA8"/>
  <rect x="6" y="5" width="268" height="200" rx="26" ry="26" fill="#4ECBA8"/>
  <rect x="12" y="10" width="256" height="188" rx="12" ry="12" fill="#90e5c8"/>
  <path d="M 60 60 L 96 74" stroke="#1a1a1a" stroke-width="5" stroke-linecap="round"/>
  <path d="M 183 74 L 219 60" stroke="#1a1a1a" stroke-width="5" stroke-linecap="round"/>
  <circle cx="80" cy="78" r="6.5" fill="#1a1a1a"/>
  <circle cx="199" cy="78" r="6.5" fill="#1a1a1a"/>
  <path d="M 116 113 Q 140 99 160 113" stroke="#1a1a1a" stroke-width="4" fill="none" stroke-linecap="round"/>
</svg>
```

### surprised.svg
```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210">
  <rect x="0" y="0" width="280" height="210" fill="#4ECBA8"/>
  <rect x="6" y="5" width="268" height="200" rx="26" ry="26" fill="#4ECBA8"/>
  <rect x="12" y="10" width="256" height="188" rx="12" ry="12" fill="#90e5c8"/>
  <circle cx="80" cy="78" r="10" fill="#1a1a1a"/>
  <circle cx="77" cy="75" r="3" fill="#e4e4e4"/>
  <circle cx="199" cy="78" r="10" fill="#1a1a1a"/>
  <circle cx="196" cy="75" r="3" fill="#e4e4e4"/>
  <ellipse cx="140" cy="121" rx="10.5" ry="13" fill="#1a1a1a"/>
  <ellipse cx="140" cy="121.3" rx="8.5" ry="11" fill="#1a7848"/>
</svg>
```

### excited.svg
```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210">
  <rect x="0" y="0" width="280" height="210" fill="#4ECBA8"/>
  <rect x="6" y="5" width="268" height="200" rx="26" ry="26" fill="#4ECBA8"/>
  <rect x="12" y="10" width="256" height="188" rx="12" ry="12" fill="#90e5c8"/>
  <path d="M 80.0 65.0 L 83.1 73.8 L 92.4 74.0 L 84.9 79.6 L 87.6 88.5 L 80.0 83.2 L 72.4 88.5 L 75.1 79.6 L 67.6 74.0 L 76.9 73.8 Z" fill="#f4c531"/>
  <path d="M 199.0 65.0 L 202.1 73.8 L 211.4 74.0 L 203.9 79.6 L 206.6 88.5 L 199.0 83.2 L 191.4 88.5 L 194.1 79.6 L 186.6 74.0 L 195.9 73.8 Z" fill="#f4c531"/>
  <rect x="98" y="101" width="84" height="43" rx="20" ry="20" fill="#1a1a1a"/>
  <path d="M 99.67 113 A 20 20 0 0 1 118 101 L 162 101 A 20 20 0 0 1 180.33 113 Z" fill="#e4e4e4"/>
  <path d="M 99.67 113 L 180.33 113 A 20 20 0 0 1 182 121 L 182 124 A 20 20 0 0 1 162 144 L 118 144 A 20 20 0 0 1 98 124 L 98 121 A 20 20 0 0 1 99.67 113 Z" fill="#1a7848"/>
  <path d="M 116 144 A 24 8 0 0 1 164 144 Z" fill="#16ae81"/>
</svg>
```

### love.svg
```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210">
  <rect x="0" y="0" width="280" height="210" fill="#4ECBA8"/>
  <rect x="6" y="5" width="268" height="200" rx="26" ry="26" fill="#4ECBA8"/>
  <rect x="12" y="10" width="256" height="188" rx="12" ry="12" fill="#90e5c8"/>
  <path d="M 80 89.4 C 66.8 75.6 68.6 66.0 80 73.8 C 91.4 66.0 93.2 75.6 80 89.4 Z" fill="#e8443b"/>
  <path d="M 199 89.4 C 185.8 75.6 187.6 66.0 199 73.8 C 210.4 66.0 212.2 75.6 199 89.4 Z" fill="#e8443b"/>
  <path d="M 116 111 Q 140 125 160 111" stroke="#1a1a1a" stroke-width="4" fill="none" stroke-linecap="round"/>
</svg>
```

### shy.svg
```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210">
  <rect x="0" y="0" width="280" height="210" fill="#4ECBA8"/>
  <rect x="6" y="5" width="268" height="200" rx="26" ry="26" fill="#4ECBA8"/>
  <rect x="12" y="10" width="256" height="188" rx="12" ry="12" fill="#90e5c8"/>
  <ellipse cx="61" cy="100" rx="12" ry="7.5" fill="#53AF66" opacity="0.55"/>
  <ellipse cx="219" cy="100" rx="12" ry="7.5" fill="#53AF66" opacity="0.55"/>
  <circle cx="80" cy="78" r="6.5" fill="#1a1a1a"/>
  <circle cx="199" cy="78" r="6.5" fill="#1a1a1a"/>
  <path d="M 128 116 q 6 -6 12 0 q 6 6 12 0" stroke="#1a1a1a" stroke-width="4" fill="none" stroke-linecap="round"/>
</svg>
```

### crying.svg
```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210">
  <rect x="0" y="0" width="280" height="210" fill="#4ECBA8"/>
  <rect x="6" y="5" width="268" height="200" rx="26" ry="26" fill="#4ECBA8"/>
  <rect x="12" y="10" width="256" height="188" rx="12" ry="12" fill="#90e5c8"/>
  <path d="M 61 85 Q 80 68 99 85" stroke="#1a1a1a" stroke-width="7" fill="none" stroke-linecap="round"/>
  <path d="M 180 85 Q 199 68 218 85" stroke="#1a1a1a" stroke-width="7" fill="none" stroke-linecap="round"/>
  <path d="M 76 90.0 C 82.0 97.0 80.2 103.3 76 103.3 C 71.8 103.3 70.0 97.0 76 90.0 Z" fill="#5bc8e8"/>
  <path d="M 76 105.3 C 82.0 112.3 80.2 118.6 76 118.6 C 71.8 118.6 70.0 112.3 76 105.3 Z" fill="#5bc8e8"/>
  <path d="M 203 90.0 C 208.9 97.0 207.2 103.3 203 103.3 C 198.8 103.3 197.1 97.0 203 90.0 Z" fill="#5bc8e8"/>
  <path d="M 203 105.3 C 208.9 112.3 207.2 118.6 203 118.6 C 198.8 118.6 197.1 112.3 203 105.3 Z" fill="#5bc8e8"/>
  <ellipse cx="140" cy="126" rx="13" ry="14" fill="#1a1a1a"/>
  <ellipse cx="140" cy="127" rx="10" ry="11" fill="#1a7848"/>
</svg>
```

### teary.svg
```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210">
  <rect x="0" y="0" width="280" height="210" fill="#4ECBA8"/>
  <rect x="6" y="5" width="268" height="200" rx="26" ry="26" fill="#4ECBA8"/>
  <rect x="12" y="10" width="256" height="188" rx="12" ry="12" fill="#90e5c8"/>
  <path d="M 64 70 L 96 58" stroke="#1a1a1a" stroke-width="5" stroke-linecap="round"/>
  <path d="M 183 58 L 215 70" stroke="#1a1a1a" stroke-width="5" stroke-linecap="round"/>
  <circle cx="80" cy="78" r="12" fill="#1a1a1a"/>
  <circle cx="76.5" cy="74.5" r="3.2" fill="#e4e4e4"/>
  <circle cx="199" cy="78" r="12" fill="#1a1a1a"/>
  <circle cx="195.5" cy="74.5" r="3.2" fill="#e4e4e4"/>
  <path d="M 80 93 C 84.2 98.0 83.0 102.5 80 102.5 C 77.0 102.5 75.8 98.0 80 93 Z" fill="#5bc8e8"/>
  <path d="M 199 93 C 203.2 98.0 202.0 102.5 199 102.5 C 196.0 102.5 194.8 98.0 199 93 Z" fill="#5bc8e8"/>
  <path d="M 128 116 q 6 -6 12 0 q 6 6 12 0" stroke="#1a1a1a" stroke-width="4" fill="none" stroke-linecap="round"/>
</svg>
```

### gloomy.svg
```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210">
  <rect x="0" y="0" width="280" height="210" fill="#4ECBA8"/>
  <rect x="6" y="5" width="268" height="200" rx="26" ry="26" fill="#4ECBA8"/>
  <rect x="12" y="10" width="256" height="188" rx="12" ry="12" fill="#90e5c8"/>
  <path d="M 61 80 Q 80 66 99 80" stroke="#1a1a1a" stroke-width="7" fill="none" stroke-linecap="round"/>
  <path d="M 180 80 Q 199 66 218 80" stroke="#1a1a1a" stroke-width="7" fill="none" stroke-linecap="round"/>
  <path d="M 214 52 C 219.1 58.0 217.6 63.4 214 63.4 C 210.4 63.4 208.9 58.0 214 52 Z" fill="#5bc8e8"/>
  <path d="M 116 113 Q 140 104 160 113" stroke="#1a1a1a" stroke-width="4" fill="none" stroke-linecap="round"/>
</svg>
```

### dizzy.svg
```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210">
  <rect x="0" y="0" width="280" height="210" fill="#4ECBA8"/>
  <rect x="6" y="5" width="268" height="200" rx="26" ry="26" fill="#4ECBA8"/>
  <rect x="12" y="10" width="256" height="188" rx="12" ry="12" fill="#90e5c8"/>
  <path d="M 80.0 78.0 L 80.2 78.1 L 80.4 78.3 L 80.4 78.6 L 80.2 79.0 L 79.8 79.2 L 79.3 79.3 L 78.7 79.2 L 78.2 78.8 L 77.8 78.1 L 77.6 77.3 L 77.8 76.4 L 78.3 75.5 L 79.2 74.9 L 80.3 74.5 L 81.6 74.6 L 82.8 75.1 L 83.8 76.1 L 84.5 77.5 L 84.6 79.1 L 84.2 80.7 L 83.2 82.2 L 81.7 83.2 L 79.8 83.7 L 77.8 83.6 L 75.9 82.7 L 74.3 81.2 L 73.3 79.2 L 73.1 76.8 L 73.7 74.4 L 75.1 72.3 L 77.2 70.8 L 79.8 70.0 L 82.5 70.2 L 85.2 71.3 L 87.4 73.3 L 88.8 76.0 L 89.2 79.1 L 88.5 82.2 L 86.8 85.0 L 84.2 87.1 L 80.9 88.2 L 77.3 88.2 L 73.9 86.9 L 71.1 84.5" stroke="#1a1a1a" stroke-width="3" fill="none" stroke-linecap="round"/>
  <path d="M 199.0 78.0 L 199.2 78.1 L 199.4 78.3 L 199.4 78.6 L 199.2 79.0 L 198.8 79.2 L 198.3 79.3 L 197.7 79.2 L 197.2 78.8 L 196.8 78.1 L 196.6 77.3 L 196.8 76.4 L 197.3 75.5 L 198.2 74.9 L 199.3 74.5 L 200.6 74.6 L 201.8 75.1 L 202.8 76.1 L 203.5 77.5 L 203.6 79.1 L 203.2 80.7 L 202.2 82.2 L 200.7 83.2 L 198.8 83.7 L 196.8 83.6 L 194.9 82.7 L 193.3 81.2 L 192.3 79.2 L 192.1 76.8 L 192.7 74.4 L 194.1 72.3 L 196.2 70.8 L 198.8 70.0 L 201.5 70.2 L 204.2 71.3 L 206.4 73.3 L 207.8 76.0 L 208.2 79.1 L 207.5 82.2 L 205.8 85.0 L 203.2 87.1 L 199.9 88.2 L 196.3 88.2 L 192.9 86.9 L 190.1 84.5" stroke="#1a1a1a" stroke-width="3" fill="none" stroke-linecap="round"/>
  <path d="M 128 116 q 6 -6 12 0 q 6 6 12 0" stroke="#1a1a1a" stroke-width="4" fill="none" stroke-linecap="round"/>
</svg>
```

### unamused.svg
```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210">
  <rect x="0" y="0" width="280" height="210" fill="#4ECBA8"/>
  <rect x="6" y="5" width="268" height="200" rx="26" ry="26" fill="#4ECBA8"/>
  <rect x="12" y="10" width="256" height="188" rx="12" ry="12" fill="#90e5c8"/>
  <line x1="69" y1="75" x2="91" y2="75" stroke="#1a1a1a" stroke-width="5" stroke-linecap="round"/>
  <circle cx="80" cy="81" r="4" fill="#1a1a1a"/>
  <line x1="188" y1="75" x2="210" y2="75" stroke="#1a1a1a" stroke-width="5" stroke-linecap="round"/>
  <circle cx="199" cy="81" r="4" fill="#1a1a1a"/>
  <line x1="122" y1="118" x2="158" y2="118" stroke="#1a1a1a" stroke-width="5" stroke-linecap="round"/>
</svg>
```

### annoyed.svg
```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210">
  <rect x="0" y="0" width="280" height="210" fill="#4ECBA8"/>
  <rect x="6" y="5" width="268" height="200" rx="26" ry="26" fill="#4ECBA8"/>
  <rect x="12" y="10" width="256" height="188" rx="12" ry="12" fill="#90e5c8"/>
  <line x1="70" y1="78" x2="90" y2="78" stroke="#1a1a1a" stroke-width="6" stroke-linecap="round"/>
  <line x1="189" y1="78" x2="209" y2="78" stroke="#1a1a1a" stroke-width="6" stroke-linecap="round"/>
  <line x1="130" y1="118" x2="150" y2="118" stroke="#1a1a1a" stroke-width="5" stroke-linecap="round"/>
</svg>
```

### skeptical.svg
```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210">
  <rect x="0" y="0" width="280" height="210" fill="#4ECBA8"/>
  <rect x="6" y="5" width="268" height="200" rx="26" ry="26" fill="#4ECBA8"/>
  <rect x="12" y="10" width="256" height="188" rx="12" ry="12" fill="#90e5c8"/>
  <path d="M 183 60 L 215 52" stroke="#1a1a1a" stroke-width="5" stroke-linecap="round"/>
  <circle cx="80" cy="78" r="6.5" fill="#1a1a1a"/>
  <line x1="188" y1="75" x2="210" y2="75" stroke="#1a1a1a" stroke-width="5" stroke-linecap="round"/>
  <circle cx="199" cy="81" r="4" fill="#1a1a1a"/>
  <path d="M 120 113 Q 140 104 156 113" stroke="#1a1a1a" stroke-width="4" fill="none" stroke-linecap="round"/>
</svg>
```

### playful.svg
```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210">
  <rect x="0" y="0" width="280" height="210" fill="#4ECBA8"/>
  <rect x="6" y="5" width="268" height="200" rx="26" ry="26" fill="#4ECBA8"/>
  <rect x="12" y="10" width="256" height="188" rx="12" ry="12" fill="#90e5c8"/>
  <circle cx="80" cy="78" r="6.5" fill="#1a1a1a"/>
  <path d="M 180 85 Q 199 70 218 85" stroke="#1a1a1a" stroke-width="7" fill="none" stroke-linecap="round"/>
  <path d="M 122 115 Q 140 124 158 115" stroke="#1a1a1a" stroke-width="4" fill="none" stroke-linecap="round"/>
  <path d="M 134 120 Q 134 134 140 134 Q 146 134 146 120 Z" fill="#e8443b" stroke="#1a1a1a" stroke-width="2"/>
</svg>
```

### kiss.svg
```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210">
  <rect x="0" y="0" width="280" height="210" fill="#4ECBA8"/>
  <rect x="6" y="5" width="268" height="200" rx="26" ry="26" fill="#4ECBA8"/>
  <rect x="12" y="10" width="256" height="188" rx="12" ry="12" fill="#90e5c8"/>
  <path d="M 73 69 L 88 78 L 73 87" stroke="#1a1a1a" stroke-width="5" fill="none" stroke-linecap="round" stroke-linejoin="round"/>
  <path d="M 209 69 L 194 78 L 209 87" stroke="#1a1a1a" stroke-width="5" fill="none" stroke-linecap="round" stroke-linejoin="round"/>
  <path d="M 132 110 C 146 110 146 120 137 120 C 146 120 146 130 132 130" stroke="#1a1a1a" stroke-width="4" fill="none" stroke-linecap="round"/>
</svg>
```

### grimace.svg
```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210">
  <rect x="0" y="0" width="280" height="210" fill="#4ECBA8"/>
  <rect x="6" y="5" width="268" height="200" rx="26" ry="26" fill="#4ECBA8"/>
  <rect x="12" y="10" width="256" height="188" rx="12" ry="12" fill="#90e5c8"/>
  <line x1="71" y1="78" x2="89" y2="78" stroke="#1a1a1a" stroke-width="6" stroke-linecap="round"/>
  <line x1="190" y1="78" x2="208" y2="78" stroke="#1a1a1a" stroke-width="6" stroke-linecap="round"/>
  <rect x="111" y="110" width="58" height="22" rx="7" ry="7" fill="#e4e4e4" stroke="#1a1a1a" stroke-width="4"/>
  <line x1="140" y1="110" x2="140" y2="132" stroke="#1a1a1a" stroke-width="2.5"/>
  <line x1="125" y1="110" x2="125" y2="132" stroke="#1a1a1a" stroke-width="2"/>
  <line x1="155" y1="110" x2="155" y2="132" stroke="#1a1a1a" stroke-width="2"/>
  <line x1="111" y1="121" x2="169" y2="121" stroke="#1a1a1a" stroke-width="2"/>
</svg>
```

### shout.svg
```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210">
  <rect x="0" y="0" width="280" height="210" fill="#4ECBA8"/>
  <rect x="6" y="5" width="268" height="200" rx="26" ry="26" fill="#4ECBA8"/>
  <rect x="12" y="10" width="256" height="188" rx="12" ry="12" fill="#90e5c8"/>
  <path d="M 60 60 L 96 74" stroke="#1a1a1a" stroke-width="5" stroke-linecap="round"/>
  <path d="M 183 74 L 219 60" stroke="#1a1a1a" stroke-width="5" stroke-linecap="round"/>
  <circle cx="80" cy="78" r="6.5" fill="#1a1a1a"/>
  <circle cx="199" cy="78" r="6.5" fill="#1a1a1a"/>
  <rect x="98" y="101" width="84" height="43" rx="20" ry="20" fill="#1a1a1a"/>
  <path d="M 99.67 113 A 20 20 0 0 1 118 101 L 162 101 A 20 20 0 0 1 180.33 113 Z" fill="#e4e4e4"/>
  <path d="M 99.67 113 L 180.33 113 A 20 20 0 0 1 182 121 L 182 124 A 20 20 0 0 1 162 144 L 118 144 A 20 20 0 0 1 98 124 L 98 121 A 20 20 0 0 1 99.67 113 Z" fill="#1a7848"/>
  <path d="M 116 144 A 24 8 0 0 1 164 144 Z" fill="#16ae81"/>
</svg>
```

### dead.svg
```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210">
  <rect x="0" y="0" width="280" height="210" fill="#4ECBA8"/>
  <rect x="6" y="5" width="268" height="200" rx="26" ry="26" fill="#4ECBA8"/>
  <rect x="12" y="10" width="256" height="188" rx="12" ry="12" fill="#90e5c8"/>
  <line x1="72" y1="70" x2="88" y2="86" stroke="#1a1a1a" stroke-width="5" stroke-linecap="round"/>
  <line x1="72" y1="86" x2="88" y2="70" stroke="#1a1a1a" stroke-width="5" stroke-linecap="round"/>
  <line x1="191" y1="70" x2="207" y2="86" stroke="#1a1a1a" stroke-width="5" stroke-linecap="round"/>
  <line x1="191" y1="86" x2="207" y2="70" stroke="#1a1a1a" stroke-width="5" stroke-linecap="round"/>
  <line x1="122" y1="118" x2="158" y2="118" stroke="#1a1a1a" stroke-width="5" stroke-linecap="round"/>
</svg>
```

### glitch.svg
```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210">
  <rect x="0" y="0" width="280" height="210" fill="#4ECBA8"/>
  <rect x="6" y="5" width="268" height="200" rx="26" ry="26" fill="#4ECBA8"/>
  <rect x="12" y="10" width="256" height="188" rx="12" ry="12" fill="#90e5c8"/>
  <rect x="72" y="70" width="8" height="8" fill="#1a1a1a"/>
  <rect x="72" y="78" width="8" height="8" fill="#1a1a1a"/>
  <rect x="80" y="70" width="8" height="8" fill="#1a1a1a"/>
  <rect x="80" y="78" width="8" height="8" fill="#1a1a1a"/>
  <rect x="191" y="70" width="8" height="8" fill="#1a1a1a"/>
  <rect x="191" y="78" width="8" height="8" fill="#1a1a1a"/>
  <rect x="199" y="70" width="8" height="8" fill="#1a1a1a"/>
  <rect x="199" y="78" width="8" height="8" fill="#1a1a1a"/>
  <rect x="116" y="116" width="8" height="8" fill="#1a1a1a"/>
  <rect x="124" y="124" width="8" height="8" fill="#1a1a1a"/>
  <rect x="132" y="128" width="8" height="8" fill="#1a1a1a"/>
  <rect x="140" y="128" width="8" height="8" fill="#1a1a1a"/>
  <rect x="148" y="128" width="8" height="8" fill="#1a1a1a"/>
  <rect x="156" y="124" width="8" height="8" fill="#1a1a1a"/>
  <rect x="164" y="116" width="8" height="8" fill="#1a1a1a"/>
</svg>
```

### dismayed.svg
```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210">
  <rect x="0" y="0" width="280" height="210" fill="#4ECBA8"/>
  <rect x="6" y="5" width="268" height="200" rx="26" ry="26" fill="#4ECBA8"/>
  <rect x="12" y="10" width="256" height="188" rx="12" ry="12" fill="#90e5c8"/>
  <path d="M 64 70 L 96 58" stroke="#1a1a1a" stroke-width="5" stroke-linecap="round"/>
  <path d="M 183 58 L 215 70" stroke="#1a1a1a" stroke-width="5" stroke-linecap="round"/>
  <circle cx="80" cy="78" r="10" fill="#1a1a1a"/>
  <circle cx="76.5" cy="74.5" r="3.2" fill="#e4e4e4"/>
  <circle cx="199" cy="78" r="10" fill="#1a1a1a"/>
  <circle cx="195.5" cy="74.5" r="3.2" fill="#e4e4e4"/>
  <ellipse cx="140" cy="124" rx="14" ry="17" fill="#1a1a1a"/>
  <ellipse cx="140" cy="125" rx="11" ry="14" fill="#1a7848"/>
</svg>
```

### adoring.svg
```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210">
  <rect x="0" y="0" width="280" height="210" fill="#4ECBA8"/>
  <rect x="6" y="5" width="268" height="200" rx="26" ry="26" fill="#4ECBA8"/>
  <rect x="12" y="10" width="256" height="188" rx="12" ry="12" fill="#90e5c8"/>
  <ellipse cx="61" cy="100" rx="12" ry="7.5" fill="#53AF66" opacity="0.55"/>
  <ellipse cx="219" cy="100" rx="12" ry="7.5" fill="#53AF66" opacity="0.55"/>
  <circle cx="80" cy="78" r="12" fill="#1a1a1a"/>
  <circle cx="76.5" cy="74.5" r="3.2" fill="#e4e4e4"/>
  <circle cx="199" cy="78" r="12" fill="#1a1a1a"/>
  <circle cx="195.5" cy="74.5" r="3.2" fill="#e4e4e4"/>
  <path d="M 72.0 66.0 L 72.9 69.1 L 76.0 70.0 L 72.9 70.9 L 72.0 74.0 L 71.1 70.9 L 68.0 70.0 L 71.1 69.1 Z" fill="#e4e4e4"/>
  <path d="M 207.0 66.0 L 207.9 69.1 L 211.0 70.0 L 207.9 70.9 L 207.0 74.0 L 206.1 70.9 L 203.0 70.0 L 206.1 69.1 Z" fill="#e4e4e4"/>
  <path d="M 116 111 Q 140 125 160 111" stroke="#1a1a1a" stroke-width="4" fill="none" stroke-linecap="round"/>
</svg>
```

### sparkle.svg
```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210">
  <rect x="0" y="0" width="280" height="210" fill="#4ECBA8"/>
  <rect x="6" y="5" width="268" height="200" rx="26" ry="26" fill="#4ECBA8"/>
  <rect x="12" y="10" width="256" height="188" rx="12" ry="12" fill="#90e5c8"/>
  <path d="M 80.0 66.0 L 82.5 75.5 L 92.0 78.0 L 82.5 80.5 L 80.0 90.0 L 77.5 80.5 L 68.0 78.0 L 77.5 75.5 Z" fill="#f4c531"/>
  <path d="M 199.0 66.0 L 201.5 75.5 L 211.0 78.0 L 201.5 80.5 L 199.0 90.0 L 196.5 80.5 L 187.0 78.0 L 196.5 75.5 Z" fill="#f4c531"/>
  <path d="M 116 111 Q 140 125 160 111" stroke="#1a1a1a" stroke-width="4" fill="none" stroke-linecap="round"/>
</svg>
```
