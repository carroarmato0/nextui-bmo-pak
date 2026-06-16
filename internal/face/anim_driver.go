package face

import "math"

const (
	curveSqrt    = "sqrt"
	modeOnce     = "once"
	modePingpong = "pingpong"
)

// Step returns the current frame index in [0, steps) for this driver.
//   clock  — absolute seconds (loop/pingpong, and amplitude idle)
//   epoch  — seconds since the expression became active (time "once")
//   signal — amplitude in [0,1]
func (d Driver) Step(clock, epoch float64, signal float32, steps int) int {
	if steps <= 1 {
		return 0
	}
	switch d.Kind {
	case DriverAmplitude:
		if signal <= 0 && d.Idle != nil {
			return timeStep(clock, d.Idle.FPS, d.Idle.Mode, steps)
		}
		v := float64(signal)
		if v < 0 {
			v = 0
		}
		if v > 1 {
			v = 1
		}
		if d.Curve == curveSqrt {
			v = math.Sqrt(v)
		}
		return clampStep(int(v*float64(steps-1)+0.5), steps)
	case DriverTime:
		elapsed := clock
		if d.Mode == modeOnce {
			elapsed = clock - epoch
			if elapsed < 0 {
				elapsed = 0
			}
		}
		return timeStep(elapsed, d.FPS, d.Mode, steps)
	default:
		return 0
	}
}

// timeStep maps elapsed seconds to a frame index given fps, mode and steps.
func timeStep(elapsed, fps float64, mode string, steps int) int {
	if steps <= 1 {
		return 0
	}
	idx := int(elapsed * fps)
	if idx < 0 {
		idx = 0
	}
	switch mode {
	case modePingpong:
		period := 2 * (steps - 1)
		p := idx % period
		if p < steps {
			return p
		}
		return period - p
	case modeOnce:
		if idx >= steps {
			return steps - 1
		}
		return idx
	default: // loop
		return idx % steps
	}
}

func clampStep(s, steps int) int {
	if s < 0 {
		return 0
	}
	if s >= steps {
		return steps - 1
	}
	return s
}
