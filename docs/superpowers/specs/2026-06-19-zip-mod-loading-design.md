# Zip Mod Loading — Design

**Date:** 2026-06-19
**Status:** Approved (brainstorming) — ready for implementation plan

## Goal

Let BMO load a mod directly from a `mods/<id>.zip` archive at runtime, in
addition to the existing `mods/<id>/` directory form, so mods can be
distributed and installed as a single file with no unzip step.

## Background

Today every mod is a directory under `<dataRoot>/BMO/mods/<id>/`. The
mod-reading layer is keyed on a directory path (`mod.Mod.Root string`), with
~28 `os`/`filepath` call sites across four packages reading from it:

- `internal/mod` — `mod.json` manifest.
- `internal/face` — `faces/*.svg`.
- `internal/config` — `CheckOverrides` validation + `persona.txt`/`voice.txt`/
  `quotes.txt`.
- `internal/clips` — `audio/*.pcm`.

No runtime code writes *into* a mod directory (the only writes are creating the
`mods/default` overlay dir and unrelated files), so a read-only zip mod is
viable. `release.sh` already packages each example mod as `dist/mods/<id>.zip`
(a top-level `<id>/` folder inside), and `MODDING.md`'s install flow already
tells users to unzip into `mods/`. This feature removes that unzip step.

## Decisions (from brainstorming)

1. **Both formats, directory wins.** BMO reads both `mods/<id>/` directories and
   `mods/<id>.zip` archives. If both exist for the same id, the **directory**
   takes precedence (an extracted/edited copy overrides the archive) and BMO
   logs a warning. Keeps the dev/edit workflow and the `default` overlay as
   directories.
2. **Filename = id; wrapping folder inside.** The mod id is the zip's filename
   stem (`evil-bmo.zip` → `evil-bmo`). Mod files live under a top-level
   `evil-bmo/` folder inside the archive — exactly what `release.sh` and
   `zip -r evil-bmo.zip evil-bmo` produce, and what unzipping yields. Files at
   the zip **root** are tolerated as a fallback, with a warning.
3. **`fs.FS` abstraction.** The reading layer is routed through `io/fs.FS`
   (`os.DirFS` for directories, `archive/zip` `*zip.Reader` for archives),
   constructed inside `internal/mod` so the dir-vs-zip decision lives in one
   place; consumers accept a plain `fs.FS`.

## Architecture

### The seam (`internal/mod` owns construction)

`mod.Mod` gains:

- `FS fs.FS` — rooted at the mod's contents (so `mod.json`, `faces/`, `audio/`,
  and the prompt files are at the FS root regardless of source).
- `Close() error` — releases the source. No-op for a directory; closes the
  `*zip.ReadCloser` for an archive.

`Root string` remains for identity/logging (`mods/<id>` or `mods/<id>.zip`).

Construction (single place in `internal/mod`):

- **Directory** `mods/<id>/`: `FS = os.DirFS(root)`, `Close` is a no-op.
- **Archive** `mods/<id>.zip`: open with `zip.OpenReader`; if the archive
  contains a top-level `<id>/` directory, `FS = fs.Sub(zr, "<id>")`; otherwise
  fall back to the zip root (`FS = zr`) and log a warning. `Close` closes the
  reader.

### Consumers read via `fs.FS`

Mechanical signature swaps from a path string to `fs.FS`, using
`fs.ReadFile` / `fs.ReadDir` / `fs.Sub`:

- `mod.LoadManifest(fsys fs.FS)` — reads `mod.json`.
- `face` library — reads `faces/<expr>.svg`; the self-contained determination
  (`FacesHasSVG`) lists `faces/` via `fs.ReadDir`.
- `config.CheckOverrides(fsys fs.FS)` + the persona/voice/quotes loader.
- `clips` loader — reads `audio/*.pcm`.

`RenderRest`, `Rasterize`, and `CheckOverrides` already operate on bytes, so
they need no logic change — only their inputs come from `fs` reads.

### Discovery & precedence (`cmd/bmo-pak`)

Scan `mods/`:

- each immediate subdirectory `<id>/` → a directory mod;
- each `<id>.zip` file → a zip mod.

If both exist for an id, **use the directory and log a warning**
(`mod %q: both directory and .zip present; using directory`). `default` is the
auto-created overlay directory and is never a zip. Settings → MOD lists the
union by id. Each loaded mod's `Close()` runs on mod-switch and on app exit so
zip file descriptors do not leak.

## Data flow

1. Startup / mod-switch: discovery builds the `mod.Mod` list (dir + zip, dir
   wins). The selected mod is constructed with its `FS`.
2. The animation engine, face library, prompt loader, and clip player are built
   from `mod.FS` (same as today, but via `fs.FS` instead of a path).
3. On switching away, the previous mod's `Close()` is called.

## Error handling

- A corrupt or unreadable `.zip` is skipped with a warning; it does not crash
  discovery (same spirit as today's tolerated malformed `mod.json`).
- A missing wrapping `<id>/` folder → root-level fallback + warning.
- A zip mod that fails `CheckOverrides` folds faces to `neutral` exactly like a
  directory mod.

## Testing

- `fstest.MapFS` unit tests for each refactored reader (manifest, face library,
  `CheckOverrides`, prompts, clips), proving they are filesystem-agnostic.
- Zip path: build an in-memory archive (`archive/zip` + `bytes.Buffer`) with a
  `<id>/` wrapper; load through the real construction path and assert the
  manifest, faces (render), and clips resolve. Plus the root-level-fallback case
  and the corrupt-zip (skipped-with-warning) case.
- Discovery/precedence: a temp `mods/` containing both `foo/` and `foo.zip`
  resolves to the directory and emits the warning.
- Regression: zip the `evil-bmo` example in-memory and assert it validates
  identically to the directory (extends `internal/examplemods`).

## Out of scope (YAGNI)

- Mod signing / encryption.
- Writing *into* a zip mod at runtime (runtime mods are read-only;
  `cmd/generate-audio` stays directory-only — generate clips into the source
  dir, then zip).
- Hot-reload of a zip while it is the selected mod (same reselect/restart story
  as directory mods today).
- The `default` overlay as a zip.

## Docs

`MODDING.md` gains a short "Distributing as a `.zip`" section: filename = id,
wrapping folder inside, drop into `mods/`, directory-wins precedence; and points
at the `dist/mods/<name>.zip` release artifacts and `scripts/deploy-mods.sh`.

## Implementation staging (for the plan)

1. Introduce `fs.FS` in each reader package, with the existing directory callers
   adapted (`os.DirFS`) — behavior unchanged, all tests still green.
2. Add zip construction + the wrapping-folder/root-fallback rule in
   `internal/mod`, plus `Mod.FS`/`Mod.Close()`.
3. Discovery + dir-wins precedence + `Close()` lifecycle in `cmd/bmo-pak`.
4. Tests (fstest + in-memory zip + precedence + example regression).
5. `MODDING.md` docs.
