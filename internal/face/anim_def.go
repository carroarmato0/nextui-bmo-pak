package face

import (
	"encoding/json"
	"fmt"
)

// DriverKind enumerates the supported animation drivers.
type DriverKind string

const (
	DriverAmplitude DriverKind = "amplitude"
	DriverTime      DriverKind = "time"
)

// Curve and mode literals shared across parsing and validation.
const (
	curveLinear = "linear"
	modeLoop    = "loop"
)

// AnimationDef is a parsed, validated animation. Exactly one frame source is
// set: Frames (explicit SVG basenames) or Template (parametric).
type AnimationDef struct {
	Frames   []string
	Template *TemplateSource
	Driver   Driver
}

// TemplateSource renders one Go-template SVG at Steps samples of Param across
// [From, To].
type TemplateSource struct {
	File  string
	Param string
	From  float64
	To    float64
	Steps int
}

// Driver selects the current step each tick.
type Driver struct {
	Kind  DriverKind
	Curve string // amplitude: "linear" (default) or "sqrt"
	Idle  *Idle  // amplitude: optional oscillation when signal <= 0
	FPS   float64
	Mode  string // time: "loop" (default), "pingpong", "once"
}

// Idle is a time oscillation used by an amplitude driver when no signal.
type Idle struct {
	FPS  float64
	Mode string
}

// Steps returns the number of frames the animation produces.
func (d AnimationDef) Steps() int {
	if d.Template != nil {
		return d.Template.Steps
	}
	return len(d.Frames)
}

type rawAnim struct {
	Frames   []string        `json:"frames"`
	Template string          `json:"template"`
	Param    string          `json:"param"`
	From     float64         `json:"from"`
	To       float64         `json:"to"`
	Steps    int             `json:"steps"`
	Driver   json.RawMessage `json:"driver"`
}

type rawDriver struct {
	Type  string `json:"type"`
	Curve string `json:"curve"`
	Idle  *struct {
		FPS  float64 `json:"fps"`
		Mode string  `json:"mode"`
	} `json:"idle"`
	FPS  float64 `json:"fps"`
	Mode string  `json:"mode"`
}

// ParseAnimations parses a map of expression name -> raw animation JSON,
// tolerating per-entry errors: a bad entry is skipped and its error collected.
func ParseAnimations(in map[string]json.RawMessage) (map[string]AnimationDef, []error) {
	out := make(map[string]AnimationDef, len(in))
	var errs []error
	for name, rawDef := range in {
		def, err := parseAnimation(rawDef)
		if err != nil {
			errs = append(errs, fmt.Errorf("animation %q: %w", name, err))
			continue
		}
		out[name] = def
	}
	return out, errs
}

func parseAnimation(data []byte) (AnimationDef, error) {
	var r rawAnim
	if err := json.Unmarshal(data, &r); err != nil {
		return AnimationDef{}, fmt.Errorf("invalid JSON: %w", err)
	}
	hasFrames := len(r.Frames) > 0
	hasTemplate := r.Template != ""
	if hasFrames == hasTemplate {
		return AnimationDef{}, fmt.Errorf("exactly one of frames or template required")
	}

	drv, err := parseDriver(r.Driver)
	if err != nil {
		return AnimationDef{}, err
	}

	if hasFrames {
		for _, n := range r.Frames {
			if !fileNameRe.MatchString(n) {
				return AnimationDef{}, fmt.Errorf("invalid frame name %q", n)
			}
		}
		return AnimationDef{Frames: r.Frames, Driver: drv}, nil
	}

	if !fileNameRe.MatchString(trimSVG(r.Template)) {
		return AnimationDef{}, fmt.Errorf("invalid template file %q", r.Template)
	}
	if r.Param == "" {
		return AnimationDef{}, fmt.Errorf("template requires param")
	}
	if r.Steps < 2 {
		return AnimationDef{}, fmt.Errorf("template requires steps >= 2")
	}
	return AnimationDef{
		Template: &TemplateSource{File: trimSVG(r.Template), Param: r.Param, From: r.From, To: r.To, Steps: r.Steps},
		Driver:   drv,
	}, nil
}

// trimSVG strips a trailing ".svg" so template files may be written with or
// without the extension; the loader always appends ".svg".
func trimSVG(name string) string {
	if len(name) > 4 && name[len(name)-4:] == ".svg" {
		return name[:len(name)-4]
	}
	return name
}

func parseDriver(data []byte) (Driver, error) {
	if len(data) == 0 {
		return Driver{}, fmt.Errorf("missing driver")
	}
	// String shorthand: "amplitude".
	var s string
	if json.Unmarshal(data, &s) == nil {
		if s == string(DriverAmplitude) {
			return Driver{Kind: DriverAmplitude, Curve: "linear"}, nil
		}
		return Driver{}, fmt.Errorf("unknown driver shorthand %q", s)
	}
	var rd rawDriver
	if err := json.Unmarshal(data, &rd); err != nil {
		return Driver{}, fmt.Errorf("invalid driver: %w", err)
	}
	switch DriverKind(rd.Type) {
	case DriverAmplitude:
		curve := rd.Curve
		switch curve {
		case "", curveLinear:
			curve = curveLinear
		case "sqrt":
			// ok
		default:
			return Driver{}, fmt.Errorf("unknown curve %q", curve)
		}
		drv := Driver{Kind: DriverAmplitude, Curve: curve}
		if rd.Idle != nil {
			if rd.Idle.FPS <= 0 {
				return Driver{}, fmt.Errorf("idle requires fps > 0")
			}
			drv.Idle = &Idle{FPS: rd.Idle.FPS, Mode: orMode(rd.Idle.Mode)}
		}
		return drv, nil
	case DriverTime:
		if rd.FPS <= 0 {
			return Driver{}, fmt.Errorf("time driver requires fps > 0")
		}
		mode := orMode(rd.Mode)
		if mode != modeLoop && mode != "pingpong" && mode != "once" {
			return Driver{}, fmt.Errorf("unknown mode %q", rd.Mode)
		}
		return Driver{Kind: DriverTime, FPS: rd.FPS, Mode: mode}, nil
	default:
		return Driver{}, fmt.Errorf("unknown driver type %q", rd.Type)
	}
}

func orMode(m string) string {
	if m == "" {
		return modeLoop
	}
	return m
}
