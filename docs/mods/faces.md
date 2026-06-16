[← Modding guide](../MODDING.md) · Faces

# Face SVGs

BMO's expressions are SVG files placed in the mod's `faces/` directory.
An overlay mod (the `default` mod) overrides only the files you supply —
missing expressions fall back to the built-in BMO art. A self-contained
mod (any named mod that ships at least one `.svg`) owns its entire face
set; missing expressions fold to the mod's own `neutral.svg`.

---

## Illustrated expressions

| Neutral | Happy | Surprised | Love | Sad |
| --- | --- | --- | --- | --- |
| ![neutral](../images/faces/neutral.png) | ![happy](../images/faces/happy.png) | ![surprised](../images/faces/surprised.png) | ![love](../images/faces/love.png) | ![sad](../images/faces/sad.png) |

---

## Expression catalog

Place any of these files in the mod's `faces/` directory to replace that expression.
You do not need to provide all of them — missing files use the built-in default
(overlay mods) or the mod's own `neutral.svg` (self-contained mods).

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

---

## SVG format

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
The TrimUI Smart Pro and Brick (tg5040) both run at 1024×768 (4:3). Design
faces in a 4:3 proportion and they will render correctly on device. Verify
resolution with `adb shell cat /sys/class/graphics/fb0/modes` if you are
unsure about a target device.

**Supported elements:** `path` (all commands), `rect`, `circle`, `ellipse`,
`line`, `polygon`, `polyline`, `g`, `defs`/`use`, `transform`,
fill/stroke/opacity, linear and radial gradients.

**Not supported:** `clipPath`, masks, filters, `text`, CSS classes or
stylesheets, `pattern`, embedded images. A file that fails to parse is logged
and the built-in default is used instead — BMO never shows a broken face.

---

## Alias names

You can also use alias filenames. For example, `cry.svg` resolves to `crying`,
`shocked.svg` to `surprised`, and `tongue.svg` to `playful` when no exact
override exists. The lookup order is: exact filename → canonical name → built-in
default. (`happy`, `laugh`, `excited`, `sad`, and `angry` are now their own
expressions, not aliases of `smile`/`concerned`.)

---

## Speaking mouth (`speaking.svg`)

The built-in `speaking.svg` is a **Go template** that is rendered at multiple
mouth-openness levels to animate the speaking mouth with audio amplitude.

If your override file contains `{{` template markers, BMO treats it as a
template and renders all levels. The available parameters are:

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

## Previewing your faces

Maintainers render the full embedded face set with:

```
go run ./cmd/render-faces
```

On-device rendering uses the `oksvg` library. ImageMagick (`rsvg`) and browsers
may render SVG arc sweeps differently. To catch device-specific differences
before deploying, preview via `face.Rasterize` (the same path the device uses)
rather than a desktop SVG viewer. In particular, degenerate arc sweeps that look
correct in ImageMagick can render opposite on the device — use Bézier curves for
rounded shapes where precision matters.

---

## Technical notes

- Face files are re-read on each expression change; editing a file takes effect
  at the next expression transition (no restart needed).
- Persona, voice, and quotes are re-read before each AI interaction; changes
  take effect immediately.
- The renderer cross-compiles for tg5040/tg5050 using LoveRetro toolchain
  containers (`scripts/release.sh`). The `internal/face` package is pure Go
  and adds no CGO or platform-specific dependencies.
