# BMO AI Assistant — Design Spec

**Date:** 2026-06-10  
**Status:** Draft

## 1. Overview

BMO is a fullscreen NextUI Pak for TrimUI handhelds that presents a charming animated face inspired by the Adventure Time character and acts as a voice-first AI assistant.

The app should feel like a playful desk companion: the face is always visible, idle animations keep it lively, and voice interactions should be fast, smooth, and easy to understand on small screens.

## 2. Goals

1. Show a scalable fullscreen BMO face on TrimUI Brick and TrimUI Smart Pro.
2. Support voice interaction through microphone input and text-to-speech output.
3. Provide first-run setup for selecting AI providers/models and entering API keys, or, not use AI at all and just stay in idle mode.
4. Keep the face expressive, animated, and reactive to the current conversation state.
5. Offer idle behaviors so the device feels alive when untouched.
6. Handle token exhaustion or quota errors in a playful, understandable way.
7. Keep the implementation performant and testable.
8. BMO should respond in character, taking inspiration from quotes from the cartoon series.
9. User can at any time switch between idle mode, and enabling the AI mode. If the AI mode is chosen, it needs to have a working config.

## 3. Non-Goals

- No local on-device model hosting for the first version.
- No attempt to perfectly clone any copyrighted voice actor.
- No complex chat history management beyond what is needed for the assistant experience.
- No heavy 3D rendering or expensive animation framework.
- No background always-on wake-word daemon in the initial MVP unless the platform proves it is practical.

## 4. Product Direction

BMO should not feel like a generic chat app with a mascot pasted on top. The face is the product.

The core loop is:

1. User wakes the app or triggers listening.
2. BMO switches to a listening expression.
3. Speech is captured and transcribed.
4. The user’s request is sent to the configured AI provider.
5. BMO thinks, then replies with speech and matching facial animation.
6. When idle, BMO blinks, looks around, whistles, smiles, dozes off, or otherwise behaves like a companion.
7. BMO should react to user putting the device to sleep and when waking up.

## 5. Assumptions

- The app will be built as a NextUI Pak.
- Go + SDL2 is the preferred implementation stack because it matches the surrounding NextUI ecosystem and can render efficiently on handheld hardware.
- The assistant will use external AI services for STT, LLM, and TTS unless the user explicitly configures a compatible local endpoint.
- The app should work without a microphone by still allowing text entry or a manual interaction fallback.
- Wake-word support is desirable, but push-to-talk must exist as the reliable baseline.

## 6. Recommended MVP Scope

The first implementation should include:

- A fullscreen animated BMO face
- First-run setup
- Provider/model selection for STT, chat, and TTS
- API key entry
- Push-to-talk voice interaction
- Conversation state transitions with appropriate expressions
- Idle animation system
- Sleep state for quota exhaustion or rate-limit lockout
- Settings screen for changing providers/models/keys later

Optional for later iterations:

- Wake-word listening
- Conversation memory summaries
- Multiple voice presets
- Sound effects and more nuanced facial animation
- Local/offline fallback models

## 7. Architecture

### 7.1 High-level structure

The application should be split into four layers:

1. **UI layer**
   - Fullscreen renderer for BMO face, overlays, and menus
   - Handles controller input and visual state

2. **State layer**
   - Tracks app state such as idle, listening, thinking, speaking, sleeping, and error
   - Chooses idle expressions and transitions

3. **Audio/AI layer**
   - Microphone capture
   - Speech-to-text provider integration
   - LLM request orchestration
   - Text-to-speech synthesis and playback

4. **Persistence layer**
   - Saves configuration, provider choice, API keys, and user preferences
   - Stores minimal state needed across launches

### 7.2 Preferred implementation style

Use an event-driven loop with background workers for slow operations.

The UI thread should never block on network calls or long audio work. Instead:

- capture audio in background
- transcribe asynchronously
- call the language model asynchronously
- synthesize speech asynchronously
- send small state updates back to the renderer

