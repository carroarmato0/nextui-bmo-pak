# BMO Renderer Specification

**Date:** 2026-06-10  
**Status:** Draft  
**Scope:** Procedural fullscreen face renderer for the NextUI BMO pak

## 1. Purpose

This document defines the renderer behavior for BMO: a low-cost, fully scalable, expressive face renderer that can run on TrimUI Brick and TrimUI Smart Pro-class devices without requiring raster face assets.

The renderer should make BMO feel alive even when the app is not actively in a conversation.

## 2. Rendering Principles

1. The face is the product.
2. All geometry must scale from screen-relative ratios, not fixed pixels.
3. Expression changes should be readable at a glance on small displays.
4. Motion should stay cheap enough for smooth frame pacing.
5. The renderer should work in two builds:
   - accelerated SDL2 path when cgo is available
   - pure-Go framebuffer path when cgo is disabled
6. The visual language should stay simple, round, and playful.
7. The face should remain recognizable even when reduced motion is enabled.

## 3. Visual Baseline

The renderer is based on the BMO reference sheets supplied by the user:

- A square mint-green body/face
- Small high-contrast eyes
- Mouth as the primary emotional carrier
- Occasional eyebrows, tears, blush, stars, hearts, or stress marks for stronger reactions
- Rare overlays such as a clock icon or “back soon” style cue for unavailable/quota states

The renderer should not try to mimic a photoreal or complex cartoon rig. It should emulate the clean tile-like expression style from the references.

## 4. Coordinate and Layout Model

### 4.1 Screen-relative sizing

The renderer must derive every layout value from the active output size.

Required supported targets:
- 640x480-class screen layouts
- 1280x720-class screen layouts
- other similar handheld ratios without distortion

### 4.2 Layout anchors

Use these rough anchors as the canonical facial map:

- Eye line: upper-middle of the face
- Brow line: above the eyes, only used for expressive states
- Mouth line: lower-middle of the face
- Clock/timeout icon: corner overlay, small and unobtrusive

### 4.3 Relative proportions

These are the intended proportions for the default face:

- Face occupies almost the full screen with a modest margin
- Eyes remain small and widely spaced
- Mouth is centered horizontally, below the eyes
- Brows sit high enough to read clearly but not crowd the eyes
- Corner icon must never obscure the face

### 4.4 Recommended geometry ratios

For a screen of width W and height H:

- margin: based on the shorter side, about 1/18 of the short dimension
- eye width: about 1/5 of W
- eye height: about 1/4 of H
- eye gap: about 1/14 of W
- mouth width: about 1/4 of W
- mouth height: about 1/8 of H
- brow height: very thin, about 1/36 of H
- clock icon size: about 1/8 of the short dimension

The renderer may clamp these to safe min/max bounds so the face stays legible on both small and large outputs.

## 5. Expression Model

The renderer should map assistant state to a small expression vocabulary.

### 5.1 Core expression enum

The canonical expressions are:

- neutral
- blink
- listening
- thinking
- speaking
- smile
- laugh
- whistle
- sleeping
- concerned
- look_around

### 5.2 Visual meaning of each expression

#### neutral
- Eyes: small dots or simple ovals
- Mouth: tiny curved smile or line
- Use: home state, default idle, calm companion mode

#### blink
- Eyes: closed or nearly closed for one frame sequence
- Mouth: unchanged from the underlying state
- Use: idle motion, transition blinks, low-cost life signal

#### listening
- Eyes: alert, open, centered
- Mouth: closed line or tiny smile
- Use: while the user is recording speech or a wake/listen action is active

#### thinking
- Eyes: slightly narrowed or offset with subtle brow tilt
- Mouth: flat line
- Use: STT wait, LLM processing, “hmm” moments

#### speaking
- Eyes: open and stable
- Mouth: animated open shapes driven by amplitude or timing
- Use: TTS playback

#### smile
- Eyes: dot eyes or relaxed open eyes
- Mouth: friendly smile
- Use: greeting, success, warm companion feedback

#### laugh
- Eyes: squinting or closed-smile arcs
- Mouth: open smile
- Use: joke response, delight, playful reactions

#### whistle
- Eyes: neutral or slightly softened
- Mouth: rounded whistling shape
- Use: idle whistling, musical state, light whimsy

#### sleeping
- Eyes: closed or half-lidded
- Mouth: relaxed smile or neutral line
- Use: idle sleep, overnight doze, quota exhaustion fallback

#### concerned
- Eyes: narrowed or slightly downturned
- Mouth: frown
- Brows: tilted to indicate concern or frustration
- Use: provider failure, error, confused assistant response

#### look_around
- Eyes: centered but slightly shifted over time
- Mouth: neutral
- Use: idle curiosity, desk-companion feel, occasional ambient motion

## 6. Eye and Mouth Construction

### 6.1 Eyes

The renderer should support:
- dot eyes
- oval eyes
- squint eyes
- closed eye arcs
- large glossy eyes for rare emotional peaks

