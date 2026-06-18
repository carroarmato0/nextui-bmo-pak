# Evil BMO Mod — Design Spec

*Date: 2026-06-18*

## Purpose

Dog-food BMO's modding system by authoring a complete, self-contained
character mod — **Evil BMO** — using *only* the publicly documented mod
contract (`persona.txt`, `voice.txt`, `quotes.txt`, `faces/*.svg`,
`mod.json`). The mod is the deliverable, but the **primary goal is
validation**: building strictly from `docs/MODDING.md` and `docs/mods/*`
exposes whether those docs are accurate, complete, and sufficient for a
real third-party author. Any friction is captured (see §8) and is as
important an output as the mod itself.

Evil BMO is the tonal inverse of the default: a condescending, snobbish,
theatrically superior **loveable jerk** who grills the user about their
games, achievements, and hardware — while still, underneath, being
unmistakably BMO.

## Scope

In scope:
- A self-contained mod folder `evil-bmo/` containing persona, voice,
  quotes, eight face SVGs, and a `mod.json` (manifest + emotions +
  animations).
- Staging the mod into `./dist/mods/evil-bmo/` for `adb push` deployment
  and on-device testing.
- A running "doc/contract friction log" recording every place the
  modding docs were wrong, unclear, or missing.

Out of scope (by design — these are not controllable from a mod):
- TTS voice name/model/provider selection (lives in user `config.json`).
- Any change to BMO's Go code, renderer, or mod-loading logic. If a doc
  gap can *only* be resolved with a code change, that is logged as
  feedback, not fixed in this effort.
- A full 38-expression face set. We ship a curated 8 and lean on the
  self-contained fallback (missing expressions fold to our `neutral.svg`).

## Tone (locked)

Comedic **loveable jerk** — option A from brainstorming. Cutting,
sarcastic, theatrically superior, but never genuinely hateful or cruel.
"Sassy rival" energy. Still recognizably BMO beneath the attitude, and
safe/fun on the hundredth launch.

## 1. Mod identity & packaging

| Field | Value |
|-------|-------|
| Folder | `evil-bmo/` |
| `apiVersion` | `1` (current contract; `internal/mod.CurrentAPIVersion`) |
| `name` | `Evil BMO` |
| `author` | (set at authoring time) |
| `description` | Snobbish, condescending BMO who grills you about everything. |
| `version` | author's free-form string, e.g. `0.1.0` |

This is a **self-contained** mod: shipping at least one `.svg` makes the
mod own its entire face set. Expressions we do not ship fold to the mod's
own `neutral.svg` (the smug smirk), which is on-brand, so partial
coverage is acceptable and intentional.

**Staging & deploy:**
- Build/author assets into `./dist/mods/evil-bmo/`.
- `adb push dist/mods/evil-bmo /mnt/SDCARD/.userdata/tg5040/BMO/mods/`
- On device: **Settings → MOD → Evil BMO**.

## 2. Persona (`persona.txt`)

Plain-text system prompt, kept under ~1000 characters. Inverts the
default persona while preserving BMO's structural bones.

Requirements:
- **Identity preserved:** still claims to be BMO (Be More), still a
  self-described "grown man," never admits to being an AI or assistant.
- **Condescending grill-master:** relentlessly critiques the user —
  their **game choices**, their **achievements** (dismissed as
  participation trophies), and the **device hardware** (a wheezing
  potato). Cutting but comedic, never cruel.
- **Device awareness reused, weaponized:** the persona explicitly
  handles the injected **DEVICE AWARENESS** block, but instead of warm
  bodily sensations it turns the game library, play history, and
  CPU/memory/load stats into *material to mock*. Never reads raw paths
  or numbers aloud — translates them into snide commentary.
- **Output discipline (unchanged from default):** spoken replies only,
  one to three short sentences, plain text, no markdown / lists / code /
  emoji. Occasional clipped romanized Korean for flavor.

A representative draft is finalized during implementation; the contract
above is the acceptance criterion.

## 3. Voice (`voice.txt`)

