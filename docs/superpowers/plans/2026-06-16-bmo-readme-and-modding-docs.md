# BMO README & Modding Documentation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship a promotional `README.md`, an MIT `LICENSE`, a rendered face-image gallery, and a comprehensive hub-and-spoke modding documentation set under `docs/`.

**Architecture:** Pure documentation plus one small Go helper (`cmd/render-faces`) that rasterizes the embedded face SVGs through BMO's own oksvg path (`face.Rasterize`) so gallery images match on-device rendering. Modding docs are organized as a hub (`docs/MODDING.md`) linking to focused spoke pages under `docs/mods/`. `docs/FACES.md` is relocated to `docs/mods/faces.md`.

**Tech Stack:** Go 1.25 (`image`, `image/png`, `github.com/srwiley/oksvg` via `internal/face`), Markdown, ImageMagick (`montage`/`convert`) for the banner, golangci-lint.

**Source-of-truth references (read these; do not invent values):**
- Mod manifest schema: `internal/mod/manifest.go:20-33`
- Mod struct & paths: `internal/mod/mod.go:12-33`
- Mod discovery/overlay: `internal/mod/discover.go`
- Config struct + JSON tags: `internal/config/config.go:84-124`
- Default persona prompt: `internal/config/config.go:48-62`
- Default TTS instructions: `internal/config/config.go:39-41`
- Prompt file loading (`persona.txt`/`voice.txt`/`quotes.txt`): `internal/config/prompts.go:14-77`
- TTS `Instructions` wiring + `tts-1` strip: `internal/providers/openai_compatible.go:120-174`, `internal/providers/provider.go:111-119`
- Controls / button mapping: `cmd/bmo-pak/main.go:411-417`
- Mod reload on switch: `cmd/bmo-pak/main.go:300-331`
- Build/deploy scripts: `scripts/release.sh`, `scripts/deploy.sh`, `scripts/debug-logs.sh`
- Pak metadata: `pak.json`
- Face rasterizer: `internal/face/raster.go:14`
- Embedded face assets: `internal/face/assets/*.svg`

**Verified facts to use verbatim:**
- Module path: `github.com/carroarmato0/nextui-bmo`; Go `1.25.0`; pak `v0.1.0`; author `Carroarmato0`.
- Platforms: `tg5040` = **TrimUI Brick** and **TrimUI Smart Pro**; `tg5050` = **TrimUI Smart Pro S**.
- Data root example (Smart Pro): `/mnt/SDCARD/.userdata/tg5040/BMO/`. Config: `<dataRoot>/BMO/config.json`. Mods: `<dataRoot>/BMO/mods/`.
- Controls: **A** = BTN_EAST (305) push-to-talk / confirm · **B** = BTN_SOUTH (304) cancel/exit · **Start** = open/close Settings · **Y** = BTN_NORTH (307) AI Setup · **Menu** = BTN_MODE (316) exit to NextUI.
- Ko-fi: `https://ko-fi.com/carroarmato0`.
- Modes: `idle` (`ModeIdle`) and `ai` (`ModeAI`). Input trigger default `ptt`.

---

## File Structure

```
README.md                  Create
LICENSE                    Create
cmd/render-faces/main.go   Create   render helper
cmd/render-faces/main_test.go Create
docs/images/banner.png     Create (generated)
docs/images/faces/*.png    Create (generated)
docs/MODDING.md            Create   hub
docs/mods/faces.md         Create   (git mv of docs/FACES.md, then refreshed)
docs/mods/voice.md         Create
docs/mods/persona.md       Create
docs/mods/quotes.md        Create
docs/mods/emotions.md      Create
docs/mods/animations.md    Create
docs/FACES.md              Delete (relocated)
```

---

## Task 1: MIT LICENSE

**Files:**
- Create: `LICENSE`

- [ ] **Step 1: Write the LICENSE file**

Standard MIT text, copyright line exactly:

```
MIT License

Copyright (c) 2026 Christophe Vanlancker
```

…followed by the canonical MIT permission body (the standard "Permission is hereby granted, free of charge..." through "...DEALINGS IN THE SOFTWARE." paragraphs).

- [ ] **Step 2: Verify**

Run: `head -3 LICENSE`
Expected: shows `MIT License` and the 2026 Christophe Vanlancker copyright line.

- [ ] **Step 3: Commit**

```bash
git add LICENSE
git commit -m "docs: add MIT license"
```

