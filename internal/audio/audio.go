package audio

import (
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/carroarmato0/nextui-bmo/internal/hardware"
)

const (
	DefaultSampleRate = 16000
	DefaultChannels   = 1
	DefaultFormat     = "S16_LE"

	// PlaybackBufferMs bounds the ALSA playback buffer so the gap between
	// writing PCM and hearing it stays small and known. The voice pipeline
	// paces its writes with a prefill cushion of the same size so the mouth
	// animation tracks the audible playback position.
	PlaybackBufferMs = 200
)

type CmdRunner func(name string, args ...string) error

type CmdFactory func(name string, args ...string) *exec.Cmd

type Config struct {
	CaptureDevice   string
	PlaybackDevice  string
	CaptureTool     string
	PlaybackTool    string
	SampleRate      int
	Channels        int
	PlaybackChannels int
	Format          string
}

type Session struct {
	cfg Config

	factory CmdFactory

	mu       sync.Mutex
	capture  *exec.Cmd
	playback *exec.Cmd
	stdin    io.WriteCloser
	stdout   io.ReadCloser
	frames   chan []byte
	done     chan struct{}
	closed   bool
}

func DefaultConfig(profile hardware.Profile) Config {
	return Config{
		CaptureDevice:  firstNonEmpty(profile.AudioALSAName, "hw:0,0"),
		PlaybackDevice: firstNonEmpty(profile.AudioALSAName, "hw:0,0"),
		CaptureTool:    "arecord",
		PlaybackTool:   "aplay",
		SampleRate:     DefaultSampleRate,
		Channels:       DefaultChannels,
		Format:         DefaultFormat,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (c Config) normalize() Config {
	if strings.TrimSpace(c.CaptureDevice) == "" {
		c.CaptureDevice = hardware.DefaultAudioCapture
	}
	if strings.TrimSpace(c.PlaybackDevice) == "" {
		c.PlaybackDevice = hardware.DefaultAudioPlayback
	}
	if strings.TrimSpace(c.CaptureTool) == "" {
		c.CaptureTool = "arecord"
	}
	if strings.TrimSpace(c.PlaybackTool) == "" {
		c.PlaybackTool = "aplay"
	}
	if c.SampleRate <= 0 {
		c.SampleRate = DefaultSampleRate
	}
	if c.Channels <= 0 {
		c.Channels = DefaultChannels
	}
	if c.PlaybackChannels <= 0 {
		c.PlaybackChannels = 2
	}
	if strings.TrimSpace(c.Format) == "" {
		c.Format = DefaultFormat
	}
	return c
}

func (c Config) Summary() string {
	c = c.normalize()
	return fmt.Sprintf("capture=%s via %s, playback=%s via %s, %dHz cap=%dch play=%dch %s", c.CaptureDevice, c.CaptureTool, c.PlaybackDevice, c.PlaybackTool, c.SampleRate, c.Channels, c.PlaybackChannels, c.Format)
}

func (c Config) CaptureArgs() []string {
	c = c.normalize()
	return []string{"-q", "-D", c.CaptureDevice, "-f", c.Format, "-c", fmt.Sprintf("%d", c.Channels), "-r", fmt.Sprintf("%d", c.SampleRate), "-t", "raw"}
}

func (c Config) PlaybackArgs() []string {
	c = c.normalize()
	return []string{"-q", "-D", c.PlaybackDevice, "-f", c.Format, "-c", fmt.Sprintf("%d", c.PlaybackChannels), "-r", fmt.Sprintf("%d", c.SampleRate), "-t", "raw",
		fmt.Sprintf("--buffer-time=%d", PlaybackBufferMs*1000)}
}

func NewSession(cfg Config) *Session {
	return &Session{
		cfg:     cfg.normalize(),
		factory: defaultFactory,
		frames:  make(chan []byte, 8),
		done:    make(chan struct{}),
	}
}

func (s *Session) SetFactory(factory CmdFactory) {
	if factory == nil {
		factory = defaultFactory
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.factory = factory
}

func (s *Session) Frames() <-chan []byte { return s.frames }

func (s *Session) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("audio session closed")
	}
	if s.capture != nil || s.playback != nil {
		return nil
	}
	factory := s.factory
	if factory == nil {
		factory = defaultFactory
	}

	capture := factory(s.cfg.CaptureTool, s.cfg.CaptureArgs()...)
	capStdout, err := capture.StdoutPipe()
	if err != nil {
		return fmt.Errorf("capture stdout pipe: %w", err)
	}
	capture.Stderr = io.Discard
	if err := capture.Start(); err != nil {
		return fmt.Errorf("start capture: %w", err)
	}

	playback := factory(s.cfg.PlaybackTool, s.cfg.PlaybackArgs()...)
	playStdin, err := playback.StdinPipe()
	if err != nil {
		_ = capture.Process.Kill()
		_ = capture.Wait()
		return fmt.Errorf("playback stdin pipe: %w", err)
	}
	playback.Stderr = io.Discard
	if err := playback.Start(); err != nil {
		_ = capture.Process.Kill()
		_ = capture.Wait()
		return fmt.Errorf("start playback: %w", err)
	}

	s.capture = capture
	s.playback = playback
	s.stdin = playStdin
	s.stdout = capStdout

	go s.streamCapture()
	return nil
}

func (s *Session) streamCapture() {
	defer close(s.done)
	defer close(s.frames)
	buf := make([]byte, 4096)
	for {
		n, err := s.stdout.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			select {
			case s.frames <- chunk:
			default:
			}
		}
		if err != nil {
			return
		}
	}
}

func (s *Session) WritePCM(pcm []byte) error {
	s.mu.Lock()
	stdin := s.stdin
	s.mu.Unlock()
	if stdin == nil {
		return errors.New("playback not started")
	}
	_, err := stdin.Write(pcm)
	return err
}

func (s *Session) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	capture := s.capture
	playback := s.playback
	stdin := s.stdin
	s.capture = nil
	s.playback = nil
	s.stdin = nil
	s.stdout = nil
	s.mu.Unlock()

	if stdin != nil {
		_ = stdin.Close()
	}
	if capture != nil && capture.Process != nil {
		_ = capture.Process.Kill()
	}
	if playback != nil && playback.Process != nil {
		_ = playback.Process.Kill()
	}
	if capture != nil {
		_ = capture.Wait()
	}
	if playback != nil {
		_ = playback.Wait()
	}
	select {
	case <-s.done:
	case <-time.After(250 * time.Millisecond):
	}
	return nil
}

func ProbeCaptureDevice(runner CmdRunner) string {
	return probeALSA(runner, "arecord")
}

func ProbePlaybackDevice(runner CmdRunner) string {
	return probeALSA(runner, "aplay")
}

func probeALSA(runner CmdRunner, tool string) string {
	if runner == nil {
		runner = func(name string, args ...string) error {
			return exec.Command(name, args...).Run()
		}
	}
	for _, dev := range []string{"hw:0,0", "default", "plughw:0,0"} {
		args := []string{"-q", "-D", dev, "-d", "1", "-f", DefaultFormat}
		if tool == "aplay" {
			args = append(args, "/dev/null")
		} else {
			args = append(args, "/dev/null")
		}
		if err := runner(tool, args...); err == nil {
			return dev
		}
	}
	return ""
}

func defaultFactory(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}
