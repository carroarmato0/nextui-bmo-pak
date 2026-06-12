package devctx

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// The rcheevos offline cache marks client-side pseudo-achievements (like
// "Warning: Unknown Emulator") with IDs at or above this floor; they are
// not real unlocks and must never reach BMO.
const syntheticAchievementIDFloor = 101_000_000

// readCachePayload extracts the JSON payload from an rcheevos offline cache
// file: 4-byte little-endian length prefix, JSON bytes, then a trailing
// checksum we ignore.
func readCachePayload(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) < 4 {
		return nil, fmt.Errorf("cache file too short: %s", path)
	}
	n := int(binary.LittleEndian.Uint32(data[:4]))
	if n <= 0 || n > len(data)-4 {
		return nil, fmt.Errorf("cache payload length %d out of range: %s", n, path)
	}
	return data[4 : 4+n], nil
}

// JSON shapes of the two cache responses we read. Field names match the
// RetroAchievements server payloads.
type raAchievement struct {
	ID          int     `json:"ID"`
	Title       string  `json:"Title"`
	Description string  `json:"Description"`
	Points      int     `json:"Points"`
	Rarity      float64 `json:"Rarity"` // % of players holding it; 0 = unknown
	Type        string  `json:"Type"`   // "progression", "win_condition", "missable", or null
}

type raSet struct {
	Achievements []raAchievement `json:"Achievements"`
}

type raGame struct {
	GameId int     `json:"GameId"`
	Title  string  `json:"Title"`
	Sets   []raSet `json:"Sets"`
}

type raUnlockStamp struct {
	ID   int   `json:"ID"`
	When int64 `json:"When"` // unix epoch
}

type raSession struct {
	Unlocks         []raUnlockStamp `json:"Unlocks"`
	HardcoreUnlocks []raUnlockStamp `json:"HardcoreUnlocks"`
}

// difficultyTag pre-digests rarity/points into a phrase BMO can react to
// proportionally: awe for rare unlocks, playful teasing for common ones.
// Beating the game always rates celebration.
func difficultyTag(points int, rarity float64, achType string) string {
	switch {
	case achType == "win_condition":
		return "beat the game!"
	case rarity == 0:
		return "" // missing data: stay neutral
	case rarity < 5:
		return "very rare — almost no players have done this"
	case rarity < 20:
		return "impressive — few players have this"
	case rarity >= 60 || points <= 5:
		return "easy — most players have this"
	default:
		return "solid"
	}
}

// AchievementsCollector reads NextUI's local rcheevos offline cache. It
// never touches the network and never reads RA credentials — the only
// minuisettings.txt key consulted is raEnable, as a respect-the-user gate.
type AchievementsCollector struct {
	CacheDir     string     // .../.ra/offline/cache
	SettingsPath string     // .../minuisettings.txt
	Rng          *rand.Rand // for RandomPastUnlock; may be nil
}

func (AchievementsCollector) Key() string { return KeyAchievements }

// raUnlock is one real, resolved unlock joined across the two cache files.
type raUnlock struct {
	game        string
	title       string
	description string
	points      int
	rarity      float64
	achType     string
	when        time.Time
	unlockedIn  int // unlocks the player has in this game
	totalIn     int // real achievements in this game
}

func (c AchievementsCollector) raEnabled() bool {
	data, err := os.ReadFile(c.SettingsPath)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "raEnable=1" {
			return true
		}
	}
	return false
}

