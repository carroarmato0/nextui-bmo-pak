package devctx

import (
	"encoding/binary"
	"encoding/json"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const reachStage7Subject = `"Reach Stage 7" in Alleyway`

// writeCacheFile encodes v as an rcheevos offline cache file: 4-byte LE
// length prefix + JSON + fake trailing checksum.
func writeCacheFile(t *testing.T, path string, v any) {
	t.Helper()
	payload, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	data := make([]byte, 4, 4+len(payload)+8)
	binary.LittleEndian.PutUint32(data, uint32(len(payload)))
	data = append(data, payload...)
	data = append(data, 0xde, 0xad, 0xbe, 0xef, 0xde, 0xad, 0xbe, 0xef)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestReadCachePayload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "login.bin")
	writeCacheFile(t, path, map[string]any{"Success": true})
	payload, err := readCachePayload(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(payload) != `{"Success":true}` {
		t.Errorf("payload = %q", payload)
	}
}

func TestReadCachePayloadCorrupt(t *testing.T) {
	dir := t.TempDir()
	short := filepath.Join(dir, "short.bin")
	if err := os.WriteFile(short, []byte{1, 0}, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readCachePayload(short); err == nil {
		t.Error("expected error for short file")
	}
	oversize := filepath.Join(dir, "oversize.bin")
	if err := os.WriteFile(oversize, []byte{0xff, 0xff, 0xff, 0x7f, '{', '}'}, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readCachePayload(oversize); err == nil {
		t.Error("expected error for oversize length prefix")
	}
	zero := filepath.Join(dir, "zero.bin")
	if err := os.WriteFile(zero, []byte{0x00, 0x00, 0x00, 0x00, '{', '}'}, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readCachePayload(zero); err == nil {
		t.Error("expected error for zero length prefix")
	}
	if _, err := readCachePayload(filepath.Join(dir, "missing.bin")); err == nil {
		t.Error("expected error for missing file")
	}
}

func TestDifficultyTag(t *testing.T) {
	cases := []struct {
		points  int
		rarity  float64
		achType string
		want    string
	}{
		{10, 50, "win_condition", "beat the game!"},
		{10, 0, "", ""},
		{25, 3.2, "", "very rare — almost no players have done this"},
		{10, 12.5, "", "impressive — few players have this"},
		{5, 86.5, "", "easy — most players have this"},
		{5, 30, "", "easy — most players have this"},
		{10, 70, "", "easy — most players have this"},
		{10, 40, "progression", "solid"},
	}
	for _, c := range cases {
		if got := difficultyTag(c.points, c.rarity, c.achType); got != c.want {
			t.Errorf("difficultyTag(%d, %v, %q) = %q, want %q", c.points, c.rarity, c.achType, got, c.want)
		}
	}
}

// fixtureRA builds a cache dir with one game (two real achievements, one
// synthetic) where achievement 7869 is unlocked, plus a minuisettings file.
func fixtureRA(t *testing.T, now time.Time, raEnable string) AchievementsCollector {
	t.Helper()
	dir := t.TempDir()
	cache := filepath.Join(dir, "cache")
	const hash = "91128778a332495f77699eaf3a37fe30"
	writeCacheFile(t, filepath.Join(cache, "achievementsets_"+hash+".bin"), raGame{
		GameId: 682,
		Title:  "Alleyway",
		Sets: []raSet{{Achievements: []raAchievement{
			{ID: 101000001, Title: "Warning: Unknown Emulator", Points: 0},
			{ID: 7869, Title: "Reach Stage 7", Description: "Reach stage 7", Points: 5, Rarity: 86.52, Type: "progression"},
			{ID: 27252, Title: "Lucky Number Seven", Description: "Get 7 lives", Points: 5, Rarity: 28.41},
		}}},
	})
	writeCacheFile(t, filepath.Join(cache, "startsession_"+hash+".bin"), raSession{
		Unlocks:         []raUnlockStamp{{ID: 101000001, When: now.Add(-3 * time.Hour).Unix()}, {ID: 7869, When: now.Add(-2 * time.Hour).Unix()}},
		HardcoreUnlocks: []raUnlockStamp{{ID: 7869, When: now.Add(-2 * time.Hour).Unix()}},
	})
	settings := filepath.Join(dir, "minuisettings.txt")
	content := "radius=20\nraEnable=" + raEnable + "\nraUsername=tester\nraToken=SECRET\n"
	if err := os.WriteFile(settings, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return AchievementsCollector{
		CacheDir:     cache,
		SettingsPath: settings,
		Rng:          rand.New(rand.NewSource(1)),
	}
}

func TestAchievementsCollector(t *testing.T) {
	now := time.Date(2026, 6, 11, 16, 0, 0, 0, time.UTC)
	s, err := fixtureRA(t, now, "1").Collect(now)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if s.Key != KeyAchievements || s.Title != "RETROACHIEVEMENTS" {
		t.Fatalf("unexpected section identity: %+v", s)
	}
	for _, want := range []string{
		reachStage7Subject,
		"Reach stage 7",
		"2 hours ago",
		"easy — most players have this",
		"Alleyway: 1 of 2 unlocked", // synthetic excluded from total
	} {
		if !strings.Contains(s.Body, want) {
			t.Errorf("body missing %q: %q", want, s.Body)
		}
	}
	if strings.Contains(s.Body, "Unknown Emulator") {
		t.Errorf("synthetic achievement leaked: %q", s.Body)
	}
	// The fixture settings file contains credentials; they must never
	// surface in collector output.
	if strings.Contains(s.Body, "SECRET") || strings.Contains(s.Body, "tester") {
		t.Errorf("credentials leaked into body: %q", s.Body)
	}
	if !s.Freshest.Equal(now.Add(-2 * time.Hour)) {
		t.Errorf("Freshest = %v, want unlock time", s.Freshest)
	}
	if s.Subject != reachStage7Subject {
		t.Errorf("section subject = %q", s.Subject)
	}
}

func TestAchievementsCollectorDisabled(t *testing.T) {
	now := time.Now()
	if _, err := fixtureRA(t, now, "0").Collect(now); err == nil {
		t.Fatal("expected error when raEnable=0")
	}
}

func TestAchievementsCollectorMissingCache(t *testing.T) {
	c := fixtureRA(t, time.Now(), "1")
	c.CacheDir = filepath.Join(t.TempDir(), "nope")
	if _, err := c.Collect(time.Now()); err == nil {
		t.Fatal("expected error for missing cache dir")
	}
}

func TestRandomPastUnlock(t *testing.T) {
	now := time.Date(2026, 6, 11, 16, 0, 0, 0, time.UTC)
	c := fixtureRA(t, now, "1")
	memory, subject, ok := c.RandomPastUnlock(now)
	if !ok {
		t.Fatal("expected a past unlock")
	}
	for _, want := range []string{`"Reach Stage 7"`, "Alleyway", "2 hours ago"} {
		if !strings.Contains(memory, want) {
			t.Errorf("memory missing %q: %q", want, memory)
		}
	}
	if subject != reachStage7Subject {
		t.Errorf("subject = %q", subject)
	}
	c2 := fixtureRA(t, now, "0")
	if _, _, ok := c2.RandomPastUnlock(now); ok {
		t.Fatal("expected no reminisce when RA disabled")
	}
}
