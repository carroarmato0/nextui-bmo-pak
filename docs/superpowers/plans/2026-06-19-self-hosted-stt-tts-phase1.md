# Self-Hosted STT/TTS (Phase 1) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make BMO's STT/TTS work against a self-hosted OpenAI-compatible server (Speaches → faster-whisper + Piper) by handling arbitrary TTS sample rates and failing loudly on non-audio responses.

**Architecture:** Reuse the existing `OpenAICompatibleClient` and per-capability `ProviderSet` config unchanged. The work is in the audio path: switch the TTS request to WAV, decode the returned WAV header to learn the true sample rate/channels, and resample from that instead of the hardcoded 24 kHz. Add a content-type guard so a 200-with-JSON-error-body (observed on LM Studio) surfaces as a provider error. Document Speaches setup.

**Tech Stack:** Go (CGO not required for these packages), `net/http`, `encoding/binary`, `net/http/httptest` for tests.

**Spec:** `docs/superpowers/specs/2026-06-19-self-hosted-stt-tts-and-wake-word-design.md` (Phase 1 sections P1.1–P1.5).

**Conventions (from CLAUDE.md / project memory):**
- Targeted tests for these pure-Go packages: `CGO_ENABLED=0 go test ./internal/<pkg>/ -run <Name> -v`.
- Full suite before finishing: `CGO_ENABLED=1 go test ./...`.
- Run `golangci-lint run ./...` after every change; new code must add no findings.
- Commit messages: **no** `Co-Authored-By` trailer.
- Branch already created: `feat/self-hosted-stt-tts-wakeword`.

---

## File Structure

- `internal/audio/wav.go` — **new.** `DecodeWAV` (RIFF/WAVE 16-bit PCM reader). Lives next to `resample.go`; the assistant package already imports `internal/audio`.
- `internal/audio/wav_test.go` — **new.** Tests for `DecodeWAV`.
- `internal/assistant/voice.go` — **modify.** Replace the 24 kHz hardcode with WAV-aware decode+resample; switch the three TTS request call sites to `Format: "wav"`.
- `internal/assistant/voice_test.go` — **modify.** Add `TestDecodeAndResampleTTS`.
- `internal/providers/openai_compatible.go` — **modify.** Add content-type guard in `Speak()`.
- `internal/providers/provider_test.go` — **modify.** Add content-type guard tests.
- `docs/self-hosted-speech.md` — **new.** Speaches setup + example BMO config.
- `README.md` — **modify.** Link to the new docs page.

---

## Task 1: WAV decoder (`audio.DecodeWAV`)

**Files:**
- Create: `internal/audio/wav.go`
- Test: `internal/audio/wav_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/audio/wav_test.go`:

```go
package audio

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// buildTestWAV writes a canonical 44-byte little-endian S16LE WAV header
// followed by pcm. Mirrors the on-the-wire format BMO's STT encoder produces.
func buildTestWAV(pcm []byte, rate, channels int) []byte {
	bitsPerSample := 16
	blockAlign := channels * bitsPerSample / 8
	byteRate := rate * blockAlign
	var b bytes.Buffer
	b.WriteString("RIFF")
	_ = binary.Write(&b, binary.LittleEndian, uint32(36+len(pcm)))
	b.WriteString("WAVE")
	b.WriteString("fmt ")
	_ = binary.Write(&b, binary.LittleEndian, uint32(16))
	_ = binary.Write(&b, binary.LittleEndian, uint16(1))
	_ = binary.Write(&b, binary.LittleEndian, uint16(channels))
	_ = binary.Write(&b, binary.LittleEndian, uint32(rate))
	_ = binary.Write(&b, binary.LittleEndian, uint32(byteRate))
	_ = binary.Write(&b, binary.LittleEndian, uint16(blockAlign))
	_ = binary.Write(&b, binary.LittleEndian, uint16(bitsPerSample))
	b.WriteString("data")
	_ = binary.Write(&b, binary.LittleEndian, uint32(len(pcm)))
	b.Write(pcm)
	return b.Bytes()
}

func TestDecodeWAV(t *testing.T) {
	pcm := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}

	t.Run("mono 22050", func(t *testing.T) {
		got, rate, ch, ok := DecodeWAV(buildTestWAV(pcm, 22050, 1))
		if !ok {
			t.Fatalf("ok = false, want true")
		}
		if rate != 22050 || ch != 1 {
			t.Fatalf("rate/ch = %d/%d, want 22050/1", rate, ch)
		}
		if !bytes.Equal(got, pcm) {
			t.Fatalf("pcm = %v, want %v", got, pcm)
		}
	})

	t.Run("stereo 24000", func(t *testing.T) {
		got, rate, ch, ok := DecodeWAV(buildTestWAV(pcm, 24000, 2))
		if !ok || rate != 24000 || ch != 2 || !bytes.Equal(got, pcm) {
			t.Fatalf("got %v rate=%d ch=%d ok=%v", got, rate, ch, ok)
		}
	})

	t.Run("raw pcm is not a WAV", func(t *testing.T) {
		if _, _, _, ok := DecodeWAV(pcm); ok {
			t.Fatalf("ok = true for raw PCM, want false")
		}
	})

	t.Run("too short", func(t *testing.T) {
		if _, _, _, ok := DecodeWAV([]byte("RIFF")); ok {
			t.Fatalf("ok = true for short buffer, want false")
		}
	})

	t.Run("8-bit unsupported", func(t *testing.T) {
		w := buildTestWAV(pcm, 16000, 1)
		w[34] = 8 // bitsPerSample field (offset 34 in canonical header)
		if _, _, _, ok := DecodeWAV(w); ok {
			t.Fatalf("ok = true for 8-bit, want false")
		}
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `CGO_ENABLED=0 go test ./internal/audio/ -run TestDecodeWAV -v`
Expected: FAIL — `undefined: DecodeWAV`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/audio/wav.go`:

