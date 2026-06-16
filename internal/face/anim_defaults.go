package face

// DefaultAnimations returns the built-in animation set baked into the binary.
// Overlay mods inherit these; self-contained mods do not (they declare their
// own). The speaking mouth is six frames driven by lip-sync amplitude, with a
// gentle idle oscillation (~1.3 Hz, matching the previous sine fallback) when
// amplitude is unavailable.
func DefaultAnimations() map[string]AnimationDef {
	return map[string]AnimationDef{
		ExprSpeaking: {
			Frames: []string{"speaking_0", "speaking_1", "speaking_2", "speaking_3", "speaking_4", "speaking_5"},
			Driver: Driver{
				Kind:  DriverAmplitude,
				Curve: "sqrt",
				Idle:  &Idle{FPS: 13, Mode: "pingpong"},
			},
		},
	}
}
