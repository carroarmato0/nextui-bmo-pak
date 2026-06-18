# Evil BMO — Mod Authoring Friction Log

Dog-fooding `docs/MODDING.md` + `docs/mods/*` by building the Evil BMO mod
end-to-end. Each entry: what the docs said, what actually happened, and the
suggested fix. The goal of this exercise is to validate the modding docs — these
findings are a primary deliverable, separate from the mod itself.

## Doc gaps / inaccuracies

- **`face.Rasterize` requires `face.RenderRest` first — undocumented for mod
  authors.** A face written as a `{{.m}}` / `{{.x}}` Go template is *not* valid
  XML until executed (template actions live inside attribute values). Tooling
  that reads raw face bytes must run them through `face.RenderRest` before
  rasterizing or XML-parsing. This is mentioned in internal code comments
  (`internal/face/anim_frames.go:108`) but **not** in `docs/mods/faces.md`. A
  mod author who writes a desktop preview script from the public docs alone
  will hit "invalid XML" / parse errors. *Fix:* add a sentence to
  `faces.md` ("Previewing your faces") noting that templated faces must be
  rendered at rest before rasterizing/validating.

- **`oksvg` silently drops unsupported elements (warns, never errors).** A face
  that uses an unsupported element (`clipPath`, `filter`, `text`, `pattern`,
  CSS classes…) still rasterizes "successfully" — the element is just omitted.
  So an automated render/rasterize check **cannot** catch unsupported elements;
  only an eyeball (on-device **Y**-step, or a rendered PNG) will. The docs list
  the supported/unsupported elements but don't warn that misuse fails *silently*
  rather than loudly. *Fix:* note in `faces.md` that unsupported elements are
  dropped without error, so visual verification is required.

- **Mod override SVGs are validated as RAW XML, but the documented template
  idiom (and the built-in `look_around`) put template actions inside attribute
  values with quotes — which is invalid raw XML.** On mod load the device runs
  `config.CheckOverrides`, which validates each `faces/*.svg` as XML on the raw
  bytes *before* template execution. A face whose attribute uses
  `cx="{{printf "%.1f" $lx}}"` (exactly what the embedded
  `internal/face/assets/look_around.svg` does) is rejected as `not valid XML`
  and silently folded to `neutral` — observed on-device as
  `[WARN] mod override error: faces/look_around.svg: not valid XML`. So an
  author who copies the built-in idle-animation idiom ships a face the validator
  refuses. Workaround: keep attribute templates quote-free (`cx="{{$lx}}"`).
  *Fix (one of):* (a) `CheckOverrides` should `RenderRest` before validating, so
  it checks what is actually drawn (the renderer already does); or (b) document
  in `faces.md`/`animations.md` that attribute-embedded template values must not
  contain quotes, and change the built-in `look_around` to a quote-free form.
  **Tooling lesson:** a validation test that renders the template first (our
  initial `TestFacesRender`) will *not* catch this — the test must call the same
  `config.CheckOverrides` path the device uses (now `TestDeviceValidation`).

## Confusing / underspecified

- **"Stage into `./dist`" collides with `dist/` being gitignored.** The natural
  reading of "place the generated assets in `./dist` and adb push" is to author
  the mod there — but `dist/` is gitignored build output, so the mod would be
  untracked and un-reviewable. Resolved by keeping the canonical source tracked
  at `examples/mods/evil-bmo/` and copying to `dist/mods/evil-bmo/` only for
  deploy (see `deploy.sh`). *Fix:* if the docs ever recommend a staging dir,
  call out that the tracked source should live outside `dist/`.

- **Self-contained mods must re-declare every animated expression, including
  `speaking`.** `animations.md` does say a self-contained mod "starts with an
  empty animation set," but it's easy to miss that this means the built-in
  lip-sync (and the `speaking` face) are gone unless re-declared. We had to add
  `neutral`/`laugh`/`angry`/`speaking` amplitude templates explicitly to get a
  talking mouth. *Fix:* a one-line "self-contained mods get no lip-sync for
  free — declare it" callout near the top of `animations.md` would help.

- **`cmd/generate-audio` honored the mod's `voice.txt` but not its
  `persona.txt`.** When generating the spoken system clips (hello/goodbye/…),
  the tool loaded the active mod's voice for TTS *delivery* but hardcoded
  `config.DefaultSystemPrompt` for the chat that writes the clip *text* — so a
  character mod's clips spoke the default cheerful BMO words in the mod's voice.
  Fixed on this branch: the tool now loads `activeMod.PersonaPath()` too, so
  clips are fully in-character. *Fix upstreamed in this branch;* worth a note in
  any "audio clips" mod doc that clip generation uses the mod persona + voice.

## Sparse-mod behavior gaps (found on device, fixed on a follow-up branch)

