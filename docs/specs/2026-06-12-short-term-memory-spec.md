# BMO Short-Term Memory — Design Spec

**Date:** 2026-06-12
**Status:** Approved for planning

## Problem

Proactive remarks are repetitive. Device logs show BMO announcing the same
achievement ("Moon Presence" in Deadeus) five times in 30 minutes with
near-identical wording. Two root causes:

1. **No topic dedup.** `ProactiveNudge()` (`internal/devctx/nudge.go`) weights
   topics only by freshness. An achievement unlocked < 24 h ago stays in the
   "fresh" bucket all day and keeps being selected. Nothing tracks what BMO
   already remarked about — not in RAM, not on disk.
2. **Stateless LLM calls.** `SpeakRemark()` (`internal/assistant/voice.go`)
   sends a single-turn request: persona + device context + nudge, no history.
   Identical input produces near-identical output; the model cannot know it
   said the same thing ten minutes ago.

Additionally, debug mode logs the remark *reply* but never the nudge text or
the assembled system prompt, so repetition causes (thin context vs. model
hyperfocus) cannot be diagnosed from logs.

## Goals

- Stop topic-level repetition deterministically (picker dedup).
- When a topic legitimately resurfaces, have BMO vary wording or build on
  prior remarks (LLM-visible short-term memory).
- Occasionally fill cooldown silence with curated Adventure Time BMO quotes,
  themselves deduplicated.
- Log the full prompt context at DEBUG level.
- Respect hardware constraints: minimal SD-card writes, small RAM footprint,
  tolerance for hard power-off (no clean shutdown guaranteed).

## Non-Goals

- Long-term memory or cross-session conversation continuity for PTT chats.
- Config knobs for cooldowns/quote odds (constants first; promote later if
  tuning proves necessary).
- On-device summarization or any extra LLM calls.

## Design

### 1. Remark journal (`internal/devctx/journal.go`)

Single source of truth consumed by both the picker and the prompt builder.
Lives in `devctx` because the nudge topic keys are defined there and both
consumers already import the package.

```go
type RemarkEntry struct {
    When    time.Time `json:"when"`
    Topic   string    `json:"topic"`   // nudge topic key, e.g. "achievements"
    Subject string    `json:"subject"` // e.g. "Moon Presence (Deadeus)"; equals topic for non-achievement remarks
    Reply   string    `json:"reply"`   // what BMO actually said
}
```

- `Journal` holds entries in RAM behind a mutex, capped at the **last 20**
  (oldest dropped on append).
- API: `Append(entry)`, `Recent(n)`, `LastRemarkedAt(subject)`,
  `Contains(subject)`, `PromptBlock(now)`.

**Persistence:**

- File: `$BMO_DATA_ROOT/BMO/remarks.json` (next to `config.json`,
  `persona.txt`), ~2–4 KB.
- Loaded once at startup. Missing or unparsable file (hard power-off
  mid-write; FAT32 rename is not truly atomic) is not an error: log a
  warning, start empty.
- Written via temp-file + rename, once per successful remark (after TTS
  succeeds). Worst case ≤ ~10 writes/hour at the chattiest 7-minute
  interval — negligible SD wear next to the existing append-only log.
- No reads-triggered writes, no timer-based writes.

### 2. Picker dedup (`internal/devctx/nudge.go`)

`Builder` gets a journal reference. `ProactiveNudge()` adds one rule on top
of the existing freshness weighting:

**A subject remarked within the last 6 hours is off the candidate list.**

- **Fresh achievements:** candidate subject (`"Moon Presence (Deadeus)"`)
  checked via `LastRemarkedAt()`. A brand-new unlock mid-session has never
  been remarked and is immediately eligible.
- **Non-achievement topics** (saves, playlog, library, system): subject ==
  topic key, so a whole topic goes on 6 h cooldown after use.
- **Reminisce fallback** (`RandomPastUnlock`): re-roll up to 3 times if the
  rolled achievement is on cooldown; if none found, the path yields nothing.
- **Everything on cooldown:** proceed to the quote fallback (below); if that
  doesn't fire, return `ok=false` — BMO stays silent and the scheduler
  reschedules via the existing no-nudge path. Silence is preferred over a
  least-recently-remarked forced repeat.

The 6 h cooldown is a package constant.

### 3. Quotes fallback

