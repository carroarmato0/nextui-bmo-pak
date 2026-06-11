package devctx

import (
	"strings"
	"testing"
	"time"
)

func nudgeBuilder(t *testing.T, reminisce func(time.Time) (string, bool), sections ...Section) *Builder {
	t.Helper()
	collectors := make([]Collector, 0, len(sections))
	for i := range sections {
		collectors = append(collectors, &fakeCollector{key: sections[i].Key, section: sections[i]})
	}
	b, _ := testBuilder(collectors...)
	if reminisce != nil {
		b.SetReminisce(reminisce)
	}
	return b
}

func TestProactiveNudgePrefersFreshNews(t *testing.T) {
	now := time.Date(2026, 6, 11, 16, 45, 0, 0, time.UTC)
	fresh := Section{Key: KeyAchievements, Title: "RETROACHIEVEMENTS", Body: "x", Freshest: now.Add(-2 * time.Hour)}
	stale := Section{Key: KeyPlayLog, Title: "PLAY HISTORY", Body: "y", Freshest: now.Add(-10 * 24 * time.Hour)}
	evergreen := Section{Key: KeyLibrary, Title: "GAME LIBRARY", Body: "z"}
	b := nudgeBuilder(t, nil, fresh, stale, evergreen)
	// Fresh news must win every single time, not just often.
	for i := 0; i < 50; i++ {
		nudge, ok := b.ProactiveNudge()
		if !ok {
			t.Fatal("expected a nudge")
		}
		if !strings.Contains(nudge, "RetroAchievements unlocks") {
			t.Fatalf("iteration %d: fresh category not picked: %q", i, nudge)
		}
		if !strings.Contains(nudge, "react excitedly") {
			t.Fatalf("missing fresh tone hint: %q", nudge)
		}
	}
}

func TestProactiveNudgeStaleAndEvergreenTones(t *testing.T) {
	now := time.Date(2026, 6, 11, 16, 45, 0, 0, time.UTC)
	stale := Section{Key: KeyPlayLog, Title: "PLAY HISTORY", Body: "y", Freshest: now.Add(-10 * 24 * time.Hour)}
	evergreen := Section{Key: KeySystem, Title: "YOUR BODY (THE DEVICE)", Body: "z"}
	b := nudgeBuilder(t, nil, stale, evergreen)
	sawStale, sawEvergreen := false, false
	for i := 0; i < 200; i++ {
		nudge, ok := b.ProactiveNudge()
		if !ok {
			t.Fatal("expected a nudge")
		}
		if strings.Contains(nudge, "play activity") {
			sawStale = true
			if !strings.Contains(nudge, "a while ago") {
				t.Fatalf("stale topic missing reminisce tone: %q", nudge)
			}
		}
		if strings.Contains(nudge, "device itself") {
			sawEvergreen = true
		}
	}
	if !sawStale || !sawEvergreen {
		t.Fatalf("expected both stale and evergreen picks over 200 runs (stale=%v evergreen=%v)", sawStale, sawEvergreen)
	}
}

func TestProactiveNudgeReminiscesWhenNothingFresh(t *testing.T) {
	now := time.Date(2026, 6, 11, 16, 45, 0, 0, time.UTC)
	stale := Section{Key: KeyAchievements, Title: "RETROACHIEVEMENTS", Body: "x", Freshest: now.Add(-10 * 24 * time.Hour)}
	called := 0
	b := nudgeBuilder(t, func(at time.Time) (string, bool) {
		called++
		return `the time the player unlocked "Reach Stage 7" in Alleyway`, true
	}, stale)
	saw := false
	for i := 0; i < 200; i++ {
		nudge, ok := b.ProactiveNudge()
		if !ok {
			t.Fatal("expected a nudge")
		}
		if strings.Contains(nudge, "suddenly remembers") {
			saw = true
			if !strings.Contains(nudge, "Reach Stage 7") {
				t.Fatalf("reminisce nudge missing memory: %q", nudge)
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
