package assistant

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