This keeps animations smooth and avoids input lag.

## 8. Screen and Flow Design

### 8.1 Startup flow

On first launch, BMO should present a setup wizard before entering the main face screen.

Required steps:

1. Choose provider family for STT / chat / TTS
2. Enter API key
3. Select STT model
4. Select chat model
5. Select TTS model / voice
6. Run a connectivity test and a short voice preview
7. Save settings and enter the main experience

If the user later changes providers or keys, the setup flow should be available from settings.

### 8.2 Main screen

The default interface is BMO’s face rendered fullscreen.

The screen should include only minimal overlays:

- small status indicators when needed
- listening/thinking/speaking state cues
- a clock/timer indicator when asleep or quota-limited
- optional subtle hint for how to start talking

The face should always scale to the device resolution and maintain aspect ratio.

### 8.3 Settings screen

The settings screen should allow the user to modify:

- STT provider and model
- LLM provider and model
- TTS provider and model/voice
- API key(s)
- push-to-talk vs wake-word mode
- audio input device selection if needed
- verbosity / personality tone presets
- animation intensity / reduced motion preference
- Switch between Idle mode, or enable the AI models

## 9. Provider Model Strategy

The app should not hardcode one AI service.

Instead, define provider interfaces for:

- STT
- Chat / LLM
- TTS

Each provider should expose:

- supported models
- authentication requirements
- optional base URL
- request/response translation
- error classification

### 9.1 Provider recommendations

The spec should support both of these classes:

- OpenAI-compatible providers and self-hosted compatible endpoints
- Provider-specific APIs where necessary

This gives the widest chance of compatibility on the handheld while keeping the app future-proof.

### 9.2 Model selection

The first-run wizard should allow the user to choose separate models for:

- speech-to-text
- conversational response generation
- text-to-speech

A single provider may power all three, but the configuration must still keep them separable.

### 9.3 API key handling

API keys must be stored locally and redacted from logs.

The app should support:

- empty key entry when the provider does not need one
- validation via a test call
- safe replacement when the user updates the key

## 10. Voice and Personality

BMO’s speaking style should feel playful, short, and expressive.

Important notes:

- The app should aim for a BMO-inspired character voice, not a direct impersonation of a real performer.
- Voice character should come from the selected TTS voice plus response formatting and playback pacing.
- Responses should be concise by default unless the user asks for more detail.

### 10.1 Voice behaviour

When speaking:

- mouth animation should stay in sync with audio amplitude or phoneme timing
- expression should shift with tone, e.g. cheerful, curious, surprised, concerned
- brief pauses should be reflected visually

### 10.2 Personality presets

The app may expose a small set of tone profiles later, for example:

- cheerful
- sleepy
- curious
- helpful
- dramatic

The initial spec should support the concept even if only one preset ships first.

## 11. Facial Animation System

The face should be treated as a lightweight animation state machine, not a static image.

### 11.1 Core facial regions

The BMO face should be composed of layered pieces such as:

- head/body silhouette
- eyes
- eyelids
- eyebrows or expression lines
- mouth
- optional cheeks / highlights / screen details
- overlay icons for state cues when needed

### 11.2 Supported expressions

At minimum, the animation system should support the expression families documented in the renderer spec: neutral/idle, blink, looking around, smiling, listening, thinking, speaking, laughing, sleepy/asleep, surprised, error/confused, and token exhausted/waiting.

The exact canonical names, aliases, and geometry rules live in `docs/specs/2026-06-10-bmo-renderer-spec.md` and should be treated as the source of truth for implementation.

### 11.3 Idle scheduler

When the app is not actively interacting, it should randomly select from idle expressions on a timed schedule.

Rules:

- avoid repeating the same idle expression too often
- keep blinking frequent but subtle
- occasionally perform a larger idle action like looking around or yawning
- allow the idle loop to be interrupted immediately by user input

### 11.4 Speaking animation

