package devctx

import (
	"strings"
	"testing"
	"time"
)

func nudgeBuilder(t *testing.T, reminisce func(time.Time) (string, string, bool), sections ...Section) *Builder {
	t.Helper()
	collectors := make([]Collector, 0, len(sections))
	for i := range sections {
		collectors = append(collectors, &fakeCollector{key: sections[i].Key, section: sections[i]})
	}
	b, _ := testBuilder(collectors...)
	// Pin the builder clock to the same wall-clock instant the cooldown
	// tests anchor their memory entries and section freshness to, so
	// relative-time math (6h cooldown, 24h fresh window) lines up.
	now := time.Now().UTC()
	b.SetClock(func() time.Time { return now })
	if reminisce != nil {
		b.SetReminisce(reminisce)
	}
	return b
}

func TestProactiveNudgePrefersFreshNews(t *testing.T) {
	now := time.Now().UTC()
	fresh := Section{Key: KeyAchievements, Title: "RETROACHIEVEMENTS", Body: "x", Freshest: now.Add(-2 * time.Hour)}
	stale := Section{Key: KeyPlayLog, Title: "PLAY HISTORY", Body: "y", Freshest: now.Add(-10 * 24 * time.Hour)}
	evergreen := Section{Key: KeyLibrary, Title: "GAME LIBRARY", Body: "z"}
	b := nudgeBuilder(t, nil, fresh, stale, evergreen)
	// Fresh news must win every single time, not just often.
	for i := 0; i < 50; i++ {
		n, ok := b.ProactiveNudge()
		if !ok {
			t.Fatal("expected a nudge")
		}
		if !strings.Contains(n.Text, "RetroAchievements unlocks") {
			t.Fatalf("iteration %d: fresh category not picked: %q", i, n.Text)
		}
		if !strings.Contains(n.Text, "react excitedly") {
			t.Fatalf("missing fresh tone hint: %q", n.Text)
		}
		if n.Topic != KeyAchievements || n.Subject != KeyAchievements || n.Verbatim {
			t.Fatalf("metadata = %+v", n)
		}
	}
}

func TestProactiveNudgeStaleAndEvergreenTones(t *testing.T) {
	now := time.Now().UTC()
	stale := Section{Key: KeyPlayLog, Title: "PLAY HISTORY", Body: "y", Freshest: now.Add(-10 * 24 * time.Hour)}
	evergreen := Section{Key: KeySystem, Title: "YOUR BODY (THE DEVICE)", Body: "z"}
	b := nudgeBuilder(t, nil, stale, evergreen)
	sawStale, sawEvergreen := false, false
	for i := 0; i < 200; i++ {
		n, ok := b.ProactiveNudge()
		if !ok {
			t.Fatal("expected a nudge")
		}
		if strings.Contains(n.Text, "play activity") {
			sawStale = true
			if !strings.Contains(n.Text, "a while ago") {
				t.Fatalf("stale topic missing reminisce tone: %q", n.Text)
			}
		}
		if strings.Contains(n.Text, "device itself") {
			sawEvergreen = true
		}
	}
	if !sawStale || !sawEvergreen {
		t.Fatalf("expected both stale and evergreen picks over 200 runs (stale=%v evergreen=%v)", sawStale, sawEvergreen)
	}
}

func TestProactiveNudgeReminiscesWhenNothingFresh(t *testing.T) {
	now := time.Now().UTC()
	stale := Section{Key: KeyAchievements, Title: "RETROACHIEVEMENTS", Body: "x", Freshest: now.Add(-10 * 24 * time.Hour)}
	called := 0
	b := nudgeBuilder(t, func(at time.Time) (string, string, bool) {
		called++
		return `the time the player unlocked "Reach Stage 7" in Alleyway`, `"Reach Stage 7" in Alleyway`, true
	}, stale)
	saw := false
	for i := 0; i < 200; i++ {
		n, ok := b.ProactiveNudge()
		if !ok {
			t.Fatal("expected a nudge")
		}
		if strings.Contains(n.Text, "suddenly remembers") {
			saw = true
			if !strings.Contains(n.Text, "Reach Stage 7") {
				t.Fatalf("reminisce nudge missing memory: %q", n.Text)
			}
			if n.Subject != `"Reach Stage 7" in Alleyway` || n.Topic != KeyAchievements {
				t.Fatalf("reminisce metadata = %+v", n)
			}
		}
	}
	if !saw || called == 0 {
		t.Fatalf("expected reminisce path over 200 runs (saw=%v called=%d)", saw, called)
	}
}

func TestProactiveNudgeNothingToSay(t *testing.T) {
	b, _ := testBuilder() // no collectors at all
	if _, ok := b.ProactiveNudge(); ok {
		t.Fatal("expected no nudge with no sections")
	}
}

