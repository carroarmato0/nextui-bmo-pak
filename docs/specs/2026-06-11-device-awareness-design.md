# BMO Device Awareness — Design Spec

Date: 2026-06-11
Status: Approved design, pending implementation plan

## Goal

Give BMO read-only insight into the NextUI device he lives on so AI replies
(and new proactive idle remarks) can reference real facts: the game library,
save files, play history, RetroAchievements unlocks, and system health. BMO
should feel aware of time — fresh news gets excitement, stale news gets
reminiscing. Every data category is individually toggleable in the Settings
menu. **No write operations of any kind.**

## Measured data volumes (from the actual device, 2026-06-11)

| Category | Raw | Summarized in context |
|---|---|---|
| ROMs | ~490 games, 13 non-empty systems (38 dirs) | counts per system ≈ 150 tokens |
| Saves | 53 files | counts + 5 recent names ≈ 100 tokens |
| Play log | `game_logs.sqlite`, 20KB, 42 roms / 102 sessions | top 5 + recent 5 ≈ 300 tokens |
| Achievements | `.ra/offline/cache/*.bin`, JSON | recent 5 + progress ≈ 250 tokens |
| System | trivial | ≈ 100 tokens |

Full context block budget: **≤ ~1K tokens per request**. The full ROM name
list (~6K tokens) is deliberately NOT injected; specific game titles reach
BMO via saves, play log, and achievements instead.

## Architecture

New package `internal/devctx`:

- **Collector** — one per category, interface returning a formatted text
  section plus a freshness timestamp (most recent event in that category;
  zero time when not applicable):
  `Collect(now time.Time) (Section, error)` where
  `Section{Title, Body string; Freshest time.Time}`.
- **Snapshot builder** — runs only the collectors enabled in config,
  concatenates sections under a `DEVICE AWARENESS` header that opens with a
  clock anchor ("It is Thursday 2026-06-11, 16:45"). Results cached with a
  ~30s TTL. Exposes `Snapshot() string`.
- **Wiring** — in `cmd/bmo-pak`, the voice pipeline's existing
  `SetSystemPromptSource` hook returns `persona + "\n\n" + devctx.Snapshot()`.
  The reactive pipeline itself is unchanged.

Collectors take their filesystem roots / DB paths as constructor parameters
(defaults: `/mnt/SDCARD/Roms`, `/mnt/SDCARD/Saves`,
`/mnt/SDCARD/.userdata/shared/game_logs.sqlite`,
`/mnt/SDCARD/.userdata/shared/.ra/offline/cache`,
`/mnt/SDCARD/.userdata/shared/minuisettings.txt`) so tests run against
fixtures.

## Collectors

### 1. Library (`Awareness: Library`)
Scan `Roms/*/`, count non-hidden files per system dir, skip empty dirs.
Output: per-system counts + grand total. No freshness signal (zero time).

### 2. Saves (`Awareness: Saves`)
Walk `Saves/<SYSTEM>/`, count per system; list the 5 most recently modified
save file names (stripped of extensions — they carry real game titles).
Freshness = newest save mtime.

