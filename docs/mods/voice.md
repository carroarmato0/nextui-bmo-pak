[← Modding guide](../MODDING.md) · Voice

# Voice style (`voice.txt`)

`voice.txt` is a plain-text file that steers *how* BMO speaks — its
pitch, pace, accent, mood, and delivery — layered on top of whichever
TTS voice the user has configured in their `config.json`.

The file's contents are sent as the `Instructions` parameter of every
TTS request. BMO re-reads the file before each interaction, so edits
take effect immediately without a restart.

When `voice.txt` is absent or blank, BMO falls back to the built-in
default:

> Speak in an extremely high-pitched, small, childlike voice — far above
> your natural register, like a sweet and excitable six-year-old robot
> child. You are BMO from Adventure Time. Use a clear, gentle Korean
> accent. Delivery: choppy sing-song staccato — each short phrase is its
> own cheerful burst. Always sound innocent, completely sincere, and
> delighted by everything.

---

## Examples

A gruff pirate character:

> Speak slowly with a deep, gravelly pirate growl. Roll your R's. Sound
> weary but a little menacing.

A terse robot:

> Bright, clipped, robotic cadence. Short bursts. Slightly metallic and
> over-enunciated.

---

## Tips

- Keep instructions short and concrete — one or two sentences describing
  pitch, pace, accent, and mood is enough.
- Write in the imperative: "Speak with …", "Use a …", "Sound like …".
- Avoid references to specific real voices or actors; describe the quality
  you want instead.

---

## Graceful degradation

Instruction-capable models (e.g. `gpt-4o-mini-tts`) apply the style
directive. Basic models (`tts-1` family) receive no instructions — BMO
strips the field before sending the request — and still speak normally.
Your mod works correctly on both model tiers without any extra handling.

---

> **What a mod CANNOT control (by design)**
>
> A mod shapes *how* BMO speaks, not *which* voice. It **cannot** select
> the TTS voice name (`nova`, `alloy`…), model, provider, or API endpoint
> — those live in the user's `config.json`, tied to their account and
> credits. This keeps mods **portable and free to install**: a mod never
> forces a user onto a paid model or breaks on a different backend.
