# BMO

> *Be more.*  Fullscreen BMO-inspired AI assistant and desk companion for NextUI handhelds.

![BMO](docs/images/banner.png)

[![Ko-Fi](https://ko-fi.com/img/githubbutton_sm.svg)](https://ko-fi.com/carroarmato0)
![Platforms](https://img.shields.io/badge/platforms-tg5040%20%C2%B7%20tg5050-blue)
![License](https://img.shields.io/badge/license-MIT-green)
![Go](https://img.shields.io/badge/go-1.25-00ADD8)

---

## What is BMO?

BMO is a fullscreen, animated AI voice assistant and desk companion packaged
as a NextUI **Tool** pak. It runs entirely on-device, speaks via configurable
STT / Chat / TTS providers, and shows a living BMO face that reacts
emotionally to every exchange.

*Unofficial Adventure Time fan project — not affiliated with Cartoon Network
or Warner Bros.*

---

## Features

- **Animated SVG face** — 30+ expressions driven by a declarative animation
  engine.
- **LLM-directed emotion** — the chat model picks BMO's expression; the face
  updates automatically.
- **Voice assistant pipeline** — push-to-talk → speech-to-text → chat →
  text-to-speech, with streaming audio playback.
- **Idle quotes & pre-recorded clips** — BMO mutters at you between
  interactions.
- **Device awareness** — BMO knows your game library, play history, and
  system stats, woven into its personality.
- **Mods** — swap BMO's face set, persona, voice style, quotes, and emotion
  vocabulary without touching code.
- **Idle and AI modes** — run BMO as a silent animated clock/companion or as
  a full voice assistant.
- **Reduced-motion option** — disable animations for lower CPU usage.

---

## Supported devices

| Platform | Devices |
| --- | --- |
| `tg5040` | TrimUI Brick · TrimUI Smart Pro |
| `tg5050` | TrimUI Smart Pro S |

The correct binary is selected automatically at launch.

---

## Gallery

| Neutral | Happy | Surprised | Love | Sparkle | Sleeping |
| --- | --- | --- | --- | --- | --- |
| ![neutral](docs/images/faces/neutral.png) | ![happy](docs/images/faces/happy.png) | ![surprised](docs/images/faces/surprised.png) | ![love](docs/images/faces/love.png) | ![sparkle](docs/images/faces/sparkle.png) | ![sleeping](docs/images/faces/sleeping.png) |

---

## Installation

1. Download `BMO.pak.zip` from the [Releases](https://github.com/carroarmato0/nextui-bmo/releases) page.
2. Unzip the archive — you will get a `BMO.pak/` folder.
3. Copy `BMO.pak/` onto your SD card:
   ```
   Tools/<platform>/BMO.pak/
   ```
   For example: `Tools/tg5040/BMO.pak/`
4. Eject the SD card, insert it into your device, and launch **BMO** from
   NextUI's Tools menu.

---

## Configuration

The config file is created automatically on first launch:

```
<dataRoot>/BMO/config.json
```

For example, on a TrimUI Smart Pro:

```
/mnt/SDCARD/.userdata/tg5040/BMO/config.json
```

BMO ships in **idle mode** (no AI). To enable the assistant you edit
`config.json` directly — provider endpoints, models, and API keys are entered
in the file, not typed on-device. (Once they're listed, you *can* switch the
active provider from the **Settings** menu — see below.) After editing, set
`"mode": "ai"` (or flip the toggle in the on-device **Settings** menu, opened
with **Start**).

### AI providers

BMO talks to three providers — **STT** (speech-to-text), **chat**, and **TTS**
(text-to-speech) — each spoken to over the **OpenAI-compatible HTTP API**. Any
backend that implements `/audio/transcriptions`, `/chat/completions`, and
`/audio/speech` works (OpenAI itself, a local server, or any compatible host).

Each of `stt`, `chat`, and `tts` is a **provider set** — not a single
provider. A set has two keys:

| Field | Description |
| --- | --- |
| `active` | `name` of the provider to use right now. If empty or unmatched, the first entry in `providers` is used. |
| `providers` | An **array** of interchangeable backends. List as many as you like; switch between them on-device from **Settings → STT / CHAT / TTS** (cycle left/right) without editing the file. |

Each entry in the `providers` array accepts these fields:

| Field | Required | Description |
| --- | --- | --- |
| `name` | yes | Display label for the provider (e.g. `openai-compatible`, `local`). Must be unique within the set; this is what `active` matches against. |
| `model` | yes | Model id to request (e.g. `whisper-1`, `gpt-4o-mini`). |
| `base_url` | **yes** | API endpoint, including the `/v1` suffix. There is **no** built-in default — requests fail if it is empty. |
| `api_key` | for hosted APIs | Bearer token. Leave empty (`""`) for most local servers. |
| `voice` | TTS only | Voice name for speech synthesis (e.g. `nova`, `alloy`). |

#### Example: OpenAI

```json
{
  "mode": "ai",
  "stt": {
    "active": "openai-compatible",
    "providers": [
      {
        "name": "openai-compatible",
        "model": "whisper-1",
        "base_url": "https://api.openai.com/v1",
        "api_key": "sk-..."
      }
    ]
  },
  "chat": {
    "active": "openai-compatible",
    "providers": [
      {
        "name": "openai-compatible",
        "model": "gpt-4o-mini",
        "base_url": "https://api.openai.com/v1",
        "api_key": "sk-..."
      }
    ]
  },
  "tts": {
    "active": "openai-compatible",
    "providers": [
      {
        "name": "openai-compatible",
        "model": "gpt-4o-mini-tts",
        "base_url": "https://api.openai.com/v1",
        "api_key": "sk-...",
        "voice": "nova"
      }
    ]
  },
  "ptt_buttons": ["BTN_EAST"],
  "active_mod": "",
  "reduced_motion": false
}
```

#### Example: local / self-hosted (OpenAI-compatible)

Point `base_url` at your own server — for example a local LLM runtime
(Ollama, LM Studio, llama.cpp, vLLM, …) or any OpenAI-compatible host on your
network. Use the LAN IP of the host machine (not `localhost`, which on the
device refers to the device itself). Most local servers ignore the key, so
`api_key` can be left empty.

The `chat` set below lists **two** models. BMO starts on `local-fast` (the
`active` one) and you can flip to `local-smart` from **Settings → CHAT** without
touching the file:

```json
{
  "mode": "ai",
  "stt": {
    "active": "local",
    "providers": [
      {
        "name": "local",
        "model": "whisper-1",
        "base_url": "http://192.168.1.50:8080/v1",
        "api_key": ""
      }
    ]
  },
  "chat": {
    "active": "local-fast",
    "providers": [
      {
        "name": "local-fast",
        "model": "llama3.1",
        "base_url": "http://192.168.1.50:11434/v1",
        "api_key": ""
      },
      {
        "name": "local-smart",
        "model": "llama3.1:70b",
        "base_url": "http://192.168.1.50:11434/v1",
        "api_key": ""
      }
    ]
  },
  "tts": {
    "active": "local",
    "providers": [
      {
        "name": "local",
        "model": "tts-1",
        "base_url": "http://192.168.1.50:8080/v1",
        "api_key": "",
        "voice": "alloy"
      }
    ]
  },
  "ptt_buttons": ["BTN_EAST"],
  "active_mod": "",
  "reduced_motion": false
}
```

You can mix and match — e.g. a local chat model with hosted STT/TTS — by giving
each provider its own `base_url` and `api_key`.

`ptt_buttons` lists the push-to-talk button(s) by Linux event name
(`BTN_EAST` is the physical **A** button); the default is `["BTN_EAST"]`.

#### Wake word (hands-free)

Turn on **Settings → WAKE WORD** to trigger BMO by voice instead of holding a
button. An on-device detector (openWakeWord via onnxruntime) listens only while
BMO is idle; when it hears the wake phrase it records your utterance and runs
the same pipeline push-to-talk uses. Detection is suppressed while BMO is
speaking (plus a short guard) so it never wakes on its own voice.

- **CONTINUED CONVO** (`continued_conversation`: `off`/`short`/`long`, default
  `short`) reopens a follow-up window after each reply so you — or another BMO —
  can keep talking without re-triggering. `long` is tuned for two-BMO
  conversations.
- **LISTEN PATIENCE** (`wake_end_silence`: `snappy`/`balanced`/`patient`, default
  `balanced`) sets how long a pause must last before BMO treats your sentence as
  finished. `patient` avoids cutting off slow or thoughtful speech; `snappy`
  responds sooner. It is independent of **CONTINUED CONVO** (which controls the
  follow-up window after a reply).
- The always-on microphone has a real battery/thermal cost, so the wake word is
  **off by default**.
- The shipped model is BMO's own **"Hey BMO"** classifier (say "Beemo"). You can
  train your own wake phrase with the documented, GPU/Colab pipeline in
  [`training/wakeword/`](training/wakeword/README.md) — it produces a drop-in
  model that obeys the same `[1,16,96] → [1,1]` contract.
- A **mod** can ship its own wake phrase as `wakeword/wake.onnx`; it replaces
  "Hey BMO" while that mod is active (see the [Modding guide](docs/MODDING.md)).

The detector's onnxruntime library and models ship inside the pak
(`lib/<platform>/libonnxruntime.so`, `assets/wakeword/*.onnx`).

### Controls

| Button | Action |
| --- | --- |
| A | Push-to-talk / confirm |
| B | Cancel / exit |
| X | Speak a random quote |
| Y | Next face / animation (gallery preview) |
| Start | Open/close Settings |
| Menu | Exit to NextUI |

---

## Documentation

- [Self-hosted speech (faster-whisper + Piper)](docs/self-hosted-speech.md)

---

## Mods

Mods are data-only directories that override BMO's face set, persona, voice
style, idle quotes, and emotion vocabulary — no code, no compilation needed.
Drop a mod folder into `<dataRoot>/BMO/mods/` and select it in
**Settings → MOD**.

See the [Modding guide](docs/MODDING.md).

---

## FAQ

**I launched BMO but it just sits there — why won't it talk to me?**
BMO ships in **idle mode** with no AI configured. It's a silent animated
companion until you fill in providers and set `"mode": "ai"` in `config.json`
(or flip the toggle in **Settings**, opened with **Start**). See
[Configuration](#configuration).

**BMO won't start / a log mentions `stt has no providers`.**
Each of `stt`, `chat`, and `tts` must be a **provider set**, not a bare
provider — wrap your provider in `{ "active": "...", "providers": [ ... ] }`.
A common slip when hand-writing the file is dropping the `providers` array and
putting the fields directly under `stt`. Copy the [examples above](#example-openai)
as your starting point.

**Do I need an OpenAI account, or can I run everything offline?**
Anything OpenAI-compatible works. Point `base_url` at a local server
(Ollama, LM Studio, llama.cpp, vLLM, faster-whisper, Piper, …) and BMO never
leaves your network. See
[Self-hosted speech](docs/self-hosted-speech.md). Use the host's **LAN IP**,
not `localhost` (on the device that means the device itself).

**How much does it cost to run?**
BMO itself is free. If you point it at a hosted API (e.g. OpenAI) you pay that
provider's usual per-request rates for STT/chat/TTS. A fully local setup costs
nothing beyond your own hardware and electricity.

**Can I switch models without re-editing the file every time?**
Yes. List several backends in a set's `providers` array and cycle the active
one from **Settings → STT / CHAT / TTS** (left/right). The local example above
shows a fast and a smart chat model side by side.

**Does the wake word drain my battery?**
Yes — an always-on microphone has a real battery and thermal cost, which is why
the wake word is **off by default**. Turn it on under **Settings → WAKE WORD**
when you want hands-free triggering.

**The wake word doesn't trigger (or triggers on its own).**
Say "**Beemo**" — the shipped model is BMO's own "Hey BMO" classifier.
Detection is suppressed while BMO is speaking (plus a short guard) so it never
wakes on its own voice. You can train your own phrase via
[`training/wakeword/`](training/wakeword/README.md), or a mod can ship one as
`wakeword/wake.onnx`.

**Which button is push-to-talk?**
**A** (`BTN_EAST`). Hold it to talk; **B** cancels or exits. Change it with
`ptt_buttons` in `config.json`. See [Controls](#controls).

**Can I change BMO's face, voice, or personality?**
Yes — drop a **mod** into `<dataRoot>/BMO/mods/` and select it in
**Settings → MOD**. Mods override the face set, persona, voice style, idle
quotes, and emotion vocabulary with no code. See the
[Modding guide](docs/MODDING.md).

**Which devices are supported?**
TrimUI Brick and Smart Pro (`tg5040`), and Smart Pro S (`tg5050`). The correct
binary is picked automatically at launch.

**Is this an official Adventure Time / Cartoon Network app?**
No. BMO is an unofficial fan project, not affiliated with or endorsed by
Cartoon Network or Warner Bros.

---

## Building from source

```bash
# Build & test locally (needs CGO + SDL2 dev libraries for the renderer)
CGO_ENABLED=1 go build ./...
CGO_ENABLED=1 go test ./...
golangci-lint run ./...

# Cross-compile + package the pak (docker/podman)
./scripts/release.sh

# Deploy to a connected device (ADB) or SD path
./scripts/deploy.sh
# Tail device logs
./scripts/debug-logs.sh
```

Requires Go 1.25. The release script uses Docker or Podman to cross-compile
for `tg5040` and `tg5050`.

---

## Support

[![Ko-Fi](https://ko-fi.com/img/githubbutton_sm.svg)](https://ko-fi.com/carroarmato0)

If BMO brightens your handheld, consider [buying me a coffee](https://ko-fi.com/carroarmato0). 💖

---

## License

Released under the [MIT License](LICENSE).

*BMO is an unofficial fan project inspired by the Adventure Time character.
Not affiliated with or endorsed by Cartoon Network or Warner Bros.*
