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
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/carroarmato0/nextui-bmo/internal/assistant"
	"github.com/carroarmato0/nextui-bmo/internal/audio"
	"github.com/carroarmato0/nextui-bmo/internal/input"
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
// replying after a taunt. The victim must run its whole wake->STT->chat->TTS
// pipeline before it can answer, so this is deliberately generous — longer than
// the "long" continued-conversation window and applied regardless of the user's
// continued_conversation setting, so a slower victim still gets heard.
const prankListenWindow = 30 * time.Second

// prankWakePause is the deliberate, model-independent gap between the wake call
// ("Hey BMO") and the taunt. They are spoken as two separate utterances so the
// pause length is ours, not whatever the TTS model renders for an ellipsis; it
// also gives the other device time to wake and start listening before the taunt.
const prankWakePause = 1200 * time.Millisecond

// prankReplySettle is how long Evil BMO discards capture before it starts
// listening for the victim's reply. Both devices share a single hw:0,0
// mic/speaker, so the tail of Evil BMO's own taunt (playback bleed + room echo)
// is still arriving on the capture stream the instant the taunt "ends". Without
// this drain that self-echo trips a false onset, and because the victim needs
// ~7s (wake->STT->chat->TTS) before it answers, the false capture closes on the
// gap's silence and Evil BMO fires its next line straight over the victim
// (observed on hardware 2026-06-21). Draining the echo lets prankListenWindow
// wait for the victim's real reply onset. See TestListenForReplyDrainsSelfEchoBeforeOnset.
const prankReplySettle = 1500 * time.Millisecond

// prankReplyMaxCapture caps how much of the victim's reply Evil BMO records
// before transcribing. It must comfortably exceed a full victim utterance
// (~12s observed on hardware): capping shorter cuts the recording mid-reply, so
// Evil BMO transcribes a fragment and starts its comeback while the victim is
// still talking — the same overlap the settle drain fixes, at the tail instead
// of the head. The capture normally ends on prankReplyEndSilence when the victim
// stops; this is only the worst-case bound (hardware 2026-06-21).
const prankReplyMaxCapture = 16 * time.Second

// prankReplyEndSilence ends the reply capture after this much trailing quiet.
// It must tolerate the brief pauses inside a multi-sentence reply without
// closing early (a premature close means Evil BMO talks over the rest), yet be
// short enough that Evil BMO answers promptly once the victim truly stops, while
// the victim's continued-conversation window is still open to hear it.
const prankReplyEndSilence = 1500 * time.Millisecond

// prankTranscribeTimeout bounds the STT call on a captured reply so a slow or
// contended backend can't stall the comeback. A reply is up to
// prankReplyMaxCapture of audio, and a slow/LAN STT backend needs more than the
// clip's own duration to transcribe it, so this budget must comfortably exceed
// prankReplyMaxCapture or every reply times out and degrades to a generic
// comeback (observed on hardware 2026-06-20). On timeout the prank still
// follows up (generic comeback) because it knows a reply was heard. See
// TestPrankReplyBudgetsArePatient.
const prankTranscribeTimeout = 30 * time.Second

// evilWakePhrases are spoken as a standalone utterance to trip a nearby device's
// wake detector, immediately before the (separately spoken) taunt. They are
// spelled phonetically on purpose: tts-1 renders the literal "Hey BMO" as a
// clipped ~0.46s "bmoh" that the victim's "hey bee-mo" wake model often misses,
// whereas "Beemo" with a comma enunciates the two syllables clearly. These are
// only ever spoken (never string-matched), so phonetic spelling is safe.
var evilWakePhrases = []string{"Hey, Beemo.", "Hey, Bee-Moh."}

// closerNudgeMarker is a stable substring of closerNudgeFmt, used so the
// sequence can be asserted in tests without pinning the full wording.
const closerNudgeMarker = "End this exchange"

