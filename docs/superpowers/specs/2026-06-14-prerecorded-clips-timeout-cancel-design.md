# Pre-recorded Clips, Request Timeout & Cancel Design

**Date:** 2026-06-14
**Status:** Approved

## Overview

Add seven pre-recorded in-character audio clips to BMO, a lightweight clip player that works independently of AI mode, a configurable per-request timeout with in-character fallback audio, and a silent B-button cancel for in-flight AI requests.

---

## 1. Pre-recorded Clip Library

### Clips

| Name | Trigger | Mode req. | Wired in this spec |
|---|---|---|---|
| `hello` | App startup | none | Yes |
| `mod_error` | Override asset validation failure, played after `hello` | none | Yes |
| `timeout` | AI request exceeded configured timeout | AI only | Yes |
| `error` | Network / API error during STT or Chat | AI only | Yes |
| `goodbye` | Any clean app exit | none | Yes |
| `sleep` | Device sleep | none | No (pre-recorded only) |
| `wake` | Device wake | none | No (pre-recorded only) |

### Storage and embedding

Raw S16LE PCM at 16 kHz **stereo** (2 channels, device-native playback rate, pre-resampled and upmixed from TTS 24 kHz mono output). Files live in `assets/audio/<name>.pcm` and are embedded in the binary via:

```go
//go:embed assets/audio/*.pcm
var embedded embed.FS
```

in a new `internal/clips/embed.go`.

### Override mechanism

Same pattern as face SVGs. `clips.Library` checks `<homeDir>/audio/<name>.pcm` first; if the file exists and is non-empty it is used, otherwise the embedded default is loaded. Missing override directory is silently ignored.

---

## 2. `internal/clips` Package

A new package providing clip loading and playback, independent of AI providers.

### `Library`

```go
type Library struct {
    dir      string   // <homeDir>/audio — may not exist
    embedded embed.FS
}

func NewLibrary(homeDir string) *Library
func (l *Library) Load(name string) []byte  // override → embedded → nil
```

`Load` returns nil if neither source has the clip (e.g. sleep/wake not yet wired). Callers treat nil as "no clip, skip silently."

### `Player`

```go
type Player struct {
    writer     AudioWriter  // same interface as VoicePipeline uses
    sampleRate int
    channels   int
    lib        *Library
}

func NewPlayer(writer AudioWriter, sampleRate, channels int, lib *Library) *Player
func (p *Player) Play(ctx context.Context, name string) error
```

`Play` loads the clip by name and streams it to the audio writer using the same `playPaced` logic already in `VoicePipeline`. It does not interact with the state machine or face renderer — callers manage state transitions if needed. Returns nil immediately if the clip is not found.

The `Player` is always constructed when audio hardware is available, regardless of AI mode.

---

## 3. Generation Tool (`cmd/generate-audio`)

A standalone Go tool that generates the PCM files for all seven clips using the OpenAI-compatible API. Run this whenever the TTS voice or model changes.

### Flags

| Flag | Env var | Default |
|---|---|---|
| `-key` | `OPENAI_API_KEY` | required |
| `-base-url` | — | `https://api.openai.com/v1` |
| `-chat-model` | — | production default |
| `-tts-model` | — | production default |
| `-voice` | — | production default (`alloy`) |
| `-instructions` | — | `DefaultTTSInstructions` |
| `-out` | — | `assets/audio/` |

### Generation flow per clip

1. Send a short "nudge" to the Chat AI with `DefaultSystemPrompt` as the system prompt, asking for an in-character one- or two-sentence response appropriate for the clip's situation.
2. Pass the returned text to TTS with the configured voice and instructions.
3. Resample the 24 kHz mono PCM output to 16 kHz using `audio.ResampleS16LE`.
4. Upmix mono to stereo by interleaving each sample as `[L, R]` (duplicate channel).
5. Write to `<out>/<name>.pcm`.

Running the tool a second time overwrites existing files (idempotent).

### Built-in nudge prompts

| Clip | Nudge sent to Chat AI |
|---|---|
| `hello` | Short, excited in-character greeting to the user. One sentence. |
| `mod_error` | Short in-character message warning that something in BMO's customisation files seems broken and BMO has fallen back to defaults. One or two sentences. |
| `timeout` | Short in-character apology for not being able to think of an answer. Ask the user to try again. One or two sentences. |
| `error` | Short in-character message saying BMO can't reach anyone right now, maybe check the connection. One or two sentences. |
| `goodbye` | Short, warm in-character farewell. One sentence. |
| `sleep` | Short in-character message for going to sleep. One sentence. |
| `wake` | Short in-character message for just waking up. One sentence. |

---

## 4. Override Asset Validation (`config.CheckOverrides`)

```go
func CheckOverrides(homeDir string) []error
```

Called once at startup after config is loaded. For each overrideable asset that **exists on disk**, validates it:

- `persona.txt` — file exists and is non-empty after trimming
- `voice.txt` — file exists and is non-empty after trimming
- `faces/<name>.svg` — file exists and is valid XML / parseable as SVG (basic parse check)
- Quotes file (if applicable) — file exists and non-empty

Returns one error per failing file. Missing files (user has no override) produce no error. Errors are collected into a slice; the main app checks `len(errs) > 0` to decide whether to play `mod_error`.

---

## 5. Request Timeout and B-Button Cancel

### Config

New field in `config.Config`:

```go
RequestTimeout int `json:"request_timeout"` // seconds; 0 → default
```

