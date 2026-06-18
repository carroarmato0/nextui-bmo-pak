[← Modding guide](../MODDING.md) · Animations

# Animations (`mod.json` → `animations`)

The `animations` map in `mod.json` lets you override the animation
definition for one or more expressions. This is an **advanced, optional**
feature. Most mods can omit the `animations` key entirely and get the
built-in speaking-mouth animation for free.

```json
{
  "animations": {
    "speaking": { ... }
  }
}
```

---

## When animations apply

- **Overlay mods** (the `default` mod) inherit the built-in animation set
  (`DefaultAnimations`). Entries in `animations` override by name.
- **Self-contained mods** (any named mod that ships at least one `.svg`)
  start with an empty animation set. If you want animated expressions you
  must declare them in `animations`.

The built-in set is larger than just `speaking`. Every core emotion
(`neutral`, `happy`, `smile`, `excited`, `content`, `concerned`, `sad`,
`angry`, and the expressive emotions) is a six-step **amplitude** template on
param `m` that lip-syncs while BMO talks; `speaking` is a six-frame amplitude
animation; and `look_around`, `whistle`, and `sleeping` are **time** animations
that play during silence. Overlay mods inherit all of them for free.

---

## Animation object shape

An animation object has exactly one frame source (`frames` or `template`,
not both) and a required `driver`.

### Frames-based animation

```json
{
  "frames": ["speaking_0", "speaking_1", "speaking_2",
             "speaking_3", "speaking_4", "speaking_5"],
  "driver": {
    "type": "amplitude",
    "curve": "sqrt",
    "idle": { "fps": 13, "mode": "pingpong" }
  }
}
```

This is the exact JSON equivalent of the built-in `speaking` animation.
Each name in `frames` must be a valid file base name (`[a-z0-9_-]+`) and
must exist as `<name>.svg` in the mod's `faces/` directory (or in the
embedded assets for overlay mods).

### Minimal valid example (three-frame amplitude mouth)

```json
{
  "frames": ["mouth_closed", "mouth_mid", "mouth_open"],
  "driver": "amplitude"
}
```

The `"amplitude"` string is a shorthand for `{"type": "amplitude",
"curve": "linear"}`. BMO steps through the frames in proportion to the
current audio amplitude.

---

## Driver types

### `amplitude`

Steps through frames in proportion to real-time audio amplitude (0–1).

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `type` | string | — | `"amplitude"` |
| `curve` | string | `"linear"` | `"linear"` or `"sqrt"` (sqrt gives more visible movement at low volumes) |
| `idle` | object | absent | Oscillation used when amplitude is unavailable or zero |
| `idle.fps` | number | — | Required if idle is present. Frames per second of the idle oscillation |
| `idle.mode` | string | `"loop"` | `"loop"`, `"pingpong"`, or `"once"` |

Shorthand: `"driver": "amplitude"` sets type=amplitude, curve=linear, no idle.

### `time`

Advances frames at a fixed rate, independent of audio.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `type` | string | — | `"time"` |
| `fps` | number | — | Required. Frames per second |
| `mode` | string | `"loop"` | `"loop"`, `"pingpong"`, or `"once"` |

---

## Template-based animation

Instead of listing explicit frame files, you can use a single Go-template SVG
rendered at multiple parameter values. This is how every built-in emotion
lip-syncs. The convention is param `m`, ranging `0` (rest) → `1` (mouth fully
open):

```json
{
  "template": "happy",
  "param": "m",
  "from": 0,
  "to": 1,
  "steps": 6,
  "driver": { "type": "amplitude", "curve": "sqrt" }
}
```

`template` is the base name of an `.svg` file containing `{{.m}}` (or whatever
name you set in `param`) placeholders. BMO renders `steps` images across
`[from, to]` and uses them as frames — here, six mouth-openness levels stepped
by voice amplitude. Inside the template, draw your rest mouth at `m == 0` and
open the shared mouth with `{{template "talkmouth" $m}}`; see
[faces.md](./faces.md#lip-syncing-mouth-m-templates) for the full idiom.

Time animations use the same template form with a clock parameter — e.g. the
built-in `look_around` renders `template: "look_around"` over `param: "x"` from
`-1` to `1` with a `time` driver, and `whistle` over `param: "t"` from `0` to
`1`.

`template` and `frames` are mutually exclusive; providing both (or
neither) is a parse error and the animation entry is silently skipped.

---

## Notes

- A malformed animation entry is skipped with a warning; the rest of the
  mod continues to load normally.
- Frame names must match `[a-z0-9_-]+` (no `.svg` extension, no path
  separators).
- For the `template` form, `steps` must be ≥ 2.
- For the `time` driver, `fps` must be > 0. For the `idle` sub-object
  of an `amplitude` driver, `idle.fps` must also be > 0.
