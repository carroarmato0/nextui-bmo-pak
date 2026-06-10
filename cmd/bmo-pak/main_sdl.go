//go:build cgo

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
	"github.com/veandco/go-sdl2/sdl"
)

func main() {
	if err := run(os.Stdout, os.Stderr); err != nil {
		log.Fatal(err)
	}
}

func run(stdout io.Writer, stderr io.Writer) error {
	_ = stderr

	platformHint := strings.TrimSpace(os.Getenv("BMO_PLATFORM"))
	hardwareProfile := hardware.Detect(platformHint)
	platform := hardwareProfile.Platform
	if platform == "" {
		platform = hardware.DefaultPlatform
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

	logger.Infof("hardware profile: %s", hardwareProfile.Summary())
	logger.Infof("hardware availability: framebuffer=%t input=%t audio=%t", hardwareProfile.FramebufferAvailable(), hardwareProfile.InputAvailable(), hardwareProfile.AudioAvailable())
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
	setActiveMenu := func(menu ui.Menu) {
		activeMenu = menu
		if activeMenu != nil {
			sdl.StartTextInput()
		} else {
			sdl.StopTextInput()
		}
	}
	if initialScreen == ui.ScreenSetup {
		setActiveMenu(providerMenu)
		logger.Infof("setup menu active: provider selection ready for first-run setup")
	}

	machine := assistant.NewMachine()
	machine.SetMode(cfg.Mode)
	machine.SetIdleSeed(time.Now().UnixNano())
	machine.RecordInteraction(time.Now().UTC())
	logger.Infof("initial state: %s", machine.State())
	logger.Debugf("assistant snapshot: %+v", machine.Snapshot())

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

	handleMenuEvent := func(ev sdl.Event) bool {
		if activeMenu == nil {
			return false
		}
		switch e := ev.(type) {
		case *sdl.KeyboardEvent:
			if e.Type != sdl.KEYDOWN {
				return true
			}
			if editor, ok := activeMenu.(interface{ IsEditing() bool; InsertText(string); Backspace(); CancelEdit(); SubmitEdit() error }); ok && editor.IsEditing() {
				switch e.Keysym.Sym {
				case sdl.K_RETURN, sdl.K_KP_ENTER:
					if err := editor.SubmitEdit(); err != nil {
						logger.Warnf("api key edit rejected: %v", err)
					}
				case sdl.K_ESCAPE:
					editor.CancelEdit()
				case sdl.K_BACKSPACE:
					editor.Backspace()
				}
				return true
			}
			switch e.Keysym.Sym {
			case sdl.K_UP, sdl.K_LEFT:
				activeMenu.Move(-1)
			case sdl.K_DOWN, sdl.K_RIGHT:
				activeMenu.Move(1)
			case sdl.K_RETURN, sdl.K_SPACE:
				if err := activeMenu.ToggleFocused(); err != nil {
					logger.Warnf("ptt toggle rejected: %v", err)
				}
			case sdl.K_e:
				if err := activeMenu.ToggleFocused(); err != nil {
					logger.Warnf("api key edit rejected: %v", err)
				}
			case sdl.K_s:
				if err := commitMenu(activeMenu); err != nil {
					logger.Warnf("menu save failed: %v", err)
				} else {
					setActiveMenu(nil)
				}
			case sdl.K_ESCAPE:
				setActiveMenu(nil)
			case sdl.K_F1:
				if activeMenu != nil && activeMenu.Title() == "AI SETUP" {
					setActiveMenu(nil)
				} else {
					setActiveMenu(providerMenu)
				}
			case sdl.K_F2:
				if activeMenu != nil && activeMenu.Title() == "SETUP" {
					setActiveMenu(nil)
				} else {
					setActiveMenu(pttMenu)
				}
			case sdl.K_F3:
				if activeMenu != nil && activeMenu.Title() == "SETTINGS" {
					setActiveMenu(nil)
				} else {
					setActiveMenu(settingsMenu)
				}
			}
			return true
		case *sdl.ControllerButtonEvent:
			if e.Type != sdl.CONTROLLERBUTTONDOWN {
				return true
			}
			if editor, ok := activeMenu.(interface{ IsEditing() bool; SubmitEdit() error; CancelEdit(); Backspace() }); ok && editor.IsEditing() {
				switch e.Button {
				case sdl.CONTROLLER_BUTTON_A, sdl.CONTROLLER_BUTTON_START:
					if err := editor.SubmitEdit(); err != nil {
						logger.Warnf("api key edit rejected: %v", err)
					}
				case sdl.CONTROLLER_BUTTON_B:
					editor.CancelEdit()
				case sdl.CONTROLLER_BUTTON_DPAD_LEFT, sdl.CONTROLLER_BUTTON_DPAD_UP:
					editor.Backspace()
				}
				return true
			}
			switch e.Button {
			case sdl.CONTROLLER_BUTTON_DPAD_UP, sdl.CONTROLLER_BUTTON_DPAD_LEFT:
				activeMenu.Move(-1)
			case sdl.CONTROLLER_BUTTON_DPAD_DOWN, sdl.CONTROLLER_BUTTON_DPAD_RIGHT:
				activeMenu.Move(1)
			case sdl.CONTROLLER_BUTTON_A:
				if err := activeMenu.ToggleFocused(); err != nil {
					logger.Warnf("ptt toggle rejected: %v", err)
				}
			case sdl.CONTROLLER_BUTTON_START:
				if err := commitMenu(activeMenu); err != nil {
					logger.Warnf("menu save failed: %v", err)
				} else {
					setActiveMenu(nil)
				}
			case sdl.CONTROLLER_BUTTON_B:
				setActiveMenu(nil)
			case sdl.CONTROLLER_BUTTON_Y:
				setActiveMenu(providerMenu)
			case sdl.CONTROLLER_BUTTON_X:
				setActiveMenu(pttMenu)
			case sdl.CONTROLLER_BUTTON_LEFTSHOULDER, sdl.CONTROLLER_BUTTON_RIGHTSHOULDER:
				setActiveMenu(settingsMenu)
			}
			return true
		}
		return false
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

		for event := sdl.PollEvent(); event != nil; event = sdl.PollEvent() {
			if handleMenuEvent(event) {
				continue
			}
			switch ev := event.(type) {
			case *sdl.QuitEvent:
				running = false
			case *sdl.KeyboardEvent:
				if ev.Type == sdl.KEYDOWN {
					switch ev.Keysym.Sym {
					case sdl.K_ESCAPE:
						running = false
					case sdl.K_F1:
						setActiveMenu(providerMenu)
					case sdl.K_F2:
						setActiveMenu(pttMenu)
					}
				}
			case *sdl.TextInputEvent:
				if editor, ok := activeMenu.(interface{ IsEditing() bool; InsertText(string) }); ok && editor.IsEditing() {
					editor.InsertText(strings.TrimRight(string(ev.Text[:]), string(rune(0))))
				}
			case *sdl.WindowEvent:
				if ev.Event == sdl.WINDOWEVENT_SIZE_CHANGED || ev.Event == sdl.WINDOWEVENT_RESIZED {
					screen.SyncSize()
				}
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
		sdl.Delay(16)
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
