package audio

import (
	"errors"
	"strings"
	"testing"

	"github.com/carroarmato0/nextui-bmo/internal/hardware"
)

func TestProbeCaptureDevicePrefersFirstWorkingCandidate(t *testing.T) {
	calls := make([]string, 0, 3)
	runner := func(name string, args ...string) error {
		calls = append(calls, name+" "+strings.Join(args, " "))
		if strings.Contains(strings.Join(args, " "), "default") {
			return nil
		}
		return errors.New("busy")
	}

	got := ProbeCaptureDevice(runner)
	if got != "default" {
		t.Fatalf("ProbeCaptureDevice() = %q, want default", got)
	}
	if len(calls) < 2 {
		t.Fatalf("ProbeCaptureDevice() used too few candidates: %v", calls)
	}
}

func TestProbePlaybackDeviceReturnsEmptyWhenNoCandidateWorks(t *testing.T) {
	runner := func(name string, args ...string) error { return errors.New("fail") }
	if got := ProbePlaybackDevice(runner); got != "" {
		t.Fatalf("ProbePlaybackDevice() = %q, want empty", got)
	}
}

func TestDefaultConfigUsesTrimUIDevicePaths(t *testing.T) {
	cfg := DefaultConfig(hardware.Profile{})
	if cfg.CaptureDevice != "hw:0,0" {
		t.Fatalf("CaptureDevice = %q, want hw:0,0", cfg.CaptureDevice)
	}
	if cfg.PlaybackDevice != "hw:0,0" {
		t.Fatalf("PlaybackDevice = %q, want hw:0,0", cfg.PlaybackDevice)
	}
	if got := cfg.CaptureArgs(); strings.Join(got, " ") == "" {
		t.Fatal("CaptureArgs() returned empty args")
	}
	if got := cfg.PlaybackArgs(); strings.Join(got, " ") == "" {
		t.Fatal("PlaybackArgs() returned empty args")
	}
}
