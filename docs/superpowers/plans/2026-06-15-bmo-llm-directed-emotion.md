# BMO LLM-Directed Facial Emotion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let the chat model embed an `[emotion]` directive in its reply that BMO strips from the spoken text and shows as a facial expression for the duration of the utterance.

**Architecture:** The assistant layer gains the full expression vocabulary as `Expression` constants and a curated `EmotionVocabulary`. A new `ParseEmotion` extracts/strips `[emotion]` directives from chat replies. The `Machine` holds the directed emotion (`SetEmotion`, exposed via `Snapshot().Emotion`, cleared on any non-speak transition). The voice pipeline parses replies before TTS and advertises the vocabulary by appending a protocol block to the system prompt. The render loop shows the emotion face during the Speaking state when one is set, otherwise the existing lip-synced talking mouth.

**Tech Stack:** Go, standard library (`regexp`, `strings`); `internal/face` for the resolution invariant; existing `internal/assistant` state machine and `VoicePipeline`.

**Spec:** `docs/specs/2026-06-15-bmo-llm-directed-emotion-design.md`

**Verification commands (per CLAUDE.md):**
- Tests: `CGO_ENABLED=0 go test ./...` (SDL packages `cmd/bmo-pak` and `internal/renderer` require CGO and will fail to build without the cross toolchain — that is pre-existing and unrelated; `internal/assistant` and `internal/face` must be green).
- Lint: `golangci-lint run ./...`
- Single package: `CGO_ENABLED=0 go test ./internal/assistant/ -run TestName -v`

**Commit convention:** No `Co-Authored-By` trailer.

---

## File Structure

- **Modify** `internal/assistant/state.go` — add 24 `Expression` constants; add `emotion` field to `Machine`; add `SetEmotion`; add `Emotion` to `Snapshot`; clear emotion on non-speak transitions.
- **Modify** `internal/assistant/state_test.go` — tests for the constants, `SetEmotion`/`Snapshot().Emotion`, and clear-on-rest.
- **Create** `internal/assistant/emotion.go` — `EmotionVocabulary`, the lookup map, `ParseEmotion`, `emotionProtocolPrompt`.
- **Create** `internal/assistant/emotion_test.go` — vocabulary resolution invariant + `ParseEmotion` table tests + protocol content test.
- **Modify** `internal/assistant/voice.go` — append the protocol in `currentSystemPrompt`; parse+strip+set emotion in `ProcessBatch` and `SpeakRemark`; speak the cleaned text.
- **Modify** `internal/assistant/voice_test.go` — update the 5 exact-equality `SystemPrompt` assertions to prefix checks; add cleaned-text + emotion-set assertions.
- **Modify** `cmd/bmo-pak/main.go` — Speaking case honors `snap.Emotion`.

---

## Task 1: Expression constants for the full face vocabulary

**Files:**
- Modify: `internal/assistant/state.go:53-65`
- Test: `internal/assistant/state_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/assistant/state_test.go`:

```go
func TestEmotionExpressionConstants(t *testing.T) {
	cases := map[Expression]string{
		ExpressionSad: "sad", ExpressionHappy: "happy", ExpressionContent: "content",
		ExpressionAngry: "angry", ExpressionSurprised: "surprised", ExpressionExcited: "excited",
		ExpressionLove: "love", ExpressionShy: "shy", ExpressionCrying: "crying",
		ExpressionTeary: "teary", ExpressionGloomy: "gloomy", ExpressionDizzy: "dizzy",
		ExpressionUnamused: "unamused", ExpressionAnnoyed: "annoyed", ExpressionSkeptical: "skeptical",
		ExpressionPlayful: "playful", ExpressionKiss: "kiss", ExpressionGrimace: "grimace",
		ExpressionShout: "shout", ExpressionDead: "dead", ExpressionGlitch: "glitch",
		ExpressionDismayed: "dismayed", ExpressionAdoring: "adoring", ExpressionSparkle: "sparkle",
	}
	for expr, want := range cases {
		if string(expr) != want {
			t.Errorf("constant = %q, want %q", string(expr), want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `CGO_ENABLED=0 go test ./internal/assistant/ -run TestEmotionExpressionConstants -v`
Expected: FAIL — `undefined: ExpressionSad` (and the rest) — compile error.

- [ ] **Step 3: Add the constants**

In `internal/assistant/state.go`, replace the existing expression block (lines 53-64, ending before the closing `)` on line 65) so it reads:

```go
	ExpressionNeutral    Expression = "neutral"
	ExpressionIdle       Expression = ExpressionNeutral
	ExpressionBlink      Expression = "blink"
	ExpressionListening  Expression = "listening"
	ExpressionThinking   Expression = "thinking"
	ExpressionSpeaking   Expression = "speaking"
	ExpressionSleeping   Expression = "sleeping"
	ExpressionConcerned  Expression = "concerned"
	ExpressionLookAround Expression = "look_around"
	ExpressionSmile      Expression = "smile"
	ExpressionLaugh      Expression = "laugh"
	ExpressionWhistle    Expression = "whistle"

	// Emotional expressions backed by the Figma face set. Each resolves to its
	// own asset via face.Canonical (see EmotionVocabulary).
	ExpressionSad       Expression = "sad"
	ExpressionHappy     Expression = "happy"
	ExpressionContent   Expression = "content"
	ExpressionAngry     Expression = "angry"
	ExpressionSurprised Expression = "surprised"
	ExpressionExcited   Expression = "excited"
	ExpressionLove      Expression = "love"
	ExpressionShy       Expression = "shy"
	ExpressionCrying    Expression = "crying"
	ExpressionTeary     Expression = "teary"
	ExpressionGloomy    Expression = "gloomy"
	ExpressionDizzy     Expression = "dizzy"
	ExpressionUnamused  Expression = "unamused"
	ExpressionAnnoyed   Expression = "annoyed"
	ExpressionSkeptical Expression = "skeptical"
	ExpressionPlayful   Expression = "playful"
	ExpressionKiss      Expression = "kiss"
	ExpressionGrimace   Expression = "grimace"
	ExpressionShout     Expression = "shout"
	ExpressionDead      Expression = "dead"
	ExpressionGlitch    Expression = "glitch"
	ExpressionDismayed  Expression = "dismayed"
	ExpressionAdoring   Expression = "adoring"
	ExpressionSparkle   Expression = "sparkle"
```

(`ExpressionSmile`, `ExpressionLaugh`, `ExpressionNeutral`, `ExpressionConcerned` already exist and are not duplicated.)

- [ ] **Step 4: Run test to verify it passes**

Run: `CGO_ENABLED=0 go test ./internal/assistant/ -run TestEmotionExpressionConstants -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/assistant/state.go internal/assistant/state_test.go
git commit -m "feat(assistant): add Expression constants for the full face vocabulary"
```

---

## Task 2: EmotionVocabulary and resolution invariant

**Files:**
- Create: `internal/assistant/emotion.go`
- Create: `internal/assistant/emotion_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/assistant/emotion_test.go`:

```go
package assistant

import (
	"testing"

	"github.com/carroarmato0/nextui-bmo/internal/face"
)

// Every advertised emotion must resolve to its OWN face. If face.Canonical
// folds it onto something else (or the asset is missing) the model would be
// told about a face BMO cannot actually show.
func TestEmotionVocabularyResolvesToItself(t *testing.T) {
	if len(EmotionVocabulary) == 0 {
		t.Fatal("EmotionVocabulary is empty")
	}
	seen := map[Expression]bool{}
	for _, e := range EmotionVocabulary {
		if seen[e] {
			t.Errorf("duplicate vocabulary entry %q", e)
		}
		seen[e] = true
		if got := face.Canonical(string(e)); got != string(e) {
			t.Errorf("face.Canonical(%q) = %q, want %q (not a self-resolving face)", e, got, e)
		}
	}
}

