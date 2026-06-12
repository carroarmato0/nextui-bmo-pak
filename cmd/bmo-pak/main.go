package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
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
	"github.com/carroarmato0/nextui-bmo/internal/devctx"
	"github.com/carroarmato0/nextui-bmo/internal/face"
	"github.com/carroarmato0/nextui-bmo/internal/hardware"
	"github.com/carroarmato0/nextui-bmo/internal/input"
	"github.com/carroarmato0/nextui-bmo/internal/observability"
	"github.com/carroarmato0/nextui-bmo/internal/providers"
	"github.com/carroarmato0/nextui-bmo/internal/renderer"
	"github.com/carroarmato0/nextui-bmo/internal/ui"
	"github.com/veandco/go-sdl2/sdl"
)

const menuTitleSettings = "SETTINGS"

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

	release, ok := acquireLock(filepath.Join(homeDir, ".lock"))
	if !ok {
		fmt.Fprintln(stdout, "BMO is already running")
		return nil
	}
	defer release()

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

	// Persona and voice prompts live in plain-text sidecar files: created
	// with defaults when missing or blank, never overwritten otherwise.
	personaPath := config.PersonaPath(homeDir)
	voicePath := config.VoicePath(homeDir)
	personaPrompt, err := config.EnsurePromptFile(personaPath, config.DefaultSystemPrompt)
	if err != nil {
		return fmt.Errorf("ensure persona file: %w", err)
	}
	voicePrompt, err := config.EnsurePromptFile(voicePath, config.DefaultTTSInstructions)
	if err != nil {
		return fmt.Errorf("ensure voice file: %w", err)
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
	settingsMenu.SetLogLevelCallback(func(level string) {
		logger.SetLevel(observability.ParseLevel(level))
		logger.Infof("log level changed to %s", level)
	})
	settingsMenu.SetRestoreDefaultsCallback(func() error {
		if err := restorePromptDefaults(personaPath, voicePath); err != nil {
			logger.Warnf("restore prompt defaults: %v", err)
			return err
		}
		logger.Infof("persona and voice prompts restored to defaults")
		return nil
	})
	setActiveMenu := func(menu ui.Menu) { activeMenu = menu }
	if initialScreen == ui.ScreenSetup {
		logger.Infof("setup flow required; press MENU to exit to NextUI, Start to open settings, Y for AI setup")
	}

	machine := assistant.NewMachine()
	machine.SetMode(cfg.Mode)
	machine.SetIdleSeed(time.Now().UnixNano())
	machine.RecordInteraction(time.Now().UTC())
	logger.Infof("initial state: %s", machine.State())
	logger.Debugf("assistant snapshot: %+v", machine.Snapshot())

	// Device awareness: read-only collectors feeding the DEVICE AWARENESS
	// block of the system prompt. BMO_SDCARD_ROOT overrides the SD card
	// location for desktop testing against pulled fixtures.
	sdRoot := strings.TrimSpace(os.Getenv("BMO_SDCARD_ROOT"))
	if sdRoot == "" {
		sdRoot = "/mnt/SDCARD"
	}
	achievementsCollector := devctx.AchievementsCollector{
		CacheDir:     filepath.Join(sdRoot, ".userdata", "shared", ".ra", "offline", "cache"),
		SettingsPath: filepath.Join(sdRoot, ".userdata", "shared", "minuisettings.txt"),
		Rng:          rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	deviceCtx := devctx.NewBuilder([]devctx.Collector{
		devctx.LibraryCollector{Root: filepath.Join(sdRoot, "Roms")},
		devctx.SavesCollector{Root: filepath.Join(sdRoot, "Saves")},
		devctx.PlayLogCollector{DBPath: filepath.Join(sdRoot, ".userdata", "shared", "game_logs.sqlite")},
		devctx.SystemCollector{
			Model:       hardwareProfile.DeviceTreeModel,
			UptimePath:  "/proc/uptime",
			MeminfoPath: "/proc/meminfo",
			DiskPath:    sdRoot,
			PowerDir:    "/sys/class/power_supply",
		},
		achievementsCollector,
	}, 30*time.Second, time.Now().UnixNano())
	deviceCtx.SetEnabled(cfg.DeviceContext)
	deviceCtx.SetReminisce(achievementsCollector.RandomPastUnlock)
	// Short-term memory: the memory feeds both the nudge picker
	// (6h cooldown dedup) and the RECENT REMARKS prompt block. A corrupt
	// file (hard power-off mid-write) just means starting empty.
	memoryPath := filepath.Join(homeDir, "memory.json")
	memory, merr := devctx.LoadMemory(memoryPath)
	if merr != nil {
		logger.Warnf("memory unreadable, starting empty: %v", merr)
	}
	deviceCtx.SetMemory(memory)
	quotesPath := config.QuotesPath(homeDir)
	quotesContent, qerr := config.EnsurePromptFile(quotesPath, config.DefaultQuotes)
	if qerr != nil {
		logger.Warnf("ensure quotes file: %v", qerr)
	}
	deviceCtx.SetQuotes(func() []string {
		content := readPromptFile(quotesPath)
		if content == "" {
			content = quotesContent
		}
		return devctx.ParseQuotes(content)
	})
	proactive := assistant.NewProactiveScheduler(machine, time.Now().UnixNano())
	proactive.SetInterval(config.ProactiveInterval(cfg.ProactiveTalk))

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
			audioPipeline = assistant.NewVoicePipeline(machine, audioRouter, sttClient, chatClient, ttsClient, cfg.STT.Model, cfg.Chat.Model, cfg.TTS.Model, cfg.TTS.Voice, personaPrompt, audioCfg.SampleRate, audioCfg.Channels)
			audioPipeline.SetLogger(logger)
			audioPipeline.SetTTSInstructions(voicePrompt)
			// Re-read both prompt files before each utterance so they can
			// be tuned without restarting the pak.
			audioPipeline.SetTTSInstructionsSource(func() string { return readPromptFile(voicePath) })
			audioPipeline.SetSystemPromptSource(func() string {
				return systemPromptWithContext(readPromptFile(personaPath), deviceCtx.Snapshot(), memory.PromptBlock(time.Now().UTC()))
			})
			stopPTT = startPushToTalk(ctx, logger, machine, cfg, hardwareProfile, audioRouter, audioPipeline, audioCfg.SampleRate, audioCfg.Channels, func() bool { return activeMenu != nil })
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
	logger.Infof("renderer ready: %s", screen.DebugInfo())

	faceLib := face.NewLibrary(config.FacesDir(homeDir))
	faceLib.SetLogf(logger.Warnf)
	faceCache := face.NewCache(faceLib)
	screen.SetFaces(faceCache)
	// Pre-rasterize current expression synchronously; warm remaining in background.
	go faceCache.Warm(screen.Size())

	// Navigation is read from raw Linux evdev (internal/input), not SDL's
	// GameController layer: SDL maps the TrimUI's Nintendo-style face buttons to
	// Xbox semantics (swapping A/B and reporting Select as Start), whereas the
	// evdev codes match the physical labels and are shared with the PTT path.
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
		// Apply the (possibly changed) mode immediately: it gates the PTT
		// watcher and the voice pipeline.
		machine.SetMode(cfg.Mode)
		// Apply awareness toggles and proactive level immediately too.
		deviceCtx.SetEnabled(cfg.DeviceContext)
		proactive.SetInterval(config.ProactiveInterval(cfg.ProactiveTalk))
		if err := config.Save(cfgPath, cfg); err != nil {
			return fmt.Errorf("save config: %w", err)
		}
		for _, secret := range cfg.Secrets() {
			logger.RegisterSecret(secret)
		}
		logger.Infof("saved %s menu with ptt buttons: %s", menu.Title(), strings.Join(cfg.PTTButtons, ", "))
		return nil
	}

	running := true // declared here so handleNav can close over it

	// handleNav maps decoded evdev navigation intents to menu/overlay actions.
	// Physical button layout (TrimUI, confirmed via getevent): A=BTN_EAST(305)
	// is confirm/PTT (handled by the PTT path), B=BTN_SOUTH(304) cancels,
	// Start opens settings, Y=BTN_NORTH(307) opens AI setup, Menu=BTN_MODE(316)
	// exits to NextUI, D-pad navigates.
	handleNav := func(action input.NavAction) {
		// MENU (BTN_MODE) always exits to NextUI.
		if action == input.NavMenu {
			running = false
			return
		}

		// B closes an open overlay; interrupts BMO mid-speech; exits otherwise.
		if action == input.NavCancel {
			if activeMenu != nil {
				setActiveMenu(nil)
			} else if audioPipeline != nil && audioPipeline.InterruptSpeech() {
				logger.Infof("speech interrupted by B press")
			} else {
				running = false
			}
			return
		}

		// Start opens/closes the settings overlay. Values auto-save on change,
		// so closing just closes.
		if action == input.NavSave {
			if activeMenu != nil {
				setActiveMenu(nil)
			} else {
				setActiveMenu(settingsMenu)
			}
			return
		}

		// Y opens/closes the AI setup overlay.
		if action == input.NavProvider {
			if activeMenu == providerMenu {
				setActiveMenu(nil)
			} else {
				setActiveMenu(providerMenu)
			}
			return
		}

		if activeMenu == nil {
			return
		}

		// Within the overlay: up/down navigate, left/right cycle the focused item.
		switch action {
		case input.NavUp:
			activeMenu.Move(-1)
		case input.NavDown:
			activeMenu.Move(1)
		case input.NavLeft, input.NavRight:
			// Cancel any keyboard-edit state (no keyboard on hardware), then cycle.
			type editable interface {
				IsEditing() bool
				CancelEdit()
			}
			if ed, ok := activeMenu.(editable); ok && ed.IsEditing() {
				ed.CancelEdit()
			}
			if err := activeMenu.ToggleFocused(); err != nil {
				logger.Debugf("toggle focused: %v", err)
			}
			// Cancel if ToggleFocused entered edit mode (API key item).
			if ed, ok := activeMenu.(editable); ok && ed.IsEditing() {
				ed.CancelEdit()
			}
			// Auto-persist after every value cycle. Validation failures
			// (e.g. AI mode without providers) are silently dropped.
			if err := commitMenu(activeMenu); err != nil {
				logger.Debugf("auto-save: %v", err)
			}
		}
	}

	scheduler := assistant.NewIdleScheduler(machine.Snapshot().IdleSeed)
	currentIdleExpression := assistant.ExpressionNeutral
	nextIdleUpdate := time.Now()
	var errorSince time.Time

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stop)

	logger.Infof("BMO ready; entering face loop")
	for running {
		select {
		case <-stop:
			running = false
			continue
		default:
		}

		// Drain decoded navigation intents from the evdev reader.
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

		// Pump SDL's event queue so the window stays responsive and display
		// resizes are picked up. Navigation input comes from evdev (above).
		for event := sdl.PollEvent(); event != nil; event = sdl.PollEvent() {
			switch ev := event.(type) {
			case *sdl.QuitEvent:
				running = false
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
			errorSince = time.Time{}
			if now.After(nextIdleUpdate) {
				step := scheduler.Next(now.Sub(snap.LastInteraction))
				currentIdleExpression = step.Expression
				nextIdleUpdate = now.Add(step.HoldFor)
			}
			expr = string(currentIdleExpression)
			if audioPipeline != nil && proactive.Due(now) {
				proactive.Reschedule(now)
				if n, ok := deviceCtx.ProactiveNudge(); ok {
					remarkPipeline := audioPipeline
					go func() {
						record := func(reply string) {
							if err := memory.Append(devctx.MemoryEntry{When: time.Now().UTC(), Topic: n.Topic, Subject: n.Subject, Reply: reply}); err != nil {
								logger.Warnf("memory save: %v", err)
							}
						}
						var err error
						if n.Verbatim {
							err = remarkPipeline.SpeakVerbatim(ctx, n.Text, record)
						} else {
							err = remarkPipeline.SpeakRemark(ctx, n.Text, record)
						}
						if err != nil {
							logger.Warnf("proactive remark failed: %v", err)
						}
					}()
				}
			}
		case assistant.StateListening:
			errorSince = time.Time{}
			expr = string(assistant.ExpressionListening)
		case assistant.StateThinking:
			errorSince = time.Time{}
			expr = string(assistant.ExpressionThinking)
		case assistant.StateSpeaking:
			errorSince = time.Time{}
			expr = string(assistant.ExpressionSpeaking)
		case assistant.StateSleeping:
			errorSince = time.Time{}
			expr = string(assistant.ExpressionSleeping)
		case assistant.StateError:
			expr = string(assistant.ExpressionConcerned)
			if errorSince.IsZero() {
				errorSince = now
				logger.Warnf("entered error state; will auto-recover in 5s")
			} else if now.Sub(errorSince) >= 5*time.Second {
				machine.Transition(assistant.EventRecover)
				errorSince = time.Time{}
				logger.Infof("auto-recovered from error state")
			}
		default:
			errorSince = time.Time{}
			if expr == "" {
				expr = string(assistant.ExpressionNeutral)
			}
		}

		if snap.Quota.Exhausted {
			expr = string(assistant.ExpressionSleeping)
		}
		if activeMenu != nil {
			if activeMenu.Title() == menuTitleSettings {
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

		var speakAmp float32
		if audioPipeline != nil {
			speakAmp = audioPipeline.CurrentAmplitude()
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
			SpeakAmplitude:  speakAmp,
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

func acquireLock(path string) (release func(), ok bool) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return func() {}, true
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, false
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
		_ = os.Remove(path)
	}, true
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