During speech playback:

- mouth should animate continuously from audio amplitude or timing cues
- the face should occasionally shift expression in response to sentiment or punctuation
- speaking animation should remain cheap enough to maintain smooth FPS

If phoneme timing is unavailable, amplitude-based mouth motion is acceptable for the first version.

## 12. Audio Pipeline

### 12.1 Microphone input

The app should support microphone capture on devices that expose it.

Baseline behavior:

- push-to-talk starts recording
- release or confirm ends recording
- recording is sent to STT

If wake-word support proves practical later, it should be an opt-in mode and not replace the baseline interaction flow.

### 12.2 Speech-to-text

Recorded speech is transcribed using the selected STT provider/model.

The transcript should be shown briefly on-screen if helpful, but the main visual focus remains BMO’s face.

### 12.3 Chat generation

The transcript and a small system prompt are sent to the chosen chat model.

The system prompt should define:

- BMO’s playful personality
- concise answer style by default
- safety/clarity preferences
- no overlong, essay-like replies unless asked

### 12.4 Text-to-speech

The chat response is sent to the configured TTS provider.

The TTS output should drive:

- audio playback
- speaking animation timing
- the transition back to idle after playback completes

## 13. Token Exhaustion / Quota Handling

If a provider returns an out-of-quota, billing, or rate-limit condition, BMO should respond in a fun but clear way.

### 13.1 Expected behaviour

- enter a sleepy or resting state
- show a clock icon or timer indicator
- display a message explaining when service may become available again if known
- if the retry time is not known, say so honestly and keep the message playful

### 13.2 Error classes

At least these cases should be distinguished:

- rate limit / temporary retry
- authentication failure
- quota exhausted / token depletion
- network unavailable
- provider unavailable
- malformed response / decode failure

Different states can map to different expressions, but they should all remain understandable.

## 14. Scaling and Performance

The app must scale cleanly across the target handheld resolutions.

### 14.1 Layout requirements

- no fixed-pixel face sizing
- use screen-relative dimensions
- maintain visual balance on both Brick and Smart Pro
- support widescreen and taller aspect ratios without distortion

### 14.2 Performance requirements

- keep the render loop light
- avoid blocking network calls on the UI thread
- reuse textures where possible
- minimize allocations inside hot animation/render paths
- keep frame timing stable during idle animation

## 15. Accessibility and Usability

The UI should remain usable even with minimal text.

Requirements:

- clear visual distinction between states
- high contrast for text overlays
- predictable controller mapping
- a way to disable motion-heavy idle animations if needed
- no dependence on tiny text for core interaction

## 16. Configuration and Persistence

Persist the following at minimum:

- provider selections
- model selections
- API key(s)
- voice/personality preference
- push-to-talk vs wake-word mode
- reduced-motion setting
- any other stable UI preference required by the assistant

Do not persist temporary conversation text unless explicitly designed later.

### 16.1 Future extensibility

The architecture should leave room for a future character-customization mode where the face assets can be replaced with user-provided art or an alternate mascot.

The architecture should also leave room for a custom system prompt that changes the assistant’s behavior and tone. Because that prompt may be too long for comfortable entry in the UI, the preferred editing path for that field is the config file rather than an on-device text form.

If a custom prompt is present, the docs should define whether it replaces the built-in persona prompt entirely or is layered on top of it; the implementation should follow one clearly documented rule rather than mixing both implicitly.

If and when these features are added, the user-facing documentation should explain:

- how to replace character assets safely
- where the custom system prompt lives in the config file
- how to restore the defaults if the customization breaks the experience
- what is supported in the UI versus what is intentionally config-file-only

## 17. Packaging, Release, and Deployment

The BMO pak should follow the same general release workflow used by the existing NextUI paks in the parent directory.

### 17.1 Target platforms

BMO is currently targeted at:

- TrimUI Brick
- TrimUI Smart Pro

Both devices use the same NextUI platform family for this project: `tg5040`.