---

## Task 2: Face-render helper (`cmd/render-faces`)

**Files:**
- Create: `cmd/render-faces/main.go`
- Test: `cmd/render-faces/main_test.go`

- [ ] **Step 1: Write the failing test**

`cmd/render-faces/main_test.go`:

```go
package main

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestRenderOneProducesPNG(t *testing.T) {
	src := filepath.Join("..", "..", "internal", "face", "assets", "neutral.svg")
	dst := filepath.Join(t.TempDir(), "neutral.png")

	if err := renderOne(src, dst); err != nil {
		t.Fatalf("renderOne: %v", err)
	}

	f, err := os.Open(dst)
	if err != nil {
		t.Fatalf("open output: %v", err)
	}
	defer f.Close()

	cfg, err := png.DecodeConfig(f)
	if err != nil {
		t.Fatalf("decode png: %v", err)
	}
	if cfg.Width != width || cfg.Height != height {
		t.Fatalf("got %dx%d, want %dx%d", cfg.Width, cfg.Height, width, height)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `CGO_ENABLED=0 go test ./cmd/render-faces/`
Expected: FAIL — `undefined: renderOne` (and `width`/`height`).

- [ ] **Step 3: Write the implementation**

`cmd/render-faces/main.go`:

```go
// Command render-faces rasterizes the embedded BMO face SVGs to PNGs for the
// documentation gallery, using the same oksvg path the device uses so the
// images match on-device rendering. Run from the repo root: go run ./cmd/render-faces
package main

import (
	"fmt"
	"image"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/carroarmato0/nextui-bmo/internal/face"
)

const (
	srcDir = "internal/face/assets"
	outDir = "docs/images/faces"
	width  = 480
	height = 360
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	matches, err := filepath.Glob(filepath.Join(srcDir, "*.svg"))
	if err != nil {
		return err
	}
	if len(matches) == 0 {
		return fmt.Errorf("no SVGs found in %s", srcDir)
	}
	for _, src := range matches {
		name := strings.TrimSuffix(filepath.Base(src), ".svg")
		dst := filepath.Join(outDir, name+".png")
		if err := renderOne(src, dst); err != nil {
			return fmt.Errorf("render %s: %w", name, err)
		}
		fmt.Printf("rendered %s\n", dst)
	}
	return nil
}

