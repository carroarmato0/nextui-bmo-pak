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
	return IdleStep{Expression: picked, HoldFor: hold}
}

func (s *IdleScheduler) poolFor(idleFor time.Duration) ([]Expression, time.Duration) {
	switch {
	case idleFor < 2*time.Second:
		return []Expression{ExpressionBlink, ExpressionNeutral, ExpressionBlink, ExpressionBlink}, 350 * time.Millisecond
	case idleFor < 8*time.Second:
		return []Expression{ExpressionBlink, ExpressionLookAround, ExpressionNeutral, ExpressionSmile, ExpressionBlink, ExpressionBlink, ExpressionWhistle}, 500 * time.Millisecond
	case idleFor < 25*time.Second:
		return []Expression{ExpressionBlink, ExpressionLookAround, ExpressionSmile, ExpressionWhistle, ExpressionNeutral, ExpressionLaugh}, 900 * time.Millisecond
	default:
		return []Expression{ExpressionBlink, ExpressionLookAround, ExpressionSmile, ExpressionWhistle, ExpressionLaugh, ExpressionSleeping, ExpressionBlink, ExpressionBlink}, 1200 * time.Millisecond
	}
}
