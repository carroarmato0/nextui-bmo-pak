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
	"github.com/carroarmato0/nextui-bmo/internal/input"
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

	var activeMenu ui.Menu
	providerMenu := ui.NewProviderMenu(cfg)
	settingsMenu := ui.NewSettingsMenu(cfg)
	pttMenu := ui.NewSetupMenu(cfg)
	setActiveMenu := func(menu ui.Menu) { activeMenu = menu }

	if initialScreen == ui.ScreenSetup {
		logger.Infof("setup flow required; press MENU to open settings, L/R for settings, Y for AI setup, X for PTT setup")
	}

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

	var navCh <-chan input.NavAction
	if nr, nerr := input.NewNavReader(hardwareProfile.InputEvent); nerr != nil {
		logger.Warnf("nav reader unavailable: %v", nerr)
	} else if nerr = nr.Start(ctx); nerr != nil {
		logger.Warnf("nav reader start failed: %v", nerr)
	} else {
		navCh = nr.Events()
		defer nr.Close()
		logger.Infof("nav reader ready: device=%s", hardwareProfile.InputEvent)
	}

	if initialScreen == ui.ScreenSetup {
		logger.Warnf("setup flow required; rendering a concerned idle face until the user opens the menu shortcuts")
	}

	commitMenu := func(menu ui.Menu) error {
		if menu == nil {
			return nil
		}
		saved, err := menu.Save()
		if err != nil {
			return err
		}
		cfg = saved
		if err := config.Save(cfgPath, cfg); err != nil {
			return fmt.Errorf("save config: %w", err)
		}
		for _, secret := range cfg.Secrets() {
			logger.RegisterSecret(secret)
		}
		logger.Infof("saved %s menu with ptt buttons: %s", menu.Title(), strings.Join(cfg.PTTButtons, ", "))
		return nil
	}

	handleNav := func(action input.NavAction) {
		// Global shortcuts — always processed.
		switch action {
		case input.NavMenu:
			if activeMenu != nil && activeMenu.Title() == "SETTINGS" {
				setActiveMenu(nil)
			} else {
				setActiveMenu(settingsMenu)
			}
			return
		}

		// When no overlay is open, shortcut buttons open specific menus.
		if activeMenu == nil {
			switch action {
			case input.NavSettings:
				setActiveMenu(settingsMenu)
			case input.NavAISetup:
				setActiveMenu(providerMenu)
			case input.NavPTTSetup:
				setActiveMenu(pttMenu)
			}
			return
		}

		// If the active menu is in editing state, only confirm/cancel actions apply.
		// Hardware has no keyboard so we immediately submit to preserve the existing value.
		type editable interface {
			IsEditing() bool
			SubmitEdit() error
			CancelEdit()
		}
		if ed, ok := activeMenu.(editable); ok && ed.IsEditing() {
			switch action {
			case input.NavConfirm, input.NavSave:
				if err := ed.SubmitEdit(); err != nil {
					logger.Warnf("edit submit: %v", err)
				}
			case input.NavCancel:
				ed.CancelEdit()
			}
			return
		}

		switch action {
		case input.NavUp, input.NavLeft:
			activeMenu.Move(-1)
		case input.NavDown, input.NavRight:
			activeMenu.Move(1)
		case input.NavConfirm:
			if err := activeMenu.ToggleFocused(); err != nil {
				logger.Warnf("toggle focused: %v", err)
			}
			// Cancel any edit state that was inadvertently triggered (no keyboard on hardware).
			if ed, ok := activeMenu.(editable); ok && ed.IsEditing() {
				ed.CancelEdit()
			}
		case input.NavSave:
			if err := commitMenu(activeMenu); err != nil {
				logger.Warnf("menu save: %v", err)
			} else {
				setActiveMenu(nil)
			}
		case input.NavCancel:
			setActiveMenu(nil)
		case input.NavSettings:
			if activeMenu.Title() == "SETTINGS" {
				setActiveMenu(nil)
			} else {
				setActiveMenu(settingsMenu)
			}
		case input.NavAISetup:
			if activeMenu.Title() == "AI SETUP" {
				setActiveMenu(nil)
			} else {
				setActiveMenu(providerMenu)
			}
		case input.NavPTTSetup:
			if activeMenu.Title() == "SETUP" {
				setActiveMenu(nil)
			} else {
				setActiveMenu(pttMenu)
			}
		}
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
			continue
		default:
		}

	drainNav:
		for {
			select {
			case action, ok := <-navCh:
				if !ok {
					navCh = nil
					break drainNav
				}
				handleNav(action)
			default:
				break drainNav
			}
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
		if activeMenu != nil {
			if activeMenu.Title() == "SETTINGS" {
				expr = string(assistant.ExpressionSmile)
			} else {
				expr = string(assistant.ExpressionConcerned)
			}
		}
		machine.SetExpression(assistant.Expression(expr))

		var overlay *renderer.OverlayState
		if activeMenu != nil {
			o := activeMenu.Overlay()
			overlay = convertOverlay(o)
		}

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
			Overlay:         overlay,
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

func convertOverlay(src ui.OverlayState) *renderer.OverlayState {
	if !src.Visible {
		return nil
	}
	items := make([]renderer.OverlayItem, 0, len(src.Items))
	for _, item := range src.Items {
		items = append(items, renderer.OverlayItem{
			Code:     item.Code,
			Label:    item.Label,
			Selected: item.Selected,
			Focused:  item.Focused,
		})
	}
	return &renderer.OverlayState{
		Visible:  true,
		Title:    src.Title,
		Subtitle: append([]string(nil), src.Subtitle...),
		Items:    items,
		Footer:   src.Footer,
	}
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