```go
package audio

import "encoding/binary"

// DecodeWAV parses a little-endian 16-bit PCM WAV (RIFF/WAVE) container and
// returns the raw S16LE sample data together with its sample rate and channel
// count. ok is false when b is not a RIFF/WAVE 16-bit PCM container (for
// example a raw headerless PCM stream), in which case the caller should treat
// b as raw PCM at an assumed rate.
func DecodeWAV(b []byte) (pcm []byte, sampleRate, channels int, ok bool) {
	if len(b) < 12 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		return nil, 0, 0, false
	}
	var (
		haveFmt, haveData bool
		rate, chans, bits int
		data              []byte
	)
	off := 12
	for off+8 <= len(b) {
		id := string(b[off : off+4])
		size := int(binary.LittleEndian.Uint32(b[off+4 : off+8]))
		body := off + 8
		if body+size > len(b) {
			size = len(b) - body // tolerate a truncated final chunk
		}
		switch id {
		case "fmt ":
			if size >= 16 {
				chans = int(binary.LittleEndian.Uint16(b[body+2 : body+4]))
				rate = int(binary.LittleEndian.Uint32(b[body+4 : body+8]))
				bits = int(binary.LittleEndian.Uint16(b[body+14 : body+16]))
				haveFmt = true
			}
		case "data":
			data = b[body : body+size]
			haveData = true
		}
		off = body + size
		if size%2 == 1 { // chunks are word-aligned; skip pad byte
			off++
		}
	}
	if !haveFmt || !haveData || bits != 16 || chans <= 0 || rate <= 0 {
		return nil, 0, 0, false
	}
	return data, rate, chans, true
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `CGO_ENABLED=0 go test ./internal/audio/ -run TestDecodeWAV -v`
Expected: PASS (all subtests).

- [ ] **Step 5: Lint**

Run: `golangci-lint run ./internal/audio/...`
Expected: no findings.

- [ ] **Step 6: Commit**

```bash
git add internal/audio/wav.go internal/audio/wav_test.go
git commit -m "feat(audio): add DecodeWAV reader for PCM WAV containers"
```

---

## Task 2: WAV-aware TTS decode + resample in the pipeline

**Files:**
- Modify: `internal/assistant/voice.go` (const at `voice.go:673`, `resampleTTS` at `voice.go:757-767`, three `Speak` call sites, three `resampleTTS` call sites)
- Test: `internal/assistant/voice_test.go`

Background: today `voice.go` has `const ttsPCMSampleRate = 24000`, `resampleTTS(pcm []byte)` assumes 24 kHz mono, and three flows call `Speak(... Format: "pcm")` then `speech = p.resampleTTS(speech)`. We make TTS request WAV, decode the real rate/channels, and resample from those — with a raw-PCM fallback.

- [ ] **Step 1: Write the failing test**

Add to `internal/assistant/voice_test.go`:

```go
func TestDecodeAndResampleTTS(t *testing.T) {
	pcm := make([]byte, 480) // 0.01s of 24kHz mono S16LE
	for i := range pcm {
		pcm[i] = byte(i)
	}

	t.Run("WAV header stripped, mono identity at matching rate", func(t *testing.T) {
		p := &VoicePipeline{sampleRate: 24000, playbackChannels: 1}
		wav := buildTestWAV(pcm, 24000, 1)
		out := p.decodeAndResampleTTS(wav)
		if len(out) != len(pcm) {
			t.Fatalf("len(out) = %d, want %d (header should be stripped, no resample)", len(out), len(pcm))
		}
	})

	t.Run("mono upmixed to stereo", func(t *testing.T) {
		p := &VoicePipeline{sampleRate: 24000, playbackChannels: 2}
		out := p.decodeAndResampleTTS(buildTestWAV(pcm, 24000, 1))
		if len(out) != 2*len(pcm) {
			t.Fatalf("len(out) = %d, want %d (mono upmixed to stereo)", len(out), 2*len(pcm))
		}
	})

	t.Run("non-WAV falls back to raw PCM", func(t *testing.T) {
		p := &VoicePipeline{sampleRate: 24000, playbackChannels: 1}
		out := p.decodeAndResampleTTS(pcm) // raw, no RIFF header
		if len(out) != len(pcm) {
			t.Fatalf("len(out) = %d, want %d (raw PCM treated as 24kHz mono)", len(out), len(pcm))
		}
	})
}

// buildTestWAV mirrors internal/audio's canonical S16LE WAV header so the
// assistant test can build TTS-response fixtures without importing test code.
func buildTestWAV(pcm []byte, rate, channels int) []byte {
	bitsPerSample := 16
	blockAlign := channels * bitsPerSample / 8
	byteRate := rate * blockAlign
	var b bytes.Buffer
	b.WriteString("RIFF")
	_ = binary.Write(&b, binary.LittleEndian, uint32(36+len(pcm)))
	b.WriteString("WAVE")
	b.WriteString("fmt ")
	_ = binary.Write(&b, binary.LittleEndian, uint32(16))
	_ = binary.Write(&b, binary.LittleEndian, uint16(1))
	_ = binary.Write(&b, binary.LittleEndian, uint16(channels))
	_ = binary.Write(&b, binary.LittleEndian, uint32(rate))
	_ = binary.Write(&b, binary.LittleEndian, uint32(byteRate))
	_ = binary.Write(&b, binary.LittleEndian, uint16(blockAlign))
	_ = binary.Write(&b, binary.LittleEndian, uint16(bitsPerSample))
	b.WriteString("data")
	_ = binary.Write(&b, binary.LittleEndian, uint32(len(pcm)))
	b.Write(pcm)
	return b.Bytes()
}
```

Ensure the test file imports `bytes` and `encoding/binary` (add to the existing import block if missing).

- [ ] **Step 2: Run test to verify it fails**

Run: `CGO_ENABLED=0 go test ./internal/assistant/ -run TestDecodeAndResampleTTS -v`
Expected: FAIL — `p.decodeAndResampleTTS undefined`.

- [ ] **Step 3: Replace the constant and resample function**

In `internal/assistant/voice.go`, replace the `ttsPCMSampleRate` const block (around `voice.go:670-673`):

```go
// ttsPCMFallbackSampleRate is assumed for a TTS response that is NOT a WAV
// container (a raw headerless PCM stream). OpenAI's legacy "pcm" format is
// 24kHz mono S16LE; servers that return WAV carry their real rate in the
// header (see decodeAndResampleTTS).
const ttsPCMFallbackSampleRate = 24000
```

Replace `resampleTTS` (around `voice.go:757-767`) with a decode-aware pair:

```go
// decodeAndResampleTTS converts a TTS response into device-rate playback PCM.
// A WAV response is decoded for its true sample rate/channels; a raw response
// is assumed to be ttsPCMFallbackSampleRate mono.
func (p *VoicePipeline) decodeAndResampleTTS(speech []byte) []byte {
	pcm, srcRate, srcChannels, ok := audio.DecodeWAV(speech)
	if !ok {
		if p.logger != nil {
			p.logger.Debugf("TTS response not WAV; assuming %dHz mono raw PCM", ttsPCMFallbackSampleRate)
		}
		pcm, srcRate, srcChannels = speech, ttsPCMFallbackSampleRate, 1
	}
	return p.resampleTTS(pcm, srcRate, srcChannels)
}

