//go:build !cgo

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/carroarmato0/nextui-bmo/internal/assistant"
	"github.com/carroarmato0/nextui-bmo/internal/audio"
	"github.com/carroarmato0/nextui-bmo/internal/config"
	"github.com/carroarmato0/nextui-bmo/internal/hardware"
	"github.com/carroarmato0/nextui-bmo/internal/observability"
	"github.com/carroarmato0/nextui-bmo/internal/providers"
	"github.com/carroarmato0/nextui-bmo/internal/renderer"
	"github.com/carroarmato0/nextui-bmo/internal/ui"
)

func main() {
	if err := run(os.Stdout, os.Stderr); err != nil {
		log.Fatal(err)
	}
}

func run(stdout io.Writer, stderr io.Writer) error {
	_ = stderr

	platform := strings.TrimSpace(os.Getenv("BMO_PLATFORM"))
	if platform == "" {
		platform = detectPlatform()
	}

	dataRoot := strings.TrimSpace(os.Getenv("BMO_DATA_ROOT"))
	if dataRoot == "" {
		dataRoot = filepath.Join(filepath.Dir(mustHomeDir()), platform)
	}

	homeDir := filepath.Join(dataRoot, "BMO")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		return fmt.Errorf("create home directory: %w", err)
	}

	cfgPath := config.Path(homeDir)
	cfg, err := config.Load(cfgPath)
	if err != nil && !errors.Is(err, config.ErrNotFound) {
		return fmt.Errorf("load config: %w", err)
	}
	if errors.Is(err, config.ErrNotFound) {
		if err := config.Save(cfgPath, cfg); err != nil {
			return fmt.Errorf("save default config: %w", err)
		}
	}
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("validate config: %w", err)
	}

	logPath := filepath.Join(dataRoot, "logs", "BMO.txt")
	logger, err := observability.NewLogger(logPath, observability.ParseLevel(cfg.LogLevel), stdout)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	defer logger.Close()

	for _, secret := range cfg.Secrets() {
		logger.RegisterSecret(secret)
	}

	logger.Infof("BMO starting (platform=%s mode=%s trigger=%s)", platform, cfg.Mode, cfg.InputTrigger)
	logger.Debugf("config path: %s", cfgPath)
	logger.Debugf("log path: %s", logPath)
	logger.Debugf("config snapshot: %+v", cfg.Redacted())

	flow := ui.NewSetupFlow(cfg)
	initialScreen := flow.InitialScreen()
	logger.Infof("initial screen: %s", initialScreen)

	machine := assistant.NewMachine()
	machine.SetMode(cfg.Mode)
	machine.SetIdleSeed(time.Now().UnixNano())
	machine.RecordInteraction(time.Now().UTC())
	logger.Infof("initial state: %s", machine.State())
	logger.Debugf("assistant snapshot: %+v", machine.Snapshot())

	hardwareProfile := hardware.Detect(platform)

	var audioSession *audio.Session
	var audioRouter *audio.CaptureRouter
	var audioPipeline *assistant.VoicePipeline
	var stopPTT func()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if cfg.UsesAI() {
		audioCfg := audio.DefaultConfig(hardwareProfile)
		audioSession = audio.NewSession(audioCfg)
		audioRouter = audio.NewCaptureRouter(audioSession, audio.BytesPerSecond(audioCfg.SampleRate, audioCfg.Channels, audio.BytesPerSampleS16LE)/2)
		if err := audioRouter.Start(); err != nil {
			logger.Warnf("audio session unavailable: %v", err)
			audioRouter = nil
			audioSession = nil
		} else {
			logger.Infof("audio session ready: %s", audioCfg.Summary())
			defer audioRouter.Close()
			defer audioSession.Close()

			sttClient := providers.NewOpenAICompatibleClient(providers.Config{BaseURL: cfg.STT.BaseURL, APIKey: cfg.STT.APIKey}, http.DefaultClient)
			chatClient := providers.NewOpenAICompatibleClient(providers.Config{BaseURL: cfg.Chat.BaseURL, APIKey: cfg.Chat.APIKey}, http.DefaultClient)
			ttsClient := providers.NewOpenAICompatibleClient(providers.Config{BaseURL: cfg.TTS.BaseURL, APIKey: cfg.TTS.APIKey}, http.DefaultClient)
			audioPipeline = assistant.NewVoicePipeline(machine, audioRouter, sttClient, chatClient, ttsClient, cfg.STT.Model, cfg.Chat.Model, cfg.TTS.Model, cfg.TTS.Voice, cfg.SystemPrompt, audioCfg.SampleRate, audioCfg.Channels)
			stopPTT = startPushToTalk(ctx, logger, machine, cfg, hardwareProfile, audioRouter, audioPipeline, audioCfg.SampleRate, audioCfg.Channels)
		}
	}
	if stopPTT != nil {
		defer stopPTT()
	}

	screen, err := renderer.NewFullscreen("BMO")
	if err != nil {
		return fmt.Errorf("create renderer: %w", err)
	}
	defer screen.Close()

	if initialScreen == ui.ScreenSetup {
		logger.Warnf("setup flow required; rendering BMO with a concerned idle face until the setup screen loop lands")
	}

	scheduler := assistant.NewIdleScheduler(machine.Snapshot().IdleSeed)
	currentIdleExpression := assistant.ExpressionNeutral
	nextIdleUpdate := time.Now()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stop)

	logger.Infof("BMO ready; entering face loop")
	running := true
	for running {
		select {
		case <-stop:
			running = false
		default:
		}

		now := time.Now().UTC()
		snap := machine.Snapshot()
		expr := string(snap.Expression)

		switch snap.Current {
		case assistant.StateIdle:
			if now.After(nextIdleUpdate) {
				step := scheduler.Next(now.Sub(snap.LastInteraction))
				currentIdleExpression = step.Expression
				nextIdleUpdate = now.Add(step.HoldFor)
			}
			expr = string(currentIdleExpression)
		case assistant.StateListening:
			expr = string(assistant.ExpressionListening)
		case assistant.StateThinking:
			expr = string(assistant.ExpressionThinking)
		case assistant.StateSpeaking:
			expr = string(assistant.ExpressionSpeaking)
		case assistant.StateSleeping:
			expr = string(assistant.ExpressionSleeping)
		case assistant.StateError:
			expr = string(assistant.ExpressionConcerned)
		default:
			if expr == "" {
				expr = string(assistant.ExpressionNeutral)
			}
		}

		if snap.Quota.Exhausted {
			expr = string(assistant.ExpressionSleeping)
		}
		machine.SetExpression(assistant.Expression(expr))

		frame := renderer.FrameState{
			Expression:      expr,
			Now:             now,
			QuotaExhausted:  snap.Quota.Exhausted,
			IdlePhase:       float64(now.Sub(snap.LastInteraction)) / float64(time.Second),
			ReducedMotion:   cfg.ReducedMotion,
			LastInteraction: snap.LastInteraction,
			Speaking:        snap.Current == assistant.StateSpeaking,
			Listening:       snap.Current == assistant.StateListening,
			Thinking:        snap.Current == assistant.StateThinking,
		}
		if snap.Quota.Exhausted && frame.SleepUntil.IsZero() {
			frame.SleepUntil = now.Add(45 * time.Minute)
		}
		if err := screen.Draw(frame); err != nil {
			return fmt.Errorf("draw frame: %w", err)
		}
		time.Sleep(16 * time.Millisecond)
	}

	logger.Infof("BMO shutting down")
	return nil
}

func detectPlatform() string {
	if data, err := os.ReadFile("/proc/cpuinfo"); err == nil && strings.Contains(strings.ToUpper(string(data)), "TG5050") {
		return "tg5050"
	}
	return "tg5040"
}

func mustHomeDir() string {
	if home := strings.TrimSpace(os.Getenv("HOME")); home != "" {
		return home
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "BMO")
	}
	return home
}
