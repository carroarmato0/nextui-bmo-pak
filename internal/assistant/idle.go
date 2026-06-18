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
	available    map[Expression]bool // nil = every expression allowed
}

func NewIdleScheduler(seed int64) *IdleScheduler {
	return &IdleScheduler{rng: rand.New(rand.NewSource(seed)), last: ExpressionNeutral}
}

// SetAvailable restricts the idle rotation to the given expressions (neutral is
// always permitted as the resting fallback). Pass nil/empty to allow every
// expression — the default, used by the built-in and overlay face sets, which
// resolve all canonical faces. A self-contained mod that ships only a few faces
// uses this so idle cycles its real faces instead of repeatedly folding
// unshipped expressions to neutral (which looks static on screen).
func (s *IdleScheduler) SetAvailable(exprs map[Expression]bool) {
	if len(exprs) == 0 {
		s.available = nil
		return
	}
	s.available = exprs
}

// filterPool drops expressions the active mod does not provide a distinct face
// for. Neutral is always kept; if nothing survives, idle holds on neutral.
func (s *IdleScheduler) filterPool(pool []Expression) []Expression {
	if s.available == nil {
		return pool
	}
	filtered := make([]Expression, 0, len(pool))
	for _, e := range pool {
		if e == ExpressionNeutral || s.available[e] {
			filtered = append(filtered, e)
		}
	}
	if len(filtered) == 0 {
		return []Expression{ExpressionNeutral}
	}
	return filtered
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
	pool = s.filterPool(pool)
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

// idleExpressive is the full set of "bonus" emotion faces BMO can wear while
// idling so the rotation never feels repetitive. It is every canonical emotion
// the model can otherwise request, including the static ones (dead, glitch,
// crying…): while idle these render as a brief held pose, while the time-driven
// idle animations (look_around, whistle) and the amplitude faces sit at their
// rest frame. They are interleaved with the always-present base idle faces in
// poolFor below.
var idleExpressive = []Expression{
	ExpressionSmile, ExpressionHappy, ExpressionContent, ExpressionExcited,
	ExpressionPlayful, ExpressionLove, ExpressionAdoring, ExpressionSparkle,
	ExpressionShy, ExpressionSurprised, ExpressionKiss, ExpressionAngry,
	ExpressionSad, ExpressionGloomy, ExpressionAnnoyed, ExpressionSkeptical,
	ExpressionUnamused, ExpressionDismayed, ExpressionConcerned, ExpressionCrying,
	ExpressionTeary, ExpressionDizzy, ExpressionGrimace, ExpressionShout,
	ExpressionDead, ExpressionGlitch,
}

func (s *IdleScheduler) poolFor(idleFor time.Duration) ([]Expression, time.Duration) {
	switch {
	case idleFor < 2*time.Second:
		// Just after interaction: quick blinks, mostly neutral. Blink hold is
		// overridden to 400ms in Next(); this 1.8s hold is for non-blink steps.
		return []Expression{ExpressionBlink, ExpressionNeutral, ExpressionBlink, ExpressionBlink}, 1800 * time.Millisecond
	case idleFor < 8*time.Second:
		// Settling in: blinks and the signature idle animations are listed
		// several times to stay frequent against the large expressive tail,
		// which is mixed in (each face individually rare) for variety.
		base := []Expression{
			ExpressionBlink, ExpressionBlink, ExpressionBlink, ExpressionBlink,
			ExpressionLookAround, ExpressionLookAround, ExpressionWhistle, ExpressionWhistle,
			ExpressionNeutral, ExpressionNeutral,
		}
		return append(base, idleExpressive...), 3000 * time.Millisecond
	case idleFor < 25*time.Second:
		base := []Expression{
			ExpressionBlink, ExpressionBlink, ExpressionBlink, ExpressionBlink,
			ExpressionLookAround, ExpressionLookAround, ExpressionWhistle, ExpressionWhistle,
			ExpressionNeutral,
		}
		return append(base, idleExpressive...), 4500 * time.Millisecond
	default:
		// Long idle: add dozing off; keep blinks and idle animations weighted so
		// the dramatic expressive faces (and sleeping) stay rarer than a blink.
		base := []Expression{
			ExpressionBlink, ExpressionBlink, ExpressionBlink, ExpressionBlink, ExpressionBlink,
			ExpressionLookAround, ExpressionLookAround, ExpressionWhistle, ExpressionWhistle,
			ExpressionSleeping,
		}
		return append(base, idleExpressive...), 6000 * time.Millisecond
	}
}
