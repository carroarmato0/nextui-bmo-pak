package assistant

import (
	"testing"
	"time"
)

func TestIdleSchedulerAvoidsImmediateRepetition(t *testing.T) {
	s := NewIdleScheduler(1)

	first := s.Next(12 * time.Second)
	second := s.Next(12 * time.Second)

	if first.Expression == second.Expression {
		t.Fatalf("consecutive expressions repeated: %q", first.Expression)
	}
}

func TestIdleSchedulerProducesVariety(t *testing.T) {
	s := NewIdleScheduler(7)
	seen := map[Expression]struct{}{}

	for i := 0; i < 64; i++ {
		step := s.Next(time.Duration(i+10) * time.Second)
		seen[step.Expression] = struct{}{}
	}

	for _, want := range []Expression{ExpressionBlink, ExpressionLookAround, ExpressionSmile, ExpressionWhistle, ExpressionLaugh, ExpressionSleeping} {
		if _, ok := seen[want]; !ok {
			t.Fatalf("missing expression %q from variety set: %#v", want, seen)
		}
	}
}

func TestIdleSchedulerBlinkIsFrequentAtShortIdleTimes(t *testing.T) {
	s := NewIdleScheduler(11)
	counts := map[Expression]int{}

	for i := 0; i < 50; i++ {
		step := s.Next(1500 * time.Millisecond)
		counts[step.Expression]++
	}

	if counts[ExpressionBlink] < 25 {
		t.Fatalf("blink count too low: %+v", counts)
	}
	if counts[ExpressionBlink] <= counts[ExpressionLaugh] {
		t.Fatalf("blink should be more frequent than laugh: %+v", counts)
	}
}

func TestIdleSchedulerLargeActionsAreRare(t *testing.T) {
	s := NewIdleScheduler(19)
	counts := map[Expression]int{}

	for i := 0; i < 120; i++ {
		step := s.Next(30 * time.Second)
		counts[step.Expression]++
	}

	large := counts[ExpressionLaugh] + counts[ExpressionSleeping]
	if large == 0 {
		t.Fatalf("expected some large actions, got %+v", counts)
	}
	if counts[ExpressionBlink] <= large {
		t.Fatalf("large actions should be rarer than blink: %+v", counts)
	}
}

func TestIdleSchedulerInterruptResetsExpression(t *testing.T) {
	s := NewIdleScheduler(23)
	first := s.Next(20 * time.Second)
	if first.Expression == ExpressionNeutral {
		t.Fatalf("expected a non-neutral idle expression before interrupt")
	}

	stopped := s.Interrupt()
	if stopped.Expression != ExpressionNeutral || stopped.HoldFor != 0 {
		t.Fatalf("Interrupt() = %+v, want neutral immediate reset", stopped)
	}

	after := s.Next(20 * time.Second)
	if after.Expression == first.Expression {
		t.Fatalf("interrupt did not reset last expression: first=%q after=%q", first.Expression, after.Expression)
	}
}
