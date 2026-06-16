package face

import "testing"

func TestTimeStepLoop(t *testing.T) {
	// fps=10, steps=3 -> indices 0,1,2,0,1,2 at 0.0,0.1,0.2,0.3...
	cases := map[float64]int{0.0: 0, 0.1: 1, 0.25: 2, 0.30: 0, 0.45: 1}
	for clock, want := range cases {
		if got := timeStep(clock, 10, "loop", 3); got != want {
			t.Errorf("timeStep(%v,loop)=%d want %d", clock, got, want)
		}
	}
}

func TestTimeStepPingpong(t *testing.T) {
	// steps=3 -> period 4: 0,1,2,1,0,1,2,1...
	want := []int{0, 1, 2, 1, 0, 1, 2, 1}
	for i, w := range want {
		clock := float64(i) / 10.0
		if got := timeStep(clock, 10, "pingpong", 3); got != w {
			t.Errorf("pingpong i=%d got %d want %d", i, got, w)
		}
	}
}

func TestTimeStepOnceHoldsLast(t *testing.T) {
	if got := timeStep(99.0, 10, "once", 3); got != 2 {
		t.Errorf("once past end = %d want 2", got)
	}
	if got := timeStep(0.1, 10, "once", 3); got != 1 {
		t.Errorf("once mid = %d want 1", got)
	}
}

func TestAmplitudeStepLinearAndSqrt(t *testing.T) {
	lin := Driver{Kind: DriverAmplitude, Curve: "linear"}
	if got := lin.Step(0, 0, 0.5, 6); got != 3 { // round(0.5*5)=3 (0.5*5=2.5 -> +0.5 -> 3)
		t.Errorf("linear 0.5 -> %d want 3", got)
	}
	if got := lin.Step(0, 0, 1.0, 6); got != 5 {
		t.Errorf("linear 1.0 -> %d want 5", got)
	}
	sq := Driver{Kind: DriverAmplitude, Curve: "sqrt"}
	// sqrt(0.25)=0.5 -> round(0.5*5)=3
	if got := sq.Step(0, 0, 0.25, 6); got != 3 {
		t.Errorf("sqrt 0.25 -> %d want 3", got)
	}
}

func TestAmplitudeIdleEngagesAtZeroSignal(t *testing.T) {
	d := Driver{Kind: DriverAmplitude, Curve: "linear", Idle: &Idle{FPS: 10, Mode: "loop"}}
	// signal<=0 -> uses timeStep(clock,10,loop,3)
	if got := d.Step(0.1, 0, 0, 3); got != 1 {
		t.Errorf("idle at clock 0.1 = %d want 1", got)
	}
	// signal>0 -> ignores idle
	if got := d.Step(0.1, 0, 1.0, 3); got != 2 {
		t.Errorf("signal 1.0 = %d want 2", got)
	}
}

func TestStepSingleFrame(t *testing.T) {
	d := Driver{Kind: DriverTime, FPS: 10, Mode: "loop"}
	if got := d.Step(5, 0, 0, 1); got != 0 {
		t.Errorf("single frame = %d want 0", got)
	}
}