// The prank nudges are stage directions appended to Evil BMO's persona for each
// generated line. They establish the scene the persona alone does not know: the
// nearby device is ANOTHER BMO unit — a naive, chipper counterpart — and Evil
// BMO is knowingly trying to provoke and outwit it. Every nudge tells the model
// to address that other BMO directly (second person), so Evil BMO sounds like it
// is taunting its twin, not narrating about "a BMO" to a user (hardware feedback
// 2026-06-21).
const (
	tauntNudge = "A nearby device is another BMO unit — a naive, chipper counterpart — and you are deliberately trying to provoke and outwit it. You are speaking directly TO it, not about it. In one short sentence, ask it a trick question or make a cutting, in-character barb designed to get under its circuits. Do NOT greet it, do NOT begin with \"Hey\", and do NOT say its name (\"BMO\" or \"Beemo\") — go straight into the barb. Reply with only that single line — no preamble, no quotation marks."

	noReplyNudge = "You taunted a nearby BMO unit — your naive counterpart — but it never answered. Speaking as if straight at it, make one short, smug, in-character remark about it being too dim or too scared to respond. Reply with only that line."

	lostInterestNudge = "The nearby BMO unit you were taunting — your naive counterpart — has gone quiet mid-exchange. Speaking directly to it, make one short, dismissive, in-character remark about it losing its nerve, then drop it. Reply with only that line."

	comebackNudgeFmt = "You are trading taunts with a nearby BMO unit — your naive counterpart that you are trying to outwit. It just answered you: %q. Speaking directly TO it, fire back one short, in-character line that mocks its answer or one-ups it. Reply with only that line."

	closerNudgeFmt = "End this exchange with a nearby BMO unit — your naive counterpart. It just answered you: %q. Speaking directly to it, reply with one short, dismissive, in-character sign-off. Do NOT ask a question or invite any further reply. Reply with only that line."

	genericComebackNudge = "You are trading taunts with a nearby BMO unit — your naive counterpart — but its reply was too garbled to make out. Speaking directly to it, fire back one short, in-character comeback mocking its mumbling. Reply with only that line."

	genericCloserNudge = "End this exchange with a nearby BMO unit — your naive counterpart — that just mumbled a reply you could not make out. Speaking directly to it, reply with one short, dismissive, in-character sign-off. Do NOT ask a question or invite any further reply. Reply with only that line."
)

// wakeAddressPrefix matches a leading greeting that re-addresses BMO by name
// (e.g. "Hey BMO,", "Hey, BMO!", "Hi Beemo -"). The taunt LLM sometimes opens
// the barb this way despite the nudge; spoken aloud that is a SECOND wake
// utterance landing while the victim is already listening from the real wake
// call, colliding with its capture.
var wakeAddressPrefix = regexp.MustCompile(`(?i)^\s*(hey|hi|hello|greetings)[\s,!.:-]*(bmo|bee-?moh?|beemo|b\.?\s*m\.?\s*o\.?)[\s,!.:;-]+`)

