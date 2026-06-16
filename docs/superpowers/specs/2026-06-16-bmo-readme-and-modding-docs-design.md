# BMO README & Comprehensive Modding Documentation — Design Spec

**Date:** 2026-06-16
**Status:** Approved (pending spec review)

## Summary

BMO has no top-level `README.md` and no consolidated modding guide. This project
delivers:

1. A formal, appealing **`README.md`** that promotes the app, documents supported
   devices and features, and explains configuration, building, and deployment.
2. A **comprehensive modding documentation set** under `docs/`, organized as a hub
   (`docs/MODDING.md`) plus focused per-topic spoke pages, covering what mods can
   customize, the limitations, and step-by-step procedures with examples.
3. A **rendered face gallery** (PNGs produced from the embedded SVG face set
   through BMO's own oksvg rasterizer) to illustrate the README and faces page.
4. An **MIT `LICENSE`** file and a **Ko-fi** donation reference.

No application code behavior changes. The one supporting code artifact is a
throwaway/maintenance face-render step used to generate documentation images.

## Motivation

- The project is public (`github.com/carroarmato0/nextui-bmo`) but has no landing
  documentation. A README is the front door for users and modders.
- Mods already support deep customization (faces, persona, voice style, quotes,
  animations, emotions) but the only documentation is `docs/FACES.md`, which is
  faces-centric and does not present a coherent "how to make a mod" story.
- A specific, deliberate boundary needs to be documented: **a mod shapes BMO's
  speaking _style_ via `voice.txt`, but cannot select the TTS voice name, model,
  provider, or endpoint.** Those live in the user's `config.json` because they are
  tied to the user's account, credits, and backend choice. Documenting this keeps
  mods portable and free to install, and preempts feature requests for
  mod-controlled providers.

## Design Decisions (resolved during brainstorming)

| Decision | Choice |
| --- | --- |
| Images | Rendered faces only — no device photos or live menu screenshots, no placeholders for them |
| Modding docs layout | Hub (`docs/MODDING.md`) + focused spoke pages |
| `FACES.md` | Relocate to `docs/mods/faces.md` (refreshed); update live references |

> **Note on `FACES.md` references:** the only in-repo mentions are in *historical*
> design docs under `docs/specs/` and `docs/superpowers/plans/` (frozen records of
> past work). Those are left untouched. Only live/navigational references (none
> exist today outside this set) would be updated.
| License | MIT (© 2026 Christophe Vanlancker) |
| Donations | Ko-fi badge in hero + short Support section: `https://ko-fi.com/carroarmato0` |

## Deliverables (file tree)

```
README.md                      NEW  promotional, comprehensive
LICENSE                        NEW  MIT, © 2026 Christophe Vanlancker
docs/
  MODDING.md                   NEW  hub: concept, layout, mod.json schema, limits, first-mod walkthrough
  mods/
    faces.md                   MOVED+REFRESHED from docs/FACES.md
    voice.md                   NEW  voice.txt speaking-style + the Tier-1 boundary
    persona.md                 NEW  persona.txt personality override
    quotes.md                  NEW  quotes.txt idle quotes
    animations.md              NEW  mod.json animations
    emotions.md                NEW  mod.json emotion vocabulary
  images/
    banner.png                 NEW  hero image (BMO face[s])
    faces/<expression>.png     NEW  rendered expression gallery
docs/FACES.md                  REMOVED (content relocated; references updated)
```

## README.md — Section Outline

1. **Hero** — name + tagline; banner image; badge row: platforms (`tg5040`,
   `tg5050`), license (MIT), Go 1.25, and Ko-fi.
2. **What is BMO?** — fullscreen BMO-inspired AI voice assistant and desk
   companion for TrimUI handhelds, packaged as a NextUI Tool pak. Unofficial
   Adventure Time fan project.
3. **Features** — animated SVG face with 30+ expressions; LLM-directed emotion;
   voice assistant pipeline (STT → Chat → TTS) with push-to-talk; idle quotes and
   pre-recorded clips; device awareness; **mods**; Idle vs AI modes;
   reduced-motion option.
4. **Supported devices** — `tg5040` (TrimUI Brick and TrimUI Smart Pro) and
   `tg5050` (TrimUI Smart Pro S); platform auto-detected at launch (`launch.sh`).
5. **Gallery** — rendered face expressions from `docs/images/faces/`.
6. **Installation** — download `BMO.pak.zip` from Releases → unzip into the SD
   card's `Tools/<platform>/` → launch from NextUI's Tools menu.
7. **Configuration** — `config.json` location (`<dataRoot>/BMO/config.json`,
   e.g. `/mnt/SDCARD/.userdata/tg5040/BMO/config.json`); key fields (`mode`,
   `stt`/`chat`/`tts` provider blocks + API keys, `ptt_buttons`,
   `device_context`); in-app **Settings** (Start) and **AI Setup** (Y) menus; a
   **controls table**:
   - **A** (BTN_EAST) — push-to-talk / confirm
   - **B** (BTN_SOUTH) — cancel / exit
   - **Start** — open/close Settings
   - **Y** (BTN_NORTH) — open AI Setup
   - **Menu** (BTN_MODE) — exit to NextUI
8. **Mods** — short pitch; link to `docs/MODDING.md`.
9. **Building from source** — `scripts/release.sh` (docker/podman cross-compile +
   package), `scripts/deploy.sh` (adb or SD path), `scripts/debug-logs.sh`; local
   dev commands (`CGO_ENABLED=0 go build ./...`, `CGO_ENABLED=0 go test ./...`,
   `golangci-lint run ./...`).
10. **Support** — Ko-fi button + one-line ask.
11. **License & credits** — MIT; unofficial Adventure Time fan project disclaimer.

## docs/MODDING.md (hub) — Outline

- **What is a mod?** — a directory under `<dataRoot>/BMO/mods/` that overrides
  BMO's appearance, personality, voice style, quotes, animations, and emotion
  vocabulary.
- **Two kinds of mod** — `mods/default` (overlay: per-asset fallback to embedded
  BMO) vs `mods/<name>` (self-contained character: owns its full face set once it
  ships ≥1 face).
- **Directory layout** — annotated tree (`mod.json`, `persona.txt`, `voice.txt`,
  `quotes.txt`, `faces/`, `audio/`).
- **`mod.json` schema** — `apiVersion` (current `1`; absent/`0` ⇒ `1`), `name`,
  `author`, `description`, `version`, `emotions`, `animations`.
- **What you can customize** — table mapping each file/field to the spoke page.
- **Limitations** — explicit list, including the voice/provider boundary:
  > A mod controls *how* BMO speaks (style) via `voice.txt`, but **cannot** choose
  > the TTS voice name, model, provider, or API endpoint. Those are the user's
  > `config.json` settings, tied to their account and credits. This keeps mods
  > portable and free to install.

  Other limits: self-contained mods don't inherit embedded faces once they ship
  one; malformed `mod.json` is tolerated (folds to defaults); no code execution —
  mods are data only.
- **Installing a mod** — unzip into `<dataRoot>/BMO/mods/<name>/`, select via
  Settings → MOD. Live re-read: persona/voice/quotes apply on the next interaction.
- **Create your first mod (step-by-step)** — a worked example building a small
  self-contained character from scratch (folder, `mod.json`, `persona.txt`,
  `voice.txt`, one or two faces), then selecting it on-device.
- **Reference** — links to the six spoke pages.

## Spoke pages — Scope

Each is a focused how-to with concrete examples.

- **faces.md** — relocated `FACES.md`, refreshed: the full expression list, SVG
  conventions, the `speaking.svg` Go-template parameters, alias resolution,
  self-contained vs overlay face semantics, and how to preview/render faces.
- **voice.md** — `voice.txt` speaking-style instructions: what they control
  (pitch, pace, accent, mood, delivery), worked examples (e.g. pirate, robotic),
  graceful degradation (applied on instruction-capable models, harmlessly ignored
  on `tts-1`), live re-read and fallback to the built-in default, and the
  **boundary** (no voice name / model / provider / endpoint — that's user config).
- **persona.md** — `persona.txt` system-prompt override; keep under ~1000 chars;
  example persona; interaction with device-awareness block.
- **quotes.md** — `quotes.txt` format (one per line, `#` comments, blanks ignored);
  example.
- **animations.md** — `mod.json` `animations` map (expression name → animation
  JSON); overlay-inherits vs self-contained-empty semantics; minimal example.
- **emotions.md** — `mod.json` `emotions` map (name → LLM description); how it
  feeds the emotion vocabulary the LLM may emit; example.

## Image Rendering Approach

- Source: the embedded face SVGs in `internal/face/assets/`.
- **Render through BMO's own oksvg rasterizer (`face.Rasterize`)**, not
  `rsvg-convert`/ImageMagick, because the device's oksvg renders some SVG arc
  sweeps differently; rendering via the BMO path makes the gallery match what
  users see on-device. Templated faces (e.g. `speaking.svg`) render through the
  same path.
- Mechanism: a small, self-contained render step (a `go test` generator or a tiny
  `cmd/` helper) that writes `docs/images/faces/<expression>.png` and
  `docs/images/banner.png`. This is a documentation/maintenance artifact, not a
  runtime feature. The implementation plan will choose the least-intrusive
  mechanism that reuses existing rasterization code.
- Output is committed PNGs so the docs render on GitHub without a build step.

## Out of Scope

- Mod-controlled TTS voice name / model / provider / endpoint (explicitly
  rejected; documented as a limitation).
- Device or settings-menu screenshots (rendered faces only).
- Any change to runtime behavior, config schema, or the mod loader.
- A Tier-2 voice-name suggestion mechanism in `mod.json` (considered and dropped).

## Testing / Verification

- `golangci-lint run ./...` and `CGO_ENABLED=0 go test ./...` stay green (the
  render helper, if added as code, must build and lint clean).
- Markdown links resolve (README → docs, hub → spokes, relocated `faces.md`
  references updated; no dangling `docs/FACES.md` links remain).
- Rendered PNGs exist and are referenced with correct relative paths.
- Spot-check that documented facts match the code: config field names, control
  mappings, data paths, `mod.json` schema, build/deploy commands.
