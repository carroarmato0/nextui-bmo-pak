package assistant

import (
	"testing"
	"time"
)

func TestIdleSchedulerRespectsAvailableFaces(t *testing.T) {
	s := NewIdleScheduler(1)
	// A sparse self-contained mod that ships only these distinct faces.
	s.SetAvailable(map[Expression]bool{
		ExpressionLookAround: true,
		ExpressionSkeptical:  true,
		ExpressionAngry:      true,
		ExpressionUnamused:   true,
	})
	allowed := map[Expression]bool{
		ExpressionNeutral:    true, // always permitted as the resting fallback
		ExpressionLookAround: true,
		ExpressionSkeptical:  true,
		ExpressionAngry:      true,
		ExpressionUnamused:   true,
	}
	// Sweep idleFor across every pool tier; never pick a face the mod lacks
	// (which would silently fold to neutral and look static).
	for i := 0; i < 1000; i++ {
		step := s.Next(time.Duration(i) * 60 * time.Millisecond)
		if !allowed[step.Expression] {
			t.Fatalf("idle picked %q which the mod does not provide", step.Expression)
		}
	}
}

func TestIdleSchedulerUnfilteredByDefault(t *testing.T) {
	s := NewIdleScheduler(7) // no SetAvailable
	seen := map[Expression]struct{}{}
	for i := 0; i < 2000; i++ {
		seen[s.Next(20*time.Second).Expression] = struct{}{}
	}
	// Without a filter, expressive faces still appear (current behavior).
	if _, ok := seen[ExpressionSmile]; !ok {
		t.Fatalf("unfiltered scheduler should still pick expressive faces; saw %#v", seen)
	}
}

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

	for _, want := range []Expression{ExpressionBlink, ExpressionLookAround, ExpressionSmile, ExpressionWhistle, ExpressionContent, ExpressionSleeping} {
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
	if counts[ExpressionBlink] <= counts[ExpressionContent] {
		t.Fatalf("blink should be more frequent than content: %+v", counts)
	}
}

func TestIdleSchedulerLargeActionsAreRare(t *testing.T) {
	s := NewIdleScheduler(19)
	counts := map[Expression]int{}

	for i := 0; i < 120; i++ {
		step := s.Next(30 * time.Second)
		counts[step.Expression]++
	}

	large := counts[ExpressionContent] + counts[ExpressionSleeping]
	if large == 0 {
		t.Fatalf("expected some large actions, got %+v", counts)
	}
	if counts[ExpressionBlink] <= large {
		t.Fatalf("large actions should be rarer than blink: %+v", counts)
	}
}

func TestIdlePoolsIncludeCoreEmotions(t *testing.T) {
	s := NewIdleScheduler(1)
	seen := map[Expression]bool{}
	for i := 0; i < 4000; i++ {
		// Sample across the time bands by advancing idleFor.
		step := s.Next(time.Duration(i%40) * time.Second)
		seen[step.Expression] = true
	}
	for _, want := range []Expression{
		ExpressionContent, ExpressionExcited, ExpressionConcerned, ExpressionHappy,
	} {
		if !seen[want] {
			t.Errorf("idle pools never produced %q", want)
		}
	}
	if seen["laugh"] {
		t.Error("laugh must no longer appear in idle pools")
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
