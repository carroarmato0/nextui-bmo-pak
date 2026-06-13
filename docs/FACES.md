# BMO: Faces, Persona, Voice, Quotes

BMO's appearance and personality can be customised by placing override files
in the BMO data directory alongside `config.json`:

```
<dataRoot>/BMO/
  faces/          ← SVG overrides (any subset of the files below)
  persona.txt     ← system-prompt override
  voice.txt       ← TTS speaking-style override
  quotes.txt      ← verbatim quotes override
```

If a file is absent or blank, BMO falls back to its built-in default.
A pak update always ships the latest built-in defaults; overrides are
untouched by updates, so your customisations persist.

---

## Face SVGs (`faces/`)

Place any of these files in the `faces/` directory to replace that expression.
You do not need to provide all of them — missing files use the built-in default.

| File | Expression | When shown |
|------|-----------|------------|
| `neutral.svg` | Idle / default | Waiting for input |
| `blink.svg` | Blink | Periodic eye blink |
| `listening.svg` | Listening | PTT recording active |
| `thinking.svg` | Thinking | AI processing |
| `speaking.svg` | Speaking | TTS playback |
| `sleeping.svg` | Sleeping | Quota exhausted |
| `concerned.svg` | Concerned | Error / setup required |
| `smile.svg` | Smile | Happy / excited |

### SVG format

Every face is a **full scene** in a `280 × 210` viewBox:

```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210">
  <!-- body -->
  <rect x="0" y="0" width="280" height="210" fill="#4ECBA8"/>
  <rect x="6" y="5" width="268" height="200" rx="26" fill="#4ECBA8"/>
  <!-- screen -->
  <rect x="22" y="20" width="202" height="155" rx="14" fill="#90e5c8"/>
  <!-- face elements go here -->
</svg>
```

The viewBox is stretched non-uniformly to fill the screen (no letterboxing).
On 16:9 devices (Smart Pro 1280×720) BMO appears slightly wider than on 4:3
devices (Brick 1024×768) — design for the 4:3 reference and it will look fine
on both.

**Supported elements:** `path` (all commands), `rect`, `circle`, `ellipse`,
`line`, `polygon`, `polyline`, `g`, `defs`/`use`, `transform`,
fill/stroke/opacity, linear and radial gradients.

**Not supported:** `clipPath`, masks, filters, `text`, CSS classes or
stylesheets, `pattern`, embedded images. A file that fails to parse is logged
and the built-in default is used instead — BMO never shows a broken face.

### Alias names

You can also use alias filenames. For example, `laugh.svg` and `excited.svg`
both resolve to the `smile` expression when no `smile.svg` override exists.
The lookup order is: exact filename → canonical name → built-in default.

### Speaking mouth (`speaking.svg`)

The built-in `speaking.svg` is a **Go template** that is rendered at 12
mouth-openness levels to animate the speaking mouth with audio amplitude.

If your override file contains `{{` template markers, BMO treats it as a
template and renders all 12 levels. The available parameters are:

| Parameter | Description |
|-----------|-------------|
| `{{.MouthH}}` | Mouth rectangle height in viewBox units (6–36) |
| `{{.MouthRx}}` | Mouth rectangle corner radius |
| `{{.TeethPath}}` | Pre-computed SVG path data for the teeth band |
| `{{.InteriorPath}}` | Pre-computed SVG path data for the mouth interior |
| `{{.TonguePath}}` | Pre-computed SVG path data for the tongue |

If your override has **no** `{{` markers it is used as a **static face** during
speech — BMO will not animate the mouth, but your design will display correctly.

---

## Persona (`persona.txt`)

Plain text. Replaces the system prompt sent to the AI on every turn.
Keep it under ~1000 characters for best results.

---

## Voice (`voice.txt`)

Plain text. Replaces the TTS speaking-style instructions.

---

## Quotes (`quotes.txt`)

One quote per line (blank lines ignored). BMO displays these while idle.

---

## Mod pack layout

A complete mod pack can be distributed as a zip:

```
MyMod.zip
  faces/
    neutral.svg
    smile.svg
    speaking.svg   ← static or template
  persona.txt
  voice.txt
  quotes.txt
```

Unzip into `<dataRoot>/BMO/` and restart BMO. Remove or blank individual
files to revert to the built-in defaults, or use the settings menu's
**Restore Defaults** option to delete all overrides at once.

---

## Technical notes

- Face files are read on each expression change; editing a file takes effect
  at the next expression transition (no restart needed).
- Persona, voice, and quotes are re-read before each AI interaction; changes
  take effect immediately.
- The renderer cross-compiles for tg5040/tg5050 using LoveRetro toolchain
  containers (`scripts/release.sh`). The `internal/face` package is pure Go
  and adds no CGO or platform-specific dependencies.