Validation: values outside [15, 60] are clamped to 15. Zero is treated as unset → 15. Validated in `validateConfig`. Exposed in the AI settings menu as a new "Timeout" item; Left/Right cycles through `[15, 20, 25, 30, 45, 60]` seconds. Auto-saves on cycle (same pattern as other settings items).

### `VoicePipeline` changes

New fields:
- `requestTimeout time.Duration`
- `batchMu sync.Mutex` guarding `batchCancel context.CancelFunc`
- `timeoutClip []byte` — pre-loaded `timeout.pcm` PCM at device rate
- `errorClip []byte` — pre-loaded `error.pcm` PCM at device rate

New methods:
- `SetRequestTimeout(d time.Duration)` — called from `commitMenu` and at startup
- `SetTimeoutClip(pcm []byte)` / `SetErrorClip(pcm []byte)` — called at startup
- `CancelBatch() bool` — cancels the in-flight batch context; returns true if something was in flight

### Context layering in `ProcessBatch`

A per-batch context wraps the parent with `context.WithTimeout(ctx, requestTimeout)`. STT and Chat calls use this `batchCtx`. TTS and playback use the parent `ctx` (timeout applies only to the AI-fetch phase, not rendering audio).

The `batchCancel` is stored in the pipeline under `batchMu` for the lifetime of the STT+Chat phase, then cleared.

### Error dispatch in `ProcessBatch`

Applied at each error return from STT and Chat:

| Condition | Phase | Action |
|---|---|---|
| `batchCtx.Err() == DeadlineExceeded` | STT | silent transition → idle |
| `batchCtx.Err() == DeadlineExceeded` | Chat | play `timeout` clip → idle |
| `batchCtx.Err() == Canceled`, parent ctx ok | either | B-press cancel: silent → idle |
| parent `ctx.Err() != nil` | either | app shutting down: propagate error |
| quota error | either | `EventQuotaExhausted` (existing behaviour, unchanged) |
| any other error | either | play `error` clip → idle |

The last case replaces the existing `EventFail → StateError` path for network/API errors in `ProcessBatch`. Other callers of `fail()` (`SpeakRemark`, `SpeakVerbatim`) retain existing error handling for now.

### B-button wiring in `handleNav`

`NavCancel` order becomes:
1. Close active menu (existing)
2. **`CancelBatch()`** — if returns true, return (no further action; silent cancel in thinking state)
3. `InterruptSpeech()` — cancel ongoing speech (existing)
4. `running = false` — exit app (existing)

---

## 6. Hello and Goodbye Clip Wiring

### Hello

After the face loop initialises (SDL window open, first render done) and before the first idle tick, the main loop calls:

```go
clipPlayer.Play(ctx, "hello")
```

This is unconditional on AI mode. If the clip is missing, skipped silently.

### mod_error (after hello)

```go
errs := config.CheckOverrides(homeDir)
if len(errs) > 0 {
    for _, e := range errs {
        logger.Warnf("mod override error: %v", e)
    }
    clipPlayer.Play(ctx, "mod_error")
}
```

Played immediately after `hello` completes. Both clips play sequentially before the app enters normal operation.

### Goodbye

After the `for running {}` face loop exits and before any deferred cleanup:

```go
if clipPlayer != nil {
    goodbyeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    _ = clipPlayer.Play(goodbyeCtx, "goodbye")
    cancel()
}
```

A 5-second timeout guards against a stuck audio session. Deferred cleanup (audio session close, context cancel) runs after this block.

---

## 7. Audio Channel Configuration

The TrimUI hardware has stereo speakers but a mono microphone. The current `audio.Config.Channels = 1` conflates both. This spec splits them:

- `audio.Config` gains `PlaybackChannels int` (default 2) alongside the existing `Channels` field which becomes the capture channel count (remains 1).
- `audio.Config.PlaybackArgs()` uses `PlaybackChannels`; `CaptureArgs()` keeps `Channels`.
- `VoicePipeline` receives `playbackChannels` separately from `captureChannels` and uses it for `ResampleS16LE` on TTS output and for the `timeout`/`error` clip playback.
- `clips.Player` is constructed with `playbackChannels = 2`.

Verified against device: `aplay --dump-hw-params` on hw:0,0 reports `CHANNELS: [1 2]` — stereo playback is supported.

---

## 8. What Is Not Changed

- `SpeakRemark` and `SpeakVerbatim` error paths — unchanged
- `EventQuotaExhausted` handling — unchanged
- Sleep/wake triggers — not implemented yet; clips are generated and embedded only
- Face expression during clip playback for `clips.Player` — no state machine interaction (callers manage expression if needed; for hello/goodbye/mod_error the machine is in idle/neutral)

---

## 9. Files Affected

| Path | Change |
|---|---|
| `assets/audio/` | New directory; 7 `.pcm` files generated by tool |
| `internal/clips/embed.go` | New; `go:embed` declaration |
| `internal/clips/library.go` | New; `Library` struct |
| `internal/clips/player.go` | New; `Player` struct with `playPaced` |
| `cmd/generate-audio/main.go` | New; generation tool |
| `internal/config/config.go` | Add `RequestTimeout` field + validation |
| `internal/config/prompts.go` | Add `CheckOverrides` function |
| `internal/assistant/voice.go` | Add timeout, cancel, clip fields and methods |
| `cmd/bmo-pak/main.go` | Wire `clips.Player`, hello/mod_error/goodbye, `CancelBatch` in `handleNav`, `SetRequestTimeout` |
| `internal/ui/screen_settings.go` | Add Timeout settings item |
| `internal/audio/audio.go` | Split `Channels` into `Channels` (capture) + `PlaybackChannels` (playback) |