- **Asset:** curated set of ~40–60 standalone one-liner BMO quotes from the
  Adventure Time series (harvested from
  https://adventuretime.fandom.com/wiki/BMO/Quotes at implementation time;
  skip scene-bound multi-party dialogue). Embedded in the binary and written
  to `$BMO_DATA_ROOT/BMO/quotes.txt` on first run if missing, following the
  existing `persona.txt` pattern (`config/prompts.go`). One quote per line;
  user-editable.
- **Picker branch:** when every real topic is on cooldown, roll a **1-in-3
  chance** (mirroring the existing reminisce odds) to emit a quote;
  otherwise stay silent. Quotes are spice, not a guarantee that every cycle
  makes noise.
- **Dedup:** pick a random quote not currently present in the journal
  (`Contains(subject)`). The 20-entry cap makes this self-regulating: the
  last ~20 remarks/quotes cannot repeat, and with 40+ quotes a fresh pick
  always exists.
- **Journal entry:** `topic: "quote"`, subject = reply = the quote text.
- **Speaking:** new `SpeakVerbatim` path in `internal/assistant/voice.go` —
  same state-machine transition (`EventRemark`) and TTS pipeline as
  `SpeakRemark`, but **no chat call**. The quote goes to TTS verbatim: zero
  chat tokens, no paraphrase risk. Logs `remark quote: %q` at DEBUG plus the
  usual TTS timing line at INFO.

### 4. Prompt injection — `RECENT REMARKS` block

`Journal.PromptBlock(now)` renders the last **5** entries (oldest → newest,
relative ages). The system-prompt lambda in `cmd/bmo-pak/main_sdl.go:194`
(and the framebuffer equivalent) appends it as a third segment after persona
and device context. Because that lambda feeds `currentSystemPrompt()` for
both remarks and PTT chats, BMO also remembers his own remarks in
conversation — no separate wiring.

```
RECENT REMARKS (things you already said out loud recently; never repeat
them — vary your angle or build on them, and never re-announce news you
already announced):
- about 2 hours ago: "Wow! You just unlocked "Moon Presence" in Deadeus! ..."
- 12 minutes ago: "Football needs my help."
```

- Empty journal → block omitted entirely.
- Cost: ~100–150 tokens, within the ~1 K device-context budget set by the
  device-awareness design.
- Quote entries appear here too (correct: BMO knows he just said it).

### 5. Debug logging

In `SpeakRemark`, immediately before the chat call, two new DEBUG lines:

- `remark nudge: %q` — the stage-direction text
- `remark system prompt: %q` — the full assembled prompt (persona + device
  awareness + recent remarks)

The logger's existing secret redaction covers these lines. INFO output is
unchanged.

## Error Handling

- Journal load failure → warn + empty journal; never blocks startup.
- Journal save failure → warn; remark still counts as spoken (RAM journal is
  current; next successful save persists everything up to the cap).
- Missing/empty `quotes.txt` after first-run write → quote branch yields
  nothing; picker falls through to silence.
- TTS failure on a quote → no journal append (same rule as remarks: only
  spoken remarks are recorded).

## Testing

- **Journal:** append/cap-at-20; load of missing and corrupt files (empty
  start, no error); temp-file+rename save round-trip; `PromptBlock`
  formatting including the empty case.
- **Picker:** extend existing nudge table tests — subject cooldown
  suppression; new unlock mid-session still eligible; all-on-cooldown →
  quote roll or silence; quote selection excludes journal contents.
- **Voice:** `SpeakVerbatim` makes zero chat-provider calls; journal append
  happens only after successful TTS; new DEBUG lines asserted via existing
  log-capture patterns in `voice_test.go`.
- `golangci-lint run ./...` introduces no new findings.

## File Touch List

| File | Change |
|---|---|
| `internal/devctx/journal.go` | new — journal type, persistence, prompt block |
| `internal/devctx/nudge.go` | cooldown filtering, quote fallback branch |
| `internal/devctx/quotes.go` | new — embedded default quotes, file loading |
| `internal/assistant/voice.go` | `SpeakVerbatim`, journal appends, DEBUG prompt/nudge logging |
| `internal/assistant/ptt_shared.go` / `cmd/bmo-pak/main_sdl.go` / `main_fb.go` | append `RECENT REMARKS` segment to system prompt; wire journal |
| `config/prompts.go` (or equivalent) | first-run write of `quotes.txt` |
