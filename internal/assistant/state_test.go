package assistant

import (
	"testing"
	"time"
)

func TestBeginRemarkOnlyStartsFromIdle(t *testing.T) {
	m := NewMachine()

	// From idle, a remark starts and the machine is thinking.
	if !m.BeginRemark() {
		t.Fatal("BeginRemark from idle should succeed")
	}
	if got := m.Snapshot().Current; got != StateThinking {
		t.Errorf("after BeginRemark, state = %v, want thinking", got)
	}

	// Already thinking: a second remark must be refused (no overlap).
	if m.BeginRemark() {
		t.Error("BeginRemark while thinking should fail")
	}

	// While speaking: still refused.
	m.Transition(EventSpeak)
	if m.BeginRemark() {
		t.Error("BeginRemark while speaking should fail")
	}

	// Back to idle: a remark can start again.
	m.Transition(EventRest)
	if got := m.Snapshot().Current; got != StateIdle {
		t.Fatalf("after EventRest, state = %v, want idle", got)
	}
	if !m.BeginRemark() {
		t.Error("BeginRemark from idle (after rest) should succeed")
	}
}

func TestBeginRemarkIsAtomicUnderConcurrency(t *testing.T) {
	m := NewMachine()
	const n = 50
	results := make(chan bool, n)
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		go func() {
			<-start
			results <- m.BeginRemark()
		}()
	}
	close(start)
	wins := 0
	for i := 0; i < n; i++ {
		if <-results {
			wins++
		}
	}
	if wins != 1 {
		t.Errorf("exactly one concurrent BeginRemark should win, got %d", wins)
	}
}

func TestTransitionFlow(t *testing.T) {
	tests := []struct {
		name    string
		current State
		event   Event
		want    State
	}{
		{name: "idle to listening", current: StateIdle, event: EventListen, want: StateListening},
		{name: "listening to thinking", current: StateListening, event: EventThink, want: StateThinking},
		{name: "thinking to speaking", current: StateThinking, event: EventSpeak, want: StateSpeaking},
		{name: "speaking to idle", current: StateSpeaking, event: EventRest, want: StateIdle},
		{name: "listening to idle", current: StateListening, event: EventRest, want: StateIdle},
		{name: "idle to sleeping", current: StateIdle, event: EventRest, want: StateSleeping},
		{name: "sleeping to idle", current: StateSleeping, event: EventWake, want: StateIdle},
		{name: "quota exhausted to sleeping", current: StateIdle, event: EventQuotaExhausted, want: StateSleeping},
		{name: "provider failure to error", current: StateThinking, event: EventProviderFailure, want: StateError},
		{name: "recover from error", current: StateError, event: EventRecover, want: StateIdle},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Transition(tt.current, tt.event); got != tt.want {
				t.Fatalf("Transition(%q, %q) = %q, want %q", tt.current, tt.event, got, tt.want)
			}
		})
	}
}

func TestMachineAIEnabled(t *testing.T) {
	m := NewMachine()
	if m.AIEnabled() {
		t.Fatal("AIEnabled() = true for default (idle) mode")
	}
	m.SetMode("ai")
	if !m.AIEnabled() {
		t.Fatal("AIEnabled() = false after SetMode(ai)")
	}
	m.SetMode("AI ") // normalization
	if !m.AIEnabled() {
		t.Fatal("AIEnabled() = false for unnormalized 'AI '")
	}
	m.SetMode("idle")
	if m.AIEnabled() {
		t.Fatal("AIEnabled() = true after SetMode(idle)")
	}
}

