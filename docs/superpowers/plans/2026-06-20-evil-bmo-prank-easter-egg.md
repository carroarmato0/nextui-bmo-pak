# Evil BMO Prank Easter Egg Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A deliberately-hidden, non-mod easter egg: when AI mode is on and the active mod is Evil BMO, BMO rarely (or on a hidden D-pad Down press) speaks "Hey BMO" + a taunt out its speaker to provoke a nearby BMO device, listens for a reply, and fires back a bounded contextual comeback — or a smug "nobody's home" line if no one answers.

**Architecture:** All logic lives in the `bmo-pak` binary, gated on `cfg.UsesAI() && activeMod.ID == "evil-bmo"`, re-checked at trigger time. The example mod directory and the mod/config systems are untouched. The prank reuses the existing `VoicePipeline` (two new pure helper methods) and the existing `CaptureRouter` capture path. A shared `atomic.Bool` provides single-flight and makes the real wake loop stand down during a prank. Auto-firing is a jittered multi-hour timer that delivers a tick onto the main goroutine (so shared UI state is read race-free).

**Tech Stack:** Go, `internal/assistant` (VoicePipeline + state Machine), `internal/audio` (CaptureRouter), `internal/input` (Buffer, NavReader), `cmd/bmo-pak` (main wiring). Tests use the existing `fakeProvider`/`fakeWriter` fakes and a scripted `PCMSource`.

## File Structure

- **Modify** `internal/assistant/voice.go` — add `GenerateRemarkText` (chat-only, no audio, no state change) and `Transcribe` (STT-only) methods.
- **Modify** `internal/assistant/voice_test.go` — tests for the two new methods.
- **Modify** `cmd/bmo-pak/wakeword.go` — `wakeLoop` gains a `prankActive *atomic.Bool` field + a `suppressed()` helper checked at the top of `handleBatch`; `startWakeWord` gains a `prankActive` parameter.
- **Modify** `cmd/bmo-pak/wakeword_test.go` — test for `suppressed()`.
- **Create** `cmd/bmo-pak/evil_prank.go` — gate constant, nudge strings, cadence constants, `prankVoice` interface, `prankSession` (sequence logic), `listenForReply` capture helper, `pipelineVoice` adapter, `evilPrank` engine (trigger + scheduler).
- **Create** `cmd/bmo-pak/evil_prank_test.go` — sequence/branch tests, single-flight + gate tests, `listenForReply` tests.
- **Modify** `cmd/bmo-pak/main.go` — declare `prank`/`prankTick`/`prankActive`; build the engine in the AI block; pass `prankActive` to `startWakeWord`; poll `prankTick` in the main loop; add a D-pad Down branch in `handleNav`.

**Untouched:** `examples/mods/evil-bmo/`, `internal/mod`, `internal/config`, `internal/examplemods`, settings menu, MODDING docs.

**Verification commands (per CLAUDE.md), used throughout:**
- Test a package: `CGO_ENABLED=1 go test ./internal/assistant/` or `CGO_ENABLED=1 go test ./cmd/bmo-pak/`
- Full build: `CGO_ENABLED=1 go build ./...`
- Full test: `CGO_ENABLED=1 go test ./...`
- Lint: `golangci-lint run ./...`

---

### Task 1: VoicePipeline `GenerateRemarkText` + `Transcribe`

**Files:**
- Modify: `internal/assistant/voice.go`
- Test: `internal/assistant/voice_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/assistant/voice_test.go`:

