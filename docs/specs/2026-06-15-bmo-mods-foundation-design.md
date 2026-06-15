# BMO `mods/` Foundation — Design Spec

**Date:** 2026-06-15
**Status:** Approved (design)
**Scope:** Foundation only. The animation engine, generalized lip-sync, LLM
emotion-vocabulary derivation, and the modder tutorial are **deferred** to
follow-on specs. This spec defines the directory layout, resolution
semantics, discovery/manifest, config + Settings integration, and runtime
switching that those later specs build on.

## Motivation

BMO already lets users override individual aspects by dropping files in the
data home (`persona.txt`, `voice.txt`, `quotes.txt`, `faces/*.svg`,
`audio/*.pcm`). These overrides are flat, global, and all-or-nothing per file.
We want a **mods** system: a `mods/` directory where each subfolder is a
self-contained, selectable customization of BMO's look, voice, and persona.
This is the foundation that a declarative animation engine and emotion-hinting
will later sit on top of.

The app has **not shipped to anyone**, so there is no backward-compatibility
requirement: the flat override layout is replaced outright, not migrated.

## Directory Layout

All customizable assets move from the flat data home into per-mod folders:

```
$home/                         # e.g. /mnt/SDCARD/.userdata/tg5040/BMO/
  config.json
  mods/
    default/                   # special name: OVERLAY on embedded BMO
      persona.txt              # optional — overrides just the persona
      voice.txt                # optional
      quotes.txt               # optional
      faces/happy.svg          # optional — overrides just this one face
      audio/timeout.pcm        # optional — overrides just this one clip
    evil/                      # any other name: a standalone character
      mod.json                 # optional manifest
      persona.txt
      voice.txt
      quotes.txt
      faces/*.svg
      audio/*.pcm
```

Every aspect that is customizable today lives under a mod folder. Audio clip
overrides (`audio/`) are folded into the mod structure for consistency, even
though the original feature request named only persona/voice/quotes/faces.
Nothing customizable remains at the flat `$home/` root.

## Resolution Semantics

There are two asset classes and two fallback rules. Exactly one mod is active
at a time (selected in Settings, stored in config).

| Asset class | `mods/default` (overlay) | `mods/<named>` (character) |
|---|---|---|
| **Text** — persona, voice, quotes | file if present, else embedded default | file if present, else embedded default |
| **Audio clips** | file if present, else embedded default | file if present, else embedded default |
| **Faces** (SVG) | per-asset fallback to embedded | if `faces/` has ≥1 SVG → that set **only**, no embedded fallback; if `faces/` is empty/absent → inherit embedded faces |

Rules in prose:

- **Text and audio always fall back** to the embedded defaults when the active
  mod does not provide them — regardless of mod name. A named mod that only
  changes art keeps stock BMO's persona/voice/quotes/clips.
- **Faces fall back per-asset only for `mods/default`.** A named mod that ships
  any face owns its entire face set: requesting an expression the mod lacks
  folds to the mod's own `neutral` (via existing `face.Canonical` behavior),
  never to embedded BMO art. This prevents stock BMO faces from leaking into a
  custom character and looking inconsistent.
- **A named mod that ships no faces inherits embedded BMO faces** (so BMO is
  never faceless). Providing even one SVG flips it to "owns the whole set."

The only behavior keyed off the literal folder name is the face-fallback rule
for `default`. This is the teachable line for modders:

> *Edit `mods/default` to tweak stock BMO. Create `mods/<yourname>` to build a
> self-contained character.*

## Discovery and Manifest

A new package `internal/mod` is the single source of truth for locating and
resolving mod assets.

- `mod.Discover(modsRoot string) []Mod` scans `mods/` for subfolders and
  returns the selectable list in a stable order (`default` first if present,
  then alphabetical by folder name).
- A mod works with **zero configuration**: display name defaults to the folder
  name.
- An optional `mod.json` manifest enriches the Settings display:

```json
{
  "name": "Evil BMO",
  "author": "someone",
  "description": "BMO's mischievous twin",
  "version": "1.0"
}
```

Malformed or partial `mod.json` is tolerated: parse errors are logged and the
mod still appears using folder-name defaults. The manifest is the reserved home
for emotion hints introduced in the later emotion-vocabulary spec; this spec
adds only `name`, `author`, `description`, `version`.

### Proposed `internal/mod` shape