func TestMachineSnapshotTracksMetadata(t *testing.T) {
	m := NewMachine()
	m.SetMode("ai")
	m.SetIdleSeed(42)
	m.SetQuotaRemaining(17)

	when := time.Unix(123, 456).UTC()
	m.RecordInteraction(when)
	m.SetExpression(ExpressionWhistle)

	snap := m.Snapshot()
	if snap.Mode != "ai" {
		t.Fatalf("Snapshot().Mode = %q, want ai", snap.Mode)
	}
	if snap.IdleSeed != 42 {
		t.Fatalf("Snapshot().IdleSeed = %d, want 42", snap.IdleSeed)
	}
	if snap.Quota.Remaining != 17 || snap.Quota.Exhausted {
		t.Fatalf("Snapshot().Quota = %+v, want remaining 17 and not exhausted", snap.Quota)
	}
	if !snap.LastInteraction.Equal(when) {
		t.Fatalf("Snapshot().LastInteraction = %v, want %v", snap.LastInteraction, when)
	}
	if snap.Expression != ExpressionWhistle {
		t.Fatalf("Snapshot().Expression = %q, want whistle", snap.Expression)
	}
}

func TestMachineTransitionUpdatesFields(t *testing.T) {
	m := NewMachine()
	at := time.Unix(456, 0).UTC()

	if got := m.TransitionAt(EventListen, at); got != StateListening {
		t.Fatalf("TransitionAt(listen) = %q, want listening", got)
	}
	snap := m.Snapshot()
	if snap.Expression != ExpressionListening {
		t.Fatalf("after listen expression = %q, want listening", snap.Expression)
	}
	if !snap.LastInteraction.Equal(at) {
		t.Fatalf("after listen last interaction = %v, want %v", snap.LastInteraction, at)
	}

	at = at.Add(time.Second)
	if got := m.TransitionAt(EventQuotaExhausted, at); got != StateSleeping {
		t.Fatalf("TransitionAt(quota exhausted) = %q, want sleeping", got)
	}
	snap = m.Snapshot()
	if snap.SleepReason != SleepReasonQuotaExhausted {
		t.Fatalf("sleep reason = %q, want %q", snap.SleepReason, SleepReasonQuotaExhausted)
	}
	if !snap.Quota.Exhausted {
		t.Fatalf("quota should be exhausted after quota event: %+v", snap.Quota)
	}
	if snap.Expression != ExpressionSleeping {
		t.Fatalf("sleep expression = %q, want sleeping", snap.Expression)
	}

	if got := m.TransitionAt(EventWake, at.Add(time.Second)); got != StateIdle {
		t.Fatalf("TransitionAt(wake) = %q, want idle", got)
	}
	snap = m.Snapshot()
	if snap.SleepReason != SleepReasonNone {
		t.Fatalf("sleep reason after wake = %q, want none", snap.SleepReason)
	}
	if snap.Expression != ExpressionNeutral {
		t.Fatalf("expression after wake = %q, want neutral", snap.Expression)
	}
}

func TestMachineTransitionPreservesStateOnUnknownEvent(t *testing.T) {
	m := NewMachine()
	if got := m.Transition(Event("unknown")); got != StateIdle {
		t.Fatalf("Transition(unknown) = %q, want idle", got)
	}
	if got := m.Transition(EventRest); got != StateSleeping {
		t.Fatalf("Transition(rest) = %q, want sleeping", got)
	}
	if got := m.Transition(Event("still-unknown")); got != StateSleeping {
		t.Fatalf("unknown event changed state = %q, want sleeping", got)
	}
}

func TestEmotionExpressionConstants(t *testing.T) {
	cases := map[Expression]string{
		ExpressionSad: "sad", ExpressionHappy: "happy", ExpressionContent: "content",
		ExpressionAngry: "angry", ExpressionSurprised: "surprised", ExpressionExcited: "excited",
		ExpressionLove: "love", ExpressionShy: "shy", ExpressionCrying: "crying",
		ExpressionTeary: "teary", ExpressionGloomy: "gloomy", ExpressionDizzy: "dizzy",
		ExpressionUnamused: "unamused", ExpressionAnnoyed: "annoyed", ExpressionSkeptical: "skeptical",
		ExpressionPlayful: "playful", ExpressionKiss: "kiss", ExpressionGrimace: "grimace",
		ExpressionShout: "shout", ExpressionDead: "dead", ExpressionGlitch: "glitch",
		ExpressionDismayed: "dismayed", ExpressionAdoring: "adoring", ExpressionSparkle: "sparkle",
	}
	for expr, want := range cases {
		if string(expr) != want {
			t.Errorf("constant = %q, want %q", string(expr), want)
		}
	}
}

