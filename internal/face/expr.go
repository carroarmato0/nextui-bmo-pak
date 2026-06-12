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
)

// CanonicalNames lists every canonical expression name in a stable order.
var CanonicalNames = []string{
	ExprNeutral, ExprBlink, ExprListening, ExprThinking,
	ExprSpeaking, ExprSleeping, ExprConcerned, ExprSmile,
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
	case "error", "confused", "angry", "sad", ExprConcerned:
		return ExprConcerned
	case "happy", "laugh", "excited", ExprSmile:
		return ExprSmile
	case ExprListening:
		return ExprListening
	case ExprThinking:
		return ExprThinking
	case ExprSpeaking:
		return ExprSpeaking
	default:
		return ExprNeutral
	}
}
