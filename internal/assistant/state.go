package assistant

import (
	"strings"
	"sync"
	"time"
)

type State string

type Event string

type Expression string

type SleepReason string

type Mode string

type QuotaStatus struct {
	Remaining int
	Exhausted bool
}

type Snapshot struct {
	Mode            string
	Current         State
	Expression      Expression
	Emotion         Expression
	LastInteraction time.Time
	SleepReason     SleepReason
	Quota           QuotaStatus
	IdleSeed        int64
}

const (
	StateIdle      State = "idle"
	StateListening State = "listening"
	StateThinking  State = "thinking"
	StateSpeaking  State = "speaking"
	StateSleeping  State = "sleeping"
	StateError     State = "error"

	EventListen          Event = "listen"
	EventThink           Event = "think"
	EventRemark          Event = "remark"
	EventSpeak           Event = "speak"
	EventRest            Event = "rest"
	EventWake            Event = "wake"
	EventFail            Event = "fail"
	EventRecover         Event = "recover"
	EventQuotaExhausted  Event = "quota_exhausted"
	EventProviderFailure Event = "provider_failure"

	ExpressionNeutral    Expression = "neutral"
	ExpressionIdle       Expression = ExpressionNeutral
	ExpressionBlink      Expression = "blink"
	ExpressionListening  Expression = "listening"
	ExpressionThinking   Expression = "thinking"
	ExpressionSpeaking   Expression = "speaking"
	ExpressionSleeping   Expression = "sleeping"
	ExpressionConcerned  Expression = "concerned"
	ExpressionLookAround Expression = "look_around"
	ExpressionSmile      Expression = "smile"
	ExpressionWhistle    Expression = "whistle"

	// Emotional expressions backed by the Figma face set. Each resolves to its
	// own asset via face.Canonical (see face.EmotionNames).
	ExpressionSad       Expression = "sad"
	ExpressionHappy     Expression = "happy"
	ExpressionContent   Expression = "content"
	ExpressionAngry     Expression = "angry"
	ExpressionSurprised Expression = "surprised"
	ExpressionExcited   Expression = "excited"
	ExpressionLove      Expression = "love"
	ExpressionShy       Expression = "shy"
	ExpressionCrying    Expression = "crying"
	ExpressionTeary     Expression = "teary"
	ExpressionGloomy    Expression = "gloomy"
	ExpressionDizzy     Expression = "dizzy"
	ExpressionUnamused  Expression = "unamused"
	ExpressionAnnoyed   Expression = "annoyed"
	ExpressionSkeptical Expression = "skeptical"
	ExpressionPlayful   Expression = "playful"
	ExpressionKiss      Expression = "kiss"
	ExpressionGrimace   Expression = "grimace"
	ExpressionShout     Expression = "shout"
	ExpressionDead      Expression = "dead"
	ExpressionGlitch    Expression = "glitch"
	ExpressionDismayed  Expression = "dismayed"
	ExpressionAdoring   Expression = "adoring"
	ExpressionSparkle   Expression = "sparkle"
)

const (
	SleepReasonNone            SleepReason = "none"
	SleepReasonUserRest        SleepReason = "user_rest"
	SleepReasonQuotaExhausted  SleepReason = "quota_exhausted"
	SleepReasonProviderFailure SleepReason = "provider_failure"
)

const (
	ModeIdle Mode = "idle"
	ModeAI   Mode = "ai"
)

type Machine struct {
	mu              sync.RWMutex
	mode            Mode
	state           State
	expression      Expression
	emotion         Expression
	lastInteraction time.Time
	sleepReason     SleepReason
	quota           QuotaStatus
	idleSeed        int64
}

func NewMachine() *Machine {
	return &Machine{
		mode:        ModeIdle,
		state:       StateIdle,
		expression:  ExpressionNeutral,
		sleepReason: SleepReasonNone,
	}
}

func (m *Machine) SetMode(mode string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mode = Mode(strings.ToLower(strings.TrimSpace(mode)))
	if m.mode == "" {
		m.mode = ModeIdle
	}
}

func (m *Machine) SetIdleSeed(seed int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.idleSeed = seed
}

func (m *Machine) SetQuotaRemaining(remaining int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.quota.Remaining = remaining
	m.quota.Exhausted = remaining <= 0
}

func (m *Machine) SetExpression(expression Expression) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.expression = expression
}

