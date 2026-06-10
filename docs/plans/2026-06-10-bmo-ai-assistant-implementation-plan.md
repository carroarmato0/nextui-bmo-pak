# BMO AI Assistant Implementation Plan

> **For Hermes:** Use subagent-driven-development to implement this plan task-by-task.

**Goal:** Build a fullscreen NextUI Pak called BMO that can run in idle-only mode or as a voice-first AI assistant with animated expressions, provider/model setup, and playful token/quota handling.

**Architecture:** Use Go + SDL2 for a lightweight fullscreen UI, with a small state machine driving BMO’s expressions and background workers handling microphone capture, STT, chat, and TTS. Keep the UI thread non-blocking, split pure logic into headless-testable packages, and make provider/model selection configurable so the app can run either as a toy companion or as a full assistant.

**Tech Stack:** Go 1.22+, SDL2, JSON config persistence, background goroutines, httptest for provider adapters, headless unit tests for state/config logic.

---

## Scope decisions from the spec

- BMO must support two operating modes:
  - idle-only mode
  - AI mode with working STT/chat/TTS configuration
- BMO should react to sleep/wake events as part of the face/state machine.
- Wake-word support is deferred unless the platform proves it reliable; push-to-talk is the MVP interaction model.
- The face is the product: fullscreen, scalable, expressive, and cheap to render.
- Packaging targets the tg5040 NextUI family only for the current scope, covering both TrimUI Brick and TrimUI Smart Pro.
- Release and deployment should follow the same build → release → ADB push workflow used by the sibling NextUI paks.

---

## Task 1: Create the pak skeleton, runtime entrypoints, and release scaffolding

**Objective:** Establish the NextUI pak directory layout, metadata, launch flow, and the scripts needed to build/release/deploy the pak.

**Files:**
- Create: `launch.sh`
- Create: `pak.json`
- Create: `go.mod`
- Create: `cmd/bmo-pak/main_sdl.go`
- Create: `scripts/release.sh`
- Create: `scripts/deploy.sh`
- Create: `assets/README.md` or a minimal asset placeholder file if needed

**Step 1: Define the app metadata and launcher**

`pak.json` should include:
- name: `BMO`
- type: `TOOL`
- description describing the BMO companion / AI assistant
- platforms: `tg5040` only for the current scope
- release filename: `BMO.pak.zip`

`launch.sh` should:
- set `HOME` under the NextUI userdata path
- detect platform libraries like the other paks in the repo
- set `LD_LIBRARY_PATH` and `PATH`
- launch the app binary

**Step 2: Create release and deploy scripts inspired by the Itch.io pak**

`scripts/release.sh` should:
- run the test suite
- build the tg5040 target
- assemble a release directory under `dist/`
- package the release into a zip archive suitable for release/distribution

`scripts/deploy.sh` should:
- expect the release directory produced by `scripts/release.sh`
- deploy to a connected device over ADB by default
- install to the NextUI tools location for the tg5040 pak
- optionally support a manual SD-card copy path if needed

**Step 3: Create the main entrypoint**

`cmd/bmo-pak/main_sdl.go` should:
- load config from `HOME/config.json`
- decide whether to start in idle-only mode or AI mode
- initialize SDL and the renderer
- enter the main event loop

**Step 4: Run a basic build check**

Run:
```bash
go test ./...
```

Expected: the repo should at least compile once the scaffolding is in place.

---

## Task 2: Add persistent configuration and mode selection

