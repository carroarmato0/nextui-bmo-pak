package assistant

import (
	"regexp"
	"strings"
)

// EmotionEntry is one advertised emotion: a face name plus an optional human
// description used to help the chat model choose it.
type EmotionEntry struct {
	Name        string
	Description string
}

// BuildEmotionVocabulary combines the built-in emotion names (empty for a
// self-contained mod) with the emotion faces the active mod ships on disk,
// de-duplicating by name (first occurrence wins, built-ins first) and attaching
// any description from the mod manifest. It is the single source of truth for
// both the system-prompt advertising and the parser whitelist, so they cannot
// drift apart.
func BuildEmotionVocabulary(builtin, disk []string, descriptions map[string]string) []EmotionEntry {
	seen := make(map[string]bool, len(builtin)+len(disk))
	entries := make([]EmotionEntry, 0, len(builtin)+len(disk))
	add := func(name string) {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		entries = append(entries, EmotionEntry{Name: name, Description: strings.TrimSpace(descriptions[name])})
	}
	for _, n := range builtin {
		add(n)
	}
	for _, n := range disk {
		add(n)
	}
	return entries
}

// emotionNameSet builds the parser whitelist from a vocabulary: lower-cased name
// -> Expression (the name itself, which is what the renderer resolves).
func emotionNameSet(entries []EmotionEntry) map[string]Expression {
	m := make(map[string]Expression, len(entries))
	for _, e := range entries {
		m[e.Name] = Expression(e.Name)
	}
	return m
}

// emotionTokenRe matches a bracketed single token of the face-filename charset,
// e.g. "[happy]" or "[side-eye]". Only tokens whose word is in the active
// vocabulary are treated as directives; anything else is left untouched.
var emotionTokenRe = regexp.MustCompile(`\[([A-Za-z0-9_-]+)\]`)

// extraSpaceRe collapses runs of spaces/tabs left behind after removing a
// directive. Newlines are preserved.
var extraSpaceRe = regexp.MustCompile(`[ \t]{2,}`)

// emotionProtocolPrompt is appended to the chat persona so the model knows how
// to drive BMO's face. Built from the supplied vocabulary so it can never
// advertise a word the parser would not accept. Entries with a description are
// rendered as "name — description"; others as the bare name.
func emotionProtocolPrompt(entries []EmotionEntry) string {
	parts := make([]string, len(entries))
	for i, e := range entries {
		if e.Description != "" {
			parts[i] = e.Name + " — " + e.Description
		} else {
			parts[i] = e.Name
		}
	}
	return "You have an animated face. You may begin your reply with exactly one " +
		"directive in square brackets to set your facial expression, for example " +
		"[happy]. The bracketed word is silent — it is never spoken aloud, only " +
		"used to choose your face. Include it only when an emotion clearly fits; " +
		"otherwise leave it out. Valid expressions: " + strings.Join(parts, ", ") + "."
}

// ParseEmotion extracts the chat model's facial directive. It removes every
// recognised [emotion] token (those whose lower-cased word is a key in valid)
// from reply, tidies the whitespace the removal leaves behind, and returns the
// spoken text plus the FIRST recognised emotion (empty Expression if none).
// Bracketed words not in valid pass through unchanged.
func ParseEmotion(reply string, valid map[string]Expression) (string, Expression) {
	var first Expression
	clean := emotionTokenRe.ReplaceAllStringFunc(reply, func(tok string) string {
		name := strings.ToLower(tok[1 : len(tok)-1]) // strip surrounding [ and ]
		if emo, ok := valid[name]; ok {
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