// load joins achievement sets with session unlocks per game hash and
// returns all real unlocks, newest first.
func (c AchievementsCollector) load() ([]raUnlock, error) {
	setPaths, err := filepath.Glob(filepath.Join(c.CacheDir, "achievementsets_*.bin"))
	if err != nil || len(setPaths) == 0 {
		return nil, fmt.Errorf("no cached achievement sets in %s", c.CacheDir)
	}
	var out []raUnlock
	for _, setPath := range setPaths {
		hash := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(setPath), "achievementsets_"), ".bin")
		payload, err := readCachePayload(setPath)
		if err != nil {
			continue
		}
		var game raGame
		if err := json.Unmarshal(payload, &game); err != nil || strings.TrimSpace(game.Title) == "" {
			continue
		}
		byID := map[int]raAchievement{}
		for _, set := range game.Sets {
			for _, a := range set.Achievements {
				if a.ID >= syntheticAchievementIDFloor {
					continue
				}
				byID[a.ID] = a
			}
		}
		sessPayload, err := readCachePayload(filepath.Join(c.CacheDir, "startsession_"+hash+".bin"))
		if err != nil {
			continue
		}
		var sess raSession
		if err := json.Unmarshal(sessPayload, &sess); err != nil {
			continue
		}
		// Union softcore+hardcore stamps, dedupe by ID keeping latest.
		stamps := map[int]int64{}
		for _, u := range sess.Unlocks {
			if u.ID < syntheticAchievementIDFloor && u.When > stamps[u.ID] {
				stamps[u.ID] = u.When
			}
		}
		for _, u := range sess.HardcoreUnlocks {
			if u.ID < syntheticAchievementIDFloor && u.When > stamps[u.ID] {
				stamps[u.ID] = u.When
			}
		}
		var gameUnlocks []raUnlock
		for id, when := range stamps {
			a, ok := byID[id]
			if !ok {
				continue
			}
			gameUnlocks = append(gameUnlocks, raUnlock{
				game:        game.Title,
				title:       a.Title,
				description: a.Description,
				points:      a.Points,
				rarity:      a.Rarity,
				achType:     a.Type,
				when:        time.Unix(when, 0).UTC(),
			})
		}
		for i := range gameUnlocks {
			gameUnlocks[i].unlockedIn = len(gameUnlocks)
			gameUnlocks[i].totalIn = len(byID)
		}
		out = append(out, gameUnlocks...)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].when.After(out[j].when) })
	return out, nil
}

func (c AchievementsCollector) Collect(now time.Time) (Section, error) {
	if !c.raEnabled() {
		return Section{}, fmt.Errorf("retroachievements disabled in minuisettings")
	}
	unlocks, err := c.load()
	if err != nil {
		return Section{}, fmt.Errorf("achievements: %w", err)
	}
	if len(unlocks) == 0 {
		return Section{}, fmt.Errorf("no achievements unlocked yet")
	}
	recent := unlocks
	if len(recent) > 5 {
		recent = recent[:5]
	}
	parts := make([]string, 0, len(recent))
	for _, u := range recent {
		p := fmt.Sprintf("%q in %s — %s (%s)", u.title, u.game, u.description, RelTime(u.when, now))
		if tag := difficultyTag(u.points, u.rarity, u.achType); tag != "" {
			p += " [" + tag + "]"
		}
		parts = append(parts, p)
	}
	// Per-game progress, ordered by most recent unlock, deduped.
	seen := map[string]bool{}
	var progress []string
	for _, u := range unlocks {
		if seen[u.game] {
			continue
		}
		seen[u.game] = true
		progress = append(progress, fmt.Sprintf("%s: %d of %d unlocked", u.game, u.unlockedIn, u.totalIn))
	}
	body := fmt.Sprintf("Recent unlocks: %s. Progress: %s.",
		strings.Join(parts, "; "), strings.Join(progress, "; "))
	return Section{
		Key:      KeyAchievements,
		Title:    "RETROACHIEVEMENTS",
		Body:     body,
		Subject:  fmt.Sprintf("%q in %s", recent[0].title, recent[0].game),
		Freshest: unlocks[0].when,
	}, nil
}

// RandomPastUnlock returns a one-line description of a randomly chosen past
// unlock for reminisce-style proactive remarks ("remember when..."), plus
// the stable subject used for memory cooldown dedup, or false when RA is
// disabled or nothing is unlocked.
func (c AchievementsCollector) RandomPastUnlock(now time.Time) (memory, subject string, ok bool) {
	if c.Rng == nil || !c.raEnabled() {
		return "", "", false
	}
	unlocks, err := c.load()
	if err != nil || len(unlocks) == 0 {
		return "", "", false
	}
	u := unlocks[c.Rng.Intn(len(unlocks))]
	subject = fmt.Sprintf("%q in %s", u.title, u.game)
	memory = fmt.Sprintf("the time the player unlocked %s (%s), %s",
		subject, u.description, RelTime(u.when, now))
	if tag := difficultyTag(u.points, u.rarity, u.achType); tag != "" {
		memory += " — " + tag
	}
	return memory, subject, true
}