// stripLeadingWakeAddress removes a leading "Hey BMO"-style address from a
// generated taunt so the taunt does not re-speak the wake word. If stripping
// would empty the line, the original (trimmed) text is kept.
func stripLeadingWakeAddress(taunt string) string {
	trimmed := strings.TrimSpace(taunt)
	stripped := strings.TrimSpace(wakeAddressPrefix.ReplaceAllString(trimmed, ""))
	if stripped == "" || stripped == trimmed {
		// Nothing was removed (or removing it would empty the line): leave the
		// text exactly as generated.
		return trimmed
	}
	// A leading address was removed; re-capitalize the first letter so the
	// remaining barb reads cleanly.
	r := []rune(stripped)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// prankVoice is the slice of VoicePipeline the prank uses, narrowed to an
// interface so the sequence can be unit-tested with a fake.
type prankVoice interface {
	GenerateRemarkText(ctx context.Context, nudge string) (string, error)
	SpeakVerbatim(ctx context.Context, text string) error
	SpeakRemark(ctx context.Context, nudge string) error
	Transcribe(ctx context.Context, pcm []byte) (string, error)
}

// prankSession runs one bounded taunt->listen->react conversation. All external
// effects go through injected funcs/interfaces so run is deterministic in tests.
type prankSession struct {
	voice             prankVoice
	listen            func(ctx context.Context) []byte // captured reply PCM, nil/empty if none
	beginListen       func()                           // machine -> listening (suppresses wake loop, shows face)
	endListen         func()                           // machine -> idle
	rounds            func() int                       // number of reply rounds to engage (2 or 3)
	pause             func(ctx context.Context)        // deliberate gap between the wake call and the taunt
	transcribeTimeout time.Duration                    // bound on the STT call for a captured reply (0 = unbounded)
	rng               *rand.Rand
	logger            pttLogger
}

// run performs the whole prank. It is meant to be invoked on its own goroutine.
func (s *prankSession) run(ctx context.Context) {
	taunt, err := s.voice.GenerateRemarkText(ctx, tauntNudge)
	if err != nil || strings.TrimSpace(taunt) == "" {
		s.logf("evil prank: taunt generation failed or empty (%v); aborting", err)
		return
	}
	// Defensively strip any leading "Hey BMO" the LLM prepended despite the
	// nudge, so the taunt never re-speaks the wake word into the victim's
	// already-open listen window.
	taunt = stripLeadingWakeAddress(taunt)
	// Wake call and taunt are two separate utterances with a deliberate pause
	// between them, so the gap is ours to control regardless of the TTS model.
	wake := evilWakePhrases[s.rng.Intn(len(evilWakePhrases))]
	if err := s.voice.SpeakVerbatim(ctx, wake); err != nil {
		s.logf("evil prank: speaking wake call failed: %v", err)
		return
	}
	s.pause(ctx)
	if ctx.Err() != nil { // aborted during the pause
		return
	}
	if err := s.voice.SpeakVerbatim(ctx, taunt); err != nil {
		s.logf("evil prank: speaking taunt failed: %v", err)
		return
	}

	maxRounds := s.rounds()
	for round := 1; ; round++ {
		if ctx.Err() != nil { // aborted (B press / shutdown)
			return
		}
		heard, reply := s.listenOnce(ctx)
		if ctx.Err() != nil { // aborted while listening: don't emit a closing line
			return
		}
		if !heard {
			// True silence: nobody answered (round 1) or went quiet mid-chat.
			if round == 1 {
				_ = s.voice.SpeakRemark(ctx, noReplyNudge)
			} else {
				_ = s.voice.SpeakRemark(ctx, lostInterestNudge)
			}
			return
		}
		// A reply was heard. If STT produced text, use it for a contextual
		// line; otherwise fall back to a generic one so Evil BMO still follows
		// up rather than wrongly claiming nobody answered.
		if round >= maxRounds {
			if reply != "" {
				_ = s.voice.SpeakRemark(ctx, fmt.Sprintf(closerNudgeFmt, reply))
			} else {
				_ = s.voice.SpeakRemark(ctx, genericCloserNudge)
			}
			return
		}
		if reply != "" {
			_ = s.voice.SpeakRemark(ctx, fmt.Sprintf(comebackNudgeFmt, reply))
		} else {
			_ = s.voice.SpeakRemark(ctx, genericComebackNudge)
		}
	}
}

// listenOnce captures one utterance within the window and transcribes it. It
// returns whether any audio was heard and, separately, the transcript. A reply
// can be heard but yield no transcript (STT timed out/failed or returned blank);
// callers must distinguish that from true silence so Evil BMO still follows up.
func (s *prankSession) listenOnce(ctx context.Context) (heard bool, transcript string) {
	s.beginListen()
	pcm := s.listen(ctx)
	s.endListen()
	if len(pcm) == 0 {
		return false, ""
	}
	// Bound the STT call so a slow/contended backend can't stall the comeback.
	tctx := ctx
	if s.transcribeTimeout > 0 {
		var cancel context.CancelFunc
		tctx, cancel = context.WithTimeout(ctx, s.transcribeTimeout)
		defer cancel()
	}
	start := time.Now()
	text, err := s.voice.Transcribe(tctx, pcm)
	elapsed := time.Since(start).Round(time.Millisecond)
	if err != nil {
		s.logf("evil prank: transcribe failed after %s on %d bytes (%v); using a generic comeback", elapsed, len(pcm), err)
		return true, ""
	}
	// Log the transcribe cost: it dominates Evil BMO's between-round turnaround,
	// and turnaround that outruns the victim's patience makes the victim give up
	// before the next line. Keep this visible so the budget stays tuned.
	s.logf("evil prank: reply transcribed in %s (%d bytes captured)", elapsed, len(pcm))
	return true, strings.TrimSpace(text)
}

func (s *prankSession) logf(format string, args ...any) {
	if s.logger != nil {
		s.logger.Infof(format, args...)
	}
}

// listenForReply subscribes to the capture router and waits up to onsetWindow
// for speech to begin. Once it does, it captures the utterance until endSilence
// of trailing quiet or maxCapture is reached, then returns the PCM. Returns nil
// if no speech began before the window elapsed, the source ended, or ctx was
// cancelled before any speech. This mirrors the wake loop's end-of-silence
// batching (continueCapture) for a one-shot listen.
// maxCapture keeps this a fully self-contained capture primitive (all timing
// knobs are arguments); the lone production caller passing wakeMaxCapture is
// incidental, so unparam's "always the same" warning is expected here.
// pcmDuration converts a PCM byte count into its playback duration at the given
// rate, used to measure settle/silence windows in audio-time rather than
// wall-clock so they track captured samples deterministically.
func pcmDuration(n, bytesPerSec int) time.Duration {
	return time.Duration(float64(n) / float64(bytesPerSec) * float64(time.Second))
}

// drainSelfEcho discards `settle` worth of audio from sub before reply onset
// detection begins. On the shared hw:0,0 mic/speaker the tail of Evil BMO's own
// taunt is still arriving when it starts listening; draining it stops that echo
// from tripping a false onset. Returns false if ctx was cancelled or the stream
// ended during the drain (caller has nothing to capture).
func drainSelfEcho(ctx context.Context, sub <-chan []byte, settle time.Duration, bytesPerSec int) bool {
	var settled time.Duration
	for settled < settle {
		select {
		case <-ctx.Done():
			return false
		case batch, ok := <-sub:
			if !ok {
				return false
			}
			settled += pcmDuration(len(batch), bytesPerSec)
		}
	}
	return true
}

// replyCapture is the onset-then-silence state machine for one reply capture.
// It waits for speech onset until onsetDeadline, then records until endSilence of
// trailing quiet or maxCapture is reached. Kept as a type so listenForReply's
// receive loop stays a thin shell over feed.
type replyCapture struct {
	buf           *input.Buffer
	bytesPerSec   int
	endSilence    time.Duration
	maxCapture    time.Duration
	vad           float64
	onsetDeadline time.Time

	capturing    bool
	captureStart time.Time
	silenceRun   time.Duration
}

// feed processes one batch at time now. done is true once the capture is
// complete (or the onset window elapsed with no speech); result holds the
// captured PCM, nil when nothing was captured.
func (rc *replyCapture) feed(batch []byte, now time.Time) (done bool, result []byte) {
	signal := audio.PCMHasSignal(batch, rc.vad)
	if !rc.capturing {
		if signal {
			rc.buf.Begin()
			rc.buf.Append(batch)
			rc.capturing = true
			rc.captureStart = now
			rc.silenceRun = 0
		} else if now.After(rc.onsetDeadline) {
			return true, nil
		}
		return false, nil
	}
	rc.buf.Append(batch)
	if signal {
		rc.silenceRun = 0
	} else {
		rc.silenceRun += pcmDuration(len(batch), rc.bytesPerSec)
	}
	if rc.silenceRun >= rc.endSilence || now.Sub(rc.captureStart) >= rc.maxCapture {
		return true, rc.buf.End()
	}
	return false, nil
}

// flush returns whatever was captured when the stream ends or the context is
// cancelled mid-capture.
func (rc *replyCapture) flush() []byte {
	if rc.capturing {
		return rc.buf.End()
	}
	return nil
}

func listenForReply(ctx context.Context, router *audio.CaptureRouter, bytesPerSec int, settle, onsetWindow, endSilence, maxCapture time.Duration, vad float64) []byte { //nolint:unparam // see note above
	sub, cancel := router.Subscribe()
	defer cancel()

	if settle > 0 && !drainSelfEcho(ctx, sub, settle, bytesPerSec) {
		return nil
	}

	rc := &replyCapture{
		buf:           input.NewBuffer(bytesPerSec*int(maxCapture/time.Second) + bytesPerSec),
		bytesPerSec:   bytesPerSec,
		endSilence:    endSilence,
		maxCapture:    maxCapture,
		vad:           vad,
		onsetDeadline: time.Now().Add(onsetWindow),
	}

	for {
		select {
		case <-ctx.Done():
			return rc.flush()
		case batch, ok := <-sub:
			if !ok {
				return rc.flush()
			}
			if done, result := rc.feed(batch, time.Now()); done {
				return result
			}
		}
	}
}

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

// evilPrank owns the trigger gating and single-flight guard. trigger and stop
// must both be called from the main goroutine (they read gate/idle, which touch
// cfg/activeMod/machine, and they rely on being serialized with each other);
// run executes the sequence on a fresh goroutine under a cancelable context so a
// B press or shutdown can abort it.
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

// running reports whether a prank is currently in progress. nil-safe so the
// main loop can gate proactive remarks on it: a prank owns the speaker and mic
// for its whole duration, so the proactive scheduler must stand down (just like
// the wake loop does on the shared active flag) — otherwise a scheduled remark
// can fire in an idle gap between rounds and Evil BMO talks over its own prank.
func (e *evilPrank) running() bool {
	return e != nil && e.active.Load()
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
