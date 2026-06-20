package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
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
	"github.com/carroarmato0/nextui-bmo/internal/buildinfo"
	"github.com/carroarmato0/nextui-bmo/internal/clips"
	"github.com/carroarmato0/nextui-bmo/internal/config"
	"github.com/carroarmato0/nextui-bmo/internal/devctx"
	"github.com/carroarmato0/nextui-bmo/internal/face"
	"github.com/carroarmato0/nextui-bmo/internal/hardware"
	"github.com/carroarmato0/nextui-bmo/internal/input"
	"github.com/carroarmato0/nextui-bmo/internal/mod"
	"github.com/carroarmato0/nextui-bmo/internal/observability"
	"github.com/carroarmato0/nextui-bmo/internal/perf"
	"github.com/carroarmato0/nextui-bmo/internal/power"
	"github.com/carroarmato0/nextui-bmo/internal/providers"
	"github.com/carroarmato0/nextui-bmo/internal/qr"
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
	mods := mod.Discover(modsRoot, nil)
	activeMod := mod.Active(mods, cfg.ActiveMod)
	// Open the active mod so its FS (directory via os.DirFS or .zip via
	// archive/zip) is populated before the prompt loads below read from it. On
	// failure, fall back to the default mod. Closed on exit and on every mod
	// switch (reloadMod).
	if err := activeMod.Open(nil); err != nil {
		activeMod = mod.Active(mods, mod.DefaultID)
		_ = activeMod.Open(nil)
	}
	// closePrev holds the previous mod's closer; on a switch it is closed one
	// generation later (not immediately) so a pipeline-goroutine read in flight
	// on the old FS cannot hit a closed zip reader. Both are closed on exit.
	var closePrev func() error
	defer func() {
		_ = activeMod.Close()
		if closePrev != nil {
			_ = closePrev()
		}
	}()

	// Sub-FS helpers rooting at the mod's faces/ and audio/ subtrees. fs.Sub
	// only errors on an invalid path; "faces"/"audio" are constant-valid, so the
	// ignored error is safe. For a directory mod with no faces/ or audio/, the
	// sub-FS simply errors on read → embedded fallback (matching prior behavior).
	modFacesSub := func(m mod.Mod) fs.FS { s, _ := fs.Sub(m.FS, "faces"); return s }
	modAudioSub := func(m mod.Mod) fs.FS { s, _ := fs.Sub(m.FS, "audio"); return s }

	// Persona, voice, and quotes prompts use the override-or-default model:
	// the built-in defaults are the source of truth; an override file is used
	// only when it exists on disk and is non-blank. The app never creates them.
	// Paths resolve against the active mod; reloadMod (below) re-points them.
	personaPath := activeMod.PersonaPath()
	voicePath := activeMod.VoicePath()
	quotesPath := activeMod.QuotesPath()
	personaPrompt := config.LoadPromptFS(activeMod.FS, "persona.txt", config.DefaultSystemPrompt)
	voicePrompt := config.LoadPromptFS(activeMod.FS, "voice.txt", config.DefaultTTSInstructions)

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
	settingsMenu.SetAbout(buildAboutState(logger))
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
		return devctx.ParseQuotes(config.LoadPromptFS(activeMod.FS, "quotes.txt", config.DefaultQuotes))
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
		clipLib := clips.NewLibrary(modAudioSub(activeMod))
		clipPlayer = clips.NewPlayer(audioSession, audioCfg.SampleRate, audioCfg.PlaybackChannels, clipLib)
	}

	var audioRouter *audio.CaptureRouter
	var audioPipeline *assistant.VoicePipeline
	var stopPTT func()
	var stopWake func()
	var restartWake func(mod.Mod)
	var wakeCleanup func()
	// currentWakeID identifies the wake classifier the live detector was built
	// for, so reloadMod can skip an onnxruntime rebuild when a mod switch does not
	// actually change the model. Maintained by restartWake.
	var currentWakeID string
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
				return config.LoadPromptFS(activeMod.FS, "voice.txt", config.DefaultTTSInstructions)
			})
			audioPipeline.SetSystemPromptSource(func() string {
				return systemPromptWithContext(
					config.LoadPromptFS(activeMod.FS, "persona.txt", config.DefaultSystemPrompt),
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
				disk := face.EmotionFaceNamesInFS(modFacesSub(activeMod))
				return assistant.BuildEmotionVocabulary(builtin, disk, activeMod.Manifest.Emotions)
			})
			if clipPlayer != nil {
				preLib := clips.NewLibrary(modAudioSub(activeMod))
				audioPipeline.SetTimeoutClip(preLib.Load("timeout"))
				audioPipeline.SetErrorClip(preLib.Load("error"))
			}
			stopPTT = startPushToTalk(ctx, logger, machine, cfg, hardwareProfile, audioRouter, audioPipeline, audioCfg.SampleRate, audioCfg.Channels, func() bool { return activeMenu != nil })

			pakDir := strings.TrimSpace(os.Getenv("BMO_PAK_DIR"))
			tmpDir := filepath.Join(os.TempDir(), "BMO")
			_ = os.MkdirAll(tmpDir, 0o755)
			gov := &power.Governor{Logf: logger.Warnf}
			// restartWake (re)builds the wake detector for a mod, resolving its
			// optional custom wake model. Called at startup and on every mod
			// switch (synchronously, on the main goroutine) so the wake word
			// changes with the mod, like the face does.
			restartWake = func(m mod.Mod) {
				if stopWake != nil {
					stopWake() // cancels the loop and Close()s the old detector
				}
				if wakeCleanup != nil {
					wakeCleanup() // remove the previous mod's extracted temp model
				}
				var assets wakeAssets
				assets, wakeCleanup = buildWakeAssets(m, pakDir, platform, tmpDir, logger)
				stopWake = startWakeWord(ctx, logger, machine, cfg, audioRouter, audioPipeline, gov, assets, audioCfg.SampleRate, audioCfg.Channels, nil)
				currentWakeID = wakeModelIdentity(m.FS, m.ID)
			}
			restartWake(activeMod)
		}
	}
	if stopPTT != nil {
		defer stopPTT()
	}
	defer func() {
		if stopWake != nil {
			stopWake()
		}
	}()

	screen, err := renderer.NewFullscreen("BMO")
	if err != nil {
		return fmt.Errorf("create renderer: %w", err)
	}
	defer screen.Close()
	logger.Infof("renderer ready: %s", screen.DebugInfo())

	faceLib := face.NewLibraryMode(modFacesSub(activeMod), activeMod.SelfContained())
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
	// scheduler is created further down (it needs the idle seed), but reloadMod
	// must update its available face set when the mod changes at runtime, so it
	// is declared here and assigned later; the nil guard covers a mod swap that
	// somehow precedes scheduler creation.
	var scheduler *assistant.IdleScheduler

	reloadMod := func(id string) {
		active := mod.Active(mods, id)
		if err := active.Open(logger.Warnf); err != nil {
			logger.Warnf("open mod %q: %v; keeping default", id, err)
			active = mod.Active(mods, mod.DefaultID)
			_ = active.Open(logger.Warnf)
		}
		// Publish the new mod's FS before closing the old one, and defer the old
		// close by one generation so an in-flight read on the previous FS finishes
		// against a still-open reader. The generation two switches back is safe to
		// close now. (Directory mods Close to a no-op; this only matters for zips.)
		prev := activeMod
		activeMod = active
		if closePrev != nil {
			_ = closePrev()
		}
		closePrev = prev.Close
		personaPath = active.PersonaPath()
		voicePath = active.VoicePath()
		quotesPath = active.QuotesPath()

		newLib := face.NewLibraryMode(modFacesSub(active), active.SelfContained())
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
			clipLib := clips.NewLibrary(modAudioSub(active))
			clipPlayer = clips.NewPlayer(audioSession, audioCfg.SampleRate, audioCfg.PlaybackChannels, clipLib)
			if audioPipeline != nil {
				audioPipeline.SetTimeoutClip(clipLib.Load("timeout"))
				audioPipeline.SetErrorClip(clipLib.Load("error"))
			}
		}
		// Rebuild the wake detector for the new mod (picks up its custom wake
		// model, or falls back to stock). Synchronous, on the main goroutine;
		// nil unless AI + audio were initialized at startup.
		// Rebuilding the wake detector recreates onnxruntime sessions, which is
		// slow enough to stall the render loop. Skip it when the new mod resolves
		// to the same wake model the live detector already runs (the common case:
		// most mods ship no custom wake word and fall back to the stock model).
		if restartWake != nil {
			if id := wakeModelIdentity(active.FS, active.ID); id == currentWakeID {
				logger.Infof("wake model unchanged (%s); keeping detector across mod switch", id)
			} else {
				restartWake(active)
			}
		}
		if scheduler != nil {
			scheduler.SetAvailable(modIdleFaces(active))
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
	var goodbyeWaitDur time.Duration

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
		// Cut any in-flight remark so the farewell clip plays alone instead of
		// mixing with a quote/proactive line on the separate speech path.
		if audioPipeline != nil {
			audioPipeline.InterruptSpeech()
		}
		if clipPlayer != nil {
			goodbyeDone = clipPlayer.PlaySequence(ctx, "goodbye")
			// Wait for the actual goodbye length so a long (e.g. modded)
			// farewell is heard in full instead of being cut at a fixed timeout.
			goodbyeWaitDur = goodbyeWait(clipPlayer.ClipDuration("goodbye"))
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
	// runtime via reloadMod). A self-contained mod cycles only the faces it
	// actually ships on disk — never the embedded-default/neutral-fold renders
	// it doesn't own — while the default BMO (or a faceless overlay mod that
	// inherits the built-in art) cycles the full canonical set.
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
		if activeMod.SelfContained() {
			for _, n := range face.FaceNamesInFS(modFacesSub(activeMod)) {
				add(n)
			}
		} else {
			for _, n := range face.CanonicalNames {
				add(n)
			}
		}
		return names
	}

	// handleNav maps decoded evdev navigation intents to menu/overlay actions.
	// Physical button layout (TrimUI, confirmed via getevent): A=BTN_EAST(305)
	// confirms the focused menu item (and is PTT outside menus, via the PTT
	// path), B=BTN_SOUTH(304) cancels, Start opens settings, Menu=BTN_MODE(316)
	// exits to NextUI, D-pad navigates, X=BTN_WEST(308) speaks a random quote,
	// Y=BTN_NORTH(307) steps the face gallery.
	handleNav := func(action input.NavAction) {
		// Once the farewell is under way, BMO is committed to exiting. A further
		// exit press (B or MENU) means "skip it": cancel the goodbye clip and any
		// speech and quit immediately and cleanly, rather than layering another
		// action — or another farewell — on top. This runs before the overlay /
		// batch / speech guards so an exit press always wins during shutdown.
		if shuttingDown && (action == input.NavCancel || action == input.NavMenu) {
			clipPlayer.Stop()
			if audioPipeline != nil {
				audioPipeline.InterruptSpeech()
			}
			running = false
			return
		}

		// While the About screen is up, any button simply returns to the
		// settings list (it never exits the app or closes the overlay).
		if am, ok := activeMenu.(interface {
			AboutActive() bool
			DismissAbout()
		}); ok && am.AboutActive() {
			am.DismissAbout()
			return
		}

		// MENU (BTN_MODE) exits to NextUI after playing the goodbye clip.
		if action == input.NavMenu {
			beginShutdown()
			return
		}

		// B is a two-stage "stop, then exit": while BMO is doing anything —
		// closing an overlay, playing a clip (e.g. the startup greeting),
		// processing a batch or speaking — the first press stops that and settles
		// back to idle. Only a B press while BMO is already idle exits to NextUI
		// after playing the goodbye clip.
		if action == input.NavCancel {
			switch {
			case activeMenu != nil:
				setActiveMenu(nil)
			case clipPlayer != nil && clipPlayer.Playing():
				// Stop the clip's audio and its speaking animation; the machine
				// is already idle during clip playback, so this returns to idle.
				clipPlayer.Stop()
				if audioPipeline != nil {
					audioPipeline.InterruptSpeech()
				}
				logger.Infof("clip stopped by B press; returning to idle")
			case audioPipeline != nil && audioPipeline.CancelBatch():
				logger.Infof("batch cancelled by B press")
			case audioPipeline != nil && audioPipeline.InterruptSpeech():
				logger.Infof("speech interrupted by B press")
			default:
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
			// Cancel any in-progress spontaneous reaction so the quote replaces
			// it (returning the machine to idle) instead of overlapping its
			// audio. A no-op when nothing is playing.
			audioPipeline.InterruptSpeech()
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
			if activeMenu != nil {
				return
			}
			// Cancel a spontaneous reaction so stepping the gallery interrupts
			// it (and frees the machine) rather than being ignored mid-speech.
			if audioPipeline != nil {
				audioPipeline.InterruptSpeech()
			}
			if machine.Snapshot().Current != assistant.StateIdle {
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
		case input.NavLeft, input.NavRight, input.NavConfirm:
			// Cancel any keyboard-edit state (no keyboard on hardware) first.
			type editable interface {
				IsEditing() bool
				CancelEdit()
			}
			if ed, ok := activeMenu.(editable); ok && ed.IsEditing() {
				ed.CancelEdit()
			}
			// A (NavConfirm) activates the focused item: it opens ABOUT, runs
			// RESTORE DEFAULTS, flips a toggle, or advances a value. Arrows only
			// adjust values left/right — they never fire pure action rows.
			var err error
			if action == input.NavConfirm {
				err = activeMenu.ToggleFocused()
			} else {
				delta := 1
				if action == input.NavLeft {
					delta = -1
				}
				err = activeMenu.Cycle(delta)
			}
			if err != nil {
				logger.Debugf("activate focused: %v", err)
			}
			// Cancel if activation entered edit mode (API key item).
			if ed, ok := activeMenu.(editable); ok && ed.IsEditing() {
				ed.CancelEdit()
			}
			// Auto-persist after every change. Validation failures (e.g. AI mode
			// without providers) are silently dropped.
			if err := commitMenu(activeMenu); err != nil {
				logger.Debugf("auto-save: %v", err)
			}
		}
	}

	scheduler = assistant.NewIdleScheduler(machine.Snapshot().IdleSeed)
	// Restrict idle to the active mod's own faces (no-op for the default set).
	scheduler.SetAvailable(modIdleFaces(activeMod))
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
		if !startupClipFired && !shuttingDown && time.Now().After(startupFaceShownAt) &&
			(animEngine.Ready(face.ExprSpeaking) || time.Since(startupFaceShownAt) > 10*time.Second) {
			startupClipFired = true
			names := []string{"hello"}
			if overrideErrs := config.CheckOverrides(activeMod.FS); len(overrideErrs) > 0 {
				for _, e := range overrideErrs {
					logger.Warnf("mod override error: %v", e)
				}
				names = append(names, "mod_error")
			}
			clipPlayer.PlaySequence(ctx, names...)
		}

		// Exit once the goodbye clip has played out (or after goodbyeWaitDur, a
		// safety timeout sized to the clip's own length), so the farewell is
		// fully heard and animated before quitting.
		if shuttingDown {
			if goodbyeDone == nil || time.Since(shuttingDownAt) > goodbyeWaitDur {
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
			if snap.WakeEngaged {
				// A hands-free wake interaction is in progress: hold the listening
				// face for the whole session, exactly like push-to-talk — no idle
				// scheduler, no proactive remarks mid-conversation. They resume on
				// the next true return to idle.
				expr = string(assistant.ExpressionListening)
				break
			}
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
			if !galleryActive && !shuttingDown && audioPipeline != nil && proactive.Due(now) {
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
		// Every amplitude-driven (lip-sync) face rests at m==0 as some distinct
		// mouth pose; without bridging, that rest pose flashes on every
		// inter-syllable gap while speaking (the regression first seen on
		// excited/angry, then unamused below the talkmouth). Keep the mouth
		// tracking the raw volume so it still reacts naturally, but never let it
		// drop below a thin opening while the release envelope is settling — this
		// bridges the gaps without flattening the dynamics, then eases shut to the
		// rest pose once the audio truly stops. Gating on IsAmplitude applies this
		// uniformly to built-in and mod faces alike (no hardcoded name list).
		speakAmp := rawAmp
		if animEngine.IsAmplitude(expr) {
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

// aboutProjectURL is the repository the About screen's QR code points to.
const aboutProjectURL = "https://github.com/carroarmato0/nextui-bmo-pak"

// buildAboutState assembles the static About-screen content for this build. The
// QR code is generated once; if encoding fails the screen still renders (just
// without the code) so About is never a hard dependency on the QR library.
func buildAboutState(logger *observability.Logger) ui.AboutState {
	matrix, err := qr.Matrix(aboutProjectURL)
	if err != nil {
		logger.Warnf("about: QR generation failed: %v", err)
	}
	return ui.AboutState{
		Name: "BMO",
		Description: []string{
			"AN INTERACTIVE BMO COMPANION",
			"FOR NEXTUI HANDHELDS",
		},
		Version: buildinfo.VersionString(),
		Attribution: []string{
			"SOME ARTWORK INSPIRED BY",
			"CHERRY HONEY'S (@CHERRYHONEY)",
			"BMO FACE TEMPLATES/ASSETS,",
			"USED WITH PERMISSION.",
		},
		URL: aboutProjectURL,
		QR:  matrix,
	}
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
			Spacer:   item.Spacer,
			Indent:   item.Indent,
		})
	}
	out := &renderer.OverlayState{
		Visible:    true,
		Title:      src.Title,
		Subtitle:   append([]string(nil), src.Subtitle...),
		Items:      items,
		Footer:     src.Footer,
		FocusIndex: src.FocusIndex,
	}
	if src.About != nil {
		out.About = &renderer.AboutState{
			Name:        src.About.Name,
			Description: append([]string(nil), src.About.Description...),
			Version:     src.About.Version,
			Attribution: append([]string(nil), src.About.Attribution...),
			URL:         src.About.URL,
			QR:          src.About.QR,
		}
	}
	return out
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
