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
`config.json` directly — the AI provider, model, endpoint, and API key are
**not** configurable from the on-device UI. After editing, set `"mode": "ai"`
(or flip the toggle in the on-device **Settings** menu, opened with **Start**).

### AI providers

BMO talks to three providers — **STT** (speech-to-text), **chat**, and **TTS**
(text-to-speech) — each spoken to over the **OpenAI-compatible HTTP API**. Any
backend that implements `/audio/transcriptions`, `/chat/completions`, and
`/audio/speech` works (OpenAI itself, a local server, or any compatible host).

Each provider block accepts these fields:

| Field | Required | Description |
| --- | --- | --- |
| `name` | yes | Display label for the provider (e.g. `openai-compatible`, `local`). |
| `model` | yes | Model id to request (e.g. `whisper-1`, `gpt-4o-mini`). |
| `base_url` | **yes** | API endpoint, including the `/v1` suffix. There is **no** built-in default — requests fail if it is empty. |
| `api_key` | for hosted APIs | Bearer token. Leave empty (`""`) for most local servers. |
| `voice` | TTS only | Voice name for speech synthesis (e.g. `nova`, `alloy`). |

#### Example: OpenAI

```json
{
  "mode": "ai",
  "stt":  { "name": "openai-compatible", "model": "whisper-1",        "base_url": "https://api.openai.com/v1", "api_key": "sk-..." },
  "chat": { "name": "openai-compatible", "model": "gpt-4o-mini",      "base_url": "https://api.openai.com/v1", "api_key": "sk-..." },
  "tts":  { "name": "openai-compatible", "model": "gpt-4o-mini-tts",  "base_url": "https://api.openai.com/v1", "api_key": "sk-...", "voice": "nova" },
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

```json
{
  "mode": "ai",
  "stt":  { "name": "local", "model": "whisper-1",   "base_url": "http://192.168.1.50:8080/v1", "api_key": "" },
  "chat": { "name": "local", "model": "llama3.1",    "base_url": "http://192.168.1.50:11434/v1", "api_key": "" },
  "tts":  { "name": "local", "model": "tts-1",       "base_url": "http://192.168.1.50:8080/v1", "api_key": "", "voice": "alloy" },
  "ptt_buttons": ["BTN_EAST"],
  "active_mod": "",
  "reduced_motion": false
}
```

You can mix and match — e.g. a local chat model with hosted STT/TTS — by giving
each provider its own `base_url` and `api_key`.

`ptt_buttons` lists the push-to-talk button(s) by Linux event name
(`BTN_EAST` is the physical **A** button); the default is `["BTN_EAST"]`.

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