// resampleTTS converts srcChannels-channel S16LE at srcRate to the device
// playback rate and channel count. Mono is resampled then upmixed (avoids the
// 2x-speed regression from passing playbackChannels to ResampleS16LE on mono
// input); already-multichannel audio is resampled in place.
func (p *VoicePipeline) resampleTTS(pcm []byte, srcRate, srcChannels int) []byte {
	if srcRate <= 0 {
		srcRate = ttsPCMFallbackSampleRate
	}
	if srcChannels <= 0 {
		srcChannels = 1
	}
	out := audio.ResampleS16LE(pcm, srcRate, p.sampleRate, srcChannels)
	if srcChannels == 1 && p.playbackChannels > 1 {
		out = audio.UpmixMonoToStereo(out)
	}
	return out
}
```

- [ ] **Step 4: Switch the three TTS request + resample call sites**

In each of `ProcessBatch`, `SpeakRemark`, and `SpeakVerbatim`, change the `Speak` request field `Format: "pcm",` to `Format: "wav",`, and change `speech = p.resampleTTS(speech)` to `speech = p.decodeAndResampleTTS(speech)`.

(`ProcessBatch`: the `Speak` call ~`voice.go:367-373` and resample ~`voice.go:383`. `SpeakRemark`: ~`voice.go:528-534` and ~`voice.go:545`. `SpeakVerbatim`: ~`voice.go:578-584` and ~`voice.go:595`.)

- [ ] **Step 5: Run tests to verify they pass**

Run: `CGO_ENABLED=0 go test ./internal/assistant/ -run TestDecodeAndResampleTTS -v`
Expected: PASS (all subtests).

Run the package suite to confirm no regression: `CGO_ENABLED=0 go test ./internal/assistant/`
Expected: ok.

- [ ] **Step 6: Lint**

Run: `golangci-lint run ./internal/assistant/...`
Expected: no findings.

- [ ] **Step 7: Commit**

```bash
git add internal/assistant/voice.go internal/assistant/voice_test.go
git commit -m "feat(assistant): decode TTS WAV for true sample rate; drop 24kHz hardcode"
```

---

## Task 3: `Speak()` rejects non-audio (JSON/text) 200 responses

**Files:**
- Modify: `internal/providers/openai_compatible.go` (`Speak`, around `openai_compatible.go:162-173`)
- Test: `internal/providers/provider_test.go`

Background: `Speak` only treats `StatusCode >= 400` as failure. LM Studio returns **HTTP 200** with a JSON error body for unknown endpoints, so the error string would be played as audio. Reject responses whose `Content-Type` is JSON/text.

- [ ] **Step 1: Write the failing test**

Add to `internal/providers/provider_test.go`:

```go
func TestSpeakRejectsJSONErrorBodyOn200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"error":"Unexpected endpoint or method."}`))
	}))
	defer server.Close()

	client := NewOpenAICompatibleClient(Config{BaseURL: server.URL}, server.Client())
	_, err := client.Speak(context.Background(), SpeechRequest{Model: "m", Voice: "alloy", Input: "hi", Format: "wav"})
	if err == nil {
		t.Fatalf("Speak() error = nil, want error for JSON body on 200")
	}
}

func TestSpeakAcceptsAudioContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write([]byte{0x52, 0x49, 0x46, 0x46}) // "RIFF"
	}))
	defer server.Close()

	client := NewOpenAICompatibleClient(Config{BaseURL: server.URL}, server.Client())
	out, err := client.Speak(context.Background(), SpeechRequest{Model: "m", Voice: "alloy", Input: "hi", Format: "wav"})
	if err != nil {
		t.Fatalf("Speak() error = %v, want nil", err)
	}
	if len(out) == 0 {
		t.Fatalf("Speak() returned no audio bytes")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `CGO_ENABLED=0 go test ./internal/providers/ -run 'TestSpeakRejectsJSONErrorBodyOn200|TestSpeakAcceptsAudioContentType' -v`
Expected: FAIL — `TestSpeakRejectsJSONErrorBodyOn200` returns nil error.

- [ ] **Step 3: Add the content-type guard**

In `internal/providers/openai_compatible.go`, in `Speak`, immediately after the existing `if resp.StatusCode >= 400 { ... }` block and before `return io.ReadAll(resp.Body)`:

```go
	// Some OpenAI-compatible servers (e.g. LM Studio) return HTTP 200 with a
	// JSON error body for endpoints they do not implement. A real audio
	// response is never JSON/text, so reject those to fail loudly instead of
	// playing the error string as audio.
	if ct := strings.ToLower(resp.Header.Get("Content-Type")); strings.Contains(ct, "application/json") || strings.Contains(ct, "text/") {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, &HTTPError{StatusCode: resp.StatusCode, Body: strings.TrimSpace(string(body))}
	}
