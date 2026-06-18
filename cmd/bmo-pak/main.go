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
	"github.com/carroarmato0/nextui-bmo/internal/clips"
	"github.com/carroarmato0/nextui-bmo/internal/config"
	"github.com/carroarmato0/nextui-bmo/internal/devctx"
	"github.com/carroarmato0/nextui-bmo/internal/face"
	"github.com/carroarmato0/nextui-bmo/internal/hardware"
	"github.com/carroarmato0/nextui-bmo/internal/input"
	"github.com/carroarmato0/nextui-bmo/internal/mod"
	"github.com/carroarmato0/nextui-bmo/internal/observability"
	"github.com/carroarmato0/nextui-bmo/internal/perf"
	"github.com/carroarmato0/nextui-bmo/internal/providers"
	"github.com/carroarmato0/nextui-bmo/internal/renderer"
	"github.com/carroarmato0/nextui-bmo/internal/ui"
	"github.com/veandco/go-sdl2/sdl"
)

const menuTitleSettings = "SETTINGS"

// mouthReleaseDecay is the per-frame factor by which the mouth-amplitude
// envelope eases toward a falling raw signal (attack is instant). At the ~60fps
// face loop this is roughly a 100ms release time constant.
const mouthReleaseDecay = 0.85

// mouthFloorCap caps how far open the release envelope can hold excited/smile
// during a gap: a thin opening (~level 1 of the six-step ladder). The mouth
// still tracks the raw volume above this, so dynamics stay natural; the floor
// only stops it snapping to the closed grin between syllables.
const mouthFloorCap = 0.04

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

	// Mods live under $home/mods/<name>. The "default" entry overlays embedded
	// BMO (per-asset fallback); any other folder is a self-contained character.
	// mods/default is created so users have an obvious place to drop overrides.
	modsRoot := filepath.Join(homeDir, "mods")
	if err := os.MkdirAll(filepath.Join(modsRoot, mod.DefaultID), 0o755); err != nil {
		return fmt.Errorf("create mods directory: %w", err)
	}
	mods := mod.Discover(modsRoot)
	activeMod := mod.Active(mods, cfg.ActiveMod)

	// Persona, voice, and quotes prompts use the override-or-default model:
	// the built-in defaults are the source of truth; an override file is used
	// only when it exists on disk and is non-blank. The app never creates them.
	// Paths resolve against the active mod; reloadMod (below) re-points them.
	personaPath := activeMod.PersonaPath()
	voicePath := activeMod.VoicePath()
	quotesPath := activeMod.QuotesPath()
	personaPrompt := config.LoadPromptFile(personaPath, config.DefaultSystemPrompt)
	voicePrompt := config.LoadPromptFile(voicePath, config.DefaultTTSInstructions)

	logPath := filepath.Join(dataRoot, "logs", "BMO.txt")
	logger, err := observability.NewLogger(logPath, observability.ParseLevel(cfg.LogLevel), stdout)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	defer logger.Close()

	// oksvg (the SVG rasterizer) emits "Cannot process svg element ..." notices
	// through Go's standard logger, which defaults to stderr — on device the same
	// pipe to NextUI/minui as stdout. If that consumer stalls, those writes would
	// block a rasterize goroutine. Send them to our structured logger's
	// best-effort console sink instead so the device pipe can never wedge them.
	log.SetOutput(logger.ConsoleWriter())
	log.SetFlags(0)

	logger.Infof("active mod: %s (self-contained=%t)", activeMod.ID, activeMod.SelfContained())

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
	settingsMenu := ui.NewSettingsMenu(cfg)
	modChoices := make([]ui.ModChoice, 0, len(mods))
	for _, md := range mods {
		modChoices = append(modChoices, ui.ModChoice{ID: md.ID, Label: strings.ToUpper(md.DisplayName())})
	}
	settingsMenu.SetModChoices(modChoices)
	settingsMenu.SetLogLevelCallback(func(level string) {
		logger.SetLevel(observability.ParseLevel(level))
		logger.Infof("log level changed to %s", level)
	})
	settingsMenu.SetRestoreDefaultsCallback(func() error {
		if err := config.RemoveOverrides(personaPath, voicePath, quotesPath); err != nil {
			logger.Warnf("restore defaults: %v", err)
			return err
		}
		logger.Infof("persona, voice, and quotes restored to built-in defaults")
		return nil
	})
	setActiveMenu := func(menu ui.Menu) { activeMenu = menu }
	if initialScreen == ui.ScreenSetup {
		logger.Infof("setup flow required; press MENU to exit to NextUI, Start to open settings")
	}

	machine := assistant.NewMachine()
	machine.SetMode(cfg.Mode)
	machine.SetIdleSeed(time.Now().UnixNano())
	machine.RecordInteraction(time.Now().UTC())
	logger.Infof("initial state: %s", machine.State())
	logger.Debugf("assistant snapshot: %+v", machine.Snapshot())

	// Opt-in profiling. All facets are inert unless their flag is set via the
	// .profile-flags file that launch.sh injects. Stop/flush hooks are deferred
	// so they run on the graceful-exit path (the same path that presents black
	// 3x); a kill -9 loses the final flush, which is why each sampler row is
	// written immediately rather than buffered.
	if pf := parsePerfFlags(os.Args[1:]); pf.enabled() {
		if pf.cpuProfile != "" {
			if stop, err := perf.StartCPUProfile(pf.cpuProfile); err != nil {
				logger.Errorf("cpuprofile: %v", err)
			} else {
				logger.Infof("cpuprofile: writing to %s", pf.cpuProfile)
				defer stop()
			}
		}
		if pf.pprofAddr != "" {
			perf.StartLiveServer(pf.pprofAddr, logger)
		}
		if pf.sampleFile != "" {
			sampler := perf.NewSampler(pf.sampleFile, pf.interval,
				func() string { return string(machine.State()) }, logger)
			if err := sampler.Start(); err != nil {
				logger.Errorf("perfsample: %v", err)
			} else {
				logger.Infof("perfsample: writing to %s every %s", pf.sampleFile, pf.interval)
				defer sampler.Stop()
			}
		}
		if pf.memProfile != "" {
			memProfile := pf.memProfile
			defer func() {
				if err := perf.WriteHeapProfile(memProfile); err != nil {
					logger.Errorf("memprofile: %v", err)
				} else {
					logger.Infof("memprofile: written to %s", memProfile)
				}
			}()
		}
	}

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
	deviceCtx.SetLibraryDetail(cfg.LibraryDetail)
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
	quotesFn := func() []string {
		return devctx.ParseQuotes(config.LoadPromptFile(quotesPath, config.DefaultQuotes))
	}
	deviceCtx.SetQuotes(quotesFn)
	proactive := assistant.NewProactiveScheduler(machine, time.Now().UnixNano())
	proactive.SetInterval(config.ProactiveInterval(cfg.ProactiveTalk))

	audioCfg := audio.DefaultConfig(hardwareProfile)
	audioSession := audio.NewSession(audioCfg)
	if err := audioSession.Start(); err != nil {
		logger.Warnf("audio session unavailable: %v", err)
		audioSession = nil
	} else {
		logger.Infof("audio session ready: %s", audioCfg.Summary())
		defer audioSession.Close()
	}

	var clipPlayer *clips.Player
	if audioSession != nil {
		clipLib := clips.NewLibrary(activeMod.AudioDir())
		clipPlayer = clips.NewPlayer(audioSession, audioCfg.SampleRate, audioCfg.PlaybackChannels, clipLib)
	}

	var audioRouter *audio.CaptureRouter
	var audioPipeline *assistant.VoicePipeline
	var stopPTT func()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if cfg.UsesAI() && audioSession != nil {
		audioRouter = audio.NewCaptureRouter(audioSession, audio.BytesPerSecond(audioCfg.SampleRate, audioCfg.Channels, audio.BytesPerSampleS16LE)/2)
		if err := audioRouter.Start(); err != nil {
			logger.Warnf("capture router unavailable: %v", err)
			audioRouter = nil
		} else {
			defer audioRouter.Close()

			sttP := cfg.STT.Current()
			chatP := cfg.Chat.Current()
			ttsP := cfg.TTS.Current()
			sttClient := providers.NewOpenAICompatibleClient(providers.Config{BaseURL: sttP.BaseURL, APIKey: sttP.APIKey}, http.DefaultClient)
			chatClient := providers.NewOpenAICompatibleClient(providers.Config{BaseURL: chatP.BaseURL, APIKey: chatP.APIKey}, http.DefaultClient)
			ttsClient := providers.NewOpenAICompatibleClient(providers.Config{BaseURL: ttsP.BaseURL, APIKey: ttsP.APIKey}, http.DefaultClient)
			audioPipeline = assistant.NewVoicePipeline(machine, audioRouter, sttClient, chatClient, ttsClient, sttP.Model, chatP.Model, ttsP.Model, ttsP.Voice, personaPrompt, audioCfg.SampleRate, audioCfg.Channels, audioCfg.PlaybackChannels)
			audioPipeline.SetLogger(logger)
			audioPipeline.SetTTSInstructions(voicePrompt)
			// Re-read both override files before each utterance so they can
			// be tuned without restarting the pak; absent or blank files fall
			// back to the built-in defaults.
			audioPipeline.SetTTSInstructionsSource(func() string {
				return config.LoadPromptFile(voicePath, config.DefaultTTSInstructions)
			})
			audioPipeline.SetSystemPromptSource(func() string {
				return systemPromptWithContext(
					config.LoadPromptFile(personaPath, config.DefaultSystemPrompt),
					deviceCtx.Snapshot(),
					memory.PromptBlock(time.Now().UTC()),
				)
			})
			// Advertise the active mod's emotions to the chat model. A
			// self-contained mod owns its set (no built-ins); otherwise the
			// embedded emotion faces are the base. activeMod is reassigned by the
			// mod-switch handler, so switching mods updates this on the next
			// utterance.
			audioPipeline.SetEmotionVocabularySource(func() []assistant.EmotionEntry {
				var builtin []string
				if !activeMod.SelfContained() {
					builtin = face.EmotionNames()
				}
				disk := face.EmotionFaceNamesInDir(activeMod.FacesDir())
				return assistant.BuildEmotionVocabulary(builtin, disk, activeMod.Manifest.Emotions)
			})
			if clipPlayer != nil {
				audioPipeline.SetTimeoutClip(clips.NewLibrary(activeMod.AudioDir()).Load("timeout"))
				audioPipeline.SetErrorClip(clips.NewLibrary(activeMod.AudioDir()).Load("error"))
			}
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

	faceLib := face.NewLibraryMode(activeMod.FacesDir(), activeMod.SelfContained())
	faceLib.SetLogf(logger.Warnf)
	faceCache := face.NewCache(faceLib)
	screen.SetFaces(faceCache)
	// Pre-rasterize current expression synchronously; warm remaining in background.
	go faceCache.Warm(screen.Size())

	animEngine := buildAnimationEngine(faceLib, activeMod, logger.Warnf)
	screen.SetAnimations(animEngine)
	{
		w, h := screen.Size()
		go animEngine.Prewarm(face.ExprSpeaking, w, h)
		// Build the pinned idle animations up front so the first idle whistle /
		// look_around / sleeping moves immediately instead of holding a static
		// frame.
		go animEngine.Prewarm(face.ExprLookAround, w, h)
		go animEngine.Prewarm(face.ExprWhistle, w, h)
		go animEngine.Prewarm(face.ExprSleeping, w, h)
	}

	// Switching mods at runtime: re-point the prompt/quote paths (read per
	// utterance via the source closures) and rebuild + re-warm the face cache
	// and clip library for the new mod. This runs on the main goroutine (from
	// the nav handler), the same goroutine as screen.Draw, so swapping the
	// face cache is race-free.
	reloadMod := func(id string) {
		active := mod.Active(mods, id)
		activeMod = active
		personaPath = active.PersonaPath()
		voicePath = active.VoicePath()
		quotesPath = active.QuotesPath()

		newLib := face.NewLibraryMode(active.FacesDir(), active.SelfContained())
		newLib.SetLogf(logger.Warnf)
		newCache := face.NewCache(newLib)
		faceCache = newCache
		screen.SetFaces(newCache)
		go newCache.Warm(screen.Size())

		animEngine = buildAnimationEngine(newLib, active, logger.Warnf)
		screen.SetAnimations(animEngine)
		{
			w, h := screen.Size()
			go animEngine.Prewarm(face.ExprSpeaking, w, h)
			go animEngine.Prewarm(face.ExprLookAround, w, h)
			go animEngine.Prewarm(face.ExprWhistle, w, h)
			go animEngine.Prewarm(face.ExprSleeping, w, h)
		}

		if audioSession != nil {
			clipLib := clips.NewLibrary(active.AudioDir())
			clipPlayer = clips.NewPlayer(audioSession, audioCfg.SampleRate, audioCfg.PlaybackChannels, clipLib)
			if audioPipeline != nil {
				audioPipeline.SetTimeoutClip(clipLib.Load("timeout"))
				audioPipeline.SetErrorClip(clipLib.Load("error"))
			}
		}
		logger.Infof("switched to mod %q (self-contained=%t)", active.ID, active.SelfContained())
	}
	settingsMenu.SetModChangeCallback(reloadMod)

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
		// Apply awareness toggles, library detail, and proactive level immediately.
		deviceCtx.SetEnabled(cfg.DeviceContext)
		deviceCtx.SetLibraryDetail(cfg.LibraryDetail)
		proactive.SetInterval(config.ProactiveInterval(cfg.ProactiveTalk))
		if audioPipeline != nil {
			audioPipeline.SetLogSystemPrompt(cfg.LogSystemPrompt)
			audioPipeline.SetRequestTimeout(time.Duration(cfg.RequestTimeout) * time.Second)
		}
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

	// shuttingDown is set when the user exits; the goodbye clip plays in the
	// clip player's own goroutine while the face loop keeps rendering so the
	// mouth animation is visible. goodbyeDone closes when the clip has played
	// out, which is the signal to actually exit.
	var shuttingDown bool
	var shuttingDownAt time.Time
	var goodbyeDone <-chan struct{}

	// beginShutdown starts BMO's farewell: it kicks off the goodbye clip in the
	// player's goroutine and leaves the face loop running so the mouth animates
	// until goodbyeDone closes. Used by both exit buttons (B and MENU). Calling
	// it again while already shutting down force-quits immediately.
	beginShutdown := func() {
		if shuttingDown {
			running = false
			return
		}
		shuttingDown = true
		shuttingDownAt = time.Now()
		if clipPlayer != nil {
			goodbyeDone = clipPlayer.PlaySequence(ctx, "goodbye")
		} else {
			running = false
		}
	}

	// Gallery override: the Y button (NavGallery) steps through every face the
	// active mod actually provides, so the user can flip through them on demand
	// instead of waiting for the idle scheduler to cycle. galleryActive freezes
	// the idle scheduler on galleryFaces[galleryIdx]; any non-idle state (a quote,
	// PTT, exit) clears it and normal idle scheduling resumes. The list is rebuilt
	// on each press so it always reflects the active mod (which may change at
	// runtime via reloadMod) and only includes faces the mod actually resolves.
	var galleryFaces []string
	galleryIdx := -1
	galleryActive := false
	galleryFaceNames := func() []string {
		var names []string
		seen := map[string]bool{}
		add := func(n string) {
			if seen[n] || faceCache.Source(n) == face.SourceNone {
				return
			}
			seen[n] = true
			names = append(names, n)
		}
		for _, n := range face.CanonicalNames {
			add(n)
		}
		for _, n := range face.EmotionFaceNamesInDir(activeMod.FacesDir()) {
			add(n)
		}
		return names
	}

	// handleNav maps decoded evdev navigation intents to menu/overlay actions.
	// Physical button layout (TrimUI, confirmed via getevent): A=BTN_EAST(305)
	// is confirm/PTT (handled by the PTT path), B=BTN_SOUTH(304) cancels,
	// Start opens settings, Menu=BTN_MODE(316) exits to NextUI, D-pad navigates,
	// X=BTN_WEST(308) speaks a random quote, Y=BTN_NORTH(307) steps the face gallery.
	handleNav := func(action input.NavAction) {
		// MENU (BTN_MODE) exits to NextUI after playing the goodbye clip.
		if action == input.NavMenu {
			beginShutdown()
			return
		}

		// B closes an open overlay; cancels in-flight batch; interrupts speech;
		// otherwise it exits to NextUI after playing the goodbye clip.
		if action == input.NavCancel {
			if activeMenu != nil {
				setActiveMenu(nil)
			} else if audioPipeline != nil && audioPipeline.CancelBatch() {
				logger.Infof("batch cancelled by B press")
			} else if audioPipeline != nil && audioPipeline.InterruptSpeech() {
				logger.Infof("speech interrupted by B press")
			} else {
				beginShutdown()
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

		// X (BTN_WEST) speaks a random verbatim quote. Ignored while a menu is
		// open; SpeakVerbatim itself no-ops unless AI/TTS is enabled and the
		// machine is idle, so a press during speech/listening does nothing.
		if action == input.NavQuote {
			if activeMenu != nil || audioPipeline == nil {
				return
			}
			quotes := quotesFn()
			if len(quotes) == 0 {
				return
			}
			text := quotes[rand.Intn(len(quotes))]
			remarkPipeline := audioPipeline
			go func() {
				if err := remarkPipeline.SpeakVerbatim(ctx, text, nil); err != nil {
					logger.Warnf("quote playback failed: %v", err)
				}
			}()
			return
		}

		// Y (BTN_NORTH) steps to the next face/animation for a quick gallery
		// preview. Only meaningful from idle (other states drive their own face);
		// it activates the override the idle branch reads each tick.
		if action == input.NavGallery {
			if activeMenu != nil || machine.Snapshot().Current != assistant.StateIdle {
				return
			}
			galleryFaces = galleryFaceNames()
			if len(galleryFaces) == 0 {
				return
			}
			galleryActive = true
			galleryIdx = (galleryIdx + 1) % len(galleryFaces)
			logger.Debugf("gallery: %s (%d/%d)", galleryFaces[galleryIdx], galleryIdx+1, len(galleryFaces))
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
			delta := 1
			if action == input.NavLeft {
				delta = -1
			}
			if err := activeMenu.Cycle(delta); err != nil {
				logger.Debugf("cycle focused: %v", err)
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
	var flog faceLogger
	var prewarmedEmotion string
	var heldAmp float32 // smoothed mouth amplitude (fast attack, slow release)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stop)

	// Startup clip: hold the default face for ~1 second, then — once the
	// speaking animation is warmed so the mouth moves smoothly — play "hello".
	startupFaceShownAt := time.Now().Add(time.Second)
	startupClipFired := clipPlayer == nil // skip if no audio

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

		// Play the startup clips once the default face has been visible for
		// ~1s and the speaking animation is warmed, so the mouth animates from
		// the first word instead of stalling the render loop on a cold, under-
		// mutex rasterization. Capped at 10s so a failed warm still plays the
		// audio. The clips run in the player's own goroutine, so audio pacing
		// is independent of the render loop's frame rate.
		if !startupClipFired && time.Now().After(startupFaceShownAt) &&
			(animEngine.Ready(face.ExprSpeaking) || time.Since(startupFaceShownAt) > 10*time.Second) {
			startupClipFired = true
			names := []string{"hello"}
			if overrideErrs := config.CheckOverrides(activeMod.Root); len(overrideErrs) > 0 {
				for _, e := range overrideErrs {
					logger.Warnf("mod override error: %v", e)
				}
				names = append(names, "mod_error")
			}
			clipPlayer.PlaySequence(ctx, names...)
		}

		// Exit once the goodbye clip has played out (or after an 8s safety
		// timeout), so the farewell is fully heard and animated before quitting.
		if shuttingDown {
			if goodbyeDone == nil || time.Since(shuttingDownAt) > 8*time.Second {
				running = false
				continue
			}
			select {
			case <-goodbyeDone:
				running = false
				continue
			default:
			}
		}

		now := time.Now().UTC()
		snap := machine.Snapshot()
		expr := string(snap.Expression)

		// Any non-idle state drives its own face, so a gallery preview ends the
		// moment BMO starts listening/thinking/speaking; normal idle scheduling
		// then resumes on the next return to idle.
		if snap.Current != assistant.StateIdle {
			galleryActive = false
		}

		// Prewarm the LLM-directed emotion's animation as soon as it is known.
		// SetEmotion fires before the TTS network round-trip, so the 6-frame
		// build completes during synthesis and the mouth opens on the very first
		// speaking frame instead of lagging while the engine builds on demand.
		if em := string(snap.Emotion); em != "" && em != prewarmedEmotion {
			prewarmedEmotion = em
			if animEngine.Has(em) {
				w, h := screen.Size()
				go animEngine.Prewarm(em, w, h)
			}
		}

		switch snap.Current {
		case assistant.StateIdle:
			errorSince = time.Time{}
			if galleryActive && galleryIdx >= 0 && galleryIdx < len(galleryFaces) {
				// Gallery preview: hold the user-selected face and freeze the idle
				// scheduler until they step again or interact.
				currentIdleExpression = assistant.Expression(galleryFaces[galleryIdx])
			} else if now.After(nextIdleUpdate) {
				step := scheduler.Next(now.Sub(snap.LastInteraction))
				currentIdleExpression = step.Expression
				nextIdleUpdate = now.Add(step.HoldFor)
			}
			expr = string(currentIdleExpression)
			if !galleryActive && audioPipeline != nil && proactive.Due(now) {
				proactive.Reschedule(now)
				remarkPipeline := audioPipeline
				// ProactiveNudge refreshes device context (sqlite play-log,
				// achievements cache, directory walks) when its cache is stale,
				// which it always is by the time a remark is due. Run it inside
				// the goroutine so that disk/DB I/O never blocks the render loop
				// (doing it inline froze the animation for the duration).
				go func() {
					n, ok := deviceCtx.ProactiveNudge()
					if !ok {
						return
					}
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
		case assistant.StateListening:
			errorSince = time.Time{}
			expr = string(assistant.ExpressionListening)
		case assistant.StateThinking:
			errorSince = time.Time{}
			expr = string(assistant.ExpressionThinking)
		case assistant.StateSpeaking:
			errorSince = time.Time{}
			if snap.Emotion != "" {
				expr = string(snap.Emotion)
			} else {
				expr = string(assistant.ExpressionSpeaking)
			}
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

		// Clip player overrides expression and amplitude while a pre-recorded
		// clip is streaming (startup/goodbye/fallback). Quota and menu overlays
		// still take priority further below.
		clipPlaying := clipPlayer != nil && clipPlayer.Playing()
		if clipPlaying {
			expr = string(assistant.ExpressionSpeaking)
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

		// log the active face once per change (debug only)
		if msg, ok := flog.note(expr, faceCache.Source(expr), animEngine.Has(expr)); ok {
			logger.Debugf("%s", msg)
		}

		var overlay *renderer.OverlayState
		if activeMenu != nil {
			o := activeMenu.Overlay()
			overlay = convertOverlay(o)
		}

		var rawAmp float32
		if clipPlaying {
			rawAmp = clipPlayer.CurrentAmplitude()
		} else if audioPipeline != nil {
			rawAmp = audioPipeline.CurrentAmplitude()
		}
		// Release-only envelope, kept warm every frame: rises instantly with the
		// audio and eases down after it.
		if rawAmp >= heldAmp {
			heldAmp = rawAmp
		} else {
			heldAmp = rawAmp + (heldAmp-rawAmp)*mouthReleaseDecay
		}
		// Excited, smile and angry rest as a prominent closed mouth that snapped
		// in and out on every inter-syllable gap. Keep the mouth tracking the raw
		// volume so it still reacts naturally, but never let it drop below a thin
		// opening while the envelope is settling — this bridges the gaps without
		// flattening the dynamics, then eases shut to the rest pose once the audio
		// truly stops. Other emotions and the clip face use the raw RMS as-is.
		speakAmp := rawAmp
		if strings.EqualFold(expr, face.ExprExcited) || strings.EqualFold(expr, face.ExprSmile) || strings.EqualFold(expr, face.ExprAngry) {
			floor := heldAmp
			if floor > mouthFloorCap {
				floor = mouthFloorCap
			}
			if speakAmp < floor {
				speakAmp = floor
			}
		}
		frame := renderer.FrameState{
			Expression:      expr,
			Now:             now,
			QuotaExhausted:  snap.Quota.Exhausted,
			IdlePhase:       float64(now.Sub(snap.LastInteraction)) / float64(time.Second),
			ReducedMotion:   cfg.ReducedMotion,
			LastInteraction: snap.LastInteraction,
			Speaking:        snap.Current == assistant.StateSpeaking || clipPlaying,
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
	// Clear the framebuffer before SDL teardown so BMO's last frame does not
	// linger in the scanout buffer; without this the launcher's menu does not
	// reclaim the screen and the frozen face stays visible after exit.
	if err := screen.Blank(); err != nil {
		logger.Warnf("blank framebuffer on exit: %v", err)
	}
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
			Disabled: item.Disabled,
			Hidden:   item.Hidden,
		})
	}
	return &renderer.OverlayState{
		Visible:    true,
		Title:      src.Title,
		Subtitle:   append([]string(nil), src.Subtitle...),
		Items:      items,
		Footer:     src.Footer,
		FocusIndex: src.FocusIndex,
	}
}

// buildAnimationEngine assembles the effective animation set for the active mod
// and returns an engine over lib. Overlay mods inherit the embedded defaults;
// self-contained mods own their set. Mod-declared animations win by name.
// Parse errors are logged and the offending entry is skipped.
func buildAnimationEngine(lib *face.Library, m mod.Mod, logf func(string, ...any)) *face.Engine {
	defs := map[string]face.AnimationDef{}
	if !m.SelfContained() {
		for k, v := range face.DefaultAnimations() {
			defs[k] = v
		}
	}
	modDefs, errs := face.ParseAnimations(m.Manifest.Animations)
	for _, e := range errs {
		logf("face: %v", e)
	}
	for k, v := range modDefs {
		defs[k] = v
	}
	eng := face.NewEngine(lib, defs)
	// The talking face backs the hello/goodbye clips, which play at the start
	// and end of a session. Pin it so a session's emotion churn cannot evict it
	// and leave goodbye's mouth rebuilding (and lagging) while audio plays.
	eng.Pin(face.ExprSpeaking)
	// look_around, whistle and sleeping are the time-driven idle animations:
	// unlike the amplitude faces (which rest at frame 0 during idle silence
	// anyway), they visibly move on their own, so a rebuild gap reads as the
	// animation "not starting". The idle rotation now cycles ~30 faces past the
	// LRU, which would evict and re-lag them repeatedly — pin them so they stay
	// resident and animate the instant they are shown.
	eng.Pin(face.ExprLookAround)
	eng.Pin(face.ExprWhistle)
	eng.Pin(face.ExprSleeping)
	return eng
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
