package main

import "time"

// goodbyeWait returns how long the face loop waits for the goodbye clip to
// finish before force-quitting on exit. It tracks the clip's actual duration
// (plus a margin so the final syllable and its mouth animation play out) rather
// than a fixed timeout, so a long farewell is heard in full while a missing or
// stuck clip still cannot hang the exit. clipDur <= 0 (no clip) falls back to a
// short default; the result is capped so a pathologically long clip cannot wedge
// the exit indefinitely.
func goodbyeWait(clipDur time.Duration) time.Duration {
	const (
		margin   = 2 * time.Second
		fallback = 8 * time.Second
		maxWait  = 30 * time.Second
	)
	if clipDur <= 0 {
		return fallback
	}
	if w := clipDur + margin; w < maxWait {
		return w
	}
	return maxWait
}