func TestMachineEmotionDirective(t *testing.T) {
	m := NewMachine()
	if got := m.Snapshot().Emotion; got != "" {
		t.Fatalf("default emotion = %q, want empty", got)
	}
	m.SetEmotion(ExpressionExcited)
	if got := m.Snapshot().Emotion; got != ExpressionExcited {
		t.Fatalf("emotion = %q, want excited", got)
	}
}

func TestMachineEmotionPreservedOnSpeak(t *testing.T) {
	m := NewMachine()
	m.SetMode("ai")
	m.Transition(EventListen) // idle -> listening
	m.Transition(EventThink)  // listening -> thinking
	m.SetEmotion(ExpressionLove)
	m.Transition(EventSpeak) // thinking -> speaking; emotion must survive
	if got := m.Snapshot().Emotion; got != ExpressionLove {
		t.Fatalf("emotion after EventSpeak = %q, want love", got)
	}
}

func TestMachineEmotionClearedOnRest(t *testing.T) {
	m := NewMachine()
	m.SetMode("ai")
	m.Transition(EventListen)
	m.Transition(EventThink)
	m.SetEmotion(ExpressionLove)
	m.Transition(EventSpeak)
	m.Transition(EventRest) // speaking -> idle; emotion must clear
	if got := m.Snapshot().Emotion; got != "" {
		t.Fatalf("emotion after EventRest = %q, want empty", got)
	}
}

func TestMachineEmotionClearedOnFail(t *testing.T) {
	m := NewMachine()
	m.SetMode("ai")
	m.Transition(EventListen)
	m.Transition(EventThink)
	m.SetEmotion(ExpressionLove)
	m.Transition(EventProviderFailure) // -> error; non-speak transition clears emotion
	if got := m.Snapshot().Emotion; got != "" {
		t.Fatalf("emotion after EventProviderFailure = %q, want empty", got)
	}
}

func TestRemarkFromIdleForProactiveRemarks(t *testing.T) {
	if got := Transition(StateIdle, EventRemark); got != StateThinking {
		t.Fatalf("Transition(idle, remark) = %q, want thinking", got)
	}
	// A remark must never hijack a conversation that already started: the
	// event is refused from listening (and everywhere else but idle).
	if got := Transition(StateListening, EventRemark); got != StateListening {
		t.Fatalf("Transition(listening, remark) = %q, want listening", got)
	}
	if got := Transition(StateSleeping, EventRemark); got != StateSleeping {
		t.Fatalf("Transition(sleeping, remark) = %q, want sleeping", got)
	}
	// EventThink stays PTT-only.
	if got := Transition(StateIdle, EventThink); got != StateIdle {
		t.Fatalf("Transition(idle, think) = %q, want idle", got)
	}
	// Machine-level: the remark transition drives the thinking expression.
	m := NewMachine()
	m.SetMode("ai")
	if got := m.Transition(EventRemark); got != StateThinking {
		t.Fatalf("machine remark transition = %q, want thinking", got)
	}
	if expr := m.Snapshot().Expression; expr != ExpressionThinking {
		t.Fatalf("expression after remark = %q, want thinking", expr)
	}
	// A refused remark must not corrupt the expression either.
	m2 := NewMachine()
	m2.SetMode("ai")
	m2.Transition(EventListen)
	if got := m2.Transition(EventRemark); got != StateListening {
		t.Fatalf("machine remark from listening = %q, want listening", got)
	}
	if expr := m2.Snapshot().Expression; expr != ExpressionListening {
		t.Fatalf("expression after refused remark = %q, want listening", expr)
	}
}
