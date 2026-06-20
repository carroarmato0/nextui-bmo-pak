package main

import (
	"context"
	"encoding/binary"
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
