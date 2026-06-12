package devctx

import "time"

// freshWindow separates "news" (react excitedly) from "old news"
// (reminisce). One day matches how players experience sessions.
const freshWindow = 24 * time.Hour

// remarkCooldown is how long a subject stays off the proactive candidate
// list after BMO remarked about it. Long enough to kill same-session
// repeats; short enough that an evening unlock is fair game next morning.
const remarkCooldown = 6 * time.Hour

// reminisceAttempts bounds re-rolls when a reminisce pick is on cooldown.
const reminisceAttempts = 3

// nudgeTopics phrases each category as something BMO would notice on his
// own screen.
var nudgeTopics = map[string]string{
	KeyLibrary:      "the game collection stored on this device",
	KeySaves:        "the save files he can see on the SD card",
	KeyPlayLog:      "the player's recent play activity",
	KeySystem:       "how the device itself — his own body — is doing right now",
	KeyAchievements: "the player's recent RetroAchievements unlocks",
}

// Nudge is a picked proactive remark: either a stage direction for the chat
// model or (Verbatim) a line to speak exactly as-is, plus the identity
// recorded in the remark journal once it has actually been spoken.
type Nudge struct {
	Text     string
	Topic    string
	Subject  string
	Verbatim bool
}

// subjectOf is the journal dedup identity of a section: the specific news
// item when the collector reports one (e.g. the newest achievement),
// otherwise the whole category.
func subjectOf(s Section) string {
	if s.Subject != "" {
		return s.Subject
	}
	return s.Key
}

// ProactiveNudge picks the topic for a spontaneous idle remark, weighted by
// freshness: categories with events from the last 24h always win; with
// nothing fresh, BMO sometimes reminisces about a random past achievement,
// otherwise falls back to stale topics (framed as old news) or evergreen
// ones. Subjects remarked about within remarkCooldown are skipped entirely
// — when everything is on cooldown BMO occasionally falls back to a curated
// verbatim quote, and otherwise stays quiet rather than repeating himself.
func (b *Builder) ProactiveNudge() (Nudge, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	sections, now := b.sectionsLocked()

	onCooldown := func(subject string) bool {
		last := b.journal.LastRemarkedAt(subject)
		return !last.IsZero() && now.Sub(last) < remarkCooldown
	}

	var fresh, rest []Section
	for _, s := range sections {
		if onCooldown(subjectOf(s)) {
			continue
		}
		if !s.Freshest.IsZero() && now.Sub(s.Freshest) < freshWindow {
			fresh = append(fresh, s)
		} else {
			rest = append(rest, s)
		}
	}
	if len(fresh) > 0 {
		s := fresh[b.rng.Intn(len(fresh))]
		return Nudge{
			Text:    nudge(nudgeTopics[s.Key], "This news is fresh — react excitedly, like it just happened."),
			Topic:   s.Key,
			Subject: subjectOf(s),
		}, true
	}
	// Nothing fresh: one time in three, dig up an old achievement instead.
	if b.reminisce != nil && b.rng.Intn(3) == 0 {
		for attempt := 0; attempt < reminisceAttempts; attempt++ {
			memory, subject, ok := b.reminisce(now)
			if !ok {
				break
			}
			if onCooldown(subject) {
				continue
			}
			return Nudge{
				Text:    "(BMO suddenly remembers " + memory + ". He reminisces about it out loud in one or two short sentences, reacting proportionally to how hard it was: awed if it is rare, playfully teasing if it is easy. Do not greet the player; just make the remark.)",
				Topic:   KeyAchievements,
				Subject: subject,
			}, true
		}
	}
	if len(rest) > 0 {
		s := rest[b.rng.Intn(len(rest))]
		tone := "Keep it playful and curious."
		if !s.Freshest.IsZero() {
			tone = "This happened a while ago — reminisce fondly or ask when they will play again."
		}
		return Nudge{
			Text:    nudge(nudgeTopics[s.Key], tone),
			Topic:   s.Key,
			Subject: subjectOf(s),
		}, true
	}
	// Everything on cooldown (or nothing to say at all): occasionally fill
	// the silence with a classic quote instead of repeating real news.
	return b.quoteNudge()
}

// quoteNudge rolls the verbatim-quote fallback: one time in three, pick a
// random quote not present in the journal window. Quotes are spice, not a
// guarantee that every proactive cycle makes noise.
func (b *Builder) quoteNudge() (Nudge, bool) {
	if b.quotes == nil || b.rng.Intn(3) != 0 {
		return Nudge{}, false
	}
	var candidates []string
	for _, q := range b.quotes() {
		if !b.journal.Contains(q) {
			candidates = append(candidates, q)
		}
	}
	if len(candidates) == 0 {
		return Nudge{}, false
	}
	q := candidates[b.rng.Intn(len(candidates))]
	return Nudge{Text: q, Topic: TopicQuote, Subject: q, Verbatim: true}, true
}

func nudge(topic, tone string) string {
	return "(BMO glances at his own screen and spontaneously says one or two short sentences about " + topic + ". " + tone + " Do not greet the player; just make the remark.)"
}
