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

The only built-in animation is `speaking`: six mouth-openness frames driven
by real-time lip-sync amplitude.

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

Instead of listing explicit frame files, you can use a Go-template SVG
rendered at multiple parameter values:

```json
{
  "template": "speaking",
  "param": "MouthH",
  "from": 6,
  "to": 36,
  "steps": 6,
  "driver": "amplitude"
}
```

`template` is the base name of an `.svg` file containing `{{.MouthH}}`
(or another `{{.Param}}`) placeholders. BMO renders `steps` images across
`[from, to]` and uses them as frames. See [faces.md](./faces.md) for the
template parameters supported by the built-in `speaking.svg`.

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
