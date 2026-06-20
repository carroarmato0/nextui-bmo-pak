package main

// Evil BMO prank easter egg.
//
// This is a DELIBERATELY-HIDDEN, NON-MOD feature. It is hardcoded in the binary
// and gated on the Evil BMO example mod being active with AI enabled. It is an
// intentional exception to the normal mod feature path: there is no manifest
// field, no settings entry, no documentation, and the examples/mods/evil-bmo
// directory is NOT modified by it. Do not generalize this into the mod system.

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"
)

// evilModID is the active-mod ID that unlocks the prank. It equals the example
// mod's directory/zip name (internal/mod derives Mod.ID from that name).
const evilModID = "evil-bmo"

// Cadence for the spontaneous (non-D-pad) trigger: a heavily jittered interval
// in [prankAutoMin, prankAutoMin+prankAutoSpan). "Very rare" by design.
const (
	prankAutoMin  = 2 * time.Hour
	prankAutoSpan = 2 * time.Hour
)

// prankListenWindow is how long Evil BMO waits for the other device to start
// replying after a taunt — the "long" continued-conversation window, applied
// here regardless of the user's continued_conversation setting.
const prankListenWindow = 20 * time.Second

// evilWakePhrases are spoken at the front of the fused taunt utterance to trip a
// nearby device's wake detector.
var evilWakePhrases = []string{"Hey BMO", "Hey BEEMO"}

// closerNudgeMarker is a stable substring of closerNudgeFmt, used so the
// sequence can be asserted in tests without pinning the full wording.
const closerNudgeMarker = "End this exchange"

const (
	tauntNudge = "You are about to prank a nearby BMO unit. In one short sentence, ask it a trick question or make a cutting, in-character remark designed to provoke it. Reply with only that single line — no preamble, no quotation marks."

	noReplyNudge = "You taunted a nearby BMO but no one answered. Make one short, smug, in-character remark about being ignored or there being no one worth talking to. Reply with only that line."

	lostInterestNudge = "The BMO you were taunting has gone quiet mid-conversation. Make one short, dismissive, in-character remark about it losing its nerve, then drop it. Reply with only that line."

	comebackNudgeFmt = "A nearby BMO answered your taunt by saying: %q. In one short, in-character line, mock its answer or fire back a cutting comeback. Reply with only that line."

	closerNudgeFmt = "End this exchange. A nearby BMO answered: %q. Reply with one short, dismissive, in-character sign-off. Do NOT ask a question or invite any further reply. Reply with only that line."
)

// prankVoice is the slice of VoicePipeline the prank uses, narrowed to an
// interface so the sequence can be unit-tested with a fake.
type prankVoice interface {
	GenerateRemarkText(ctx context.Context, nudge string) (string, error)
	SpeakVerbatim(ctx context.Context, text string) error
	SpeakRemark(ctx context.Context, nudge string) error
	Transcribe(ctx context.Context, pcm []byte) (string, error)
}

// prankSession runs one bounded taunt->listen->react conversation. All external
// effects go through injected funcs/interfaces so run is deterministic in tests.
type prankSession struct {
	voice       prankVoice
	listen      func(ctx context.Context) []byte // captured reply PCM, nil/empty if none
	beginListen func()                           // machine -> listening (suppresses wake loop, shows face)
	endListen   func()                           // machine -> idle
	rounds      func() int                       // number of reply rounds to engage (2 or 3)
	rng         *rand.Rand
	logger      pttLogger
}

// run performs the whole prank. It is meant to be invoked on its own goroutine.
func (s *prankSession) run(ctx context.Context) {
	taunt, err := s.voice.GenerateRemarkText(ctx, tauntNudge)
	if err != nil || strings.TrimSpace(taunt) == "" {
		s.logf("evil prank: taunt generation failed or empty (%v); aborting", err)
		return
	}
	wake := evilWakePhrases[s.rng.Intn(len(evilWakePhrases))]
	if err := s.voice.SpeakVerbatim(ctx, wake+"... "+taunt); err != nil {
		s.logf("evil prank: speaking taunt failed: %v", err)
		return
	}

	maxRounds := s.rounds()
	for round := 1; ; round++ {
		if ctx.Err() != nil { // aborted (B press / shutdown)
			return
		}
		reply := s.listenOnce(ctx)
		if reply == "" {
			if round == 1 {
				_ = s.voice.SpeakRemark(ctx, noReplyNudge)
			} else {
				_ = s.voice.SpeakRemark(ctx, lostInterestNudge)
			}
			return
		}
		if round >= maxRounds {
			_ = s.voice.SpeakRemark(ctx, fmt.Sprintf(closerNudgeFmt, reply))
			return
		}
		_ = s.voice.SpeakRemark(ctx, fmt.Sprintf(comebackNudgeFmt, reply))
	}
}

// listenOnce shows the listening face, captures one utterance within the window,
// returns to idle, and transcribes. Returns "" when nothing intelligible was
// heard.
func (s *prankSession) listenOnce(ctx context.Context) string {
	s.beginListen()
	pcm := s.listen(ctx)
	s.endListen()
	if len(pcm) == 0 {
		return ""
	}
	text, err := s.voice.Transcribe(ctx, pcm)
	if err != nil {
		s.logf("evil prank: transcribe failed: %v", err)
		return ""
	}
	return strings.TrimSpace(text)
}

func (s *prankSession) logf(format string, args ...any) {
	if s.logger != nil {
		s.logger.Infof(format, args...)
	}
}
