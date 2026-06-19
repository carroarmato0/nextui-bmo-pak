# Self-Hosted STT/TTS + On-Device Wake-Word — Design

**Date:** 2026-06-19
**Status:** Approved design, pending implementation plan

## Summary

Make BMO work with self-hosted speech models (faster-whisper for STT, Piper for
TTS) as alternatives to OpenAI, and add an optional on-device wake-word ("Hey
BMO") so BMO can be triggered hands-free — including the playful case of two
BMOs holding a conversation.

The work is one design, delivered in **two phases**:

- **Phase 1 — Self-hosted STT/TTS.** Independently valuable and shippable.
  Reuses the existing OpenAI-compatible HTTP client; the real work is making TTS
  audio handling robust. De-risks the audio plumbing Phase 2 builds on.
- **Phase 2 — On-device wake-word + continued conversation.** Adds an always-on
  listening mode and a follow-up window, layered on top of the Phase 1 pipeline.

## Goals

- BMO's STT and TTS can point at a self-hosted, OpenAI-compatible server
  (Speaches) instead of OpenAI, with no loss of function.
- TTS playback is correct regardless of the server's audio sample rate (Piper is
  typically 22050 Hz; OpenAI is 24000 Hz).
- Misconfigured/incompatible audio endpoints fail loudly instead of playing
  garbage.
- Optional, default-off wake-word that puts the mic in always-on mode and
  triggers BMO on "Hey BMO".
- Push-to-talk always preempts wake-word listening.
- Optional continued-conversation mode (auto follow-up window) that also enables
  BMO-to-BMO conversation.

## Non-Goals

- Native Wyoming / Porcupine integration (rejected — see Decisions).
- Streaming-playback rewrite to exploit chunked TTS (the pipeline plays the full
  synthesized buffer; out of scope).
- Cloud or server-side "voice satellite" topology (rejected — BMO stays
  autonomous and on-device).
- Acoustic echo cancellation hardware/DSP (we gate detection during playback
  instead).

## Decisions (settled during brainstorming)

| Decision | Choice | Rationale |
|---|---|---|
| STT/TTS transport | **OpenAI-compatible HTTP (Speaches)** | Speaches serves faster-whisper + Piper from one `/v1` server; reuses `OpenAICompatibleClient`; device stays HTTP-only |
| Wyoming protocol | **Rejected** | Built for always-on server "satellites"; its STT isn't truly streaming; needs a hand-rolled TCP client; BMO is push-to-talk batch and stays autonomous |
| Topology | **Autonomous on-device BMO** | The two-BMO use case requires each unit to wake and run its own persona; the opposite of a shared server satellite |
| TTS audio format | **Request WAV, parse header for sample rate** | Removes the 24 kHz hardcode; works for Piper (22050) and OpenAI alike; one code path |
| Wake-word engine | **openWakeWord** | Apache-2.0, ONNX, ~200 KB, runs many models on one RPi3 core; custom words trained via Piper; Porcupine rejected (proprietary, 30-day key expiry, commercial licensing) |
| Wake-word runtime | **ONNX via `onnxruntime_go` (CGO)** | We already cross-compile with CGO for SDL; Brick has NEON (`asimd`) for ARM64 kernels |
| Continued conversation | **One setting, configurable window (Off / Short / Long)** | One mechanism subsumes both human follow-ups and BMO-to-BMO latency spanning |

## Hardware facts (TrimUI Brick, serial 4c00…2229d, verified via adb)

- CPU: 4× Cortex-A53 (`0xd03`), ARMv8-A, up to 2.0 GHz; `Features: fp asimd aes
  pmull sha1 sha2 crc32` → NEON present.
- RAM: ~1 GB total, ~753 MB available.
- cpufreq governors available: `interactive conservative userspace powersave
  ondemand performance schedutil`; default `schedutil`. The `performance`
  governor can pin max frequency for a wake/STT burst.

Conclusion: Phase 2 is feasible on this hardware. The only residual unknown is
the cross-compiled `onnxruntime` CGO build for the A133 target — addressed by a
spike (see Phase 2, task 0).

## Background: current architecture

- `internal/providers/OpenAICompatibleClient` speaks four OpenAI REST paths:
  `POST /chat/completions`, `POST /audio/transcriptions`, `POST /audio/speech`,
  `GET /models`. `base_url` + `api_key` are configured **per capability**
  (`config.ProviderSet`), so STT, chat, and TTS can each point at a different
  server.
- `assistant.VoicePipeline.ProcessBatch` is the batch flow: PCM utterance → STT
  → chat → TTS → paced playback. `cmd/bmo-pak/ptt_shared.go:startPushToTalk`
  feeds it: button held → `audio.CaptureRouter.Batches()` appended to a buffer →
  release → `pipeline.ProcessBatch(ctx, utterance)`.
