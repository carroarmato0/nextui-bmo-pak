package assistant

import (
	"testing"
	"time"
)

func proactiveFixture() (*Machine, *ProactiveScheduler, time.Time) {
	m := NewMachine()
	m.SetMode("ai")
	t0 := time.Date(2026, 6, 11, 16, 0, 0, 0, time.UTC)
	m.RecordInteraction(t0.Add(-10 * time.Minute)) // long quiet
	return m, NewProactiveScheduler(m, 1), t0
}

func TestProactiveSchedulerDisabledByDefault(t *testing.T) {
	_, s, t0 := proactiveFixture()
	if s.Due(t0.Add(24 * time.Hour)) {
		t.Fatal("scheduler with no interval must never be due")
	}
}

func TestProactiveSchedulerFiresWithinJitterBounds(t *testing.T) {
	_, s, t0 := proactiveFixture()
	s.SetInterval(10 * time.Minute)
	// First Due() call arms the timer and reports not-due.
	if s.Due(t0) {
		t.Fatal("first tick must arm, not fire")
	}
	// ±40% jitter: never before 6m, always by 14m (plus a tick of slack).
	if s.Due(t0.Add(6*time.Minute - time.Second)) {
		t.Fatal("fired before jitter lower bound")
	}
	if !s.Due(t0.Add(14*time.Minute + time.Second)) {
		t.Fatal("not due after jitter upper bound")
	}
}

func TestProactiveSchedulerRescheduleSpreadsFires(t *testing.T) {
	_, s, t0 := proactiveFixture()
	s.SetInterval(10 * time.Minute)
	s.Due(t0) // arm
	fire1 := t0.Add(14*time.Minute + time.Second)
	if !s.Due(fire1) {
		t.Fatal("expected due at upper bound")
	}
	s.Reschedule(fire1)
	if s.Due(fire1.Add(6*time.Minute - time.Second)) {
		t.Fatal("fired again before lower bound after reschedule")
	}
	if !s.Due(fire1.Add(14*time.Minute + time.Second)) {
		t.Fatal("not due after upper bound after reschedule")
	}
}

func TestProactiveSchedulerGates(t *testing.T) {
	m, s, t0 := proactiveFixture()
	s.SetInterval(10 * time.Minute)
	s.Due(t0) // arm
	due := t0.Add(15 * time.Minute)

	m.SetMode("idle") // AI off
	if s.Due(due) {
		t.Fatal("must not fire outside AI mode")
	}
	m.SetMode("ai")

	// Synthetic timestamps throughout: mixing the real clock into
	// lastInteraction would make the quiet-window assertion below depend
	// on when the test runs.
	m.TransitionAt(EventListen, due.Add(-5*time.Minute)) // mid-conversation
	if s.Due(due) {
		t.Fatal("must not fire while not idle")
	}
	m.TransitionAt(EventRest, due.Add(-time.Second)) // back to idle, but lastInteraction just updated

	if s.Due(due) {
		t.Fatal("must not fire within the 2-minute quiet window")
	}
	m.RecordInteraction(due.Add(-3 * time.Minute))
	if !s.Due(due) {
		t.Fatal("expected due once idle, AI-on, and quiet")
	}
}

func TestProactiveSchedulerSetIntervalZeroDisables(t *testing.T) {
	_, s, t0 := proactiveFixture()
	s.SetInterval(10 * time.Minute)
	s.Due(t0) // arm
	s.SetInterval(0)
	if s.Due(t0.Add(24 * time.Hour)) {
		t.Fatal("interval 0 must disable the scheduler")
	}
}