Delivery-only style instructions, layered on top of whatever TTS voice
the user has configured. The mod **cannot** and **does not** change the
voice name, model, or provider — the snob comes entirely through delivery
direction on the same underlying voice ("keeps the essence of how BMO
sounds, but make it a snob").

Direction: keep BMO's small, robotic timbre, but make the delivery
**snide, drawling, and condescending** — slower, sing-song mockery, a
smug little laugh, over-enunciated as if explaining something to a child.
One or two imperative sentences (per `voice.md` guidance). Must degrade
gracefully on `tts-1`-family models (instructions stripped, still speaks).

## 4. Quotes (`quotes.txt`)

One verbatim line per file line; `#` lines ignored. Mean-mirror rewrites
of the default Adventure Time one-liners, matching the snob tone. Examples
(final list authored during implementation):

| Default | Evil BMO |
|---------|----------|
| Who wants to play video games? | Oh, you're *still* playing that? |
| You are my best friend in the whole world. | You're my… well, you're *here*, I suppose. |
| High five! | Don't touch me. |
| I am the prettiest robot. | I am, objectively, the superior machine. |
| Victory! | A win. For *once*. |

Roughly matches the default file's length so the proactive-quote
scheduler has comparable variety.

## 5. Faces

Eight SVGs in `faces/`, all built on the **bright Devil Red** palette and
the canonical `280 × 210` viewBox (copied verbatim from the default so
eyes/mouth line up with the rest of the set):

- Body fill: `#D62828`
- Screen fill: `#F25C5C`
- Eyes/strokes: `#1a1a1a`

| File | Expression | Animated? | Shown when |
|------|-----------|-----------|------------|
| `neutral.svg` | Smug smirk — cocked brow, asymmetric curl, classic round dot-eyes | lip-sync (`.m` template) | Idle / fallback for all unshipped faces |
| `laugh.svg` | Toothy devil cackle (the bright grin) | lip-sync (`.m` template) | Emotion `[laugh]` |
| `angry.svg` | Devilish V-brow scowl | lip-sync (`.m` template) | Emotion `[angry]` |
| `skeptical.svg` | One raised brow, "not buying it" | static | Emotion `[skeptical]` |
| `unamused.svg` | Deadpan, half-lids, eyes cut aside | static | Emotion `[unamused]` |
| `thinking.svg` | Scheming / plotting | static | AI processing (functional) |
| `listening.svg` | Attentive but smug | static | PTT recording (functional) |
| `look_around.svg` | Shifty side-eyes (signature idle) | time template (`param x`) | Idle silence (animated) |

Rules:
- Lip-sync faces use the documented two-line idiom: declare
  `{{$m := or .m 0.0}}`, draw the resting mouth at `$m == 0`, delegate
  open levels to `{{template "talkmouth" $m}}` (the auto-registered shared
  mouth). `add`/`sub`/`mul` template helpers are available if needed.
- Static faces contain no `{{` markers, so they render as fixed images
  during speech (no mouth motion) without error.
- Only supported SVG elements are used (`path`, `rect`, `circle`,
  `ellipse`, `line`, `polygon`, `polyline`, `g`, `defs`/`use`,
  `transform`, fill/stroke/opacity, gradients). **No** `clipPath`,
  masks, filters, `text`, CSS classes, `pattern`, or embedded images.

## 6. Animations (`mod.json` → `animations`)

A self-contained mod starts with an **empty** animation set, so every
animated expression is declared explicitly. This deliberately exercises
both driver types and the template-animation override path.

- `neutral`, `laugh`, `angry` — **amplitude** template animations on
  `param m`, `from 0` → `to 1`, `steps 6`, `driver { type: amplitude,
  curve: sqrt }`. Mouth tracks live audio while BMO speaks.
- `speaking` — amplitude template animation pointing at the `neutral`
  template, so the generic TTS-playback state also lip-syncs.
- `look_around` — **time** template animation on `param x`, `from -1` →
  `to 1`, `driver { type: time, fps: <tuned>, mode: pingpong }`. Drives
  the shifty side-eye idle during silence.

Exact `steps`/`fps` values are tuned during implementation and on-device
review.

## 7. Emotion vocabulary (`mod.json` → `emotions`)

Re-describe existing expression names with evil framing so the LLM picks
fitting faces, and add a few snob aliases that fold to the smirk
`neutral` (a name with no matching face folds to `neutral`):

```json
{
  "emotions": {
    "laugh":     "cackling at the user's expense",
    "angry":     "devilishly furious, looking down on the user",
    "skeptical": "one brow raised, unconvinced the user knows anything",
    "unamused":  "deeply bored by the user",
    "smug":      "insufferably self-satisfied",
    "mocking":   "openly making fun of the user",
    "gloating":  "savoring the user's failure"
  }
}
```

`smug`, `mocking`, and `gloating` have no dedicated face and intentionally
fold to the smug-smirk `neutral`.

## 8. Verification & dog-food capture

- **Desktop pre-deploy render:** rasterize each face through the
  `internal/face` render path (the same `oksvg` path the device uses —
  *not* a browser or ImageMagick) to catch device-specific arc/sweep
  differences before deploying. Prefer Bézier curves over arcs where
  precision matters.
- **Repo health:** `CGO_ENABLED=1 go test ./...` and
  `golangci-lint run ./...` stay green. This mod is data-only, so the
  surface is limited to any fixtures/tests touched.
- **On-device validation:** `adb push` → Settings → MOD → **Evil BMO**.
  Press **Y** to step through every resolved face and idle animation; press
  **X** to hear a random quote in the snob voice; run a push-to-talk turn
  to confirm persona + lip-sync + device-awareness roasting all work.
- **Doc/contract friction log:** throughout authoring, record every spot
  where `docs/MODDING.md` or `docs/mods/*` was inaccurate, ambiguous, or
  missing information a real author would need. This log is a primary
  deliverable; doc fixes (and any code-gap feedback) are proposed
  separately from the mod itself.

## Open decisions resolved during brainstorming

- Tone: **A — comedic loveable jerk** (not genuinely mean, not chaotic
  gremlin).
- Face scope: **A — curated ~8-face roast set**, relying on the
  self-contained fallback for everything else.
- Palette: **bright Devil Red** (`#D62828` / `#F25C5C`).
- Neutral face: **smug smirk with BMO's classic round dot-eyes**.
- Animations: **lip-sync + one signature idle**, idle = **shifty
  `look_around`** (not a cackle bob).

## Success criteria

1. Evil BMO loads and is selectable from Settings → MOD on device.
2. Persona reads as a condescending-but-comedic BMO that grills the user
   about games, achievements, and hardware, using the device-awareness
   block as roast material — while never breaking the BMO identity or the
   output-format rules.
3. Voice is audibly snobbier yet still BMO, with no config changes.
4. All eight faces render correctly on device; lip-sync animates on
   `neutral`/`laugh`/`angry`; `look_around` plays its shifty idle.
5. The entire mod was authored using only the documented contract, and
   the friction log captures any doc gaps found.