```go
type Manifest struct {
    Name        string `json:"name"`
    Author      string `json:"author"`
    Description string `json:"description"`
    Version     string `json:"version"`
}

type Mod struct {
    ID        string   // folder name; "" / "default" => the default entry
    Root      string   // absolute path to mods/<id>
    Manifest  Manifest // zero value if no mod.json
    IsDefault bool     // ID == "default"
}

// Resolution helpers (return resolved path + whether it exists on disk):
func (m Mod) PersonaPath() (string, bool)
func (m Mod) VoicePath()   (string, bool)
func (m Mod) QuotesPath()  (string, bool)
func (m Mod) FacesDir()    (string, bool) // dir + whether it holds ≥1 svg
func (m Mod) AudioDir()    (string, bool)

func Discover(modsRoot string) []Mod
func Load(modsRoot, id string) Mod        // resolve the active mod by id
```

Existing consumers (`config.LoadPromptFile`, `face.NewLibrary`,
`clips.NewLibrary`, quote parsing) keep their current signatures and are fed the
resolved paths/dirs by the active `Mod`.

## Config and Settings Integration

- New config field `ActiveMod string` (default `""`).
  - `""` resolves to the **"BMO (Default)"** entry: the `mods/default` overlay
    on embedded assets. This works even when `mods/default` does not exist, in
    which case it is pure embedded behavior (identical to today's stock BMO).
  - A non-empty value names a discovered `mods/<name>` folder. If the named
    folder is missing at load time, fall back to the default entry and log.
- Settings gains a **Mod** menu item:
  - Always lists **"BMO (Default)"** first.
  - Then every discovered `mods/<name>`, showing the manifest `name` and
    `description` when present, otherwise the folder name.
  - Selecting an entry writes `ActiveMod` to config and triggers a live reload.
- `config.CheckOverrides` / `config.RemoveOverrides` become **mod-aware**: they
  operate on the active mod's folder rather than the flat `$home/` root.

## Runtime Switching

Selecting a mod reloads live; no app restart:

- **Text and audio:** the existing per-utterance source callbacks
  (`systemPromptSource`, `ttsInstructionsSource`, quotes parsing, clip lookup)
  re-point at the newly active mod's root. Because these are already re-read per
  use, no extra machinery is needed beyond updating the active root.
- **Faces:** the `face.Library` + `face.Cache` are rebuilt against the new mod
  and re-warmed in the background, paying the same ~1s warm cost already
  incurred at startup. Until the warm completes, `Frame`/`Speak` lazily
  rasterize on demand (existing behavior), so there is no blank-face window.

## Face Library Change

`face.NewLibrary` (or a wrapper) gains a `selfContained bool` mode:

- `selfContained == false` (the `default` overlay): current behavior — raw name
  on disk → canonical on disk → embedded fallback.
- `selfContained == true` (a named mod with ≥1 face): raw → canonical on disk
  only; **no embedded fallback**. Missing expressions fold to the mod's
  `neutral` through `face.Canonical`.
- A named mod with an empty/absent `faces/` dir is constructed with
  `selfContained == false` against the embedded set (inherits embedded faces).

This `selfContained` flag is also what the later emotion-vocabulary spec keys
off: for a self-contained mod, the available expression set equals the mod's
face filenames.

## Testing

- `internal/mod` resolution table tests covering the full matrix: default vs
  named; each asset present vs missing; `faces/` empty vs populated; named mod
  with no faces inheriting embedded.
- `mod.Discover` ordering (`default` first, then alphabetical) and tolerance of
  non-directory entries / dotfiles.
- Manifest parsing: valid, partial, missing, and malformed `mod.json`.
- `config` round-trip with `ActiveMod` (default `""`, named value, and a named
  value whose folder is absent → falls back to default).
- Face library `selfContained` behavior: missing expression folds to mod neutral
  and does not fall back to embedded.
- Verification commands: `CGO_ENABLED=0 go test ./...` and
  `golangci-lint run ./...` (new code adds no findings).

## Explicitly Out of Scope (deferred to later specs)

1. **Declarative animation engine + spec format** — how named SVG
   elements/layers animate (transform, opacity, morph), replacing the hardcoded
   `speaking.svg` template + `speakParams`. Animated faces and their specs will
   live under each mod's `faces/`.
2. **Generalized lip-sync** — binding any mouth-like channel to live audio
   amplitude, rather than the current welded-to-`speaking.svg` path.
3. **LLM emotion-vocabulary derivation** — building the emotion protocol list
   from the active mod's face set + `mod.json` hints, instead of the hardcoded
   28-word vocabulary in `internal/assistant/emotion.go`.
4. **Modder tutorial / documentation** — written once the animation format is
   stable.

The layout in this spec reserves the homes for all four: `faces/` for animated
SVGs and their specs, `mod.json` for emotion hints.