```go
func TestGenerateRemarkTextStripsEmotionAndDoesNotSpeak(t *testing.T) {
	m := NewMachine()
	writer := &fakeWriter{}
	stt := &fakeProvider{}
	chat := &fakeProvider{reply: "[laugh] you call that a high score?"}
	tts := &fakeProvider{}
	pipe := NewVoicePipeline(m, writer, stt, chat, tts, "whisper-1", "gpt-4o-mini", "tts-1", "alloy", "be evil bmo", 16000, 1, 2)

	got, err := pipe.GenerateRemarkText(context.Background(), "taunt a nearby bmo")
	if err != nil {
		t.Fatalf("GenerateRemarkText: %v", err)
	}
	if got != "you call that a high score?" {
		t.Fatalf("text = %q, want emotion stripped", got)
	}
	if writer.writeCount() != 0 {
		t.Fatalf("expected no audio written, got %d writes", writer.writeCount())
	}
	if m.State() != StateIdle {
		t.Fatalf("state = %q, want idle (no transition)", m.State())
	}
	if chat.lastChat.Messages[0].Content != "taunt a nearby bmo" {
		t.Fatalf("nudge not sent as user message: %+v", chat.lastChat.Messages)
	}
}

func TestTranscribeReturnsTextWithoutTransition(t *testing.T) {
	m := NewMachine()
	stt := &fakeProvider{transcript: "  I scored nine thousand  "}
	pipe := NewVoicePipeline(m, &fakeWriter{}, stt, &fakeProvider{}, &fakeProvider{}, "whisper-1", "gpt-4o-mini", "tts-1", "alloy", "", 16000, 1, 2)

	// 200ms of constant-amplitude mono PCM so PCMHasSignal passes.
	pcm := make([]byte, 16000*2*200/1000)
	for i := 0; i+1 < len(pcm); i += 2 {
		binary.LittleEndian.PutUint16(pcm[i:i+2], uint16(int16(8192)))
	}

	got, err := pipe.Transcribe(context.Background(), pcm)
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if got != "I scored nine thousand" {
		t.Fatalf("transcript = %q, want trimmed text", got)
	}
	if m.State() != StateIdle {
		t.Fatalf("state = %q, want idle (no transition)", m.State())
	}
}

func TestTranscribeSkipsSilentAudio(t *testing.T) {
	stt := &fakeProvider{transcript: "should not be used"}
	pipe := NewVoicePipeline(NewMachine(), &fakeWriter{}, stt, &fakeProvider{}, &fakeProvider{}, "whisper-1", "gpt-4o-mini", "tts-1", "alloy", "", 16000, 1, 2)
	got, err := pipe.Transcribe(context.Background(), make([]byte, 640)) // all-zero = silent
	if err != nil || got != "" {
		t.Fatalf("Transcribe(silence) = %q, %v; want \"\", nil", got, err)
	}
	if stt.lastTranscribe != nil {
		t.Fatalf("STT provider should not be called for silent audio")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `CGO_ENABLED=1 go test ./internal/assistant/ -run 'TestGenerateRemarkText|TestTranscribe' -v`
Expected: FAIL — `pipe.GenerateRemarkText undefined` / `pipe.Transcribe undefined`.

- [ ] **Step 3: Implement the two methods**

In `internal/assistant/voice.go`, add after `SpeakVerbatim` (which ends around line 600):

```go
// GenerateRemarkText runs the chat model with nudge (a stage direction sent as
// the user message) under the active system prompt and returns the spoken text
// with any leading emotion directive stripped. Unlike SpeakRemark it neither
// touches the state machine nor synthesizes audio, so a caller can prepare a
// line before deciding when (or whether) to speak it. Returns "" when AI is
// disabled or the model yields no speakable text.
func (p *VoicePipeline) GenerateRemarkText(ctx context.Context, nudge string) (string, error) {
	if p == nil || !p.aiModeEnabled() {
		return "", nil
	}
	reqCtx, cancel := p.requestCtx(ctx)
	defer cancel()
	chat, err := p.chat.Reply(reqCtx, providers.ChatRequest{
		Model:        p.chatModel,
		Messages:     []providers.Message{{Role: "user", Content: nudge}},
		SystemPrompt: p.currentSystemPrompt(),
	})
	if err != nil {
		return "", err
	}
	reply := strings.TrimSpace(chat.Text)
	if reply == "" {
		return "", nil
	}
	spoken, _ := ParseEmotion(reply, emotionNameSet(p.currentEmotionVocab()))
	return strings.TrimSpace(spoken), nil
}

