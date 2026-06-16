package assistant

import (
	"math/rand"
	"time"
)

type IdleStep struct {
	Expression Expression
	HoldFor    time.Duration
}

type IdleScheduler struct {
	rng          *rand.Rand
	last         Expression
	blocked      Expression
	interruptLat bool
	cycle        int
}

func NewIdleScheduler(seed int64) *IdleScheduler {
	return &IdleScheduler{rng: rand.New(rand.NewSource(seed)), last: ExpressionNeutral}
}

func (s *IdleScheduler) Interrupt() IdleStep {
	s.blocked = s.last
	s.last = ExpressionNeutral
	s.interruptLat = true
	return IdleStep{Expression: ExpressionNeutral, HoldFor: 0}
}

func (s *IdleScheduler) Next(idleFor time.Duration) IdleStep {
	if s == nil {
		return IdleStep{Expression: ExpressionNeutral, HoldFor: 0}
	}

	pool, hold := s.poolFor(idleFor)
	idx := (s.cycle + s.rng.Intn(len(pool))) % len(pool)
	picked := pool[idx]
	if len(pool) > 1 && picked == s.last {
		picked = pool[(idx+1)%len(pool)]
	}
	if s.interruptLat && picked == s.blocked {
		picked = pool[(idx+1)%len(pool)]
	}
	if s.interruptLat && picked == s.last {
		picked = ExpressionBlink
	}
	s.interruptLat = false
	s.last = picked
	s.cycle++
	// Blinks are always brief regardless of which pool they came from.
	if picked == ExpressionBlink {
		hold = 400 * time.Millisecond
	}
	return IdleStep{Expression: picked, HoldFor: hold}
}

func (s *IdleScheduler) poolFor(idleFor time.Duration) ([]Expression, time.Duration) {
	switch {
	case idleFor < 2*time.Second:
		// Just after interaction: quick blinks, mostly neutral. Blink hold is
		// overridden to 400ms in Next(); this 1.8s hold is for non-blink steps.
		return []Expression{ExpressionBlink, ExpressionNeutral, ExpressionBlink, ExpressionBlink}, 1800 * time.Millisecond
	case idleFor < 8*time.Second:
		return []Expression{ExpressionBlink, ExpressionLookAround, ExpressionNeutral, ExpressionSmile, ExpressionHappy, ExpressionBlink, ExpressionWhistle}, 3000 * time.Millisecond
	case idleFor < 25*time.Second:
		return []Expression{ExpressionBlink, ExpressionLookAround, ExpressionSmile, ExpressionWhistle, ExpressionContent, ExpressionExcited, ExpressionNeutral}, 4500 * time.Millisecond
	default:
		return []Expression{ExpressionBlink, ExpressionLookAround, ExpressionSmile, ExpressionWhistle, ExpressionContent, ExpressionExcited, ExpressionConcerned, ExpressionSleeping, ExpressionBlink, ExpressionBlink}, 6000 * time.Millisecond
	}
}
