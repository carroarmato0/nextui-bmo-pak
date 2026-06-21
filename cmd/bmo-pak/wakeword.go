package main

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/carroarmato0/nextui-bmo/internal/assistant"
	"github.com/carroarmato0/nextui-bmo/internal/audio"
	"github.com/carroarmato0/nextui-bmo/internal/config"
	"github.com/carroarmato0/nextui-bmo/internal/input"
	"github.com/carroarmato0/nextui-bmo/internal/power"
	"github.com/carroarmato0/nextui-bmo/internal/wakeword"
)

const (
	wakeGuardTail   = 600 * time.Millisecond // suppress detection this long after TTS ends
	wakeMaxCapture  = 10 * time.Second       // hard cap on a single utterance
	wakeMaxFollowUp = 6                      // continued-conversation follow-up cap
	wakeVADLevel    = 0.01                   // matches voice.go silence rejection
)

// wakeAssets locates the ONNX runtime library and openWakeWord models.
type wakeAssets struct {
	ORTLib    string
	MelModel  string
	EmbModel  string
	WakeModel string
}

// wakeController holds the pure gating / follow-up-window logic so it can be
// unit-tested without ONNX, audio, or the state machine.
type wakeController struct {
	now             time.Time
	speaking        bool
	speechEndedAt   time.Time
	guardTail       time.Duration
	continuedWindow time.Duration // 0 = continued conversation off
	windowUntil     time.Time
	maxFollowUps    int
	followUps       int
	engaged         bool
}

// observeState tracks whether BMO is currently speaking, recording the moment
// speech ends so the guard tail can be measured from it.
func (c *wakeController) observeState(speaking bool, t time.Time) {
	if speaking {
		c.speaking = true
		return
	}
	if c.speaking {
		c.speaking = false
		c.speechEndedAt = t
	}
}

// onDetection reports whether a detection at time t should trigger capture:
// only when BMO is not speaking and the post-speech guard tail has elapsed.
func (c *wakeController) onDetection(t time.Time) bool {
	if c.speaking {
		return false
	}
	if !c.speechEndedAt.IsZero() && t.Sub(c.speechEndedAt) < c.guardTail {
		return false
	}
	return true
}

// noteReply records a productive utterance (one that actually carried speech).
// It (re)anchors the continued-conversation window to this reply and, for
// follow-up turns, spends one unit of the turn budget. Silent/dropped captures
// never call this, so the window stays anchored to the last genuine reply and
// the budget is spent only on real back-and-forth — otherwise the seconds the
// other party spends transcribing/thinking between turns generate no-signal
// captures that each used to burn a slot, draining the budget after a couple of
// exchanges (observed cross-device 2026-06-21). The window closes once the turn
// cap is reached or continued conversation is off.
func (c *wakeController) noteReply(t time.Time, isFollowUp bool) {
	c.speaking = false
	c.speechEndedAt = t
	if isFollowUp {
		c.followUps++
	}
	if c.continuedWindow <= 0 || c.followUps >= c.maxFollowUps {
		c.windowUntil = time.Time{}
		return
	}
	c.windowUntil = t.Add(c.continuedWindow)
}

// windowOpen reports whether the follow-up window is still open at time t.
func (c *wakeController) windowOpen(t time.Time) bool {
	return !c.windowUntil.IsZero() && t.Before(c.windowUntil)
}

// startSession marks the beginning of a wake interaction. Idempotent: it is
// called on the first capture and on every follow-up capture, so the session
// stays engaged for the whole conversation.
func (c *wakeController) startSession() {
	c.engaged = true
}

// Engaged reports whether a wake interaction is in progress (from the first
// capture until the conversation returns to idle).
func (c *wakeController) Engaged() bool {
	return c.engaged
}

// resetFollowUps is called when the conversation returns fully to idle.
func (c *wakeController) resetFollowUps() {
	c.followUps = 0
	c.engaged = false
}

func continuedWindowFor(mode string) time.Duration {
	switch mode {
	case config.ContinuedConvoShort:
		return 8 * time.Second
	case config.ContinuedConvoLong:
		return 20 * time.Second
	default:
		return 0
	}
}

