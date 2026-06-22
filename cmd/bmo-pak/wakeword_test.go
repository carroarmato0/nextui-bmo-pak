package main

import (
	"context"
	"encoding/binary"
	"sync/atomic"
	"testing"
	"time"

	"github.com/carroarmato0/nextui-bmo/internal/config"
	"github.com/carroarmato0/nextui-bmo/internal/input"
)

// signalPCM returns nBytes of S16LE audio well above wakeVADLevel, standing in
// for a speaker tail / speech batch in capture tests.
func signalPCM(nBytes int) []byte {
	if nBytes%2 != 0 {
		nBytes++
	}
	b := make([]byte, nBytes)
	for i := 0; i+1 < len(b); i += 2 {
		binary.LittleEndian.PutUint16(b[i:i+2], uint16(int16(6000)))
	}
	return b
}

func newTestController() *wakeController {
	return &wakeController{
		now:          time.Now(),
		guardTail:    wakeGuardTail,
		maxFollowUps: wakeMaxFollowUp,
	}
}

func TestWakeIgnoredWhileSpeaking(t *testing.T) {
	c := newTestController()
	c.speaking = true
	if c.onDetection(c.now) {
		t.Fatal("must not trigger while BMO is speaking")
	}
}

func TestWakeIgnoredDuringGuardTail(t *testing.T) {
	c := newTestController()
	c.speaking = false
	c.speechEndedAt = c.now // just stopped speaking
	if c.onDetection(c.now.Add(200 * time.Millisecond)) {
		t.Fatal("must not trigger within guard tail after speech")
	}
	if !c.onDetection(c.now.Add(c.guardTail + time.Millisecond)) {
		t.Fatal("should trigger after guard tail elapses")
	}
}

func TestWakeObserveStateRecordsSpeechEnd(t *testing.T) {
	c := newTestController()
	c.observeState(true, c.now)
	end := c.now.Add(time.Second)
	c.observeState(false, end)
	if c.speaking {
		t.Fatal("should no longer be speaking")
	}
	if !c.speechEndedAt.Equal(end) {
		t.Fatalf("speechEndedAt = %v, want %v", c.speechEndedAt, end)
	}
}

func TestContinuedConversationReopensWindow(t *testing.T) {
	c := newTestController()
	c.continuedWindow = 8 * time.Second
	c.captureFinished(c.now, true, false)
	if !c.windowOpen(c.now.Add(3 * time.Second)) {
		t.Fatal("follow-up window should be open")
	}
	if c.windowOpen(c.now.Add(9 * time.Second)) {
		t.Fatal("window should have expired")
	}
}

func TestContinuedConversationOffNeverOpens(t *testing.T) {
	c := newTestController()
	c.continuedWindow = 0 // off
	c.captureFinished(c.now, true, false)
	if c.windowOpen(c.now.Add(time.Second)) {
		t.Fatal("window must stay closed when continued conversation is off")
	}
}

func TestFollowUpCapBacksOff(t *testing.T) {
	c := newTestController()
	c.continuedWindow = 8 * time.Second
	c.maxFollowUps = 2
	c.captureFinished(c.now, true, false) // initial command reply: free, opens window
	c.captureFinished(c.now, true, true)  // follow-up 1
	c.captureFinished(c.now, true, true)  // follow-up 2 -> hits the cap
	if c.windowOpen(c.now.Add(time.Second)) {
		t.Fatal("window must stay closed after max follow-ups")
	}
}

// TestInitialSilentCommandKeepsListening is the regression for the victim
// ignoring the Evil BMO taunt (hardware 2026-06-21): the taunt lands a beat
// after the wake word, so the victim's first capture is silence. It must keep
// listening (window open) to catch the taunt instead of dropping straight to
// idle and cycling idle-face animations.
func TestInitialSilentCommandKeepsListening(t *testing.T) {
	c := newTestController()
	c.continuedWindow = 20 * time.Second
	if !c.captureFinished(c.now, false, false) {
		t.Fatal("a silent initial command must keep BMO listening for the real utterance")
	}
	if c.followUps != 0 {
		t.Fatalf("a silent initial command must not spend budget, followUps=%d", c.followUps)
	}
	if !c.windowOpen(c.now.Add(5 * time.Second)) {
		t.Fatal("window should be open to catch the delayed utterance")
	}
}