### 3. Play log (`Awareness: Play Log`)
Read `game_logs.sqlite` read-only via pure-Go `modernc.org/sqlite`
(CGO_ENABLED=0 build; ~+4MB binary, accepted). Schema:
`rom(id, type, name, file_path, image_path)` and
`play_activity(rom_id, play_time, created_at)`.
Output: top 5 most-played (name, total hours) and 5 most recent sessions
(name, relative time, duration); plus an explicit gap line ("last play
session was 5 days ago") for reunion awareness. Freshness = newest
`play_activity.created_at`.

### 4. System (`Awareness: System`)
Device model via existing `hardware.Detect` (device-tree model), uptime,
RAM used %, SD card used/free, battery level from
`/sys/class/power_supply` (best-effort). Always "fresh" but evergreen —
participates in proactive fallback, never claims news.

### 5. Achievements (`Awareness: Achievements`)
Gate: `raEnable=1` in `minuisettings.txt`, else section omitted entirely.
Credentials (`raUsername`, `raPassword`, `raToken`) are **never read**.
No network access — local cache only.

Parse `.ra/offline/cache/`:
- File format: 4-byte little-endian JSON length, JSON payload, trailing
  checksum bytes (ignored).
- `achievementsets_<gamehash>.bin` → game `Title`, `GameId`, and per-set
  `Achievements[]` with `ID`, `Title`, `Description`, `Points`, `Rarity`
  (% of players), `RarityHardcore`, `Type`
  (`progression` / `win_condition` / `missable` / null).
- `startsession_<gamehash>.bin` → `Unlocks[]` and `HardcoreUnlocks[]` as
  `{ID, When}` (unix). rcheevos keeps these current as unlocks happen.
- Join sets with unlocks per game hash. Filter synthetic pseudo-achievements
  (`ID >= 101000000`, e.g. "Warning: Unknown Emulator").

Output: 5 most recent unlocks as
`game · achievement title · description · relative time · difficulty tag`,
plus per-game progress ("Alleyway: 1 of 18 unlocked").
Difficulty tag derived from rarity/points:
- rarity ≥ 60% or points ≤ 5 → "easy — most players have this"
- rarity 20–60% → "solid" (no special emphasis)
- rarity < 20% → "impressive — few players have this"
- rarity < 5% → "very rare!"
- rarity == 0 (missing data) → no tag, neutral
- `Type == win_condition` → always flagged "beat the game!" regardless

Freshness = newest unlock `When`.

## Time awareness

- Clock anchor opens the context block (date, weekday, time).
- All timestamps rendered relative ("20 minutes ago", "yesterday",
  "5 days ago") — never raw epochs. Relative rendering is a shared devctx
  helper used by all collectors.
- Each section carries its freshness timestamp for the proactive picker.

## Proactive remarks

New scheduler in `internal/assistant`. Fires only when ALL hold:
- AI mode enabled (same gate as PTT)
- `ProactiveTalk` level ≠ off
- State machine idle, and ≥ 2 minutes since last interaction
  (never barge in right after a conversation)

Levels (jittered: each fire schedules the next at `base ± 40%`):

| Level | Base interval |
|---|---|
| Off | — (default) |
| Chatty | ~7 min |
| Regular | ~30 min |
| Occasional | ~1 h |
| Rare | ~3 h |

On fire, pick one enabled category, **weighted by freshness**:
- Category with events < 24h old → strongly preferred; remark instruction:
  "this just happened — react excitedly".
- Stale categories → rarely picked; instruction: "this was a while ago —
  reminisce or ask if they'll play again".
- Nothing fresh → evergreen fallback (library shape, system status, or
  "you haven't played anything in N days").

**Achievement reminisce mode:** when achievement news is stale, the
scheduler may instead pick one random older unlock from the full cache and
inject its details (title, description, game, rarity tag, age) into that
single remark's prompt — long memory without bloating every turn. Tone
follows the difficulty tag: awe for rare, playful teasing for common.

The remark request goes through the normal chat→TTS→`speak()` path with the
full persona + device context, plus a user-role nudge like:
`(BMO glances at his own screen and spontaneously says one or two short
sentences about <category focus>)`. Existing PTT interrupt, amplitude-driven
mouth, state transitions, and quota handling all apply unchanged. On
chat/TTS failure the remark is skipped and the next one scheduled.

## Config & Settings menu

`config.Config` gains:

```go
DeviceContext struct {
    Library      bool // default true
    Saves        bool // default true
    PlayLog      bool // default true
    System       bool // default true
    Achievements bool // default true
}
ProactiveTalk string // "off" (default) | "chatty" | "regular" | "occasional" | "rare"
```

Awareness categories default **on** (read-only, harmless, BMO is smart out
of the box). Proactive defaults **off** (only feature that spends API money
unprompted). `Normalize()` fills zero values; `Validate()` rejects unknown
proactive levels.

Settings menu: six new flat items following the existing toggle pattern —
five `Awareness: <Category>` on/off toggles and `Proactive Talk` cycling
through the five levels.

## Error handling

Every collector is best-effort: missing path, locked/corrupt DB, malformed
JSON, unreadable settings file → that section is omitted (debug log only).
The snapshot builder never returns an error; worst case the context block is
just the clock anchor, and the voice pipeline behaves exactly as today. The
proactive scheduler treats an empty snapshot as "nothing to say" and skips.

## Testing

- Collectors: temp-dir fixtures (rom/save trees), fixture sqlite DB,
  fixture `.bin` files generated by tests (length-prefix + JSON + junk
  trailer), fixture minuisettings.txt. Cover: happy path, missing paths,
  corrupt data, synthetic-ID filtering, raEnable gating, difficulty tags.
- Snapshot builder: fake collectors — category gating from config, TTL
  caching, section omission on error, clock anchor format.
- Relative-time helper: table-driven.
- Proactive scheduler: fake clock (matches existing idle test style) —
  jitter bounds, idle/AI-mode/recent-interaction gating, freshness
  weighting, reminisce fallback, level changes taking effect.
- Settings menu: existing menu test pattern for new toggles and level cycle.

## Out of scope (possible later phases)

- Write operations of any kind.
- Network calls to retroachievements.org (plan B, rejected — local cache
  covers the need).
- Tool calling / function-call drill-down into the full ROM list.
- Conversation history / multi-turn memory.
- Parsing `ledger.bin` (encrypted pending-sync journal; unnecessary).
