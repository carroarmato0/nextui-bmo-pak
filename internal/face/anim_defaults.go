package face

// DefaultAnimations returns the built-in animation set baked into the binary.
// Overlay mods inherit these; self-contained mods do not (they declare their
// own). Each core-set emotion is a six-step template whose mouth is driven by
// the voice amplitude on param "m" (0 = rest, 1 = fully open). With NO Idle the
// amplitude driver rests at frame 0 during silence and opens the mouth as the
// signal rises, so the same face that shows the emotion also "talks". Built-in
// faces are therefore the reference implementation modders copy.
func DefaultAnimations() map[string]AnimationDef {
	tmpl := func(file string) AnimationDef {
		return AnimationDef{
			Template: &TemplateSource{File: file, Param: "m", From: 0, To: 1, Steps: 6},
			Driver:   Driver{Kind: DriverAmplitude, Curve: curveSqrt},
		}
	}
	return map[string]AnimationDef{
		ExprNeutral:   tmpl(ExprNeutral),
		ExprHappy:     tmpl(ExprHappy),
		ExprSmile:     tmpl(ExprSmile),
		ExprExcited:   tmpl(ExprExcited),
		ExprContent:   tmpl(ExprContent),
		ExprConcerned: tmpl(ExprConcerned),
		ExprSad:       tmpl(ExprSad),
		ExprAngry:     tmpl(ExprAngry),
		// Expressive emotions whose resting mouth is incidental: they keep their
		// own mouth at silence and open the shared talkmouth while speaking, so the
		// model can pick almost any emotion mid-reply and BMO still lip-syncs.
		ExprPlayful:   tmpl(ExprPlayful),
		ExprAdoring:   tmpl(ExprAdoring),
		ExprSparkle:   tmpl(ExprSparkle),
		ExprLove:      tmpl(ExprLove),
		ExprShy:       tmpl(ExprShy),
		ExprSurprised: tmpl(ExprSurprised),
		ExprGloomy:    tmpl(ExprGloomy),
		ExprAnnoyed:   tmpl(ExprAnnoyed),
		ExprSkeptical: tmpl(ExprSkeptical),
		ExprDismayed:  tmpl(ExprDismayed),
		ExprUnamused:  tmpl(ExprUnamused),
		// speaking is the dedicated talking face: vertical-bar eyes and an open
		// mouth with teeth and tongue. Six discrete frames (closed→open) are
		// driven by voice amplitude, with a gentle ping-pong idle oscillation
		// (~1.3 Hz) when amplitude is unavailable (e.g. pre-recorded clips).
		ExprSpeaking: {
			Frames: []string{"speaking_0", "speaking_1", "speaking_2", "speaking_3", "speaking_4", "speaking_5"},
			Driver: Driver{
				Kind:  DriverAmplitude,
				Curve: curveSqrt,
				Idle:  &Idle{FPS: 13, Mode: modePingpong},
			},
		},
		// Idle-only, no audio: a gentle horizontal eye scan. Pingpong over
		// x ∈ {-1,-0.5,0,0.5,1} at 3 fps gives a ~2.7s left↔right sweep.
		ExprLookAround: {
			Template: &TemplateSource{File: ExprLookAround, Param: "x", From: -1, To: 1, Steps: 5},
			Driver:   Driver{Kind: DriverTime, FPS: 3, Mode: modePingpong},
		},
		// Idle-only, no audio: pursed mouth with a music note that floats up and
		// fades. Loop over t ∈ {0..1} at 4 fps → note rises every ~1.5s.
		ExprWhistle: {
			Template: &TemplateSource{File: ExprWhistle, Param: "t", From: 0, To: 1, Steps: 6},
			Driver:   Driver{Kind: DriverTime, FPS: 4, Mode: modeLoop},
		},
	}
}