// TestSilentCapturesDoNotSpendFollowUpBudget is the regression for the victim
// dropping out mid-prank (hardware 2026-06-21): the other device is silent for
// several seconds each round while it transcribes/thinks/speaks, and those
// no-signal captures used to each spend a follow-up slot, exhausting the budget
// after only a couple of real exchanges. Only productive follow-ups may spend it.
func TestSilentCapturesDoNotSpendFollowUpBudget(t *testing.T) {
	c := newTestController()
	c.continuedWindow = 20 * time.Second
	c.maxFollowUps = 2

	c.captureFinished(c.now, true, false) // initial command reply: free
	if c.followUps != 0 {
		t.Fatalf("initial reply must not spend budget, followUps=%d", c.followUps)
	}
	// A silent follow-up within the window keeps listening without spending.
	if !c.captureFinished(c.now.Add(5*time.Second), false, true) {
		t.Fatal("silent follow-up within the window should keep listening")
	}
	if c.followUps != 0 {
		t.Fatalf("silence must not spend budget, followUps=%d", c.followUps)
	}
	// A real follow-up spends one unit and re-anchors the window.
	c.captureFinished(c.now.Add(6*time.Second), true, true)
	if c.followUps != 1 {
		t.Fatalf("real follow-up should spend exactly one unit, followUps=%d", c.followUps)
	}
	if !c.windowOpen(c.now.Add(7 * time.Second)) {
		t.Fatal("window should re-anchor to the real follow-up reply")
	}
}

// TestFollowUpWindowExpiresFromLastRealReply confirms silence is bounded by the
// window (time since the last genuine reply), not by burning the turn budget: a
// silent follow-up past the window returns to idle.
func TestFollowUpWindowExpiresFromLastRealReply(t *testing.T) {
	c := newTestController()
	c.continuedWindow = 20 * time.Second
	c.captureFinished(c.now, true, false)
	if !c.windowOpen(c.now.Add(19 * time.Second)) {
		t.Fatal("window should be open up to the continued-window duration")
	}
	if c.captureFinished(c.now.Add(21*time.Second), false, true) {
		t.Fatal("a silent follow-up past the window must return to idle")
	}
}

// TestFollowUpCaptureDrainsSelfEcho is the regression for the victim talking
// over Evil BMO (hardware 2026-06-21): a continued-conversation follow-up
// capture armed the instant BMO stopped speaking and recorded its own decaying
// speaker tail on the shared mic. The low VAD floor kept that self-echo alive
// long enough for Whisper to hallucinate an utterance, so the victim fired a
// premature reply on top of Evil BMO's real one. A follow-up capture must drain
// the self-echo settle window before it records anything.
func TestFollowUpCaptureDrainsSelfEcho(t *testing.T) {
	const bytesPerSec = 32000
	l := &wakeLoop{
		buffer:      input.NewBuffer(bytesPerSec * 15),
		bytesPerSec: bytesPerSec,
		endSilence:  1300 * time.Millisecond,
	}
	start := time.Unix(100, 0)
	l.buffer.Begin()
	l.capturing = true
	l.captureStart = start
	l.settleUntil = start.Add(wakeFollowUpSettle)

	// Self-echo tail arriving within the settle window must be discarded.
	echo := signalPCM(bytesPerSec / 5) // 0.2s of speaker tail
	l.continueCapture(context.Background(), echo, start.Add(200*time.Millisecond))
	if got := l.buffer.End(); len(got) != 0 {
		t.Fatalf("self-echo within settle must be discarded, captured %d bytes", len(got))
	}
}

// TestFollowUpCaptureRecordsAfterSettle confirms the settle only suppresses the
// immediate post-speech tail: once it elapses, the other party's real reply is
// captured normally.
func TestFollowUpCaptureRecordsAfterSettle(t *testing.T) {
	const bytesPerSec = 32000
	l := &wakeLoop{
		buffer:      input.NewBuffer(bytesPerSec * 15),
		bytesPerSec: bytesPerSec,
		endSilence:  1300 * time.Millisecond,
	}
	start := time.Unix(100, 0)
	l.buffer.Begin()
	l.capturing = true
	l.captureStart = start
	l.settleUntil = start.Add(wakeFollowUpSettle)

	speech := signalPCM(bytesPerSec / 5)
	l.continueCapture(context.Background(), speech, start.Add(wakeFollowUpSettle+10*time.Millisecond))
	if got := l.buffer.End(); len(got) != len(speech) {
		t.Fatalf("post-settle speech must be captured, got %d of %d bytes", len(got), len(speech))
	}
}

func TestContinuedWindowForMapsModes(t *testing.T) {
	if continuedWindowFor(config.ContinuedConvoShort) != 8*time.Second {
		t.Fatal("short window wrong")
	}
	if continuedWindowFor(config.ContinuedConvoLong) != 20*time.Second {
		t.Fatal("long window wrong")
	}
	if continuedWindowFor("off") != 0 {
		t.Fatal("off window should be zero")
	}
}

func TestWakeSessionEngagedLifecycle(t *testing.T) {
	c := newTestController()
	if c.Engaged() {
		t.Fatal("fresh controller must not be engaged")
	}
	c.startSession()
	if !c.Engaged() {
		t.Fatal("startSession must engage")
	}
	// A follow-up capture keeps the session engaged (startSession is idempotent;
	// every capture, initial or follow-up, runs it).
	c.startSession()
	if !c.Engaged() {
		t.Fatal("follow-up must remain engaged")
	}
	// Conversation over.
	c.resetFollowUps()
	if c.Engaged() {
		t.Fatal("resetFollowUps must disengage")
	}
}

