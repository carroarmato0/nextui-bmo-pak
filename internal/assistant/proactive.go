package assistant

import (
	"math/rand"
	"sync"
	"time"
)

// proactiveMinQuiet is how long BMO stays silent after any interaction
// before a proactive remark may fire — remarks must feel spontaneous, not
// like he is butting back into a conversation that just ended.
const proactiveMinQuiet = 2 * time.Minute

// ProactiveScheduler decides WHEN BMO may make a spontaneous remark. It is
// pure timing and gating; WHAT to say comes from devctx.Builder's
// ProactiveNudge, and saying it is VoicePipeline.SpeakRemark's job. Each
// fire is rescheduled at the base interval ±40% jitter so remarks never
// feel like clockwork. Safe for concurrent use.
type ProactiveScheduler struct {
	mu       sync.Mutex
	machine  *Machine
	rng      *rand.Rand
	interval time.Duration
	next     time.Time
}

func NewProactiveScheduler(machine *Machine, seed int64) *ProactiveScheduler {
	return &ProactiveScheduler{machine: machine, rng: rand.New(rand.NewSource(seed))}
}

// SetInterval sets the base interval between remarks; 0 disables. The next
// fire time is re-armed lazily on the next Due tick, so changing levels in
// the settings menu takes effect immediately without firing instantly.
func (s *ProactiveScheduler) SetInterval(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.interval == d {
		return
	}
	s.interval = d
	s.next = time.Time{}
}

// Due reports whether a proactive remark should fire now. The first call
// after enabling (or changing) the interval arms the timer and returns
// false. When the timer has elapsed but a gate blocks (not idle, AI off,
// too soon after an interaction), Due keeps returning false and fires as
// soon as the gates clear. Callers must Reschedule after acting on true.
func (s *ProactiveScheduler) Due(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.interval <= 0 {
		return false
	}
	if s.next.IsZero() {
		s.next = now.Add(s.jittered())
		return false
	}
	if now.Before(s.next) {
		return false
	}
	if s.machine == nil || !s.machine.AIEnabled() || s.machine.State() != StateIdle {
		return false
	}
	if now.Sub(s.machine.Snapshot().LastInteraction) < proactiveMinQuiet {
		return false
	}
	return true
}

// Reschedule arms the next fire after a remark was attempted (whether or
// not it produced speech — a failed remark should not retry immediately).
func (s *ProactiveScheduler) Reschedule(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.interval > 0 {
		s.next = now.Add(s.jittered())
	}
}

// jittered returns the base interval ±40%. Caller must hold s.mu.
func (s *ProactiveScheduler) jittered() time.Duration {
	f := 0.6 + 0.8*s.rng.Float64()
	return time.Duration(float64(s.interval) * f)
}