// wakeEndSilenceFor maps a config end-of-turn silence tier to the trailing-
// silence duration that ends a capture. Unknown values map to balanced.
func wakeEndSilenceFor(tier string) time.Duration {
	switch tier {
	case config.WakeEndSilenceSnappy:
		return 1000 * time.Millisecond
	case config.WakeEndSilencePatient:
		return 1600 * time.Millisecond
	default: // balanced / empty / unknown
		return 1300 * time.Millisecond
	}
}

// startWakeWord runs the on-device wake-word detector and, on a detection,
// drives the same capture -> ProcessBatch path as push-to-talk. It returns a
// stop func. It is a no-op unless AI mode and the wake-word trigger are active.
func startWakeWord(ctx context.Context, logger pttLogger, machine *assistant.Machine, cfg config.Config, router *audio.CaptureRouter, pipeline *assistant.VoicePipeline, gov *power.Governor, assets wakeAssets, sampleRate, channels int, prankActive *atomic.Bool) func() {
	if ctx == nil || logger == nil || machine == nil || router == nil || pipeline == nil {
		return func() {}
	}
	if !cfg.UsesAI() || !cfg.WakeWordEnabled || cfg.InputTrigger != config.InputTriggerWakeWord {
		return func() {}
	}

	detector, err := wakeword.New(wakeword.Config{
		LibraryPath: assets.ORTLib,
		MelModel:    assets.MelModel,
		EmbModel:    assets.EmbModel,
		WakeModel:   assets.WakeModel,
		Threads:     2,
	})
	if err != nil {
		logger.Warnf("wake word disabled: %v", err)
		return func() {}
	}

	sub, cancel := router.Subscribe()
	bytesPerSec := audio.BytesPerSecond(sampleRate, channels, audio.BytesPerSampleS16LE)
	buffer := input.NewBuffer(bytesPerSec * 15)
	wc := &wakeController{
		guardTail:       wakeGuardTail,
		continuedWindow: continuedWindowFor(cfg.ContinuedConversation),
		maxFollowUps:    wakeMaxFollowUp,
	}

	logger.Infof("wake word ready: continued=%s", cfg.ContinuedConversation)

	loop := &wakeLoop{
		logger:      logger,
		machine:     machine,
		pipeline:    pipeline,
		gov:         gov,
		detector:    detector,
		buffer:      buffer,
		wc:          wc,
		bytesPerSec: bytesPerSec,
		endSilence:  wakeEndSilenceFor(cfg.WakeEndSilence),
		prankActive: prankActive,
	}
	done := make(chan struct{})
	go func() {
		loop.run(ctx, sub)
		close(done)
	}()

	return func() {
		cancel()
		<-done
		_ = detector.Close()
	}
}

// wakeLoop holds the capture state machine that consumes capture batches and
// manages detection, the listening window, and continued-conversation
// follow-ups. Splitting it into methods keeps each unit simple.
type wakeLoop struct {
	logger      pttLogger
	machine     *assistant.Machine
	pipeline    *assistant.VoicePipeline
	gov         *power.Governor
	detector    *wakeword.Detector
	buffer      *input.Buffer
	wc          *wakeController
	bytesPerSec int
	endSilence  time.Duration

	// prankActive, when set and true, makes the loop stand down entirely so the
	// Evil BMO prank flow owns the mic and the loop never grabs the overheard
	// reply nor self-triggers. Nil outside the prank build.
	prankActive *atomic.Bool

	capturing    bool
	captureStart time.Time
	silenceRun   time.Duration
	// captureIsFollowUp distinguishes a continued-conversation follow-up capture
	// from the initial post-wake command, so only follow-up turns spend the
	// follow-up budget when they prove productive.
	captureIsFollowUp bool
}

func (l *wakeLoop) run(ctx context.Context, sub <-chan []byte) {
	defer l.machine.SetWakeEngaged(false)
	for {
		select {
		case <-ctx.Done():
			return
		case batch, ok := <-sub:
			if !ok {
				return
			}
			l.handleBatch(ctx, batch)
		}
	}
}