Eyes should be drawn in high-contrast dark ink.

### 6.2 Mouths

The renderer should support at least these mouth variants:
- neutral line
- small smile
- large open grin
- frown
- whistle / rounded mouth
- open speaking mouth

Optional internal highlights or tongue shapes are allowed if they remain cheap to draw.

### 6.3 Expression accents

Secondary accents may be added for rare states:
- eyebrow slants for anger or concern
- tiny tear streaks for sadness or exhaustion
- blush dots for affection or delight
- stars or hearts for celebratory states
- stress marks or spirals for confusion

These accents should be used sparingly so the face remains readable.

## 7. Assistant State to Renderer Mapping

### 7.1 Idle state mapping

Default idle should use a small rotation of:
- neutral
- smile
- look_around
- blink
- whistle

Rules:
- no immediate repetition of the same idle expression when avoidable
- blinking should be frequent but subtle
- larger idle actions should be rarer than micro-expressions

### 7.2 Listening mapping

When listening is active:
- use listening as the base expression
- keep the mouth minimal
- favor eye focus and visual readiness over motion

### 7.3 Thinking mapping

When waiting on STT, LLM, or any external response:
- use thinking as the base expression
- keep motion restrained
- allow tiny eye drift or a slow blink

### 7.4 Speaking mapping

While TTS audio is playing:
- use speaking as the base expression
- animate the mouth continuously
- optionally drift between speaking, smile, and laugh if the response is upbeat

If phoneme timing is unavailable, amplitude-based mouth animation is sufficient.

### 7.5 Sleep and quota mapping

If the app is sleeping, rate-limited, or quota-exhausted:
- use sleeping as the base expression
- add a small clock icon in a screen corner
- keep the overlay clear and playful
- if a retry time is known, show it; otherwise, keep the message honest and simple

### 7.6 Error mapping

For user-visible failures:
- authentication issues should read as concerned or disappointed
- network failures should read as concerned or tired
- decode/malformed responses should read as confused or concerned

## 8. Animation Rules

### 8.1 Idle cadence

The renderer should support a timed idle loop with several cadence bands:
- fast: blink and tiny micro-motions
- medium: look-around or smile shifts
- slow: sleepy or whistle moments

The animator must be interruptible immediately by user input.

### 8.2 Speaking mouth animation

Speaking mouth animation should cycle through a small number of shapes:
- closed
- small open
- medium open
- wide open
- rounded open

The motion should feel playful rather than realistic lip-sync.

### 8.3 Reduced motion mode

When reduced motion is enabled:
- keep state transitions
- minimize drifting and bounce
- preserve expression changes so the assistant remains readable

## 9. Rendering Pipeline

### 9.1 Draw order

The renderer should draw in this order:
1. background / backdrop wash
2. body/face silhouette
3. facial accents or glows
4. eyes
5. brows and eyelids if needed
6. mouth
7. state overlays such as clock or timeout markers

### 9.2 Backdrop

Use a simple mint/teal background treatment with subtle shading.

The renderer should avoid external image assets for the base face.

### 9.3 Performance profile

The renderer should:
- avoid allocating per frame where practical
- use simple primitives and short geometry paths
- keep frame timing stable during idle animation
- allow the UI loop to remain non-blocking

## 10. State Vocabulary for Implementation

The implementation can use a simple string or enum mapping, but the renderer must recognize these semantic states:

- neutral
- blink
- listening
- thinking
- speaking
- smile
- laugh
- whistle
- sleeping
- concerned
- look_around

Recommended internal grouping:
- idle group: neutral, blink, look_around, smile, whistle
- active group: listening, thinking, speaking
- distress group: sleeping, concerned

## 11. Test Expectations

The renderer should have tests for:
- layout scaling across at least two resolutions
- expression-to-style mapping
- mouth variant selection
- clock overlay behavior for sleep/quota states
- no invalid geometry values
- stable values when the output size changes

At minimum, the tests should prove that the face scales cleanly and that the expression mapper returns the correct mouth/eye family for each supported state.

## 12. Implementation Notes

The current implementation strategy should remain asset-free by default.

That leaves room for future upgrades such as:
- optional custom face art
- alternate mascot packs
- richer mouth-sync data
- more advanced emotional overlays

However, the MVP renderer should remain fully functional without any bitmap assets beyond the generic pak structure.

## 13. Acceptance Criteria

The renderer is acceptable when:

- BMO fills the screen cleanly on the target devices
- expressions are distinct at a glance
- speaking has visible mouth animation
- idle never feels static for long
- sleep/quota states are understandable and playful
- the code runs under both cgo and non-cgo build paths
- tests cover the geometry and expression mapping

## 14. Reference to Current Code

The present renderer implementation follows this spec with:
- screen-relative geometry
- a procedural face renderer
- a shared expression style mapper
- separate SDL and framebuffer build paths
- unit tests for scaling and expression mapping

This spec is the source of truth for future renderer changes.
