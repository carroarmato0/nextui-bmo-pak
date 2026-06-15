package assistant

import (
	"regexp"
	"strings"
)

// EmotionVocabulary lists the conversational expressions the chat model may
// request via an [emotion] directive. It excludes the functional, state-driven
// faces (listening/thinking/speaking/sleeping/blink/look_around) and whistle
// (which has no asset and folds to neutral). Every entry resolves to its own
// face via face.Canonical — enforced by TestEmotionVocabularyResolvesToItself.
// The list is the single source of truth for both the parser whitelist and the
// system-prompt advertising, so the two cannot drift apart.
var EmotionVocabulary = []Expression{
	ExpressionNeutral, ExpressionSmile, ExpressionHappy, ExpressionLaugh,
	ExpressionContent, ExpressionSad, ExpressionAngry, ExpressionSurprised,
	ExpressionExcited, ExpressionLove, ExpressionShy, ExpressionCrying,
	ExpressionTeary, ExpressionGloomy, ExpressionDizzy, ExpressionUnamused,
	ExpressionAnnoyed, ExpressionSkeptical, ExpressionPlayful, ExpressionKiss,
	ExpressionGrimace, ExpressionShout, ExpressionDead, ExpressionGlitch,
	ExpressionDismayed, ExpressionAdoring, ExpressionSparkle, ExpressionConcerned,
}

// emotionByName maps a lower-cased emotion name to its Expression for O(1)
// whitelist lookups during parsing.
var emotionByName = func() map[string]Expression {
	m := make(map[string]Expression, len(EmotionVocabulary))
	for _, e := range EmotionVocabulary {
		m[string(e)] = e
	}
	return m
}()

// emotionTokenRe matches a bracketed single word of letters/underscores, e.g.
// "[happy]". Only tokens whose word is in EmotionVocabulary are treated as
// directives; anything else (e.g. "[pauses]", "[1]") is left untouched.
var emotionTokenRe = regexp.MustCompile(`\[([A-Za-z_]+)\]`)

// extraSpaceRe collapses runs of spaces/tabs left behind after removing a
// directive. Newlines are preserved.
var extraSpaceRe = regexp.MustCompile(`[ \t]{2,}`)

// ParseEmotion extracts the chat model's facial directive. It removes every
// recognised [emotion] token from reply, tidies the whitespace the removal
// leaves behind, and returns the spoken text plus the FIRST recognised emotion
// (empty Expression if none). Bracketed words that are not in the vocabulary
// pass through unchanged.
func ParseEmotion(reply string) (string, Expression) {
	var first Expression
	clean := emotionTokenRe.ReplaceAllStringFunc(reply, func(tok string) string {
		name := strings.ToLower(tok[1 : len(tok)-1])
		if emo, ok := emotionByName[name]; ok {
			if first == "" {
				first = emo
			}
			return ""
		}
		return tok
	})
	if first != "" {
		clean = strings.TrimSpace(extraSpaceRe.ReplaceAllString(clean, " "))
	}
	return clean, first
}