func TestProactiveNudgeSkipsSubjectsOnCooldown(t *testing.T) {
	now := time.Now().UTC()
	ach := Section{Key: KeyAchievements, Title: "RETROACHIEVEMENTS", Body: "x",
		Subject: `"Moon Presence" in Deadeus`, Freshest: now.Add(-30 * time.Minute)}
	saves := Section{Key: KeySaves, Title: "SAVE FILES", Body: "y", Freshest: now.Add(-1 * time.Hour)}
	b := nudgeBuilder(t, nil, ach, saves)
	b.SetMemory(&Memory{entries: []MemoryEntry{
		{When: now.Add(-10 * time.Minute), Topic: KeyAchievements, Subject: `"Moon Presence" in Deadeus`, Reply: "wow"},
	}})
	// Moon Presence was just remarked: only saves may be picked.
	for i := 0; i < 50; i++ {
		n, ok := b.ProactiveNudge()
		if !ok {
			t.Fatal("expected a nudge (saves is not on cooldown)")
		}
		if n.Topic != KeySaves {
			t.Fatalf("iteration %d: picked %+v, want saves", i, n)
		}
	}
}

func TestProactiveNudgeCooldownExpires(t *testing.T) {
	now := time.Now().UTC()
	ach := Section{Key: KeyAchievements, Title: "RETROACHIEVEMENTS", Body: "x",
		Subject: `"Moon Presence" in Deadeus`, Freshest: now.Add(-10 * time.Hour)}
	b := nudgeBuilder(t, nil, ach)
	b.SetMemory(&Memory{entries: []MemoryEntry{
		{When: now.Add(-7 * time.Hour), Topic: KeyAchievements, Subject: `"Moon Presence" in Deadeus`, Reply: "wow"},
	}})
	if _, ok := b.ProactiveNudge(); !ok {
		t.Fatal("a 7h-old remark must no longer suppress its subject")
	}
}

func TestProactiveNudgeNewUnlockEligibleDespiteOldRemark(t *testing.T) {
	now := time.Now().UTC()
	// Newest unlock changed the section subject: no longer on cooldown.
	ach := Section{Key: KeyAchievements, Title: "RETROACHIEVEMENTS", Body: "x",
		Subject: `"Knife Party" in Deadeus`, Freshest: now.Add(-5 * time.Minute)}
	b := nudgeBuilder(t, nil, ach)
	b.SetMemory(&Memory{entries: []MemoryEntry{
		{When: now.Add(-20 * time.Minute), Topic: KeyAchievements, Subject: `"Moon Presence" in Deadeus`, Reply: "wow"},
	}})
	n, ok := b.ProactiveNudge()
	if !ok || n.Subject != `"Knife Party" in Deadeus` {
		t.Fatalf("new unlock must be eligible: ok=%v n=%+v", ok, n)
	}
}

func TestProactiveNudgeAllOnCooldownIsSilent(t *testing.T) {
	now := time.Now().UTC()
	saves := Section{Key: KeySaves, Title: "SAVE FILES", Body: "y", Freshest: now.Add(-1 * time.Hour)}
	b := nudgeBuilder(t, nil, saves)
	b.SetMemory(&Memory{entries: []MemoryEntry{
		{When: now.Add(-30 * time.Minute), Topic: KeySaves, Subject: KeySaves, Reply: "saves!"},
	}})
	// No quotes installed (Task 4): everything on cooldown means silence.
	for i := 0; i < 50; i++ {
		if n, ok := b.ProactiveNudge(); ok {
			t.Fatalf("expected silence, got %+v", n)
		}
	}
}

func TestProactiveNudgeReminisceRespectsCooldown(t *testing.T) {
	now := time.Now().UTC()
	stale := Section{Key: KeyAchievements, Title: "RETROACHIEVEMENTS", Body: "x",
		Subject: `"Moon Presence" in Deadeus`, Freshest: now.Add(-10 * 24 * time.Hour)}
	b := nudgeBuilder(t, func(at time.Time) (string, string, bool) {
		return `the time the player unlocked "Moon Presence" in Deadeus`, `"Moon Presence" in Deadeus`, true
	}, stale)
	b.SetMemory(&Memory{entries: []MemoryEntry{
		{When: now.Add(-1 * time.Hour), Topic: KeyAchievements, Subject: `"Moon Presence" in Deadeus`, Reply: "wow"},
	}})
	// Both the stale section subject and every reminisce roll are on
	// cooldown: the picker must never emit anything.
	for i := 0; i < 200; i++ {
		if n, ok := b.ProactiveNudge(); ok {
			t.Fatalf("expected silence, got %+v", n)
		}
	}
}