```

(`strings`, `io`, and `HTTPError` are already imported/defined in the package.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `CGO_ENABLED=0 go test ./internal/providers/ -run 'TestSpeakRejectsJSONErrorBodyOn200|TestSpeakAcceptsAudioContentType' -v`
Expected: PASS.

Confirm no regression in the existing `TestSpeakInstructions` (it writes a bare `0x01` byte → sniffed `application/octet-stream`, not rejected):
Run: `CGO_ENABLED=0 go test ./internal/providers/`
Expected: ok.

- [ ] **Step 5: Lint**

Run: `golangci-lint run ./internal/providers/...`
Expected: no findings.

- [ ] **Step 6: Commit**

```bash
git add internal/providers/openai_compatible.go internal/providers/provider_test.go
git commit -m "fix(providers): reject JSON/text 200 responses in Speak() so bad TTS endpoints fail loudly"
```

---

## Task 4: Documentation + example config

**Files:**
- Create: `docs/self-hosted-speech.md`
- Modify: `README.md` (add a link in the existing docs/links section)

- [ ] **Step 1: Write the docs page**

Create `docs/self-hosted-speech.md`:

```markdown
# Self-Hosted Speech (faster-whisper + Piper via Speaches)

BMO's chat, speech-to-text (STT), and text-to-speech (TTS) each speak the
OpenAI REST API and are configured **independently** in `config.json`. That
means you can point STT and TTS at a self-hosted, OpenAI-compatible server
instead of OpenAI.