### 17.2 Release artifact format

The project should produce a distributable pak directory and a zip archive suitable for release.

The release process should be able to assemble:

- a deployable pak directory containing `launch.sh`, `pak.json`, the app binary, assets, and any bundled libraries
- a release archive that can be copied to a device or used for SD-card installation

### 17.3 Deployment workflow

The preferred workflow is to build locally and deploy to a connected handheld over ADB.

The deployment process should:

- build the tg5040 target
- assemble the pak directory under `dist/`
- push the release directory to the device over ADB
- install to the standard NextUI tools location on the device, under the tg5040 pak path
- allow repeatable redeploys without manual cleanup whenever possible

A manual SD-card copy path may be supported as a fallback, but ADB deployment is the primary target.

### 17.4 Consistency with other paks

Packaging should take inspiration from the existing Itch.io pak in the parent directory:

- separate build and release steps
- launch script responsible for runtime environment setup
- platform-specific runtime layout if needed
- simple shell-based deploy flow for the connected device

## 18. Testing Strategy

The project should be test-driven where practical.

### 18.1 Pure logic tests

Cover:

- config load/save
- provider selection logic
- idle expression scheduling
- state transitions
- token-exhaustion handling
- model validation rules

### 18.2 Integration tests

Use local fake HTTP servers to test:

- STT adapter requests
- chat provider requests
- TTS provider requests
- error classification and retry behavior

### 18.3 UI smoke tests

If possible, add at least one render smoke test or headless validation for the fullscreen screen composition.

The test suite should be able to guard against regressions without requiring a physical handheld for every run.

### 18.4 Performance testing and diagnostics

The project should also collect enough performance information to identify bottlenecks on target devices.

At minimum, the implementation should support:

- frame timing and UI responsiveness measurements
- audio capture / STT / LLM / TTS latency breakdowns
- memory and CPU sampling during assistant interactions
- optional benchmark or replay runs for regression comparison
- logging at configurable verbosity levels so request/response flow can be debugged when needed
- redacted conversation logs that preserve troubleshooting value without leaking secrets
- a file-based profiling toggle, similar to the Itch.io pak’s `.profile-flags` workflow, so profiling can be enabled without changing the normal launch path
- both offline profile capture and live profiling when useful, with the same general CPU/memory/pprof split used by the Itch.io pak

The primary runtime log should live at `/mnt/SDCARD/.userdata/<PLATFORM>/logs/BMO.txt`, where `<PLATFORM>` matches the active system target (for example `tg5040` or `tg5050`).

Performance checks should be lightweight enough to run during development and in CI where practical, with deeper profiling available on-device when needed.

## 19. Risks and Open Questions

1. **Wake-word feasibility on device**  
   The Brick and Smart Pro may support microphone input, but the exact capture path and wake-word reliability need validation.

2. **Provider differences**  
   Not all AI providers will expose STT, chat, and TTS in a compatible way. The abstraction must be flexible enough to support mixed providers.

3. **Voice identity**  
   The desired BMO-like feel must be achieved without relying on a problematic voice clone approach.

4. **Memory / context handling**  
   The initial spec does not require long-term memory, but the architecture should leave room for it later.

5. **Device audio latency**  
   Speaking latency and microphone capture latency will affect the perceived quality more than raw model quality alone.

## 20. Proposed MVP Acceptance Criteria

The first implementation is acceptable when:

- the app launches into a fullscreen BMO face
- the user can configure providers and keys on first run
- a spoken command can be captured, transcribed, answered, and read back
- BMO visibly changes expression while listening, thinking, and speaking
- idle expressions run automatically
- quota errors switch BMO into a playful sleep state with timing feedback
- the app remains responsive and visually stable at handheld resolutions
- core logic is covered by tests

## 21. Next Step

Once this spec is approved, the next document should be an implementation plan under `docs/plans/` with bite-sized tasks, exact file paths, test cases, and verification steps.
