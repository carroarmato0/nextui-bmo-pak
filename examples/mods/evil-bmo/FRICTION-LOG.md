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