// SetEmotion records the LLM-directed facial emotion for the utterance about to
// be spoken. It is shown during the Speaking state and cleared by any non-speak
// transition (see applyTransitionEffects), so it never leaks into a later turn.
func (m *Machine) SetEmotion(emotion Expression) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.emotion = emotion
}

func (m *Machine) RecordInteraction(at time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastInteraction = at.UTC()
}

func (m *Machine) State() State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state
}

// AIEnabled reports whether the assistant is in AI mode. Outside AI mode the
// voice pipeline must not run and no provider/API traffic may happen.
func (m *Machine) AIEnabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.mode == ModeAI
}

func (m *Machine) Snapshot() Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return Snapshot{
		Mode:            string(m.mode),
		Current:         m.state,
		Expression:      m.expression,
		Emotion:         m.emotion,
		LastInteraction: m.lastInteraction,
		SleepReason:     m.sleepReason,
		Quota:           m.quota,
		IdleSeed:        m.idleSeed,
	}
}

func (m *Machine) Transition(event Event) State {
	return m.TransitionAt(event, time.Now().UTC())
}

func (m *Machine) TransitionAt(event Event, at time.Time) State {
	m.mu.Lock()
	defer m.mu.Unlock()

	current, next := transitionState(m.state, event)
	m.state = next
	m.lastInteraction = at.UTC()
	applyTransitionEffects(m, current, next, event)
	return m.state
}

func Transition(current State, event Event) State {
	_, next := transitionState(current, event)
	return next
}

func transitionState(current State, event Event) (State, State) {
	switch event {
	case EventQuotaExhausted:
		return current, StateSleeping
	case EventProviderFailure, EventFail:
		return current, StateError
	case EventRecover:
		if current == StateError {
			return current, StateIdle
		}
	case EventWake:
		if current == StateSleeping || current == StateError {
			return current, StateIdle
		}
	case EventListen:
		if current == StateIdle {
			return current, StateListening
		}
	case EventThink:
		if current == StateListening {
			return current, StateThinking
		}
	case EventRemark:
		// A proactive remark thinks straight from idle (no user speech to
		// listen to). Deliberately illegal from listening so a remark can
		// never hijack a PTT conversation that started a moment earlier.
		if current == StateIdle {
			return current, StateThinking
		}
	case EventSpeak:
		if current == StateThinking {
			return current, StateSpeaking
		}
	case EventRest:
		switch current {
		case StateSpeaking, StateListening, StateThinking:
			return current, StateIdle
		case StateIdle:
			return current, StateSleeping
		}
	}
	return current, current
}

func applyTransitionEffects(m *Machine, current State, next State, event Event) {
	switch event {
	case EventListen:
		m.expression = ExpressionListening
		m.sleepReason = SleepReasonNone
	case EventThink:
		m.expression = ExpressionThinking
	case EventRemark:
		if next == StateThinking {
			m.expression = ExpressionThinking
		}
	case EventSpeak:
		m.expression = ExpressionSpeaking
	case EventRest:
		m.expression = ExpressionNeutral
		m.sleepReason = SleepReasonNone
		if current != StateSpeaking {
			m.expression = ExpressionSleeping
			m.sleepReason = SleepReasonUserRest
		}
	case EventWake:
		m.expression = ExpressionNeutral
		m.sleepReason = SleepReasonNone
		m.quota.Exhausted = false // user is explicitly retrying; let the pipeline decide
	case EventQuotaExhausted:
		m.expression = ExpressionSleeping
		m.sleepReason = SleepReasonQuotaExhausted
		m.quota.Remaining = 0
		m.quota.Exhausted = true
	case EventProviderFailure, EventFail:
		m.expression = ExpressionConcerned
		m.sleepReason = SleepReasonProviderFailure
	case EventRecover:
		m.expression = ExpressionNeutral
		m.sleepReason = SleepReasonNone
	}

	if next == StateIdle && event == EventRest {
		m.expression = ExpressionNeutral
		m.sleepReason = SleepReasonNone
	}

	if next == StateIdle && event == EventWake {
		m.expression = ExpressionNeutral
		m.sleepReason = SleepReasonNone
	}

	// A directed emotion only applies to the utterance being spoken. Any
	// transition other than the one that starts speech clears it.
	if event != EventSpeak {
		m.emotion = ""
	}
}