- **Idle looked static for a sparse self-contained mod.** The idle scheduler
  (`internal/assistant/idle.go`) cycles the full embedded emotion vocabulary
  (~26 faces) with no knowledge of which faces the active mod ships. Evil BMO
  has 8 faces, so ~80% of idle ticks folded to its `neutral` — the screen sat on
  the smug smirk while the logs named the full variety. Not a render bug.
  *Fix:* `IdleScheduler.SetAvailable` + `face.FaceNamesInDir`, restricting idle
  to the mod's shipped faces (unfiltered for the default/overlay set). Lesson for
  mod authors: a self-contained mod's idle is only as lively as the faces it
  ships — include a few idle-friendly expressions (look_around, plus a couple of
  emotion faces).

- **A long modded goodbye clip was cut off on exit.** The shutdown waited a
  fixed 8s for the goodbye clip; the generated evil farewell was ~10s, so it was
  force-quit ~2s early. *Fix:* `clips.Player.ClipDuration` + size the wait to the
  clip's own length (plus margin, capped) so any farewell is heard in full
  without letting a stuck clip hang the exit. Lesson: clip length matters — keep
  system clips short, or rely on the now-dynamic wait.

- **A missing `sleeping.svg` looked broken, not absent.** The renderer always
  draws the floating `zzZ` sleep marks for the `sleeping` expression, but a
  self-contained mod without a `sleeping.svg` folds the face itself to its
  `neutral` — so Evil BMO "slept" as a wide-awake smirk with zzZ stuck to the
  side. Functional faces that the engine decorates (sleeping's Z marks) really
  need a matching face in a self-contained mod. *Fix (mod-side):* ship a
  closed-eye `sleeping.svg`. Worth a doc note that `sleeping` pairs with
  engine-drawn Z marks, so its face should have closed/resting eyes.

- **Spontaneous reactions could overlap user-triggered ones (app bug, fixed).**
  Pressing X (quote) while a proactive remark was mid-flight played both at once.
  Root cause was a state-guard that didn't treat "already thinking" as busy.
  Fixed with an atomic `Machine.BeginRemark` and making X/Y interrupt the current
  reaction. Not mod-specific, but surfaced while exercising the mod's quotes.

- **Shutdown was not authoritative — audio could play over the goodbye (app bug,
  fixed).** The goodbye **clip** (`clips.Player`) and BMO's **speech**
  (`VoicePipeline`) are independent paths into the same audio device, unserialized
  against each other. While the goodbye clip played, the machine stayed idle, so a
  proactive remark (or, on a fast exit, the startup `hello` clip) could start on
  top of it; and a second exit press was swallowed by the `CancelBatch`/
  `InterruptSpeech` guards (or merely set `running = false` without stopping the
  clip), so the farewell kept playing during teardown. *Fix:* shutdown is now
  authoritative — `beginShutdown` interrupts in-flight speech, the startup-clip and
  proactive-remark paths are gated on `!shuttingDown`, and a second exit press
  (B/MENU) calls `clipPlayer.Stop()` + `InterruptSpeech()` and quits immediately.
  Surfaced while exercising the mod's longer farewell clip; not mod-specific.

## Worked as documented (worth noting)

- The `{{$m := or .m 0.0}}` … `{{template "talkmouth" $m}}` lip-sync idiom from
  `faces.md` worked verbatim. The `add`/`sub`/`mul` template helpers behaved as
  documented and were exactly what the `look_around` pupil-shift needed.
- The self-contained fallback (missing expression folds to the mod's own
  `neutral.svg`) worked as documented — `smug`/`mocking`/`gloating` emotions
  with no dedicated face correctly fold to the smug-smirk neutral.
- `mod.json` tolerance, `apiVersion` defaulting, and `emotions`/`animations`
  parsing all matched the docs.
- The **Y** (step faces) and **X** (speak a quote) on-device dev aids are
  genuinely the fastest way to verify a mod — exactly as `faces.md` claims.

## Process notes (not doc issues)

- When this plan was executed by a fresh subagent, the exact SVG geometry in the
  plan had been compressed out of the subagent's context, so it reconstructed
  the faces from the prose description and they drifted from the approved design
  (neutral lost its smirk; laugh became a blob). Caught in review and corrected.
  Lesson for *our* tooling, not the mod docs: face geometry that must be
  reproduced exactly should be delivered as files, not inlined in a plan that
  may be summarized.

## Suggested doc/code follow-ups (separate from this mod)

1. `docs/mods/faces.md`: document the `RenderRest`-before-rasterize requirement
   and the silent-drop behavior of unsupported elements.
2. `docs/mods/animations.md`: add an upfront callout that self-contained mods
   inherit no animations (no free lip-sync) and must declare them.
3. Consider shipping `examples/mods/evil-bmo/` as the canonical worked example
   referenced from `docs/MODDING.md` (a complete, tested self-contained mod).