// The functional, state-driven faces must NOT be advertised to the model.
func TestEmotionVocabularyExcludesFunctionalFaces(t *testing.T) {
	excluded := []Expression{
		ExpressionListening, ExpressionThinking, ExpressionSpeaking,
		ExpressionSleeping, ExpressionBlink, ExpressionLookAround, ExpressionWhistle,
	}
	for _, e := range EmotionVocabulary {
		for _, x := range excluded {
			if e == x {
				t.Errorf("vocabulary must not include functional face %q", e)
			}
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `CGO_ENABLED=0 go test ./internal/assistant/ -run TestEmotionVocabulary -v`
Expected: FAIL — `undefined: EmotionVocabulary` — compile error.

- [ ] **Step 3: Create the vocabulary**

Create `internal/assistant/emotion.go`:

```go
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `CGO_ENABLED=0 go test ./internal/assistant/ -run TestEmotionVocabulary -v`
Expected: PASS (both tests)

- [ ] **Step 5: Commit**

```bash
git add internal/assistant/emotion.go internal/assistant/emotion_test.go
git commit -m "feat(assistant): add EmotionVocabulary with face-resolution invariant"
```

---

## Task 3: ParseEmotion — extract and strip directives

**Files:**
- Modify: `internal/assistant/emotion.go`
- Modify: `internal/assistant/emotion_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/assistant/emotion_test.go`:

```go
func TestParseEmotion(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		wantClean string
		wantEmo   Expression
	}{
		{"no directive", "Hello there!", "Hello there!", ""},
		{"leading directive", "[happy] Hello there!", "Hello there!", ExpressionHappy},
		{"leading no space", "[happy]Hello", "Hello", ExpressionHappy},
		{"embedded directive", "Oh [excited] I love it", "Oh I love it", ExpressionExcited},
		{"trailing directive", "Goodbye [sad]", "Goodbye", ExpressionSad},
		{"case insensitive", "[HAPPY] hi", "hi", ExpressionHappy},
		{"unknown bracket kept", "Wait [pauses] then go", "Wait [pauses] then go", ""},
		{"numeric bracket kept", "See note [1] here", "See note [1] here", ""},
		{"multiple first wins all stripped", "[sad] no [happy] yes", "no yes", ExpressionSad},
		{"only a directive", "[happy]", "", ExpressionHappy},
		{"directive with surrounding spaces tidy", "hi  [happy]  there", "hi there", ExpressionHappy},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clean, emo := ParseEmotion(tt.in)
			if clean != tt.wantClean {
				t.Errorf("clean = %q, want %q", clean, tt.wantClean)
			}
			if emo != tt.wantEmo {
				t.Errorf("emotion = %q, want %q", emo, tt.wantEmo)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `CGO_ENABLED=0 go test ./internal/assistant/ -run TestParseEmotion -v`
Expected: FAIL — `undefined: ParseEmotion` — compile error.

- [ ] **Step 3: Implement ParseEmotion**

Add to `internal/assistant/emotion.go` (and add `"regexp"` and `"strings"` to its imports — change the file's top from `package assistant` to include an import block):

```go
import (
	"regexp"
	"strings"
)
```

```go
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
```

Note: the whitespace tidy only runs when a directive was actually removed, so replies without directives are returned verbatim.

- [ ] **Step 4: Run test to verify it passes**

Run: `CGO_ENABLED=0 go test ./internal/assistant/ -run TestParseEmotion -v`
Expected: PASS (all 11 cases)

- [ ] **Step 5: Commit**

```bash
git add internal/assistant/emotion.go internal/assistant/emotion_test.go
git commit -m "feat(assistant): add ParseEmotion to extract and strip facial directives"
```

---

## Task 4: emotionProtocolPrompt and system-prompt advertising

**Files:**
- Modify: `internal/assistant/emotion.go`
- Modify: `internal/assistant/voice.go:191-199`
- Modify: `internal/assistant/emotion_test.go`
- Modify: `internal/assistant/voice_test.go:156,164,173,399`

- [ ] **Step 1: Write the failing test (protocol content)**

This test uses `strings`, which `emotion_test.go` does not yet import. Change its import block to:

```go
import (
	"strings"
	"testing"

	"github.com/carroarmato0/nextui-bmo/internal/face"
)
```

Then add to `internal/assistant/emotion_test.go`:

```go
func TestEmotionProtocolPrompt(t *testing.T) {
	p := emotionProtocolPrompt()
	if !strings.Contains(p, "[happy]") {
		t.Errorf("protocol missing [happy] example: %q", p)
	}
	// Lists every vocabulary name.
	for _, e := range EmotionVocabulary {
		if !strings.Contains(p, string(e)) {
			t.Errorf("protocol missing vocabulary word %q", e)
		}
	}
	// States the directive is silent.
	if !strings.Contains(strings.ToLower(p), "never spoken") {
		t.Errorf("protocol must say the directive is never spoken: %q", p)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `CGO_ENABLED=0 go test ./internal/assistant/ -run TestEmotionProtocolPrompt -v`
Expected: FAIL — `undefined: emotionProtocolPrompt`.

- [ ] **Step 3: Implement emotionProtocolPrompt**

Add to `internal/assistant/emotion.go`:

```go
// emotionProtocolPrompt is appended to the chat persona so the model knows how
// to drive BMO's face. Built from EmotionVocabulary so it can never advertise a
// word the parser would not accept.
func emotionProtocolPrompt() string {
	names := make([]string, len(EmotionVocabulary))
	for i, e := range EmotionVocabulary {
		names[i] = string(e)
	}
	return "You have an animated face. You may begin your reply with exactly one " +
		"directive in square brackets to set your facial expression, for example " +
		"[happy]. The bracketed word is silent — it is never spoken aloud, only " +
		"used to choose your face. Include it only when an emotion clearly fits; " +
		"otherwise leave it out. Valid expressions: " + strings.Join(names, ", ") + "."
}
```

- [ ] **Step 4: Run protocol test to verify it passes**

Run: `CGO_ENABLED=0 go test ./internal/assistant/ -run TestEmotionProtocolPrompt -v`
Expected: PASS

- [ ] **Step 5: Append the protocol in currentSystemPrompt**

In `internal/assistant/voice.go`, replace `currentSystemPrompt` (lines 191-199):

```go
// currentSystemPrompt resolves the chat persona for the next utterance and
// appends the emotion protocol so the model can drive BMO's face. With no
// persona configured the protocol is returned on its own.
func (p *VoicePipeline) currentSystemPrompt() string {
	persona := p.systemPrompt
	if p.systemPromptSource != nil {
		if prompt := strings.TrimSpace(p.systemPromptSource()); prompt != "" {
			persona = prompt
		}
	}
	if strings.TrimSpace(persona) == "" {
		return emotionProtocolPrompt()
	}
	return persona + "\n\n" + emotionProtocolPrompt()
}
```

- [ ] **Step 6: Update the broken exact-equality assertions**

Appending the protocol changes the system prompt sent to chat, so update these existing assertions in `internal/assistant/voice_test.go`:

Line ~156 (`TestSystemPromptSourceReadPerUtterance`):
```go
	if got := chat.lastChat.SystemPrompt; !strings.HasPrefix(got, "persona one") {
		t.Fatalf("first utterance system prompt = %q, want prefix %q", got, "persona one")
	}
```
Line ~164:
```go
	if got := chat.lastChat.SystemPrompt; !strings.HasPrefix(got, "persona two") {
		t.Fatalf("second utterance system prompt = %q, want prefix %q", got, "persona two")
	}
```
Line ~173:
```go
	if got := chat.lastChat.SystemPrompt; !strings.HasPrefix(got, "static persona") {
		t.Fatalf("empty-source system prompt = %q, want fallback prefix", got)
	}
```
Line ~399 (`TestSpeakRemarkHappyPath`):
```go
	if !strings.HasPrefix(chat.lastChat.SystemPrompt, "persona plus device context") {
		t.Errorf("system prompt = %q", chat.lastChat.SystemPrompt)
	}
```

`strings` is already imported in `voice_test.go`. Verify it is; if not, add it.

- [ ] **Step 7: Add a test that the protocol reaches chat**

Add to `internal/assistant/voice_test.go`:

```go
func TestProcessBatchAppendsEmotionProtocol(t *testing.T) {
	m := NewMachine()
	m.SetMode("ai")
	stt := &fakeProvider{transcript: "hello"}
	chat := &fakeProvider{reply: "hi"}
	tts := &fakeProvider{speech: make([]byte, 2400)}
	pipe := NewVoicePipeline(m, &fakeWriter{}, stt, chat, tts, "whisper-1", "gpt-4o-mini", "tts-1", "alloy", "be bmo", 16000, 1, 2)

	if err := pipe.ProcessBatch(context.Background(), []byte{0x00, 0x40, 0x00, 0x40}); err != nil {
		t.Fatalf("ProcessBatch() error = %v", err)
	}
	sp := chat.lastChat.SystemPrompt
	if !strings.HasPrefix(sp, "be bmo") {
		t.Errorf("persona not preserved as prefix: %q", sp)
	}
	if !strings.Contains(sp, "[happy]") || !strings.Contains(sp, "never spoken") {
		t.Errorf("emotion protocol not appended: %q", sp)
	}
}
```

- [ ] **Step 8: Run the assistant tests to verify all pass**

Run: `CGO_ENABLED=0 go test ./internal/assistant/ -v -run 'SystemPrompt|EmotionProtocol|AppendsEmotion|SpeakRemarkHappyPath'`
Expected: PASS (no exact-equality failures)

- [ ] **Step 9: Commit**

```bash
git add internal/assistant/emotion.go internal/assistant/emotion_test.go internal/assistant/voice.go internal/assistant/voice_test.go
git commit -m "feat(assistant): advertise emotion vocabulary in the chat system prompt"
```

---

## Task 5: Machine emotion directive

**Files:**
- Modify: `internal/assistant/state.go` (`Machine` struct, `Snapshot` struct + builder, `applyTransitionEffects`, new `SetEmotion`)
- Modify: `internal/assistant/state_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/assistant/state_test.go`:

```go
func TestMachineEmotionDirective(t *testing.T) {
	m := NewMachine()
	if got := m.Snapshot().Emotion; got != "" {
		t.Fatalf("default emotion = %q, want empty", got)
	}
	m.SetEmotion(ExpressionExcited)
	if got := m.Snapshot().Emotion; got != ExpressionExcited {
		t.Fatalf("emotion = %q, want excited", got)
	}
}

func TestMachineEmotionPreservedOnSpeak(t *testing.T) {
	m := NewMachine()
	m.SetMode("ai")
	m.Transition(EventListen) // idle -> listening
	m.Transition(EventThink)  // listening -> thinking
	m.SetEmotion(ExpressionLove)
	m.Transition(EventSpeak) // thinking -> speaking; emotion must survive
	if got := m.Snapshot().Emotion; got != ExpressionLove {
		t.Fatalf("emotion after EventSpeak = %q, want love", got)
	}
}

func TestMachineEmotionClearedOnRest(t *testing.T) {
	m := NewMachine()
	m.SetMode("ai")
	m.Transition(EventListen)
	m.Transition(EventThink)
	m.SetEmotion(ExpressionLove)
	m.Transition(EventSpeak)
	m.Transition(EventRest) // speaking -> idle; emotion must clear
	if got := m.Snapshot().Emotion; got != "" {
		t.Fatalf("emotion after EventRest = %q, want empty", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `CGO_ENABLED=0 go test ./internal/assistant/ -run TestMachineEmotion -v`
Expected: FAIL — `m.SetEmotion undefined` and `Snapshot().Emotion` unknown field — compile error.

- [ ] **Step 3: Add the emotion field, setter, snapshot field, and clear rule**

In `internal/assistant/state.go`:

(a) Add `Emotion` to the `Snapshot` struct (after the existing `Expression Expression` field, around line 27):

```go
	Expression      Expression
	Emotion         Expression
```

(b) Add an `emotion` field to the `Machine` struct (alongside `expression`):

```go
	expression      Expression
	emotion         Expression
```

(c) Add the setter next to `SetExpression`:

```go
// SetEmotion records the LLM-directed facial emotion for the utterance about to
// be spoken. It is shown during the Speaking state and cleared by any non-speak
// transition (see applyTransitionEffects), so it never leaks into a later turn.
func (m *Machine) SetEmotion(expression Expression) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.emotion = expression
}
```

(d) Populate the snapshot — in `Snapshot()` add the field to the returned struct:

```go
		Expression:      m.expression,
		Emotion:         m.emotion,
```

(e) At the very end of `applyTransitionEffects` (after the existing `if next == StateIdle && event == EventWake { ... }` block, before the closing brace), add:

```go
	// A directed emotion only applies to the utterance being spoken. Any
	// transition other than the one that starts speech clears it.
	if event != EventSpeak {
		m.emotion = ""
	}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `CGO_ENABLED=0 go test ./internal/assistant/ -run TestMachineEmotion -v`
Expected: PASS (all three)

- [ ] **Step 5: Commit**

```bash
git add internal/assistant/state.go internal/assistant/state_test.go
git commit -m "feat(assistant): hold LLM-directed emotion on the state machine"
```

---

## Task 6: Wire parsing into the voice pipeline

**Files:**
- Modify: `internal/assistant/voice.go` (`ProcessBatch` lines ~289-311; `SpeakRemark` lines ~409-427)
- Modify: `internal/assistant/voice_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `internal/assistant/voice_test.go`:

```go
func TestProcessBatchStripsAndSetsEmotion(t *testing.T) {
	m := NewMachine()
	m.SetMode("ai")
	stt := &fakeProvider{transcript: "hello"}
	chat := &fakeProvider{reply: "[excited] I love that idea!"}
	tts := &fakeProvider{speech: make([]byte, 2400)}
	pipe := NewVoicePipeline(m, &fakeWriter{}, stt, chat, tts, "whisper-1", "gpt-4o-mini", "tts-1", "alloy", "be bmo", 16000, 1, 2)

	if err := pipe.ProcessBatch(context.Background(), []byte{0x00, 0x40, 0x00, 0x40}); err != nil {
		t.Fatalf("ProcessBatch() error = %v", err)
	}
	if got := tts.lastSpeech.Input; got != "I love that idea!" {
		t.Errorf("TTS input = %q, want stripped text", got)
	}
}

func TestProcessBatchNoDirectiveSpeaksVerbatim(t *testing.T) {
	m := NewMachine()
	m.SetMode("ai")
	stt := &fakeProvider{transcript: "hello"}
	chat := &fakeProvider{reply: "just a normal reply"}
	tts := &fakeProvider{speech: make([]byte, 2400)}
	pipe := NewVoicePipeline(m, &fakeWriter{}, stt, chat, tts, "whisper-1", "gpt-4o-mini", "tts-1", "alloy", "be bmo", 16000, 1, 2)

	if err := pipe.ProcessBatch(context.Background(), []byte{0x00, 0x40, 0x00, 0x40}); err != nil {
		t.Fatalf("ProcessBatch() error = %v", err)
	}
	if got := tts.lastSpeech.Input; got != "just a normal reply" {
		t.Errorf("TTS input = %q, want verbatim", got)
	}
}

func TestProcessBatchDirectiveOnlyReplySkipsTTS(t *testing.T) {
	m := NewMachine()
	m.SetMode("ai")
	stt := &fakeProvider{transcript: "hello"}
	chat := &fakeProvider{reply: "[happy]"}
	tts := &fakeProvider{speech: make([]byte, 2400)}
	pipe := NewVoicePipeline(m, &fakeWriter{}, stt, chat, tts, "whisper-1", "gpt-4o-mini", "tts-1", "alloy", "be bmo", 16000, 1, 2)

	if err := pipe.ProcessBatch(context.Background(), []byte{0x00, 0x40, 0x00, 0x40}); err != nil {
		t.Fatalf("ProcessBatch() error = %v", err)
	}
	if tts.lastSpeech.Model != "" {
		t.Errorf("TTS called for directive-only reply; input = %q", tts.lastSpeech.Input)
	}
	if got := m.State(); got != StateIdle {
		t.Errorf("state = %v, want idle", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `CGO_ENABLED=0 go test ./internal/assistant/ -run TestProcessBatch -v`
Expected: FAIL — `TestProcessBatchStripsAndSetsEmotion` gets TTS input `"[excited] I love that idea!"`; `TestProcessBatchDirectiveOnlyReplySkipsTTS` calls TTS with empty input.

- [ ] **Step 3: Wire ParseEmotion into ProcessBatch**

In `internal/assistant/voice.go` `ProcessBatch`, the current block is:

```go
	reply := strings.TrimSpace(chat.Text)
	if reply == "" {
		if p.machine != nil {
			p.machine.Transition(EventRest)
		}
		return nil
	}
	if p.logger != nil {
		p.logger.Debugf("pipeline reply: %q", reply)
	}

	if p.logger != nil && p.logSystemPrompt {
		p.logger.Debugf("pipeline TTS instructions: %q", p.currentTTSInstructions())
	}

	ttsStart := time.Now()
	speech, err := p.tts.Speak(ctx, providers.SpeechRequest{
		Model:        p.ttsModel,
		Voice:        p.ttsVoice,
		Input:        reply,
		Format:       "pcm",
		Instructions: p.currentTTSInstructions(),
	})
```

Replace it with:

```go
	reply := strings.TrimSpace(chat.Text)
	if reply == "" {
		if p.machine != nil {
			p.machine.Transition(EventRest)
		}
		return nil
	}
	if p.logger != nil {
		p.logger.Debugf("pipeline reply: %q", reply)
	}

	spoken, emotion := ParseEmotion(reply)
	if spoken == "" {
		// Reply was only a facial directive with no speakable text.
		if p.machine != nil {
			p.machine.Transition(EventRest)
		}
		return nil
	}
	if emotion != "" {
		if p.machine != nil {
			p.machine.SetEmotion(emotion)
		}
		if p.logger != nil {
			p.logger.Debugf("pipeline emotion: %q", emotion)
		}
	}

	if p.logger != nil && p.logSystemPrompt {
		p.logger.Debugf("pipeline TTS instructions: %q", p.currentTTSInstructions())
	}

	ttsStart := time.Now()
	speech, err := p.tts.Speak(ctx, providers.SpeechRequest{
		Model:        p.ttsModel,
		Voice:        p.ttsVoice,
		Input:        spoken,
		Format:       "pcm",
		Instructions: p.currentTTSInstructions(),
	})
```

Also update the TTS info log a few lines below to count the spoken text. Change:

```go
		p.logger.Infof("pipeline TTS: %dms (%d bytes) | input: %d chars | total: %dms",
			time.Since(ttsStart).Milliseconds(), len(speech), len(reply),
			time.Since(totalStart).Milliseconds())
```

to use `len(spoken)`:

```go
		p.logger.Infof("pipeline TTS: %dms (%d bytes) | input: %d chars | total: %dms",
			time.Since(ttsStart).Milliseconds(), len(speech), len(spoken),
			time.Since(totalStart).Milliseconds())
```

- [ ] **Step 4: Wire ParseEmotion into SpeakRemark**

In `internal/assistant/voice.go` `SpeakRemark`, the current block is:

```go
	reply := strings.TrimSpace(chat.Text)
	if reply == "" {
		if p.machine != nil {
			p.machine.Transition(EventRest)
		}
		return nil
	}
	if p.logger != nil {
		p.logger.Debugf("remark reply: %q", reply)
	}

	ttsStart := time.Now()
	speech, err := p.tts.Speak(ctx, providers.SpeechRequest{
		Model:        p.ttsModel,
		Voice:        p.ttsVoice,
		Input:        reply,
		Format:       "pcm",
		Instructions: p.currentTTSInstructions(),
	})
	if err != nil {
		return p.fail(err)
	}
	if p.logger != nil {
		p.logger.Infof("remark TTS: %dms (%d bytes) | input: %d chars",
			time.Since(ttsStart).Milliseconds(), len(speech), len(reply))
	}
	if onSpoken != nil {
		onSpoken(reply)
	}
```

Replace it with:

```go
	reply := strings.TrimSpace(chat.Text)
	if reply == "" {
		if p.machine != nil {
			p.machine.Transition(EventRest)
		}
		return nil
	}
	if p.logger != nil {
		p.logger.Debugf("remark reply: %q", reply)
	}

	spoken, emotion := ParseEmotion(reply)
	if spoken == "" {
		if p.machine != nil {
			p.machine.Transition(EventRest)
		}
		return nil
	}
	if emotion != "" && p.machine != nil {
		p.machine.SetEmotion(emotion)
	}

	ttsStart := time.Now()
	speech, err := p.tts.Speak(ctx, providers.SpeechRequest{
		Model:        p.ttsModel,
		Voice:        p.ttsVoice,
		Input:        spoken,
		Format:       "pcm",
		Instructions: p.currentTTSInstructions(),
	})
	if err != nil {
		return p.fail(err)
	}
	if p.logger != nil {
		p.logger.Infof("remark TTS: %dms (%d bytes) | input: %d chars",
			time.Since(ttsStart).Milliseconds(), len(speech), len(spoken))
	}
	if onSpoken != nil {
		onSpoken(spoken)
	}
```

- [ ] **Step 5: Run the pipeline tests to verify they pass**

Run: `CGO_ENABLED=0 go test ./internal/assistant/ -run 'TestProcessBatch|TestSpeakRemark|TestVoicePipeline' -v`
Expected: PASS (new emotion tests plus all existing pipeline tests; the existing `TestSpeakRemarkInvokesOnSpoken` uses a directive-free reply so `onSpoken` still receives the same text).

- [ ] **Step 6: Commit**

```bash
git add internal/assistant/voice.go internal/assistant/voice_test.go
git commit -m "feat(assistant): strip and apply emotion directives before TTS"
```

---

## Task 7: Render the directed emotion during speech

**Files:**
- Modify: `cmd/bmo-pak/main.go:541-543`

(`cmd/bmo-pak` requires CGO+SDL and is not unit-tested; verify via `go vet`/build reasoning. The change is small and isolated.)

- [ ] **Step 1: Update the Speaking case**

In `cmd/bmo-pak/main.go`, replace the Speaking case (lines 541-543):

```go
		case assistant.StateSpeaking:
			errorSince = time.Time{}
			expr = string(assistant.ExpressionSpeaking)
```

with:

```go
		case assistant.StateSpeaking:
			errorSince = time.Time{}
			if snap.Emotion != "" {
				expr = string(snap.Emotion)
			} else {
				expr = string(assistant.ExpressionSpeaking)
			}
```

- [ ] **Step 2: Verify it compiles (best-effort without SDL)**

Run: `CGO_ENABLED=0 go vet ./cmd/bmo-pak/ 2>&1 | head -20`
Expected: The only errors are the pre-existing SDL/CGO build constraints (e.g. `undefined: sdl...` from the renderer), NOT a reference to `snap.Emotion` or `assistant.ExpressionSpeaking`. If you see an error naming `Emotion` or a syntax error in main.go, fix it. `snap` is `machine.Snapshot()` (already in scope at line 501) and `Emotion` exists after Task 5.

- [ ] **Step 3: Commit**

```bash
git add cmd/bmo-pak/main.go
git commit -m "feat(bmo-pak): show LLM-directed emotion face during speech"
```

---

## Task 8: Full verification

**Files:** none (verification only)

- [ ] **Step 1: Run the full test suite**

Run: `CGO_ENABLED=0 go test ./... 2>&1 | tail -30`
Expected: `internal/assistant` and `internal/face` (and all non-SDL packages) `ok`. The only failures permitted are `cmd/bmo-pak` and `internal/renderer` failing to **build** due to CGO/SDL — pre-existing and unrelated. If any other package fails, fix it.

- [ ] **Step 2: Race check on the touched package**

Run: `CGO_ENABLED=1 go test -race ./internal/assistant/ 2>&1 | tail -15`
Expected: PASS, no data races (guards the `SetEmotion`/`Snapshot` mutex usage).

- [ ] **Step 3: Lint**

Run: `golangci-lint run ./... 2>&1 | tail -30`
Expected: No new findings in `internal/assistant` or `cmd/bmo-pak`.

- [ ] **Step 4: Final commit (only if any fixups were made)**

```bash
git add -A
git commit -m "chore(assistant): verification fixups for LLM-directed emotion"
```

(If no fixups were needed, skip this step.)

---

## Notes for the implementer

- **No `git add -A`** except the explicit Task 8 fixup step — the repo has untracked tool dirs (`.claude/`, `.headroom/`, `.remember/`, `.serena/`, `.superpowers/`) that must never be staged. Stage only the named files in each task.
- **No `Co-Authored-By`** trailer on commits.
- BMO button layout for reference (unchanged here): A=BTN_EAST(305) confirm/PTT, B=BTN_SOUTH(304) cancel/exit.
- Out of scope (do not build): the animation engine (mouth motion under a held emotion), multiple emotions per utterance with positional timing, idle-scheduler use of the new faces.
