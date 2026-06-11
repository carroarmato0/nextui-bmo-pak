package devctx

import "time"

// freshWindow separates "news" (react excitedly) from "old news"
// (reminisce). One day matches how players experience sessions.
const freshWindow = 24 * time.Hour

// nudgeTopics phrases each category as something BMO would notice on his
// own screen.
var nudgeTopics = map[string]string{
	KeyLibrary:      "the game collection stored on this device",
	KeySaves:        "the save files he can see on the SD card",
	KeyPlayLog:      "the player's recent play activity",
	KeySystem:       "how the device itself — his own body — is doing right now",
	KeyAchievements: "the player's recent RetroAchievements unlocks",
}

// ProactiveNudge picks the topic for a spontaneous idle remark, weighted by
// freshness: categories with events from the last 24h always win; with
// nothing fresh, BMO sometimes reminisces about a random past achievement,
// otherwise falls back to stale topics (framed as old news) or evergreen
// ones. Returns false when there is nothing at all to talk about.
func (b *Builder) ProactiveNudge() (string, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	sections, now := b.sectionsLocked()
	if len(sections) == 0 {
		return "", false
	}

	var fresh, rest []Section
	for _, s := range sections {
		if !s.Freshest.IsZero() && now.Sub(s.Freshest) < freshWindow {
			fresh = append(fresh, s)
		} else {
			rest = append(rest, s)
		}
	}
	if len(fresh) > 0 {
		s := fresh[b.rng.Intn(len(fresh))]
		return nudge(nudgeTopics[s.Key], "This news is fresh — react excitedly, like it just happened."), true
	}
	// Nothing fresh: one time in three, dig up an old achievement instead.
	if b.reminisce != nil && b.rng.Intn(3) == 0 {
		if memory, ok := b.reminisce(now); ok {
			return "(BMO suddenly remembers " + memory + ". He reminisces about it out loud in one or two short sentences, reacting proportionally to how hard it was: awed if it is rare, playfully teasing if it is easy. Do not greet the player; just make the remark.)", true
		}
	}
	// rest is guaranteed non-empty here: fresh is empty (we would have
	// returned above), so every section landed in rest, and sections is
	// non-empty.
	s := rest[b.rng.Intn(len(rest))]
	tone := "Keep it playful and curious."
	if !s.Freshest.IsZero() {
		tone = "This happened a while ago — reminisce fondly or ask when they will play again."
	}
	return nudge(nudgeTopics[s.Key], tone), true
}

func nudge(topic, tone string) string {
	return "(BMO glances at his own screen and spontaneously says one or two short sentences about " + topic + ". " + tone + " Do not greet the player; just make the remark.)"
}