// Transcribe runs speech-to-text on a captured PCM buffer and returns the
// trimmed transcript. It is the bare STT step ProcessBatch performs internally,
// exposed for callers that need the text without the chat/TTS round-trip or any
// state-machine transitions. Returns "" for empty, signal-less, or
// AI-disabled input.
func (p *VoicePipeline) Transcribe(ctx context.Context, pcm []byte) (string, error) {
	if p == nil || !p.aiModeEnabled() {
		return "", nil
	}
	if len(pcm) == 0 || !audio.PCMHasSignal(pcm, 0.01) {
		return "", nil
	}
	reqCtx, cancel := p.requestCtx(ctx)
	defer cancel()
	res, err := p.stt.Transcribe(reqCtx, providers.TranscriptionRequest{
		Model:      p.sttModel,
		Audio:      pcm,
		SampleRate: p.sampleRate,
		Channels:   p.captureChannels,
		Format:     "wav",
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(res.Text), nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `CGO_ENABLED=1 go test ./internal/assistant/ -run 'TestGenerateRemarkText|TestTranscribe' -v`
Expected: PASS (all three).

- [ ] **Step 5: Commit**

```bash
git add internal/assistant/voice.go internal/assistant/voice_test.go
git commit -m "feat(assistant): add chat-only GenerateRemarkText and STT-only Transcribe"
```

---

### Task 2: Wake-loop suppression hook

**Files:**
- Modify: `cmd/bmo-pak/wakeword.go`
- Test: `cmd/bmo-pak/wakeword_test.go`

- [ ] **Step 1: Write the failing test**

Append to `cmd/bmo-pak/wakeword_test.go`:

```go
func TestWakeLoopSuppressed(t *testing.T) {
	var l wakeLoop
	if l.suppressed() {
		t.Fatal("nil prankActive must report not suppressed")
	}
	flag := &atomic.Bool{}
	l.prankActive = flag
	if l.suppressed() {
		t.Fatal("flag false must report not suppressed")
	}
	flag.Store(true)
	if !l.suppressed() {
		t.Fatal("flag true must report suppressed")
	}
}
```

Ensure `wakeword_test.go` imports `"sync/atomic"` (add to its import block if absent).

- [ ] **Step 2: Run test to verify it fails**

Run: `CGO_ENABLED=1 go test ./cmd/bmo-pak/ -run TestWakeLoopSuppressed -v`
Expected: FAIL — `l.suppressed undefined` and `l.prankActive undefined`.

- [ ] **Step 3: Add the field, helper, parameter, and guard**

In `cmd/bmo-pak/wakeword.go`:

1. Add `"sync/atomic"` to the import block.

2. Add a field to the `wakeLoop` struct (after `endSilence  time.Duration`):

```go
	// prankActive, when set and true, makes the loop stand down entirely so the
	// Evil BMO prank flow owns the mic and the loop never grabs the overheard
	// reply nor self-triggers. Nil outside the prank build.
	prankActive *atomic.Bool
```

3. Add the helper near `captureShouldFinish`:

```go
// suppressed reports whether an Evil BMO prank currently owns the mic, in which
// case the wake loop ignores all batches.
func (l *wakeLoop) suppressed() bool {
	return l.prankActive != nil && l.prankActive.Load()
}
```

4. At the very top of `handleBatch`, before `now := time.Now()`:

```go
	if l.suppressed() {
		l.detector.Reset()
		return
	}
```

5. Change the `startWakeWord` signature to accept the flag (add the parameter at the end):

```go
func startWakeWord(ctx context.Context, logger pttLogger, machine *assistant.Machine, cfg config.Config, router *audio.CaptureRouter, pipeline *assistant.VoicePipeline, gov *power.Governor, assets wakeAssets, sampleRate, channels int, prankActive *atomic.Bool) func() {
```

6. Set the field where the `loop` literal is built (after `endSilence:  wakeEndSilenceFor(cfg.WakeEndSilence),`):

```go
		prankActive: prankActive,
```

7. Update the existing caller in `cmd/bmo-pak/main.go` (inside `restartWake`, the `stopWake = startWakeWord(...)` line) to pass `nil` for now — Task 6 replaces it with the real flag:

```go
				stopWake = startWakeWord(ctx, logger, machine, cfg, audioRouter, audioPipeline, gov, assets, audioCfg.SampleRate, audioCfg.Channels, nil)
```

- [ ] **Step 4: Run test + build to verify**

Run: `CGO_ENABLED=1 go test ./cmd/bmo-pak/ -run TestWakeLoopSuppressed -v`
Expected: PASS.
Run: `CGO_ENABLED=1 go build ./...`
Expected: builds clean.

- [ ] **Step 5: Commit**

```bash
git add cmd/bmo-pak/wakeword.go cmd/bmo-pak/wakeword_test.go cmd/bmo-pak/main.go
git commit -m "feat(bmo-pak): add prank-active suppression hook to the wake loop"
```

---

### Task 3: Prank sequence logic (`prankSession`)

**Files:**
- Create: `cmd/bmo-pak/evil_prank.go`
- Test: `cmd/bmo-pak/evil_prank_test.go`

This task adds the gate constant, nudge/cadence constants, the `prankVoice` interface, and the bounded sequence (`run`/`listenOnce`). The capture helper, adapter, and engine come in Tasks 4–5.

- [ ] **Step 1: Write the failing tests**

Create `cmd/bmo-pak/evil_prank_test.go`:

```go
package main

import (
	"context"
	"math/rand"
	"strings"
	"testing"
)

// fakeVoice records the ordered sequence of pipeline calls the prank makes and
// returns scripted values.
type fakeVoice struct {
	taunt      string
	tauntErr   error
	transcript string
	calls      []string
}

func (f *fakeVoice) GenerateRemarkText(ctx context.Context, nudge string) (string, error) {
	f.calls = append(f.calls, "generate")
	return f.taunt, f.tauntErr
}
func (f *fakeVoice) SpeakVerbatim(ctx context.Context, text string) error {
	f.calls = append(f.calls, "verbatim:"+text)
	return nil
}
func (f *fakeVoice) SpeakRemark(ctx context.Context, nudge string) error {
	f.calls = append(f.calls, "remark:"+nudge)
	return nil
}
func (f *fakeVoice) Transcribe(ctx context.Context, pcm []byte) (string, error) {
	f.calls = append(f.calls, "transcribe")
	return f.transcript, nil
}

// scriptedListen returns the next scripted capture result per call.
func scriptedListen(results ...[]byte) func(context.Context) []byte {
	i := 0
	return func(context.Context) []byte {
		if i >= len(results) {
			return nil
		}
		r := results[i]
		i++
		return r
	}
}

func newTestSession(v prankVoice, rounds int, listen func(context.Context) []byte) *prankSession {
	return &prankSession{
		voice:       v,
		listen:      listen,
		beginListen: func() {},
		endListen:   func() {},
		rounds:      func() int { return rounds },
		rng:         rand.New(rand.NewSource(1)),
	}
}

func lastRemark(calls []string) string {
	for i := len(calls) - 1; i >= 0; i-- {
		if strings.HasPrefix(calls[i], "remark:") {
			return calls[i]
		}
	}
	return ""
}

func TestPrankNoReplyOnFirstListen(t *testing.T) {
	v := &fakeVoice{taunt: "ha, nice try"}
	s := newTestSession(v, 3, scriptedListen(nil))
	s.run(context.Background())

	if v.calls[0] != "generate" {
		t.Fatalf("first call = %q, want generate", v.calls[0])
	}
	if !strings.HasPrefix(v.calls[1], "verbatim:Hey B") || !strings.Contains(v.calls[1], "ha, nice try") {
		t.Fatalf("second call = %q, want fused wake+taunt", v.calls[1])
	}
	if !strings.Contains(lastRemark(v.calls), noReplyNudge) {
		t.Fatalf("expected no-reply remark, got %q", lastRemark(v.calls))
	}
	for _, c := range v.calls {
		if c == "transcribe" {
			t.Fatal("must not transcribe when nobody replied")
		}
	}
}

func TestPrankRunsToCloserWhenAlwaysAnswered(t *testing.T) {
	v := &fakeVoice{taunt: "t", transcript: "nope"}
	s := newTestSession(v, 2, scriptedListen([]byte{1}, []byte{1}))
	s.run(context.Background())

	remarks := 0
	transcribes := 0
	for _, c := range v.calls {
		if strings.HasPrefix(c, "remark:") {
			remarks++
		}
		if c == "transcribe" {
			transcribes++
		}
	}
	if remarks != 2 || transcribes != 2 {
		t.Fatalf("calls=%v; want 2 remarks (comeback+closer) and 2 transcribes", v.calls)
	}
	if !strings.Contains(lastRemark(v.calls), closerNudgeMarker) {
		t.Fatalf("final remark must be the closer, got %q", lastRemark(v.calls))
	}
}

func TestPrankClosesOnMidStreamSilence(t *testing.T) {
	v := &fakeVoice{taunt: "t", transcript: "huh"}
	s := newTestSession(v, 3, scriptedListen([]byte{1}, nil))
	s.run(context.Background())

	if !strings.Contains(lastRemark(v.calls), lostInterestNudge) {
		t.Fatalf("expected lost-interest remark on mid-stream silence, got %q", lastRemark(v.calls))
	}
}

func TestPrankAbortsOnEmptyTaunt(t *testing.T) {
	v := &fakeVoice{taunt: ""}
	s := newTestSession(v, 2, scriptedListen([]byte{1}))
	s.run(context.Background())
	if len(v.calls) != 1 || v.calls[0] != "generate" {
		t.Fatalf("calls=%v; want only the failed generate, then abort", v.calls)
	}
}

func TestPrankRoundsAlwaysTwoOrThree(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	roundsFn := func() int { return 2 + rng.Intn(2) }
	for i := 0; i < 200; i++ {
		if n := roundsFn(); n != 2 && n != 3 {
			t.Fatalf("rounds = %d, want 2 or 3", n)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `CGO_ENABLED=1 go test ./cmd/bmo-pak/ -run TestPrank -v`
Expected: FAIL — `prankVoice`, `prankSession`, `noReplyNudge`, `closerNudgeMarker`, `lostInterestNudge` undefined.

- [ ] **Step 3: Create `evil_prank.go` with constants, interface, and sequence**

Create `cmd/bmo-pak/evil_prank.go`:

```go
package main

// Evil BMO prank easter egg.
//
// This is a DELIBERATELY-HIDDEN, NON-MOD feature. It is hardcoded in the binary
// and gated on the Evil BMO example mod being active with AI enabled. It is an
// intentional exception to the normal mod feature path: there is no manifest
// field, no settings entry, no documentation, and the examples/mods/evil-bmo
// directory is NOT modified by it. Do not generalize this into the mod system.

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"
)

// evilModID is the active-mod ID that unlocks the prank. It equals the example
// mod's directory/zip name (internal/mod derives Mod.ID from that name).
const evilModID = "evil-bmo"

// Cadence for the spontaneous (non-D-pad) trigger: a heavily jittered interval
// in [prankAutoMin, prankAutoMin+prankAutoSpan). "Very rare" by design.
const (
	prankAutoMin  = 2 * time.Hour
	prankAutoSpan = 2 * time.Hour
)

// prankListenWindow is how long Evil BMO waits for the other device to start
// replying after a taunt — the "long" continued-conversation window, applied
// here regardless of the user's continued_conversation setting.
const prankListenWindow = 20 * time.Second

// evilWakePhrases are spoken at the front of the fused taunt utterance to trip a
// nearby device's wake detector.
var evilWakePhrases = []string{"Hey BMO", "Hey BEEMO"}

// closerNudgeMarker is a stable substring of closerNudge, used so the sequence
// can be asserted in tests without pinning the full wording.
const closerNudgeMarker = "End this exchange"

const (
	tauntNudge = "You are about to prank a nearby BMO unit. In one short sentence, ask it a trick question or make a cutting, in-character remark designed to provoke it. Reply with only that single line — no preamble, no quotation marks."

	noReplyNudge = "You taunted a nearby BMO but no one answered. Make one short, smug, in-character remark about being ignored or there being no one worth talking to. Reply with only that line."

	lostInterestNudge = "The BMO you were taunting has gone quiet mid-conversation. Make one short, dismissive, in-character remark about it losing its nerve, then drop it. Reply with only that line."

	comebackNudgeFmt = "A nearby BMO answered your taunt by saying: %q. In one short, in-character line, mock its answer or fire back a cutting comeback. Reply with only that line."

	closerNudgeFmt = "End this exchange. A nearby BMO answered: %q. Reply with one short, dismissive, in-character sign-off. Do NOT ask a question or invite any further reply. Reply with only that line."
)

// prankVoice is the slice of VoicePipeline the prank uses, narrowed to an
// interface so the sequence can be unit-tested with a fake.
type prankVoice interface {
	GenerateRemarkText(ctx context.Context, nudge string) (string, error)
	SpeakVerbatim(ctx context.Context, text string) error
	SpeakRemark(ctx context.Context, nudge string) error
	Transcribe(ctx context.Context, pcm []byte) (string, error)
}

// prankSession runs one bounded taunt→listen→react conversation. All external
// effects go through injected funcs/interfaces so run is deterministic in tests.
type prankSession struct {
	voice       prankVoice
	listen      func(ctx context.Context) []byte // captured reply PCM, nil/empty if none
	beginListen func()                           // machine → listening (suppresses wake loop, shows face)
	endListen   func()                           // machine → idle
	rounds      func() int                       // number of reply rounds to engage (2 or 3)
	rng         *rand.Rand
	logger      pttLogger
}

// run performs the whole prank. It is meant to be invoked on its own goroutine.
func (s *prankSession) run(ctx context.Context) {
	taunt, err := s.voice.GenerateRemarkText(ctx, tauntNudge)
	if err != nil || strings.TrimSpace(taunt) == "" {
		s.logf("evil prank: taunt generation failed or empty (%v); aborting", err)
		return
	}
	wake := evilWakePhrases[s.rng.Intn(len(evilWakePhrases))]
	if err := s.voice.SpeakVerbatim(ctx, wake+"... "+taunt); err != nil {
		s.logf("evil prank: speaking taunt failed: %v", err)
		return
	}

	maxRounds := s.rounds()
	for round := 1; ; round++ {
		if ctx.Err() != nil { // aborted (B press / shutdown)
			return
		}
		reply := s.listenOnce(ctx)
		if reply == "" {
			if round == 1 {
				_ = s.voice.SpeakRemark(ctx, noReplyNudge)
			} else {
				_ = s.voice.SpeakRemark(ctx, lostInterestNudge)
			}
			return
		}
		if round >= maxRounds {
			_ = s.voice.SpeakRemark(ctx, fmt.Sprintf(closerNudgeFmt, reply))
			return
		}
		_ = s.voice.SpeakRemark(ctx, fmt.Sprintf(comebackNudgeFmt, reply))
	}
}

// listenOnce shows the listening face, captures one utterance within the window,
// returns to idle, and transcribes. Returns "" when nothing intelligible was
// heard.
func (s *prankSession) listenOnce(ctx context.Context) string {
	s.beginListen()
	pcm := s.listen(ctx)
	s.endListen()
	if len(pcm) == 0 {
		return ""
	}
	text, err := s.voice.Transcribe(ctx, pcm)
	if err != nil {
		s.logf("evil prank: transcribe failed: %v", err)
		return ""
	}
	return strings.TrimSpace(text)
}

func (s *prankSession) logf(format string, args ...any) {
	if s.logger != nil {
		s.logger.Infof(format, args...)
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `CGO_ENABLED=1 go test ./cmd/bmo-pak/ -run TestPrank -v`
Expected: PASS (all five).

- [ ] **Step 5: Commit**

```bash
git add cmd/bmo-pak/evil_prank.go cmd/bmo-pak/evil_prank_test.go
git commit -m "feat(bmo-pak): bounded Evil BMO prank sequence logic"
```

---

### Task 4: `listenForReply` capture helper

**Files:**
- Modify: `cmd/bmo-pak/evil_prank.go`
- Test: `cmd/bmo-pak/evil_prank_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `cmd/bmo-pak/evil_prank_test.go` (add `"time"` and the project audio import path to the import block):

```go
// framesSource is a scripted audio.PCMSource that emits the given frames then
// closes, so listenForReply can be driven deterministically.
type framesSource struct{ frames [][]byte }

func (s *framesSource) Start() error { return nil }
func (s *framesSource) Frames() <-chan []byte {
	ch := make(chan []byte, len(s.frames))
	for _, f := range s.frames {
		ch <- f
	}
	close(ch)
	return ch
}

func signalFrame(n int) []byte {
	b := make([]byte, n)
	for i := 0; i+1 < n; i += 2 {
		binary.LittleEndian.PutUint16(b[i:i+2], uint16(int16(8192)))
	}
	return b
}
func silenceFrame(n int) []byte { return make([]byte, n) }

func TestListenForReplyCapturesThenEndsOnSilence(t *testing.T) {
	const frame = 100             // bytes per frame
	const bps = 1000              // bytes/sec -> each frame = 0.1s
	src := &framesSource{frames: [][]byte{
		silenceFrame(frame),          // pre-onset quiet (ignored)
		signalFrame(frame),           // onset
		signalFrame(frame),           // speech
		silenceFrame(frame),          // trailing silence (0.1s)
		silenceFrame(frame),          // trailing silence (0.2s) -> finishes
	}}
	router := audio.NewCaptureRouter(src, frame)
	if err := router.Start(); err != nil {
		t.Fatalf("router start: %v", err)
	}
	defer router.Close()

	pcm := listenForReply(context.Background(), router, bps, prankListenWindow, 200*time.Millisecond, 10*time.Second, 0.01)
	if len(pcm) == 0 {
		t.Fatal("expected captured PCM, got none")
	}
	// Capture starts at the onset signal frame and includes everything after.
	if len(pcm) != 4*frame {
		t.Fatalf("captured %d bytes, want %d (onset + 3 following frames)", len(pcm), 4*frame)
	}
}

func TestListenForReplyReturnsNilWhenSilent(t *testing.T) {
	const frame = 100
	src := &framesSource{frames: [][]byte{silenceFrame(frame), silenceFrame(frame)}}
	router := audio.NewCaptureRouter(src, frame)
	if err := router.Start(); err != nil {
		t.Fatalf("router start: %v", err)
	}
	defer router.Close()

	pcm := listenForReply(context.Background(), router, 1000, prankListenWindow, 200*time.Millisecond, 10*time.Second, 0.01)
	if pcm != nil {
		t.Fatalf("expected nil on all-silence input, got %d bytes", len(pcm))
	}
}
```

Add to the test file's import block: `"encoding/binary"`, `"time"`, and `"github.com/carroarmato0/nextui-bmo/internal/audio"` (use the module path; confirm it from another `cmd/bmo-pak` file's import of `internal/audio`).

- [ ] **Step 2: Run tests to verify they fail**

Run: `CGO_ENABLED=1 go test ./cmd/bmo-pak/ -run TestListenForReply -v`
Expected: FAIL — `listenForReply` undefined.

- [ ] **Step 3: Implement `listenForReply`**

Add to `cmd/bmo-pak/evil_prank.go` (add `"github.com/carroarmato0/nextui-bmo/internal/audio"` and `"github.com/carroarmato0/nextui-bmo/internal/input"` to its import block — match the module path used elsewhere in `cmd/bmo-pak`):

```go
// listenForReply subscribes to the capture router and waits up to onsetWindow
// for speech to begin. Once it does, it captures the utterance until endSilence
// of trailing quiet or maxCapture is reached, then returns the PCM. Returns nil
// if no speech began before the window elapsed, the source ended, or ctx was
// cancelled before any speech. This mirrors the wake loop's end-of-silence
// batching (continueCapture) for a one-shot listen.
func listenForReply(ctx context.Context, router *audio.CaptureRouter, bytesPerSec int, onsetWindow, endSilence, maxCapture time.Duration, vad float64) []byte {
	sub, cancel := router.Subscribe()
	defer cancel()

	buf := input.NewBuffer(bytesPerSec*int(maxCapture/time.Second) + bytesPerSec)
	onsetDeadline := time.Now().Add(onsetWindow)
	capturing := false
	var captureStart time.Time
	var silenceRun time.Duration

	for {
		select {
		case <-ctx.Done():
			if capturing {
				return buf.End()
			}
			return nil
		case batch, ok := <-sub:
			if !ok {
				if capturing {
					return buf.End()
				}
				return nil
			}
			now := time.Now()
			signal := audio.PCMHasSignal(batch, vad)
			if !capturing {
				if signal {
					buf.Begin()
					buf.Append(batch)
					capturing = true
					captureStart = now
					silenceRun = 0
				} else if now.After(onsetDeadline) {
					return nil
				}
				continue
			}
			buf.Append(batch)
			if signal {
				silenceRun = 0
			} else {
				silenceRun += time.Duration(float64(len(batch)) / float64(bytesPerSec) * float64(time.Second))
			}
			if silenceRun >= endSilence || now.Sub(captureStart) >= maxCapture {
				return buf.End()
			}
		}
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `CGO_ENABLED=1 go test ./cmd/bmo-pak/ -run TestListenForReply -v`
Expected: PASS (both).

- [ ] **Step 5: Commit**

```bash
git add cmd/bmo-pak/evil_prank.go cmd/bmo-pak/evil_prank_test.go
git commit -m "feat(bmo-pak): one-shot listen-for-reply capture helper"
```

---

### Task 5: `evilPrank` engine + `pipelineVoice` adapter + scheduler

**Files:**
- Modify: `cmd/bmo-pak/evil_prank.go`
- Test: `cmd/bmo-pak/evil_prank_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `cmd/bmo-pak/evil_prank_test.go` (add `"sync/atomic"` to imports):

```go
func TestEvilPrankTriggerGate(t *testing.T) {
	ran := 0
	e := &evilPrank{
		gate:   func() bool { return false },
		idle:   func() bool { return true },
		run:    func(context.Context) { ran++ },
		active: &atomic.Bool{},
	}
	e.trigger(context.Background())
	if ran != 0 {
		t.Fatal("gate false must not run the prank")
	}
	e.gate = func() bool { return true }
	e.idle = func() bool { return false }
	e.trigger(context.Background())
	if ran != 0 {
		t.Fatal("non-idle must not run the prank")
	}
}

func TestEvilPrankTriggerSingleFlight(t *testing.T) {
	start := make(chan struct{})
	release := make(chan struct{})
	var ran atomic.Int32
	e := &evilPrank{
		gate:   func() bool { return true },
		idle:   func() bool { return true },
		active: &atomic.Bool{},
		run: func(context.Context) {
			ran.Add(1)
			close(start)
			<-release
		},
	}
	e.trigger(context.Background())
	<-start            // first prank is now running
	e.trigger(context.Background()) // must be ignored (single-flight)
	close(release)
	// Give the goroutine a moment to clear the flag.
	for i := 0; i < 100 && e.active.Load(); i++ {
		time.Sleep(time.Millisecond)
	}
	if got := ran.Load(); got != 1 {
		t.Fatalf("run called %d times, want 1 (single-flight)", got)
	}
	if e.active.Load() {
		t.Fatal("active flag should be cleared after run returns")
	}
}

func TestEvilPrankStopCancelsRun(t *testing.T) {
	done := make(chan struct{})
	e := &evilPrank{
		gate:   func() bool { return true },
		idle:   func() bool { return true },
		active: &atomic.Bool{},
		run:    func(ctx context.Context) { <-ctx.Done(); close(done) },
	}
	e.trigger(context.Background())
	for i := 0; i < 100 && !e.active.Load(); i++ {
		time.Sleep(time.Millisecond)
	}
	if !e.stop() {
		t.Fatal("stop should report an active prank")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("run was not cancelled by stop")
	}
	if e.stop() {
		t.Fatal("stop with no active prank should return false")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `CGO_ENABLED=1 go test ./cmd/bmo-pak/ -run TestEvilPrankTrigger -v`
Expected: FAIL — `evilPrank` undefined.

- [ ] **Step 3: Implement engine, adapter, and scheduler**

Add to `cmd/bmo-pak/evil_prank.go` (add `"sync"` and `"sync/atomic"` to imports, and `"github.com/carroarmato0/nextui-bmo/internal/assistant"` for the adapter — match the module path used elsewhere in `cmd/bmo-pak`):

```go
// pipelineVoice adapts *assistant.VoicePipeline to the prankVoice interface,
// dropping the onSpoken callbacks the prank does not use.
type pipelineVoice struct{ p *assistant.VoicePipeline }

func (v pipelineVoice) GenerateRemarkText(ctx context.Context, nudge string) (string, error) {
	return v.p.GenerateRemarkText(ctx, nudge)
}
func (v pipelineVoice) SpeakVerbatim(ctx context.Context, text string) error {
	return v.p.SpeakVerbatim(ctx, text, nil)
}
func (v pipelineVoice) SpeakRemark(ctx context.Context, nudge string) error {
	return v.p.SpeakRemark(ctx, nudge, nil)
}
func (v pipelineVoice) Transcribe(ctx context.Context, pcm []byte) (string, error) {
	return v.p.Transcribe(ctx, pcm)
}

// evilPrank owns the trigger gating and single-flight guard. gate and idle are
// read on the main goroutine (they touch cfg/activeMod/machine); run executes
// the sequence on a fresh goroutine under a cancelable context so a B press or
// shutdown can abort it.
type evilPrank struct {
	gate   func() bool               // UsesAI && active mod is Evil BMO
	idle   func() bool               // machine is idle
	run    func(ctx context.Context) // the bounded sequence (prankSession.run in prod)
	active *atomic.Bool              // shared with the wake loop for suppression
	logger pttLogger

	mu     sync.Mutex
	cancel context.CancelFunc // non-nil while a prank is running
}

// trigger starts a prank if gating allows and none is already running. Safe to
// call from the main goroutine on either a D-pad press or an auto tick.
func (e *evilPrank) trigger(ctx context.Context) {
	if e == nil || e.gate == nil || !e.gate() {
		return
	}
	if e.idle == nil || !e.idle() {
		return
	}
	if !e.active.CompareAndSwap(false, true) {
		return // a prank is already running
	}
	prankCtx, cancel := context.WithCancel(ctx)
	e.mu.Lock()
	e.cancel = cancel
	e.mu.Unlock()
	if e.logger != nil {
		e.logger.Infof("evil prank: triggered")
	}
	go func() {
		defer func() {
			cancel()
			e.mu.Lock()
			e.cancel = nil
			e.mu.Unlock()
			e.active.Store(false)
		}()
		e.run(prankCtx)
	}()
}

// stop cancels a running prank. Returns true if one was active, so callers (the
// B button, shutdown) can take it as "handled".
func (e *evilPrank) stop() bool {
	if e == nil {
		return false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.cancel == nil {
		return false
	}
	e.cancel()
	return true
}

// startEvilPrankScheduler fires a tick on a heavily jittered interval until ctx
// is cancelled. A pending tick is never queued more than one deep; the main
// loop decides (with full UI state) whether to actually start a prank.
func startEvilPrankScheduler(ctx context.Context, tick chan<- struct{}, rng *rand.Rand, minInterval, span time.Duration) {
	go func() {
		for {
			d := minInterval + time.Duration(rng.Int63n(int64(span)))
			select {
			case <-ctx.Done():
				return
			case <-time.After(d):
			}
			select {
			case tick <- struct{}{}:
			default:
			}
		}
	}()
}
```

- [ ] **Step 4: Run tests + build**

Run: `CGO_ENABLED=1 go test ./cmd/bmo-pak/ -run 'TestEvilPrank|TestPrank|TestListenForReply' -v`
Expected: PASS.
Run: `CGO_ENABLED=1 go build ./...`
Expected: builds clean (the file now imports `assistant`).

- [ ] **Step 5: Commit**

```bash
git add cmd/bmo-pak/evil_prank.go cmd/bmo-pak/evil_prank_test.go
git commit -m "feat(bmo-pak): Evil BMO prank trigger engine, adapter, and scheduler"
```

---

### Task 6: Wire the engine into `main.go`

**Files:**
- Modify: `cmd/bmo-pak/main.go`

No new unit test (this is process wiring of already-tested units); verified by full build + the existing suite + manual two-device check.

- [ ] **Step 1: Declare the outer-scope variables**

In the block around line 307 (with `var audioRouter ...`, `var stopPTT func()`), add:

```go
	var prank *evilPrank
	prankTick := make(chan struct{}, 1)
```

- [ ] **Step 2: Declare the shared suppression flag before `restartWake`**

Just before `gov := &power.Governor{Logf: logger.Warnf}` (inside the `else` branch of the router setup, ~line 372), add:

```go
			prankActive := &atomic.Bool{}
```

- [ ] **Step 3: Pass the flag to `startWakeWord`**

In `restartWake`, change the Task-2 placeholder `nil` to the real flag:

```go
				stopWake = startWakeWord(ctx, logger, machine, cfg, audioRouter, audioPipeline, gov, assets, audioCfg.SampleRate, audioCfg.Channels, prankActive)
```

- [ ] **Step 4: Build the engine after `restartWake(activeMod)`**

Immediately after the `restartWake(activeMod)` line (~389), still inside the same `else` block, add:

```go
			// Evil BMO prank easter egg (hidden, non-mod; see evil_prank.go).
			// Gated on AI + the Evil BMO mod, re-checked at trigger time so a
			// runtime mod/AI change enables or disables it immediately.
			prankBytesPerSec := audio.BytesPerSecond(audioCfg.SampleRate, audioCfg.Channels, audio.BytesPerSampleS16LE)
			prankEndSilence := wakeEndSilenceFor(cfg.WakeEndSilence)
			session := &prankSession{
				voice: pipelineVoice{p: audioPipeline},
				listen: func(c context.Context) []byte {
					return listenForReply(c, audioRouter, prankBytesPerSec, prankListenWindow, prankEndSilence, wakeMaxCapture, wakeVADLevel)
				},
				beginListen: func() { machine.Transition(assistant.EventListen) },
				endListen: func() {
					if machine.State() == assistant.StateListening {
						machine.Transition(assistant.EventRest)
					}
				},
				rounds: func() int { return 2 + prankRand.Intn(2) },
				rng:    prankRand,
				logger: logger,
			}
			prank = &evilPrank{
				gate:   func() bool { return cfg.UsesAI() && activeMod.ID == evilModID },
				idle:   func() bool { return machine.Snapshot().Current == assistant.StateIdle },
				run:    session.run,
				active: prankActive,
				logger: logger,
			}
			startEvilPrankScheduler(ctx, prankTick, rand.New(rand.NewSource(time.Now().UnixNano()^0x5eed)), prankAutoMin, prankAutoSpan)
```

Add, just before the `session := &prankSession{...}` literal, the shared RNG (kept separate from the scheduler RNG so taunt/round randomness is independent of fire timing):

```go
			prankRand := rand.New(rand.NewSource(time.Now().UnixNano()))
```

Ensure `cmd/bmo-pak/main.go` imports `"math/rand"` and `"sync/atomic"` (add any missing).

- [ ] **Step 5: Add the D-pad Down branch in `handleNav`**

In `handleNav`, immediately before the `if activeMenu == nil { return }` guard that precedes the `switch action` block (~line 752), add:

```go
		// D-pad Down with no menu open is the hidden Evil BMO prank trigger
		// (AI on + Evil BMO mod). No-op otherwise. With a menu open, Down still
		// navigates via the switch below.
		if action == input.NavDown && activeMenu == nil {
			if prank != nil && !shuttingDown {
				prank.trigger(ctx)
			}
			return
		}
```

- [ ] **Step 6: Let B (NavCancel) abort a running prank**

In `handleNav`'s `if action == input.NavCancel {` block, inside the `switch {` , add a new case immediately after the `case activeMenu != nil:` case:

```go
		case prank != nil && prank.stop():
			if audioPipeline != nil {
				audioPipeline.InterruptSpeech()
			}
			logger.Infof("evil prank aborted by B press")
```

(`prank.stop()` returns false when no prank is running, so this case is skipped in normal operation and the existing clip/batch/speech/exit cases are unaffected.)

- [ ] **Step 7: Abort a running prank on shutdown**

In `beginShutdown` (~line 575), next to its existing `audioPipeline.InterruptSpeech()` call, add:

```go
		if prank != nil {
			prank.stop()
		}
```

- [ ] **Step 8: Poll the auto-tick on the main goroutine**

In the main loop, right after the `drainNav` labelled `for { ... }` block (~line 840, before the SDL event pump), add:

```go
		// Deliver any pending auto prank tick on the main goroutine, where the
		// UI state it gates on (menu, shutdown) is safe to read.
		select {
		case <-prankTick:
			if prank != nil && activeMenu == nil && !shuttingDown {
				prank.trigger(ctx)
			}
		default:
		}
```

- [ ] **Step 9: Build and run the full suite**

Run: `CGO_ENABLED=1 go build ./...`
Expected: builds clean.
Run: `CGO_ENABLED=1 go test ./cmd/bmo-pak/ ./internal/assistant/`
Expected: PASS.

- [ ] **Step 10: Commit**

```bash
git add cmd/bmo-pak/main.go
git commit -m "feat(bmo-pak): wire hidden Evil BMO prank (D-pad Down + rare auto-trigger)"
```

---

### Task 7: Full verification

**Files:** none (verification only)

- [ ] **Step 1: Full build**

Run: `CGO_ENABLED=1 go build ./...`
Expected: no errors.

- [ ] **Step 2: Full test suite**

Run: `CGO_ENABLED=1 go test ./...`
Expected: all packages PASS (or `[no test files]`).

- [ ] **Step 3: Lint**

Run: `golangci-lint run ./...`
Expected: no new findings. If `.go_cache/` permission errors appear, run `chmod -R u+w .go_cache` first (see CLAUDE.md).

- [ ] **Step 4: Confirm the example mod and mod system are untouched**

Run: `git diff --name-only main -- examples/mods/evil-bmo internal/mod internal/config internal/examplemods docs/MODDING.md`
Expected: empty output (no files there changed).

- [ ] **Step 5: Manual two-device verification (record results, do not block on hardware)**

Deploy with `./scripts/deploy.sh` to a device running the Evil BMO mod with AI enabled. With a second BMO nearby (wake word enabled), press **D-pad Down** and observe:
1. BMO speaks "Hey BMO…" then a taunt.
2. It shows the listening face for up to ~20s.
3. If the second device replies, BMO fires a contextual comeback, bounded to 2–3 volleys ending in a dismissive sign-off.
4. With no second device, BMO makes a smug "nobody's home" remark.
Tail logs with `./scripts/debug-logs.sh` and confirm `evil prank: triggered`. Note: the acoustic wake-trip between devices is best-effort and not unit-tested.

- [ ] **Step 6: Final confirmation**

All automated gates green; behavior confirmed on-device (or noted as pending hardware). Feature is complete.
```