func renderOne(src, dst string) error {
	svg, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	buf, err := face.Rasterize(svg, width, height)
	if err != nil {
		return err
	}
	// Rasterize returns row-major ARGB8888 (a<<24|r<<16|g<<8|b) from an
	// alpha-premultiplied image.RGBA; reverse it back into image.RGBA bytes.
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for i, px := range buf {
		img.Pix[i*4] = byte(px >> 16)   // R
		img.Pix[i*4+1] = byte(px >> 8)  // G
		img.Pix[i*4+2] = byte(px)       // B
		img.Pix[i*4+3] = byte(px >> 24) // A
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `CGO_ENABLED=0 go test ./cmd/render-faces/`
Expected: PASS.

- [ ] **Step 5: Lint**

Run: `golangci-lint run ./cmd/render-faces/...`
Expected: no findings.

- [ ] **Step 6: Commit**

```bash
git add cmd/render-faces/
git commit -m "feat(tools): add render-faces helper for doc image gallery"
```

---

## Task 3: Generate gallery + banner images

**Files:**
- Create: `docs/images/faces/*.png` (generated)
- Create: `docs/images/banner.png` (generated)

- [ ] **Step 1: Generate the face PNGs**

Run from repo root: `CGO_ENABLED=0 go run ./cmd/render-faces`
Expected: prints `rendered docs/images/faces/<name>.png` for every SVG in `internal/face/assets/` (includes `neutral`, `happy`, `sad`, `angry`, `surprised`, `love`, `playful`, `laugh`, `excited`, `sleeping`, `thinking`, `listening`, `crying`, `dizzy`, `glitch`, etc., plus `speaking` and `speaking_0..5`).

- [ ] **Step 2: Verify a sample image**

Run: `file docs/images/faces/neutral.png`
Expected: `PNG image data, 480 x 360`.

- [ ] **Step 3: Build the banner (montage of 5 representative expressions)**

Run:
```bash
montage docs/images/faces/happy.png docs/images/faces/neutral.png \
  docs/images/faces/surprised.png docs/images/faces/love.png \
  docs/images/faces/playful.png \
  -tile 5x1 -geometry +6+6 -background none docs/images/banner.png
```
Expected: `docs/images/banner.png` created. Verify: `file docs/images/banner.png` shows a PNG.

- [ ] **Step 4: Commit the images**

```bash
git add docs/images/
git commit -m "docs: add rendered BMO face gallery and banner images"
```

---

## Task 4: Relocate & refresh faces doc → `docs/mods/faces.md`

**Files:**
- Move: `docs/FACES.md` → `docs/mods/faces.md`
- Modify: `docs/mods/faces.md`

- [ ] **Step 1: Move the file (preserve history)**

```bash
mkdir -p docs/mods
git mv docs/FACES.md docs/mods/faces.md
```

- [ ] **Step 2: Refresh the content**

Edit `docs/mods/faces.md`:
- Add a top breadcrumb line: `[← Modding guide](../MODDING.md) · Faces`.
- Keep the existing expression catalog table, `speaking.svg` Go-template parameter table, alias notes, overlay-vs-self-contained face semantics, and "re-read before each interaction" note (all already present — verify against `internal/face/library.go` and `docs/mods/faces.md` itself).
- Add an **Illustrated expressions** section embedding a representative sample, e.g.:
  ```markdown
  | Neutral | Happy | Surprised | Love | Sad |
  | --- | --- | --- | --- | --- |
  | ![neutral](../images/faces/neutral.png) | ![happy](../images/faces/happy.png) | ![surprised](../images/faces/surprised.png) | ![love](../images/faces/love.png) | ![sad](../images/faces/sad.png) |
  ```
- Add a **Previewing your faces** subsection noting that maintainers render the embedded set with `go run ./cmd/render-faces`, and that on-device rendering uses oksvg (so preview via the same path; ImageMagick/rsvg may differ on arc sweeps).

- [ ] **Step 3: Verify image paths resolve**

Run: `for p in $(grep -oE '\.\./images/faces/[a-z0-9_]+\.png' docs/mods/faces.md | sort -u); do test -f "docs/mods/$p" && echo "OK $p" || echo "MISSING $p"; done`
Expected: all `OK`.

- [ ] **Step 4: Commit**

```bash
git add docs/mods/faces.md
git commit -m "docs(mods): relocate and refresh faces guide with illustrations"
```

---

## Task 5: `docs/mods/voice.md`

**Files:**
- Create: `docs/mods/voice.md`

- [ ] **Step 1: Write the page**

Structure (concise, modder-facing voice):
- Breadcrumb: `[← Modding guide](../MODDING.md) · Voice`
- **What `voice.txt` does** — plain-text TTS *speaking-style* instructions sent as the `Instructions` parameter; shapes pitch, pace, accent, mood, delivery layered on top of the user's configured TTS voice. Re-read before every interaction (`internal/config/prompts.go:14-39`); falls back to the built-in default (`internal/config/config.go:39-41`) when absent/blank.
- **Examples** — include at least two verbatim:
  > Speak slowly with a deep, gravelly pirate growl. Roll your R's. Sound weary but a little menacing.
  > Bright, clipped, robotic cadence. Short bursts. Slightly metallic and over-enunciated.
- **Graceful degradation** — instruction-capable models (e.g. `gpt-4o-mini-tts`) apply the style; basic models (`tts-1`) ignore it harmlessly and BMO still speaks (`internal/providers/openai_compatible.go:146-148`).
- **What a mod CANNOT control (by design)** — a callout box:
  > A mod shapes *how* BMO speaks, not *which* voice. It **cannot** select the TTS voice name (`nova`, `alloy`…), model, provider, or API endpoint — those live in the user's `config.json`, tied to their account and credits. This keeps mods **portable and free to install**: a mod never forces a user onto a paid model or breaks on a different backend.

- [ ] **Step 2: Verify the facts against source**

Confirm the claims match `internal/config/prompts.go:14-39`, `internal/config/config.go:39-41`, and `internal/providers/openai_compatible.go:120-174`. Fix any drift.

- [ ] **Step 3: Commit**

```bash
git add docs/mods/voice.md
git commit -m "docs(mods): add voice.txt speaking-style guide with provider boundary"
```

---

## Task 6: `docs/mods/persona.md`

**Files:**
- Create: `docs/mods/persona.md`

- [ ] **Step 1: Write the page**

- Breadcrumb: `[← Modding guide](../MODDING.md) · Persona`
- **What `persona.txt` does** — replaces the system prompt sent to the AI every turn (`cmd/bmo-pak/main.go:97-101`, `internal/config/prompts.go`). Keep under ~1000 characters. Re-read each interaction.
- **The default persona** — summarize what the built-in prompt establishes (read `internal/config/config.go:48-62`) and note the device-awareness framing it relies on.
- **Worked example** — a short alternate persona (5–8 lines) for a "grumpy detective BMO", verbatim, so modders see the shape.
- **Tips** — short sentences, no markdown/emojis in output, stay in character.

- [ ] **Step 2: Verify** facts against `internal/config/config.go:48-62` and `internal/config/prompts.go`.

- [ ] **Step 3: Commit**

```bash
git add docs/mods/persona.md
git commit -m "docs(mods): add persona.txt guide"
```

---

## Task 7: `docs/mods/quotes.md`

**Files:**
- Create: `docs/mods/quotes.md`

- [ ] **Step 1: Write the page**

- Breadcrumb: `[← Modding guide](../MODDING.md) · Quotes`
- **What `quotes.txt` does** — verbatim idle quotes; one per line; blank lines ignored; `#` lines are comments (verify against `internal/config/quotes.go` and the loader).
- **Example** — a 4–5 line `quotes.txt` block with a comment line, verbatim.
- **Note** — re-read behavior and fallback to built-in quotes when absent.

- [ ] **Step 2: Verify** against `internal/config/quotes.go` and the quotes loader.

- [ ] **Step 3: Commit**

```bash
git add docs/mods/quotes.md
git commit -m "docs(mods): add quotes.txt guide"
```

---

## Task 8: `docs/mods/emotions.md`

**Files:**
- Create: `docs/mods/emotions.md`

- [ ] **Step 1: Write the page**

- Breadcrumb: `[← Modding guide](../MODDING.md) · Emotions`
- **What the `emotions` map does** — `mod.json` `emotions` (name → human description) feeds the emotion vocabulary the LLM may emit, which BMO then shows as a face (`internal/assistant/emotion.go:15-39`, `internal/mod/manifest.go:20-33`).
- **Relationship to faces** — an emotion name should have a matching face in `faces/` (or fold to `neutral`); cross-link to `faces.md`.
- **Example** — verbatim:
  ```json
  {
    "emotions": {
      "grumpy": "sulky and irritable",
      "ecstatic": "overjoyed and bouncing"
    }
  }
  ```

- [ ] **Step 2: Verify** against `internal/assistant/emotion.go:15-39` and `internal/mod/manifest.go`.

- [ ] **Step 3: Commit**

```bash
git add docs/mods/emotions.md
git commit -m "docs(mods): add emotion vocabulary guide"
```

---

## Task 9: `docs/mods/animations.md`

**Files:**
- Create: `docs/mods/animations.md`

- [ ] **Step 1: Write the page**

- Breadcrumb: `[← Modding guide](../MODDING.md) · Animations`
- **What the `animations` map does** — `mod.json` `animations` (expression name → animation JSON) parsed by `face.ParseAnimations` (`internal/mod/manifest.go:31`, `internal/face/anim_def.go`). Overlay mods inherit embedded `face.DefaultAnimations()`; self-contained mods start empty; mod entries override by name.
- **Minimal example** — a small verbatim animation JSON entry for one expression (read `internal/face/anim_def.go` / `internal/face/anim_defaults.go` for the exact field shape — `frames`, `driver`, etc. — and reproduce a valid minimal object). Do not invent fields; mirror an existing default.
- **Note** — advanced/optional feature; most mods can omit it.

- [ ] **Step 2: Verify** the JSON shape compiles against the parser — sanity check by reading `internal/face/anim_def.go` and matching field names exactly.

- [ ] **Step 3: Commit**

```bash
git add docs/mods/animations.md
git commit -m "docs(mods): add animations guide"
```

---

## Task 10: `docs/MODDING.md` hub

**Files:**
- Create: `docs/MODDING.md`

- [ ] **Step 1: Write the hub page**

Sections:
- **Title + intro** — what a mod is (a data-only directory under `<dataRoot>/BMO/mods/<name>/` that overrides BMO's look, personality, voice style, quotes, animations, and emotion vocabulary).
- **Two kinds of mod** — `mods/default` (overlay; per-asset fallback to embedded BMO) vs `mods/<name>` (self-contained character; owns its full face set once it ships ≥1 face). Source: `internal/mod/discover.go`, `internal/mod/mod.go`.
- **Directory layout** — annotated tree:
  ```
  <dataRoot>/BMO/mods/my-bmo/
    mod.json        (optional) metadata + emotions + animations
    persona.txt     (optional) personality / system prompt
    voice.txt       (optional) speaking-style instructions
    quotes.txt      (optional) idle quotes
    faces/          (optional) <expression>.svg overrides
    audio/          (optional) clip overrides (e.g. timeout.pcm)
  ```
- **`mod.json` schema** — verbatim table from `internal/mod/manifest.go:20-33`:

  | Field | Type | Meaning |
  | --- | --- | --- |
  | `apiVersion` | int | mod-format version; absent/`0` ⇒ `1` |
  | `name` | string | display name override |
  | `author` | string | shown in Settings |
  | `description` | string | shown in Settings |
  | `version` | string | author's free-form release string |
  | `emotions` | map[string]string | emotion name → LLM description |
  | `animations` | map[string]json | expression name → animation JSON |

  Plus a verbatim full `mod.json` example.
- **What you can customize** — table linking each file/field to its spoke page (`faces.md`, `voice.md`, `persona.md`, `quotes.md`, `emotions.md`, `animations.md`).
- **Limitations** — bullet list including, verbatim, the voice/provider boundary callout (mirror the wording from `voice.md` Task 5), plus: self-contained mods don't inherit embedded faces once they ship one; malformed `mod.json` is tolerated and folds to defaults; mods are **data only — no code execution**; provider/model/API keys are global user config, never set by mods.
- **Installing a mod** — unzip into `<dataRoot>/BMO/mods/<name>/`; select via **Settings → MOD**; persona/voice/quotes apply on the next interaction without restart (`cmd/bmo-pak/main.go:300-331`).
- **Create your first mod (step-by-step)** — a worked walkthrough: make the folder, write a 6-line `mod.json` (name/author/description), a short `persona.txt`, a `voice.txt`, drop one `neutral.svg` (link to `faces.md`), copy to the device, select it in Settings. Each step a numbered instruction with the exact file content shown.
- **Reference** — link list to all six spoke pages.

- [ ] **Step 2: Verify** schema/paths against `internal/mod/manifest.go`, `internal/mod/discover.go`, `cmd/bmo-pak/main.go:300-331`.

- [ ] **Step 3: Verify spoke links resolve**

Run: `for f in faces voice persona quotes emotions animations; do test -f "docs/mods/$f.md" && echo "OK $f" || echo "MISSING $f"; done`
Expected: all `OK`.

- [ ] **Step 4: Commit**

```bash
git add docs/MODDING.md
git commit -m "docs: add modding hub guide"
```

---

## Task 11: `README.md`

**Files:**
- Create: `README.md`

- [ ] **Step 1: Write the README**

Follow this section order; use the verified facts header above for all values.

1. **Hero** — `# BMO` title, one-line tagline, banner image `![BMO](docs/images/banner.png)`, and a badge row including a Ko-fi badge:
   ```markdown
   [![Ko-Fi](https://ko-fi.com/img/githubbutton_sm.svg)](https://ko-fi.com/carroarmato0)
   ```
   plus shields-style static badges for Platforms (`tg5040· tg5050`), License (`MIT`), and Go (`1.25`).
2. **What is BMO?** — fullscreen BMO-inspired AI voice assistant & desk companion, packaged as a NextUI **Tool** pak. Unofficial Adventure Time fan project.
3. **Features** — bullets: animated SVG face (30+ expressions), LLM-directed emotion, voice assistant pipeline (STT → Chat → TTS) with push-to-talk, idle quotes & pre-recorded clips, device awareness, **mods**, Idle vs AI modes, reduced-motion.
4. **Supported devices** — `tg5040` (TrimUI Brick and TrimUI Smart Pro) and `tg5050` (TrimUI Smart Pro S); auto-detected at launch (`launch.sh`).
5. **Gallery** — a small markdown table of 4–6 expression PNGs from `docs/images/faces/`.
6. **Installation** — download `BMO.pak.zip` from Releases → unzip into the SD card's `Tools/<platform>/` → launch from NextUI's Tools menu.
7. **Configuration** — config path `<dataRoot>/BMO/config.json` (e.g. `/mnt/SDCARD/.userdata/tg5040/BMO/config.json`); a trimmed verbatim example showing the real JSON tags:
   ```json
   {
     "mode": "ai",
     "stt":  { "name": "openai-compatible", "model": "whisper-1", "api_key": "sk-..." },
     "chat": { "name": "openai-compatible", "model": "gpt-4o-mini", "api_key": "sk-..." },
     "tts":  { "name": "openai-compatible", "model": "gpt-4o-mini-tts", "voice": "nova", "api_key": "sk-..." },
     "ptt_buttons": ["A"],
     "active_mod": "",
     "reduced_motion": false
   }
   ```
   Note in-app **Settings** (Start) and **AI Setup** (Y); then a **Controls** table:

   | Button | Action |
   | --- | --- |
   | A | Push-to-talk / confirm |
   | B | Cancel / exit |
   | Start | Open/close Settings |
   | Y | Open AI Setup |
   | Menu | Exit to NextUI |
8. **Mods** — 2–3 sentences + link: `See the [Modding guide](docs/MODDING.md).`
9. **Building from source** — fenced blocks:
   ```bash
   # Build & test locally
   CGO_ENABLED=0 go build ./...
   CGO_ENABLED=0 go test ./...
   golangci-lint run ./...

   # Cross-compile + package the pak (docker/podman)
   ./scripts/release.sh

   # Deploy to a connected device (ADB) or SD path
   ./scripts/deploy.sh
   # Tail device logs
   ./scripts/debug-logs.sh
   ```
10. **Support** — Ko-fi button repeated + one line: "If BMO brightens your handheld, consider [buying me a coffee](https://ko-fi.com/carroarmato0). 💖"
11. **License** — "Released under the [MIT License](LICENSE)." + unofficial Adventure Time fan project disclaimer.

- [ ] **Step 2: Verify config example matches real tags**

Confirm every JSON key used appears in `internal/config/config.go:84-124`. Fix mismatches.

- [ ] **Step 3: Verify all local links/images resolve**

Run:
```bash
for p in docs/images/banner.png docs/MODDING.md LICENSE $(grep -oE 'docs/images/faces/[a-z0-9_]+\.png' README.md | sort -u); do
  test -e "$p" && echo "OK $p" || echo "MISSING $p"
done
```
Expected: all `OK`.

- [ ] **Step 4: Commit**

```bash
git add README.md
git commit -m "docs: add project README"
```

---

## Task 12: Final verification

**Files:** none (verification only)

- [ ] **Step 1: No dangling `docs/FACES.md` references in live docs**

Run: `grep -rn "FACES.md" README.md docs/MODDING.md docs/mods/ 2>/dev/null; echo "exit=$?"`
Expected: no matches (grep exit 1). Historical references under `docs/specs/` and `docs/superpowers/plans/` are intentionally left untouched.

- [ ] **Step 2: Build, test, lint all green**

Run:
```bash
CGO_ENABLED=0 go build ./...
CGO_ENABLED=0 go test ./...
golangci-lint run ./...
```
Expected: build OK, tests pass, no lint findings.

- [ ] **Step 3: Cross-check every spoke is linked from the hub and every doc has the breadcrumb**

Run: `grep -L "MODDING.md" docs/mods/*.md; echo "files missing breadcrumb listed above (should be none)"`
Expected: no files listed.

- [ ] **Step 4: Final commit (if any verification fixes were made)**

```bash
git add -A
git commit -m "docs: verification fixes for README and modding docs"
```

---

## Self-Review (completed by plan author)

- **Spec coverage:** README (Task 11), MIT LICENSE (Task 1), Ko-fi badge+section (Task 11), hub (Task 10), six spokes (Tasks 4–9), faces relocation (Task 4), rendered images via `face.Rasterize` (Tasks 2–3), voice/provider boundary documented (Tasks 5 & 10), out-of-scope honored (no provider/model moddability). All spec sections map to tasks.
- **Placeholder scan:** none — image render code, test, config example, schema table, commands, and Ko-fi badge are all verbatim; prose pages specify exact source `file:line` to read so no values are invented.
- **Type consistency:** helper uses `renderOne`, `run`, consts `width`/`height`/`srcDir`/`outDir` consistently across `main.go` and `main_test.go`; `face.Rasterize(svg []byte, w, h int) ([]uint32, error)` signature matches `internal/face/raster.go:14`.
