package main

import (
	"context"
	"encoding/binary"
	"errors"
	"math/rand"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/carroarmato0/nextui-bmo/internal/audio"
)

// fakeVoice records the ordered sequence of pipeline calls the prank makes and
// returns scripted values.
type fakeVoice struct {
	taunt         string
	tauntErr      error
	transcript    string
	transcribeErr error
	calls         []string
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
	return f.transcript, f.transcribeErr
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
		pause:       func(context.Context) {},
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

func firstRemark(calls []string) string {
	for _, c := range calls {
		if strings.HasPrefix(c, "remark:") {
			return c
		}
	}
	return ""
}

// isWakePhrase reports whether s is one of the spoken wake phrases, so tests do
// not pin the exact (phonetic) wording.
func isWakePhrase(s string) bool {
	for _, w := range evilWakePhrases {
		if s == w {
			return true
		}
	}
	return false
}

func TestPrankNoReplyOnFirstListen(t *testing.T) {
	v := &fakeVoice{taunt: "ha, nice try"}
	s := newTestSession(v, 3, scriptedListen(nil))
	s.run(context.Background())

	if v.calls[0] != "generate" {
		t.Fatalf("first call = %q, want generate", v.calls[0])
	}
	// Wake call and taunt are spoken as two separate utterances (controlled pause).
	if w, ok := strings.CutPrefix(v.calls[1], "verbatim:"); !ok || !isWakePhrase(w) {
		t.Fatalf("second call = %q, want a standalone wake phrase", v.calls[1])
	}
	if v.calls[2] != "verbatim:ha, nice try" {
		t.Fatalf("third call = %q, want the taunt as its own utterance", v.calls[2])
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

func TestPrankSplitsWakeAndTaunt(t *testing.T) {
	v := &fakeVoice{taunt: "you call that a backlog?"}
	s := newTestSession(v, 2, scriptedListen(nil))
	s.run(context.Background())

	var verbatims []string
	for _, c := range v.calls {
		if after, ok := strings.CutPrefix(c, "verbatim:"); ok {
			verbatims = append(verbatims, after)
		}
	}
	if len(verbatims) != 2 {
		t.Fatalf("want exactly 2 verbatim utterances (wake, taunt), got %v", verbatims)
	}
	if !isWakePhrase(verbatims[0]) {
		t.Fatalf("first utterance = %q, want a bare wake phrase (no taunt fused in)", verbatims[0])
	}
	if verbatims[1] != "you call that a backlog?" {
		t.Fatalf("second utterance = %q, want the taunt alone", verbatims[1])
	}
}

func TestPrankPauseRunsBetweenWakeAndTaunt(t *testing.T) {
	v := &fakeVoice{taunt: "t"}
	s := newTestSession(v, 2, scriptedListen(nil))
	order := []string{}
	s.pause = func(context.Context) { order = append(order, "pause") }
	// Wrap voice to record verbatim ordering relative to the pause.
	s.voice = &orderingVoice{fakeVoice: v, order: &order}
	s.run(context.Background())

	// Expect: wake utterance, then pause, then taunt utterance.
	want := []string{"wake", "pause", "taunt"}
	if len(order) < 3 || order[0] != want[0] || order[1] != want[1] || order[2] != want[2] {
		t.Fatalf("ordering = %v, want wake -> pause -> taunt", order)
	}
}

// orderingVoice records the order of the two verbatim calls relative to pause.
type orderingVoice struct {
	*fakeVoice
	order *[]string
	spoke bool
}

func (o *orderingVoice) SpeakVerbatim(ctx context.Context, text string) error {
	if !o.spoke {
		*o.order = append(*o.order, "wake")
		o.spoke = true
	} else {
		*o.order = append(*o.order, "taunt")
	}
	return o.fakeVoice.SpeakVerbatim(ctx, text)
}

func TestPrankGenericComebackWhenTranscribeFails(t *testing.T) {
	// Heard a reply but STT failed: Evil must still follow up (generic comeback),
	// not claim nobody answered.
	v := &fakeVoice{taunt: "t", transcribeErr: errors.New("context deadline exceeded")}
	s := newTestSession(v, 3, scriptedListen([]byte{1}, nil))
	s.run(context.Background())

	first := firstRemark(v.calls)
	if !strings.Contains(first, genericComebackNudge) {
		t.Fatalf("expected generic comeback after transcribe failure, got %q", first)
	}
	if strings.Contains(first, noReplyNudge) {
		t.Fatal("must NOT use the no-reply line when a reply was heard")
	}
}

func TestPrankGenericComebackWhenTranscriptEmpty(t *testing.T) {
	// Heard audio but the transcript came back blank: still a generic comeback.
	v := &fakeVoice{taunt: "t", transcript: "   "}
	s := newTestSession(v, 3, scriptedListen([]byte{1}, nil))
	s.run(context.Background())
	if !strings.Contains(firstRemark(v.calls), genericComebackNudge) {
		t.Fatalf("expected generic comeback for blank transcript, got %q", firstRemark(v.calls))
	}
}

func TestPrankGenericCloserOnFinalRoundWhenTranscribeFails(t *testing.T) {
	v := &fakeVoice{taunt: "t", transcribeErr: errors.New("boom")}
	s := newTestSession(v, 1, scriptedListen([]byte{1}))
	s.run(context.Background())
	last := lastRemark(v.calls)
	if !strings.Contains(last, genericCloserNudge) || !strings.Contains(last, closerNudgeMarker) {
		t.Fatalf("expected generic closer on final round, got %q", last)
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

func TestStripLeadingWakeAddress(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Hey, BMO, how does it feel to be second best?", "How does it feel to be second best?"},
		{"Hey BMO... Why are you always on standby?", "Why are you always on standby?"},
		{"Hi Beemo! Enjoying your sad little library?", "Enjoying your sad little library?"},
		{"Hello, B.M.O. - still buffering?", "Still buffering?"},
		// No wake address at the front: left untouched.
		{"Do you ever wish you were a toaster?", "Do you ever wish you were a toaster?"},
		// A greeting WITHOUT the name does not re-trigger the wake word: keep it.
		{"Hey, slow much?", "Hey, slow much?"},
		// Degenerate: stripping would empty it, so the original is kept.
		{"Hey BMO", "Hey BMO"},
	}
	for _, c := range cases {
		if got := stripLeadingWakeAddress(c.in); got != c.want {
			t.Errorf("stripLeadingWakeAddress(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestPrankStripsWakeAddressFromSpokenTaunt is the regression for the acoustic
// double-"Hey BMO" conflict seen on hardware (2026-06-21): the taunt LLM opened
// the barb with its own "Hey, BMO,", which spoken aloud was a second wake
// utterance landing in the victim's open listen window. run must speak a taunt
// that no longer re-addresses the wake word.
func TestPrankStripsWakeAddressFromSpokenTaunt(t *testing.T) {
	v := &fakeVoice{taunt: "Hey, BMO, how does it feel to be the second best version of yourself?"}
	s := newTestSession(v, 2, scriptedListen(nil))
	s.run(context.Background())

	var taunt string
	for _, c := range v.calls {
		if after, ok := strings.CutPrefix(c, "verbatim:"); ok && !isWakePhrase(after) {
			taunt = after
		}
	}
	if taunt == "" {
		t.Fatal("no taunt utterance recorded")
	}
	low := strings.ToLower(taunt)
	if strings.HasPrefix(low, "hey") || strings.Contains(low, "bmo") {
		t.Fatalf("spoken taunt still re-addresses the wake word: %q", taunt)
	}
	if taunt != "How does it feel to be the second best version of yourself?" {
		t.Fatalf("spoken taunt = %q, want the barb with the leading address stripped", taunt)
	}
}

// TestPrankReplyBudgetsArePatient guards the latency budget for a captured
// reply. Observed on hardware (2026-06-20): the victim's reply was heard well
// within the onset window, but Evil BMO's STT call timed out at 10s on every
// reply ("context deadline exceeded") because a captured reply is up to
// wakeMaxCapture of audio and a slow/LAN STT backend needs more than the
// clip's own duration to transcribe it. The transcribe budget must therefore
// comfortably exceed the maximum reply length, and the onset window must stay
// at least as long, so Evil BMO is patient enough to both hear and process a
// reply regardless of the user's continued_conversation setting.
func TestPrankReplyBudgetsArePatient(t *testing.T) {
	if prankTranscribeTimeout < wakeMaxCapture+10*time.Second {
		t.Fatalf("prankTranscribeTimeout (%v) must be >= wakeMaxCapture (%v) + 10s headroom: a reply clip cannot transcribe in less time than its own duration on a slow backend", prankTranscribeTimeout, wakeMaxCapture)
	}
	if prankListenWindow < wakeMaxCapture {
		t.Fatalf("prankListenWindow (%v) must be >= wakeMaxCapture (%v): the wait for a reply to begin should cover at least one full utterance", prankListenWindow, wakeMaxCapture)
	}
}

// framesSource is a scripted audio.PCMSource that emits the given frames then
// closes, so listenForReply can be driven deterministically.
type framesSource struct{ frames [][]byte }

func (s *framesSource) Start() error          { return nil }
func (s *framesSource) Close() error          { return nil }
func (s *framesSource) WritePCM([]byte) error { return nil }
func (s *framesSource) Frames() <-chan []byte {
	ch := make(chan []byte, len(s.frames))
	for _, f := range s.frames {
		ch <- f
	}
	close(ch)
	return ch
}

func signalFrame(n int) []byte { //nolint:unparam // helper parameterized for clarity; tests pass a constant frame size
	b := make([]byte, n)
	for i := 0; i+1 < n; i += 2 {
		binary.LittleEndian.PutUint16(b[i:i+2], uint16(int16(8192)))
	}
	return b
}
func silenceFrame(n int) []byte { return make([]byte, n) } //nolint:unparam // helper parameterized for clarity; tests pass a constant frame size

func TestListenForReplyCapturesThenEndsOnSilence(t *testing.T) {
	const frame = 100 // bytes per frame
	const bps = 1000  // bytes/sec -> each frame = 0.1s
	src := &framesSource{frames: [][]byte{
		silenceFrame(frame), // pre-onset quiet (ignored)
		signalFrame(frame),  // onset
		signalFrame(frame),  // speech
		silenceFrame(frame), // trailing silence (0.1s)
		silenceFrame(frame), // trailing silence (0.2s) -> finishes
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
	<-start                         // first prank is now running
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

func TestListenForReplyReturnsNilWhenOnsetWindowElapses(t *testing.T) {
	const frame = 100
	// Only silence frames; with a sub-microsecond onset window the first batch
	// arrives after the deadline, exercising the onset-timeout return rather than
	// the channel-closed path the other silent test covers.
	src := &framesSource{frames: [][]byte{silenceFrame(frame), silenceFrame(frame), silenceFrame(frame)}}
	router := audio.NewCaptureRouter(src, frame)
	if err := router.Start(); err != nil {
		t.Fatalf("router start: %v", err)
	}
	defer router.Close()

	pcm := listenForReply(context.Background(), router, 1000, time.Nanosecond, 200*time.Millisecond, 10*time.Second, 0.01)
	if pcm != nil {
		t.Fatalf("expected nil when onset window elapses, got %d bytes", len(pcm))
	}
}

func TestPrankSkipsRoundWhenContextAlreadyCancelled(t *testing.T) {
	// A context cancelled before the first round must bail at the top-of-loop
	// guard: the taunt is spoken, but no listen/transcribe/remark happens.
	v := &fakeVoice{taunt: "t", transcript: "nope"}
	s := newTestSession(v, 3, scriptedListen([]byte{1}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.run(ctx)
	for _, c := range v.calls {
		if c == "transcribe" || strings.HasPrefix(c, "remark:") {
			t.Fatalf("pre-cancelled run must not listen or remark, got calls=%v", v.calls)
		}
	}
}

func TestPrankAbortDuringListenSkipsClosingLine(t *testing.T) {
	// If the context is cancelled while listening (B press / shutdown), the
	// post-listen guard must prevent any closing remark even though audio was
	// captured and transcribed.
	v := &fakeVoice{taunt: "t", transcript: "heard something"}
	ctx, cancel := context.WithCancel(context.Background())
	s := newTestSession(v, 3, func(context.Context) []byte {
		cancel()         // context is cancelled during the listen step
		return []byte{1} // ...and we did capture audio
	})
	s.run(ctx)
	for _, c := range v.calls {
		if strings.HasPrefix(c, "remark:") {
			t.Fatalf("abort-during-listen must not emit a remark, got calls=%v", v.calls)
		}
	}
}
