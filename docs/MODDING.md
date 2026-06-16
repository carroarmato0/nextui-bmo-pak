# Modding guide

A **mod** is a data-only directory that customises BMO's look, personality,
voice style, idle quotes, face animations, and emotion vocabulary — no code,
no compilation required.

Mods live under your device's data root:

```
<dataRoot>/BMO/mods/<name>/
```

For example, on a TrimUI Smart Pro:

```
/mnt/SDCARD/.userdata/tg5040/BMO/mods/my-bmo/
```

Select the active mod in **Settings → MOD**. Changes take effect immediately
— persona, voice, and quotes reload on the next interaction without a restart.

---

## Two kinds of mod

| Kind | Directory name | Fallback behaviour |
| --- | --- | --- |
| **Overlay** | `mods/default` | Each asset falls back individually to the embedded BMO art/text. Supply only the files you want to change. |
| **Self-contained** | `mods/<any-other-name>` | Once the mod ships at least one `.svg` face file, missing expressions fold to the mod's own `neutral.svg` (not to the embedded art). |

Use the overlay when you want to tweak one or two things and keep the rest of
BMO intact. Use a self-contained mod when you are building a distinct
character.

---

## Directory layout

```
<dataRoot>/BMO/mods/my-bmo/
  mod.json        (optional) metadata + emotions + animations
  persona.txt     (optional) personality / system prompt
  voice.txt       (optional) speaking-style instructions
  quotes.txt      (optional) idle quotes, one per line
  faces/          (optional) <expression>.svg overrides
  audio/          (optional) clip overrides (e.g. timeout.pcm)
```

Every file is optional. A directory with nothing in it is a valid (no-op) mod.

---

## `mod.json` schema

| Field | Type | Meaning |
| --- | --- | --- |
| `apiVersion` | int | mod-format version; absent/`0` ⇒ `1` |
| `name` | string | display name override |
| `author` | string | shown in Settings |
| `description` | string | shown in Settings |
| `version` | string | author's free-form release string |
| `emotions` | map[string]string | emotion name → LLM description |
| `animations` | map[string]json | expression name → animation JSON |

All fields are optional. A missing or malformed `mod.json` is tolerated —
BMO folds every unreadable field to its default.

### Full example

```json
{
  "apiVersion": 1,
  "name": "Glitch BMO",
  "author": "yourname",
  "description": "A corrupted, glitchy BMO variant.",
  "version": "0.1.0",
  "emotions": {
    "corrupted": "BMO's screen flickers and speech breaks up — like a corrupted memory card.",
    "rebooting": "BMO's eyes go blank and it counts down silently, as if restarting."
  }
}
```

---

## What you can customise

| File / field | What it controls | Details |
| --- | --- | --- |
| `faces/<expression>.svg` | Animated face graphics | [Face SVGs](mods/faces.md) |
| `voice.txt` | Speaking style (pitch, pace, accent, mood) | [Voice style](mods/voice.md) |
| `persona.txt` | Personality / LLM system prompt | [Persona](mods/persona.md) |
| `quotes.txt` | Idle quotes shown between interactions | [Quotes](mods/quotes.md) |
| `emotions` in `mod.json` | Emotion vocabulary sent to the LLM | [Emotions](mods/emotions.md) |
| `animations` in `mod.json` | SVG animation sequences | [Animations](mods/animations.md) |

---

## Limitations

- **Data only.** Mods contain no executable code. BMO never runs scripts
  from a mod directory.
- **Provider config is the user's domain.** A mod cannot set the TTS voice
  name, model, provider, or API endpoint — those live in `config.json`,
  tied to the user's account and credits.

  > A mod controls *how* BMO speaks (style) via `voice.txt`, but **cannot**
  > choose the TTS voice name, model, provider, or API endpoint. Those are
  > the user's `config.json` settings, tied to their account and credits.
  > This keeps mods portable and free to install.

- **Self-contained mods own their face set.** Once a self-contained mod ships
  at least one `.svg`, missing expressions fall back to that mod's own
  `neutral.svg`, not to the embedded BMO art. Include a `neutral.svg` if you
  supply any other faces.
- **Malformed `mod.json` is tolerated.** A JSON parse error causes the entire
  file to be treated as absent; valid fields in an otherwise broken file are
  not partially applied.
- **Emotion keys are additive.** The `emotions` map extends (or overrides)
  BMO's built-in emotion vocabulary; it does not replace it entirely.

---

## Installing a mod

1. Obtain the mod (e.g. unzip `my-bmo.zip`).
2. Copy the folder to `<dataRoot>/BMO/mods/my-bmo/` on the device SD card.
3. On BMO, press **Start** to open Settings.
4. Navigate to **MOD** and select `my-bmo`.
5. Close Settings. Persona, voice, and quotes apply on the next interaction
   — no restart needed.

For a TrimUI Smart Pro the full path would be:

```
/mnt/SDCARD/.userdata/tg5040/BMO/mods/my-bmo/
```

---

## Create your first mod

The steps below build a small self-contained character called **Pixel** — a
cheerful 8-bit robot sidekick.

### 1. Create the mod folder

On your SD card (mounted or via ADB):

```bash
mkdir -p /mnt/SDCARD/.userdata/tg5040/BMO/mods/pixel
```

### 2. Write `mod.json`

```json
{
  "apiVersion": 1,
  "name": "Pixel",
  "author": "yourname",
  "description": "A cheerful 8-bit robot sidekick.",
  "version": "0.1.0",
  "emotions": {
    "excited": "Pixel flashes bright colours and beeps rapidly.",
    "curious": "Pixel tilts its head and emits a soft question-mark chime."
  }
}
```

Save as `mods/pixel/mod.json`.

### 3. Write `persona.txt`

```
You are Pixel, a cheerful 8-bit robot sidekick living inside a retro handheld.
You speak in short, enthusiastic bursts, occasionally inserting retro sound
effects like "bleep!", "bloop!", or "zap!". You are endlessly optimistic and
treat every question as a new adventure.
```

Save as `mods/pixel/persona.txt`.

### 4. Write `voice.txt`

```
Bright, clipped, robotic cadence. Short bursts. Slightly metallic and
over-enunciated, like an old video-game announcer. Always upbeat.
```

Save as `mods/pixel/voice.txt`.

### 5. Add a face

Create `mods/pixel/faces/neutral.svg` with your custom expression. See the
[Face SVGs guide](mods/faces.md) for dimensions, coordinate system, and
the full expression catalogue.

If you skip this step, Pixel falls back to the embedded BMO neutral face.

### 6. Copy to device and select

```bash
adb push mods/pixel /mnt/SDCARD/.userdata/tg5040/BMO/mods/
```

Open **Settings → MOD** on the device and select **Pixel**. BMO becomes
Pixel on the next interaction.

---

## Reference

- [Face SVGs](mods/faces.md) — expression names, SVG canvas, colour palette
- [Voice style](mods/voice.md) — `voice.txt` format and examples
- [Persona](mods/persona.md) — `persona.txt` format and what the default establishes
- [Quotes](mods/quotes.md) — `quotes.txt` format and tips
- [Emotions](mods/emotions.md) — custom emotion vocabulary in `mod.json`
- [Animations](mods/animations.md) — animation JSON schema in `mod.json`
