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
