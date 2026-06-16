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
	}
}
