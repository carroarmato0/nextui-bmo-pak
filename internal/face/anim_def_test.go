package face

import (
	"encoding/json"
	"testing"
)

func raw(s string) json.RawMessage { return json.RawMessage(s) }

func TestParseAnimationFramesForm(t *testing.T) {
	def, err := parseAnimation(raw(`{"frames":["a","b","c"],"driver":"amplitude"}`))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if def.Template != nil {
		t.Fatalf("expected list form, got template")
	}
	if got := def.Steps(); got != 3 {
		t.Fatalf("Steps()=%d want 3", got)
	}
	if def.Driver.Kind != DriverAmplitude || def.Driver.Curve != "linear" {
		t.Fatalf("driver=%+v want amplitude/linear", def.Driver)
	}
}

func TestParseAnimationTemplateForm(t *testing.T) {
	def, err := parseAnimation(raw(`{"template":"dots.svg","param":"V","from":0,"to":3,"steps":4,"driver":{"type":"time","fps":6,"mode":"loop"}}`))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if def.Template == nil || def.Template.Param != "V" || def.Template.Steps != 4 {
		t.Fatalf("template=%+v", def.Template)
	}
	if def.Driver.Kind != DriverTime || def.Driver.FPS != 6 || def.Driver.Mode != "loop" {
		t.Fatalf("driver=%+v", def.Driver)
	}
}

func TestParseAnimationAmplitudeObjectWithIdle(t *testing.T) {
	def, err := parseAnimation(raw(`{"frames":["a","b"],"driver":{"type":"amplitude","curve":"sqrt","idle":{"fps":13,"mode":"pingpong"}}}`))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if def.Driver.Curve != "sqrt" || def.Driver.Idle == nil || def.Driver.Idle.FPS != 13 || def.Driver.Idle.Mode != "pingpong" {
		t.Fatalf("driver=%+v idle=%+v", def.Driver, def.Driver.Idle)
	}
}

func TestParseAnimationRejectsBothSources(t *testing.T) {
	if _, err := parseAnimation(raw(`{"frames":["a"],"template":"x.svg","param":"V","steps":2,"driver":"amplitude"}`)); err == nil {
		t.Fatal("expected error for both frames and template")
	}
}

func TestParseAnimationRejectsNoDriver(t *testing.T) {
	if _, err := parseAnimation(raw(`{"frames":["a","b"]}`)); err == nil {
		t.Fatal("expected error for missing driver")
	}
}

func TestParseAnimationsSkipsMalformed(t *testing.T) {
	in := map[string]json.RawMessage{
		"good": raw(`{"frames":["a","b"],"driver":"amplitude"}`),
		"bad":  raw(`{"driver":"amplitude"}`), // no frame source
	}
	defs, errs := ParseAnimations(in)
	if _, ok := defs["good"]; !ok {
		t.Fatal("good animation missing")
	}
	if _, ok := defs["bad"]; ok {
		t.Fatal("bad animation should be skipped")
	}
	if len(errs) != 1 {
		t.Fatalf("len(errs)=%d want 1", len(errs))
	}
}