The recommended server is **[Speaches](https://github.com/speaches-ai/speaches)**
(the maintained successor to faster-whisper-server). One Speaches instance
serves both:

- **STT** via `POST /v1/audio/transcriptions`, backed by **faster-whisper**.
- **TTS** via `POST /v1/audio/speech`, backed by **Piper** (or Kokoro).

> Note: plain LLM servers such as Ollama or LM Studio implement only
> `/v1/chat/completions` and `/v1/models` — they do **not** serve audio
> endpoints. Use them for `chat` only, and Speaches (or OpenAI) for STT/TTS.

## Run Speaches

CPU-only:

```bash
docker run --rm -p 8000:8000 \
  -e WHISPER__MODEL=Systran/faster-whisper-small \
  ghcr.io/speaches-ai/speaches:latest-cpu
```

For GPU, use the `latest-cuda` image. See the Speaches docs for model and
voice selection.

## Configure BMO

In `config.json`, set the `base_url` of the STT and TTS providers to your
Speaches instance (`api_key` may be any non-empty string; Speaches ignores it
unless you enabled auth). `chat` can stay on OpenAI, an LLM box, or Speaches.

```json
{
  "stt": {
    "active": "speaches",
    "providers": [
      {
        "name": "speaches",
        "model": "Systran/faster-whisper-small",
        "base_url": "http://192.168.50.90:8000/v1",
        "api_key": "local"
      }
    ]
  },
  "tts": {
    "active": "speaches",
    "providers": [
      {
        "name": "speaches",
        "model": "speaches-ai/piper-en_US-amy-low",
        "voice": "amy",
        "base_url": "http://192.168.50.90:8000/v1",
        "api_key": "local"
      }
    ]
  }
}
```

BMO requests TTS as WAV and reads the sample rate from the response header, so
Piper voices (commonly 22050 Hz) play at the correct pitch with no extra
configuration.
```

- [ ] **Step 2: Link it from the README**

In `README.md`, add a bullet linking to `docs/self-hosted-speech.md` in the
existing documentation/links list (match the surrounding bullet style), e.g.:

```markdown
- [Self-hosted speech (faster-whisper + Piper)](docs/self-hosted-speech.md)
```

- [ ] **Step 3: Verify the build and the link target exists**

Run: `CGO_ENABLED=1 go build ./...`
Expected: builds clean (docs change is inert, but confirm nothing else broke).

Run: `test -f docs/self-hosted-speech.md && echo OK`
Expected: `OK`.

- [ ] **Step 4: Commit**

```bash
git add docs/self-hosted-speech.md README.md
git commit -m "docs: self-hosted speech setup (Speaches: faster-whisper + Piper)"
```

---

## Task 5: Full verification

**Files:** none (verification only).

- [ ] **Step 1: Full test suite**

Run: `CGO_ENABLED=1 go test ./...`
Expected: all packages `ok` (ignore `[no test files]` and any `Cannot process svg element` log noise).

- [ ] **Step 2: Full lint**

Run: `golangci-lint run ./...`
Expected: no findings.

- [ ] **Step 3: Manual on-device / live check (optional but recommended)**

Point a BMO config's STT + TTS at a running Speaches instance, keep `chat` on
the LM Studio box (`http://192.168.50.90:1234/v1`, model `google/gemma-4-e4b`),
deploy with `./scripts/deploy.sh`, and confirm a push-to-talk exchange
transcribes, replies, and speaks at correct pitch. Tail logs with
`./scripts/debug-logs.sh`.

- [ ] **Step 4: No commit** (verification only). Phase 1 complete.

---

## Self-Review (completed by plan author)

- **Spec coverage:** P1.1 → Tasks 1–2; P1.2 → Task 3; P1.3 → no code change (documented in Task 4 note); P1.4 → Task 4; P1.5 → Tasks 1–3 unit tests + Task 5 manual. All covered.
- **Placeholders:** none — every code/test step contains full code and exact commands.
- **Type consistency:** `DecodeWAV(b []byte) (pcm []byte, sampleRate, channels int, ok bool)` defined in Task 1 and consumed identically in Task 2; `decodeAndResampleTTS(speech []byte) []byte` and `resampleTTS(pcm []byte, srcRate, srcChannels int) []byte` consistent across Task 2 steps; `buildTestWAV` signature identical in both test files.
</content>