func TestWakeEndSilenceForMapsTiers(t *testing.T) {
	cases := map[string]time.Duration{
		config.WakeEndSilenceSnappy:   1000 * time.Millisecond,
		config.WakeEndSilenceBalanced: 1300 * time.Millisecond,
		config.WakeEndSilencePatient:  1600 * time.Millisecond,
		"":                            1300 * time.Millisecond, // unknown -> balanced
	}
	for in, want := range cases {
		if got := wakeEndSilenceFor(in); got != want {
			t.Errorf("wakeEndSilenceFor(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestCaptureShouldFinish(t *testing.T) {
	l := &wakeLoop{endSilence: 1300 * time.Millisecond}
	now := time.Unix(100, 0)
	l.captureStart = now

	// Silence below the configured threshold: keep capturing.
	l.silenceRun = 1200 * time.Millisecond
	if l.captureShouldFinish(now) {
		t.Error("should keep capturing below endSilence threshold")
	}
	// Silence at/above the threshold: finish.
	l.silenceRun = 1300 * time.Millisecond
	if !l.captureShouldFinish(now) {
		t.Error("should finish at endSilence threshold")
	}
	// Max-capture cap also finishes regardless of silence.
	l.silenceRun = 0
	if !l.captureShouldFinish(now.Add(wakeMaxCapture)) {
		t.Error("should finish at wakeMaxCapture cap")
	}
}

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

func TestWakeSettingsChanged(t *testing.T) {
	base := config.Config{
		Mode:                  config.ModeAI,
		WakeWordEnabled:       true,
		InputTrigger:          config.InputTriggerWakeWord,
		ContinuedConversation: config.ContinuedConvoShort,
		WakeEndSilence:        config.WakeEndSilenceBalanced,
	}
	if wakeSettingsChanged(base, base) {
		t.Fatal("identical configs must not report a wake-relevant change")
	}

	cases := map[string]func(*config.Config){
		"mode":        func(c *config.Config) { c.Mode = config.ModeIdle },
		"enabled":     func(c *config.Config) { c.WakeWordEnabled = false },
		"trigger":     func(c *config.Config) { c.InputTrigger = config.InputTriggerPTT },
		"continued":   func(c *config.Config) { c.ContinuedConversation = config.ContinuedConvoLong },
		"end_silence": func(c *config.Config) { c.WakeEndSilence = config.WakeEndSilenceSnappy },
	}
	for name, mut := range cases {
		next := base
		mut(&next)
		if !wakeSettingsChanged(base, next) {
			t.Errorf("%s change must report a wake-relevant change", name)
		}
	}

	// A change to an unrelated field must NOT trigger a costly detector rebuild.
	next := base
	next.LogLevel = "debug"
	if wakeSettingsChanged(base, next) {
		t.Error("unrelated field change must not report a wake-relevant change")
	}
}

func TestRestartNotice(t *testing.T) {
	idle := config.Config{Mode: config.ModeIdle}
	ai := config.Config{Mode: config.ModeAI}
	aiWakeOff := config.Config{Mode: config.ModeAI, WakeWordEnabled: false, InputTrigger: config.InputTriggerPTT}
	aiWakeOn := config.Config{Mode: config.ModeAI, WakeWordEnabled: true, InputTrigger: config.InputTriggerWakeWord}

	// A live AI subsystem applies changes in place (mode gating / restartWake):
	// never a restart notice, whatever changed.
	if _, ok := restartNotice(idle, aiWakeOn, true); ok {
		t.Error("a live AI subsystem must not prompt a restart")
	}

	// Subsystem not live: switching into AI mode needs a relaunch.
	if msg, ok := restartNotice(idle, ai, false); !ok || msg != aiRestartMessage {
		t.Errorf("switching to AI with no live subsystem must prompt restart; got (%q, %v)", msg, ok)
	}
	// Leaving AI mode does not (the subsystem stays unused either way).
	if _, ok := restartNotice(ai, idle, false); ok {
		t.Error("leaving AI mode must not prompt a restart")
	}

	// Subsystem not live, already in AI: a wake on/off change needs a relaunch.
	if msg, ok := restartNotice(aiWakeOff, aiWakeOn, false); !ok || msg != wakeRestartMessage {
		t.Errorf("enabling wake with no live subsystem must prompt restart; got (%q, %v)", msg, ok)
	}
	if msg, ok := restartNotice(aiWakeOn, aiWakeOff, false); !ok || msg != wakeRestartMessage {
		t.Errorf("disabling wake with no live subsystem must prompt restart; got (%q, %v)", msg, ok)
	}

	// No relevant change (or staying in idle): no notice.
	if _, ok := restartNotice(aiWakeOn, aiWakeOn, false); ok {
		t.Error("no relevant change must not prompt a restart")
	}
	if _, ok := restartNotice(idle, idle, false); ok {
		t.Error("staying in idle must not prompt a restart")
	}
}