- Three concrete gaps block self-hosting today:
  1. **TTS sample-rate hardcode.** `voice.go` has `ttsPCMSampleRate = 24000` and
     `resampleTTS` assumes raw headerless 24 kHz mono PCM. Piper at 22050 Hz
     would play ~9% slow/low.
  2. **`Speak()` error detection.** It only treats `StatusCode >= 400` as
     failure; a server that returns HTTP 200 with a JSON error body (observed on
     LM Studio for unknown endpoints) would feed the error string to the audio
     player as if it were audio.
  3. Plain LLM servers (Ollama/LM Studio) implement no audio endpoints — hence
     Speaches, which does.

---

## Phase 1 — Self-Hosted STT/TTS

### P1.1 TTS audio-format handling (the core change)

- Change the TTS request to `response_format=wav` (instead of `pcm`). OpenAI,
  Speaches/Piper, and openedai-speech all support WAV.
- Parse the returned WAV header to read the **actual** sample rate and channel
  count, then resample from that rate to the device playback rate. Reuse the
  existing little-endian WAV knowledge in `providers.wavBytes` (writer) by adding
  a matching minimal WAV **reader** (RIFF/`fmt `/`data` chunk parse).
- Delete the `ttsPCMSampleRate = 24000` constant and make `resampleTTS` take the
  source rate/channels parsed from the header.
- Keep a fallback: if a server ignores `response_format` and returns raw PCM (no
  RIFF header), treat it as the configured/assumed rate and log a warning.

### P1.2 `Speak()` error detection

- After the existing `StatusCode >= 400` check, also reject the response when its
  `Content-Type` indicates JSON/text rather than audio (e.g. contains
  `application/json` or `text/`), surfacing it as a provider error. This makes a
  200-with-error-body fail loudly. Classify via the existing `HTTPError`/error
  path so the pipeline's error clip plays.

### P1.3 STT

- No change required: `POST /audio/transcriptions` is OpenAI-compatible on
  Speaches. The nonstandard `sample_rate`/`channels` multipart fields are ignored
  by compliant servers; leave them. (Optional future: pass `language` to cut
  Whisper latency — out of scope.)

### P1.4 Config + docs

- The `ProviderSet` already supports multiple providers per capability and
  cycling them from the settings menu — no schema change. Ship example
  `config.json` provider entries pointing STT + TTS at a Speaches base URL.
- Add a docs page: running Speaches (Docker, CPU/GPU image), choosing
  faster-whisper and Piper models/voices, and example BMO config. Cross-link from
  README and the existing modding/config docs.

### P1.5 Testing

- Unit: WAV header parse (various rates/channels, mono/stereo, truncated/invalid
  headers, raw-PCM fallback). `Speak()` JSON-body-on-200 rejection. Resample from
  22050 and 24000 produces correct output length.
- Manual: verify against a real Speaches instance (and against the existing LM
  Studio box at 192.168.50.90 for chat) on device.

---

## Phase 2 — On-Device Wake-Word + Continued Conversation

### P2.0 Feasibility spike (first, gating task)

Cross-compile a minimal `onnxruntime_go` test binary for the Brick target, push
via adb, and confirm: the runtime loads, an openWakeWord model runs on 16 kHz
frames, and detection latency/CPU fit budget. If it fails: fall back to a TFLite
runtime, or a small wake-word sidecar. No further Phase 2 work proceeds until
this passes.

### P2.1 Capture fan-out

`router.Batches()` is a single Go channel consumed today only by the PTT
goroutine. Add fan-out so the capture stream can feed **multiple** batch
consumers (PTT buffer and the wake-word detector) without stealing batches from
each other. Implement as a small broadcast/tee in `audio.CaptureRouter` (register
N subscriber channels) or a dedicated capture dispatcher. The mic stays opened by
the router; the separate `Levels()` channel and only the batch *consumers*
change.

### P2.2 Wake-word detector

- New component (e.g. `internal/wakeword`) wrapping the openWakeWord ONNX
  pipeline (melspectrogram + shared embedding + per-word classifier) via
  `onnxruntime_go`. Consumes 16 kHz mono frames from the capture fan-out while
  BMO is **idle and wake-word is enabled**; emits a detection event when the
  score crosses a threshold.
- Ship a pre-trained **"Hey BMO"** model (trained via openWakeWord's
  Piper-based synthesis pipeline) as an embedded/bundled asset, plus the shared
  embedding model.

### P2.3 Trigger integration

- A detection fires the **same path as PTT**: capture the following utterance and
  call `pipeline.ProcessBatch`. Wake-word is a second trigger source, not a
  pipeline change.
- **Push-to-talk priority:** a deliberate A-press preempts wake-word listening or
  an in-progress wake-triggered capture immediately (PTT is intentional). Use the
  existing interrupt/cancel hooks (`InterruptSpeech`, `CancelBatch`).

### P2.4 Wake-triggered listening window

