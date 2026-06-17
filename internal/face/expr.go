// Package face resolves expression names, rasterizes SVG assets and caches
// ARGB8888 pixel buffers for the SDL renderer. Defaults are embedded in the
// binary; an optional on-disk faces/ directory overrides per file.
package face

import "strings"

const (
	ExprNeutral   = "neutral"
	ExprBlink     = "blink"
	ExprListening = "listening"
	ExprThinking  = "thinking"
	ExprSpeaking  = "speaking"
	ExprSleeping  = "sleeping"
	ExprConcerned = "concerned"
	ExprSmile     = "smile"

	// New expressions (Figma "BMO Face Templates" set).
	ExprSad       = "sad"
	ExprHappy     = "happy"
	ExprContent   = "content"
	ExprAngry     = "angry"
	ExprSurprised = "surprised"
	ExprExcited   = "excited"
	ExprLove      = "love"
	ExprShy       = "shy"
	ExprCrying    = "crying"
	ExprTeary     = "teary"
	ExprGloomy    = "gloomy"
	ExprDizzy     = "dizzy"
	ExprUnamused  = "unamused"
	ExprAnnoyed   = "annoyed"
	ExprSkeptical = "skeptical"
	ExprPlayful   = "playful"
	ExprKiss      = "kiss"
	ExprGrimace   = "grimace"
	ExprShout     = "shout"
	ExprDead      = "dead"
	ExprGlitch    = "glitch"
	ExprDismayed  = "dismayed"
	ExprAdoring   = "adoring"
	ExprSparkle   = "sparkle"

	// Idle-only animated faces. Never requested by the model (see
	// FunctionalNames); driven by the idle scheduler in internal/assistant.
	ExprLookAround = "look_around"
	ExprWhistle    = "whistle"
)

// CanonicalNames lists every canonical expression name in a stable order.
var CanonicalNames = []string{
	ExprNeutral, ExprBlink, ExprListening, ExprThinking,
	ExprSpeaking, ExprSleeping, ExprConcerned, ExprSmile,
	// New expressions.
	ExprSad, ExprHappy, ExprContent, ExprAngry, ExprSurprised,
	ExprExcited, ExprLove, ExprShy, ExprCrying, ExprTeary, ExprGloomy,
	ExprDizzy, ExprUnamused, ExprAnnoyed, ExprSkeptical, ExprPlayful,
	ExprKiss, ExprGrimace, ExprShout, ExprDead, ExprGlitch, ExprDismayed,
	ExprAdoring, ExprSparkle,
	// Idle-only animated faces (functional; excluded from the LLM vocab).
	ExprLookAround, ExprWhistle,
}

// Canonical maps any expression alias the assistant may emit to its canonical
// face name. Unknown inputs fall back to neutral.
func Canonical(expr string) string {
	switch strings.ToLower(strings.TrimSpace(expr)) {
	case "", "idle", ExprNeutral:
		return ExprNeutral
	case ExprBlink:
		return ExprBlink
	case "asleep", "sleep", ExprSleeping:
		return ExprSleeping
	// System states keep their meaning.
	case "error", "confused", ExprConcerned:
		return ExprConcerned
	case ExprListening:
		return ExprListening
	case ExprThinking:
		return ExprThinking
	case ExprSpeaking:
		return ExprSpeaking
	case ExprSmile:
		return ExprSmile
	// Expressions that used to alias onto smile/concerned now resolve to their
	// own assets.
	case ExprHappy:
		return ExprHappy
	case ExprExcited:
		return ExprExcited
	case ExprSad:
		return ExprSad
	case ExprAngry:
		return ExprAngry
	case ExprContent:
		return ExprContent
	case "surprised", "shocked", "surprise":
		return ExprSurprised
	case ExprLove:
		return ExprLove
	case ExprShy:
		return ExprShy
	case "crying", "cry":
		return ExprCrying
	case ExprTeary:
		return ExprTeary
	case ExprGloomy:
		return ExprGloomy
	case ExprDizzy:
		return ExprDizzy
	case ExprUnamused:
		return ExprUnamused
	case ExprAnnoyed:
		return ExprAnnoyed
	case ExprSkeptical:
		return ExprSkeptical
	case "playful", "tongue":
		return ExprPlayful
	case "kiss", "kissing":
		return ExprKiss
	case ExprGrimace:
		return ExprGrimace
	case ExprShout:
		return ExprShout
	case ExprDead:
		return ExprDead
	case ExprGlitch:
		return ExprGlitch
	case ExprDismayed:
		return ExprDismayed
	case ExprAdoring:
		return ExprAdoring
	case "sparkle", "sparkles":
		return ExprSparkle
	case ExprLookAround, "lookaround":
		return ExprLookAround
	case ExprWhistle:
		return ExprWhistle
	default:
		return ExprNeutral
	}
}