func (l *wakeLoop) beginCapture(now time.Time, followUp bool) {
	l.captureIsFollowUp = followUp
	l.wc.startSession()
	l.machine.SetWakeEngaged(l.wc.Engaged())
	l.machine.Transition(assistant.EventListen)
	l.buffer.Begin()
	l.capturing = true
	l.captureStart = now
	l.silenceRun = 0
	l.detector.Reset()
}

func (l *wakeLoop) handleBatch(ctx context.Context, batch []byte) {
	if l.suppressed() {
		l.detector.Reset()
		return
	}
	now := time.Now()
	l.wc.observeState(l.machine.State() == assistant.StateSpeaking, now)

	if l.capturing {
		l.continueCapture(ctx, batch, now)
		return
	}
	// A follow-up window can open capture without a fresh wake word. The turn is
	// only counted later, in finishCapture, if it proves productive.
	if l.wc.windowOpen(now) {
		l.beginCapture(now, true)
		return
	}
	// Only detect while fully idle and not gated by playback / guard tail.
	if l.machine.State() != assistant.StateIdle || !l.wc.onDetection(now) {
		l.detector.Reset()
		return
	}
	for _, det := range l.detector.Write(batch) {
		l.logger.Infof("wake word detected: score=%.2f", det.Score)
		l.beginCapture(now, false)
		break
	}
}

// continueCapture appends a batch to the current utterance and finishes it once
// trailing silence or the hard duration cap is reached.
func (l *wakeLoop) continueCapture(ctx context.Context, batch []byte, now time.Time) {
	l.buffer.Append(batch)
	if audio.PCMHasSignal(batch, wakeVADLevel) {
		l.silenceRun = 0
	} else {
		l.silenceRun += time.Duration(float64(len(batch)) / float64(l.bytesPerSec) * float64(time.Second))
	}
	if !l.captureShouldFinish(now) {
		return
	}
	l.finishCapture(ctx)
}

// suppressed reports whether an Evil BMO prank currently owns the mic, in which
// case the wake loop ignores all batches.
func (l *wakeLoop) suppressed() bool {
	return l.prankActive != nil && l.prankActive.Load()
}

// captureShouldFinish reports whether the current capture is over: either a
// trailing silence of at least the configured end-of-turn duration, or the hard
// max-capture cap.
func (l *wakeLoop) captureShouldFinish(now time.Time) bool {
	return l.silenceRun >= l.endSilence || now.Sub(l.captureStart) >= wakeMaxCapture
}

// finishCapture processes the captured utterance and either opens a follow-up
// or returns to idle.
func (l *wakeLoop) finishCapture(ctx context.Context) {
	l.capturing = false
	if l.machine.State() == assistant.StateListening {
		l.machine.Transition(assistant.EventRest)
	}
	payload := l.buffer.End()
	wasFollowUp := l.captureIsFollowUp
	productive := processWakeUtterance(ctx, l.logger, l.pipeline, l.gov, payload)
	l.detector.Reset()
	if productive {
		// Only a real reply (re)anchors the window and, for follow-ups, spends a
		// turn. Silent captures during the other party's processing gaps fall
		// through untouched, so the window stays anchored to the last real reply.
		l.wc.noteReply(time.Now(), wasFollowUp)
	}
	if l.wc.windowOpen(time.Now()) {
		l.logger.Debugf("continued conversation: follow-up window open")
		l.beginCapture(time.Now(), true)
		return
	}
	l.wc.resetFollowUps()
	l.machine.SetWakeEngaged(l.wc.Engaged())
}

// processWakeUtterance runs the voice pipeline for a captured utterance with the
// performance governor held for the burst. Near-silent payloads are skipped. It
// reports whether the utterance was productive (carried speech and was handed to
// the pipeline) so the caller can spend the follow-up budget only on real turns.
func processWakeUtterance(ctx context.Context, logger pttLogger, pipeline *assistant.VoicePipeline, gov *power.Governor, payload []byte) bool {
	if !audio.PCMHasSignal(payload, wakeVADLevel) {
		logger.Debugf("wake utterance dropped: no signal (%d bytes)", len(payload))
		return false
	}
	if gov != nil {
		_ = gov.Request()
		defer func() { _ = gov.Restore() }()
	}
	if err := pipeline.ProcessBatch(ctx, payload); err != nil {
		logger.Warnf("voice pipeline error: %v", err)
	}
	return true
}