On detection, open a listening window:
- A short **grace period** for the user to start speaking.
- **End-of-utterance via VAD**: stop on ~N seconds of silence (reuse/extend
  `audio.PCMHasSignal`-style level detection; `CaptureRouter` already emits
  `Levels()`).
- A hard max duration cap so a noisy room can't capture forever.
These timings are tunable constants (with sensible defaults).

### P2.5 Self-trigger gating

The detector and any auto-listen window must be **suppressed during BMO's own
TTS playback plus a short guard tail**, so BMO never wakes on or transcribes its
own voice/echo. The pipeline already knows when it is speaking (machine state /
`playPaced`); gate detection on that.

### P2.6 Continued conversation

- One setting: **Off / Short / Long**.
- When enabled, after BMO finishes speaking it auto-reopens the listening window
  (P2.4) **without** requiring "Hey BMO" again. Speech → processed as a
  follow-up; silence past the window → return to idle.
- Window length presets: **Short** ≈ human follow-up (a couple seconds);
  **Long** ≈ sized to span another BMO's STT+chat+TTS latency, enabling
  self-sustaining BMO-to-BMO conversation.
- The window opens only **after** playback + the P2.5 guard, so it never captures
  BMO's own tail.
- **Runaway safety:** window expiry naturally ends a two-BMO loop. Add a simple
  max-consecutive-follow-ups cap as a backstop (no elaborate loop detection).

### P2.7 Performance governor

When entering a wake/STT burst (detection → capture → transcription), request the
**`performance`** governor to pin max CPU, then restore the prior governor when
returning to idle. Wire via NextUI/MinUI's performance-mode mechanism (exact
mechanism to be pinned down during planning; no existing governor handling in the
pak). Keep it scoped to bursts to avoid battery/thermal cost of always-max.

### P2.8 Settings UX

Under the existing **AI mode** group in `internal/ui/settings_menu.go` (AI-only
rows, `aiToggle`/`aiCycle`, hidden unless `Mode == ModeAI`):
- **Wake word: Off (default) / On.** On → mic always-on listening; Off → mic only
  during PTT. Toggling drives whether the detector + capture fan-out subscriber
  are active.
- **Continued conversation: Off / Short / Long** (cycle row). Meaningful only when
  Wake word is On.
- New config fields on the `Config` struct (e.g. `WakeWordEnabled bool`,
  `ContinuedConversation string`/enum), persisted like existing settings.

### P2.9 Testing

- Unit: detector threshold/event logic with synthetic frames; listening-window
  state machine (grace, VAD end, max cap); self-trigger gating during playback;
  continued-conversation window open/expire and max-follow-up cap; PTT-preempts-
  wake-word.
- Capture fan-out: multiple subscribers each receive every batch.
- On-device: "Hey BMO" detection latency/false-trigger sanity; two-BMO
  conversation smoke test with the Long window; governor set/restore.

---

## Data flow

**Phase 1 utterance (unchanged trigger):**

```
PTT held → CaptureRouter.Batches() → buffer → (release) → ProcessBatch
  → STT (POST /audio/transcriptions)  [Speaches/faster-whisper]
  → chat (POST /chat/completions)     [LM Studio / OpenAI / Speaches]
  → TTS (POST /audio/speech, wav)     [Speaches/Piper] → parse WAV rate
  → resample → paced playback
```

**Phase 2 wake-word:**

```
CaptureRouter ──┬── PTT buffer (held)            [PTT priority]
                ├── wake-word detector (idle+enabled, gated during playback)
                └── level meter
detector fires → listening window (grace + VAD end) → ProcessBatch (as above)
reply playback ends → [continued conversation On] → reopen window (Short/Long)
                                                   → follow-up or idle
```

## Risks & mitigations

| Risk | Mitigation |
|---|---|
| `onnxruntime` CGO build for A133 fails | P2.0 spike before committing; fallbacks: TFLite, sidecar |
| Always-on mic battery/thermal cost | Default off; `performance` governor only during bursts; detector is tiny |
| BMO self-wakes on its own TTS | P2.5 self-trigger gating during playback + guard tail |
| Two-BMO runaway loop | Window expiry ends it; max-follow-up backstop |
| Server ignores `response_format=wav` | Raw-PCM fallback + warning in P1.1 |
| RAM pressure (device has OOM'd before) | Measure in P2.0; load embedding model once; reuse buffers |

## Phasing / sequencing

1. **Phase 1** (P1.1–P1.5) — ship self-hosted STT/TTS independently.
2. **Phase 2** — P2.0 spike → P2.1 fan-out → P2.2–P2.5 detector + trigger →
   P2.6 continued conversation → P2.7 governor → P2.8 settings → P2.9 tests.

## Open questions

- Exact NextUI/MinUI mechanism to request the `performance` governor (resolve in
  planning).
- Whether to bundle the "Hey BMO" model in the binary or ship as a pak asset
  file (resolve in planning; affects binary size).
</content>
</invoke>
