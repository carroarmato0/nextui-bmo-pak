package assistant

import (
	"testing"
	"time"
)

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