**Objective:** Store user preferences, provider selections, and the idle-only vs AI-mode choice.

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`
- Modify: `cmd/bmo-pak/main_sdl.go`

**Step 1: Write tests for config defaults and round-tripping**

Cover:
- default mode is idle-only until configured otherwise
- config saves and reloads cleanly
- API keys are persisted but redacted from logs
- empty provider/model fields remain valid when idle-only mode is selected

**Step 2: Implement the config struct**

Include fields for:
- `Mode` (`idle` or `ai`)
- STT provider/model
- chat provider/model
- TTS provider/model
- API key(s)
- base URL / endpoint overrides
- push-to-talk vs wake-word mode
- reduced motion / animation intensity
- personality preset

**Step 3: Verify defaults**

Run:
```bash
go test ./internal/config -v
```

Expected: config tests pass, and idle-only mode works with empty AI fields.

---

## Task 3: Define the assistant state machine

**Objective:** Model BMO as a set of explicit states with transitions for idle, listening, thinking, speaking, sleeping, and error conditions.

**Files:**
- Create: `internal/assistant/state.go`
- Create: `internal/assistant/state_test.go`
- Modify: `cmd/bmo-pak/main_sdl.go`

**Step 1: Write state transition tests**

Cover transitions for:
- idle -> listening
- listening -> thinking
- thinking -> speaking
- speaking -> idle
- idle -> asleep
- asleep -> awake
- quota exhausted -> asleep
- network/provider failure -> error state

**Step 2: Implement the state machine**

State should include:
- current mode
- current expression
- timestamp of last interaction
- sleep/wake reason
- token/quota status
- idle animation selection seed

**Step 3: Verify transitions**

Run:
```bash
go test ./internal/assistant -v
```

Expected: all state transition tests pass.

---

## Task 4: Build the idle expression scheduler

**Objective:** Make BMO feel alive when the user is not interacting.

**Files:**
- Create: `internal/assistant/idle.go`
- Create: `internal/assistant/idle_test.go`
- Modify: `internal/assistant/state.go`

**Step 1: Write tests for idle behavior**

Cover:
- no immediate repetition of the same idle expression
- random selection across allowed idle expressions
- blink cadence is frequent but subtle
- large idle actions occur less often
- user input interrupts idle animation immediately

**Step 2: Implement the scheduler**

Support expressions such as:
- blink
- look around
- smile
- laugh
- sleepy
- whistle
- neutral

**Step 3: Verify the scheduler**

Run:
```bash
go test ./internal/assistant -run Idle -v
```

Expected: idle tests pass and expression selection is stable.

---

## Task 5: Create provider interfaces for STT, chat, and TTS

**Objective:** Make AI providers swappable instead of hardcoding one vendor.

**Files:**
- Create: `internal/providers/provider.go`
- Create: `internal/providers/provider_test.go`
- Create: `internal/providers/openai_compatible.go`
- Create: `internal/providers/errors.go`

**Step 1: Write interface-focused tests**

Cover:
- provider capability detection
- model listing
- auth requirements
- error classification for quota/auth/network/provider failures

**Step 2: Implement shared interfaces**

Define separate interfaces for:
- `STTProvider`
- `ChatProvider`
- `TTSProvider`

Each provider should expose:
- supported models
- validation/test call
- request execution
- error categorization

**Step 3: Add at least one OpenAI-compatible adapter path**

This gives the MVP a broad compatibility baseline and supports many hosted or self-hosted setups.

**Step 4: Verify**

Run:
```bash
go test ./internal/providers -v
```

Expected: provider abstraction tests pass.

---

## Task 6: Add the first-run setup flow

**Objective:** Let the user select idle-only or AI mode and, if AI mode is chosen, configure providers and models before entering the main experience.

**Files:**
- Create: `internal/ui/screen_setup.go`
- Create: `internal/ui/screen_settings.go`
- Create: `internal/ui/screen_setup_test.go`
- Modify: `cmd/bmo-pak/main_sdl.go`

**Step 1: Write screen-flow tests**

Cover:
- first launch enters setup when AI mode is unconfigured
- idle-only mode can be selected and saved without provider configuration
- AI mode requires a valid provider configuration before exit
- settings can reopen setup later

**Step 2: Implement the setup wizard**

Wizard steps should include:
- select idle-only or AI mode
- choose provider family
- enter API key
- select STT model
- select chat model
- select TTS model / voice
- run a connectivity test
- save and continue

**Step 3: Verify the flow**

Run:
```bash
go test ./internal/ui -run Setup -v
```

Expected: setup flow tests pass.

---

## Task 7: Build the fullscreen BMO renderer

**Objective:** Render BMO’s face fullscreen, scaled correctly for the device, with cheap state-driven animations.

**Files:**
- Create: `internal/ui/bmo_face.go`
- Create: `internal/ui/layout.go`
- Create: `internal/ui/layout_test.go`
- Create: `internal/renderer/renderer.go`
- Create: `internal/ui/bmo_face_test.go`

**Step 1: Write tests for layout scaling**

Cover:
- Brick / Smart Pro screen sizes scale cleanly
- the face remains centered and proportional
- overlay text does not crowd the face
- reduced-motion mode changes animation intensity

**Step 2: Implement layout helpers**

Use screen-relative measurements rather than fixed pixel sizes.

**Step 3: Implement face layers and expressions**

The renderer should support layered pieces such as:
- silhouette
- eyes
- eyelids
- eyebrows / expression lines
- mouth
- optional state icon overlay

**Step 4: Verify the renderer logic**

Run:
```bash
go test ./internal/ui -v
```

Expected: layout and face rendering tests pass.

---

## Task 8: Add audio capture and playback plumbing

**Objective:** Capture microphone input, send it through STT, and play back TTS output without blocking the UI thread.

**Files:**
- Create: `internal/audio/capture.go`
- Create: `internal/audio/playback.go`
- Create: `internal/audio/audio_test.go`
- Modify: `cmd/bmo-pak/main_sdl.go`

**Step 1: Write audio pipeline tests**

Cover:
- capture starts and stops cleanly
- playback handles empty / invalid buffers safely
- errors propagate into the state machine
- long-running work does not block the main loop

**Step 2: Implement background workers**

Keep capture, STT, chat, and TTS work off the UI thread.

**Step 3: Verify**

Run:
```bash
go test ./internal/audio -v
```

Expected: audio plumbing tests pass.

---

## Task 9: Add token/quota exhaustion handling

**Objective:** Present rate-limit and quota errors as a playful sleep state with timing feedback.

**Files:**
- Create: `internal/assistant/quota.go`
- Create: `internal/assistant/quota_test.go`
- Modify: `internal/assistant/state.go`
- Modify: `internal/ui/bmo_face.go`

**Step 1: Write classification tests**

Cover:
- rate limit / retry after
- quota exhausted
- authentication failure
- provider unavailable
- network unavailable
- malformed response

**Step 2: Implement the sleepy/error presentation**

When quota is exhausted:
- BMO falls asleep
- a clock/timer indicator appears
- the UI explains the wait if a retry time is known

**Step 3: Verify**

Run:
```bash
go test ./internal/assistant -run Quota -v
```

Expected: quota classification tests pass.

---

## Task 10: Wire the main event loop and user actions

**Objective:** Connect UI input, state transitions, idle mode, and AI mode into one smooth application loop.

**Files:**
- Modify: `cmd/bmo-pak/main_sdl.go`
- Modify: `internal/ui/bmo_face.go`
- Modify: `internal/assistant/state.go`

**Step 1: Define user input mapping**

Support:
- push-to-talk start/stop
- open settings/setup
- switch between idle-only and AI mode
- wake/sleep interactions

**Step 2: Ensure the UI loop never blocks**

All slow operations must happen in goroutines or worker pipelines.

**Step 3: Verify responsiveness**

Run the app in a smoke-test build and confirm the face continues to animate during idle and waiting states.

---

## Task 11: Add logging, diagnostics, and performance instrumentation

**Objective:** Make troubleshooting practical by capturing configurable logs, request/response flow, and timing data without leaking secrets.

**Files:**
- Create: `internal/observability/logger.go`
- Create: `internal/observability/logger_test.go`
- Create: `internal/observability/perf.go`
- Create: `internal/observability/perf_test.go`
- Modify: `cmd/bmo-pak/main_sdl.go`
- Modify: provider and assistant worker entrypoints as needed to emit structured events

**Step 1: Write tests for logging and redaction**

Cover:
- log level filtering
- API key / secret redaction
- request and response summaries are still useful when verbose logging is enabled
- chat / STT / TTS payloads can be logged safely at a debug level without exposing secrets

**Step 2: Implement observability helpers**

Support:
- configurable log levels such as error, info, debug, and trace
- request / response timing markers for STT, chat, and TTS calls
- frame timing and UI loop duration sampling
- lightweight counters for assistant states and failures
- optional benchmark or replay hooks for regression comparisons
- a log file under the user data path at `/mnt/SDCARD/.userdata/<PLATFORM>/logs/BMO.txt`, with a `logs` command that can stream the latest session the way the Itch.io pak does
- redaction of API keys / tokens before anything reaches disk or stdout

**Step 3: Verify observability behavior**

Run:
```bash
go test ./internal/observability -v
```

Expected: log redaction tests pass, and the performance helper APIs are available for later profiling and regression work.

**Step 4: Define profiling workflow**

Mirror the Itch.io pak’s two styles of profiling:
- file-based profiling enabled by a small marker file in the pak directory, read by `launch.sh`
- live pprof when remote inspection is needed

Make the workflow explicit enough that the user can:
- enable profiling without editing code
- launch the app normally from NextUI
- pull CPU and memory profiles after a session
- remove profiling flags and return to normal launch behavior

---

## Task 12: Add end-to-end tests for the provider adapters

**Objective:** Validate the provider layer against fake HTTP servers before real API calls are involved.

**Files:**
- Create: `internal/providers/openai_compatible_test.go`
- Create: `internal/providers/fake_server_test.go`
- Modify: `internal/providers/provider.go`

**Step 1: Write httptest coverage**

Cover:
- request formatting
- response parsing
- auth header handling
- retry/error classification
- model selection behavior

**Step 2: Verify provider adapters**

Run:
```bash
go test ./internal/providers -v
```

Expected: adapter tests pass against fakes.

---

## Task 13: Package verification and ADB deployment checks

**Objective:** Make sure the pak boots, packages cleanly, and deploys to a connected tg5040 device over ADB.

**Files:**
- Modify: `launch.sh`
- Modify: `pak.json`
- Create or modify: `scripts/release.sh`
- Create or modify: `scripts/deploy.sh`
- Add: any required assets under `assets/`

**Step 1: Verify launcher behavior**

Check that `launch.sh` sets the correct `HOME`, library path, and executable path.

**Step 2: Verify the package metadata**

Ensure the pak metadata matches the final app name, the tg5040-only target scope, and the release naming.

**Step 3: Verify release/deploy scripts**

Check that `scripts/release.sh` produces the expected dist/ layout and zip archive, and that `scripts/deploy.sh` pushes the released pak to a connected ADB device in the correct NextUI tools directory.

**Step 4: Run final verification**

Run:
```bash
go test ./...
```

If you have a handheld build environment available, also run the standard pak build/release/deploy process used by the other NextUI projects in this repository.

Expected: all tests pass, and the pak layout is ready for release and ADB deployment.

---

## Implementation order

1. Skeleton and launcher
2. Config and mode selection
3. State machine
4. Idle scheduler
5. Provider interfaces
6. Setup/settings flow
7. Renderer and face layers
8. Audio plumbing
9. Quota/sleep handling
10. Main loop wiring
11. Logging, diagnostics, and performance instrumentation
12. Provider adapter integration tests
13. Packaging verification

---

## Notes for later refinement

- If wake-word support is validated on the target devices, add it after the MVP as an opt-in path.
- If you later want a richer BMO voice, keep it in the TTS layer and not in the UI logic.
- If the face asset pipeline becomes more complex, split sprite composition and expression state into separate packages before adding more animation states.
- Leave room for a future character-customization mode where users can swap BMO’s assets for other mascots without rewriting the assistant core.
- If a custom system prompt is added later, define one explicit policy for how it interacts with the built-in persona prompt instead of letting both behave implicitly.
- Prefer config-file editing over a UI textbox for custom prompts so longer prompts remain practical on-device.
- Follow the Itch.io pak’s logging/profiling pattern: a levelled log file, redaction, and `.profile-flags`-style toggles for CPU/memory/live pprof.
- Plan user-facing documentation for any customization features so the default experience, the customization rules, and the recovery path stay clear.
