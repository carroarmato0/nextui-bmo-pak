package devctx

import (
	"encoding/binary"
	"fmt"
	"os"
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
