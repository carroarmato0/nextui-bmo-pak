# BMO Short-Term Memory Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stop BMO's proactive remarks from repeating by adding a persisted remark journal (picker cooldown + LLM-visible `RECENT REMARKS` prompt block), a verbatim quotes fallback, and DEBUG logging of the full prompt context.

**Architecture:** A `Journal` in `internal/devctx` is the single source of truth for what BMO recently said. The nudge picker filters candidates whose subject was remarked within 6 h and falls back to curated Adventure Time quotes (spoken verbatim, no chat call). The system prompt gains a `RECENT REMARKS` segment. The journal persists as one ~2–4 KB JSON file written atomically once per spoken remark.

**Tech Stack:** Go (stdlib only — `encoding/json`, `os.Rename` for atomic writes). Spec: `docs/specs/2026-06-12-short-term-memory-spec.md`.

**Conventions for this repo:**
- Run all commands from the repo root: `/home/carroarmato0/Applications/Development/NextUI/Paks/BMO`
- After each task: `gofmt -l ./internal ./cmd` must print nothing, and `golangci-lint run ./...` must introduce no new findings.
- Commit messages: conventional-commit style, **no Co-Authored-By trailer**.
- `cmd/bmo-pak/main_sdl.go` and `cmd/bmo-pak/main_fb.go` are near-identical mains behind build tags. **Every change to one must be mirrored in the other** — the same code blocks exist in both (line numbers differ slightly).

---

### Task 1: Remark journal (`internal/devctx/journal.go`)

**Files:**
- Create: `internal/devctx/journal.go`
- Test: `internal/devctx/journal_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/devctx/journal_test.go`:

```go
package devctx

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func journalEntry(when time.Time, topic, subject, reply string) RemarkEntry {
	return RemarkEntry{When: when, Topic: topic, Subject: subject, Reply: reply}
}

func TestJournalAppendPersistsAndCaps(t *testing.T) {
	path := filepath.Join(t.TempDir(), "remarks.json")
	j, err := LoadJournal(path)
	if err != nil {
		t.Fatalf("load fresh journal: %v", err)
	}
	base := time.Date(2026, 6, 12, 1, 0, 0, 0, time.UTC)
	for i := 0; i < 25; i++ {
		e := journalEntry(base.Add(time.Duration(i)*time.Minute), KeyAchievements, "subj", "reply")
		e.Reply = e.When.Format("15:04")
		if err := j.Append(e); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	if got := len(j.Recent(100)); got != journalCap {
		t.Fatalf("in-memory entries = %d, want %d", got, journalCap)
	}
	// Reload from disk: capped, newest entries kept, oldest first.
	j2, err := LoadJournal(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	entries := j2.Recent(100)
	if len(entries) != journalCap {
		t.Fatalf("persisted entries = %d, want %d", len(entries), journalCap)
	}
	if entries[0].Reply != "01:05" || entries[len(entries)-1].Reply != "01:24" {
		t.Fatalf("wrong window kept: first=%q last=%q", entries[0].Reply, entries[len(entries)-1].Reply)
	}
	// No stray temp file left behind.
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("temp file left behind after rename")
	}
}

func TestLoadJournalMissingFile(t *testing.T) {
	j, err := LoadJournal(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatalf("missing file must not error: %v", err)
	}
	if got := len(j.Recent(10)); got != 0 {
		t.Fatalf("expected empty journal, got %d entries", got)
	}
}

func TestLoadJournalCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "remarks.json")
	if err := os.WriteFile(path, []byte("{half-written"), 0o600); err != nil {
		t.Fatal(err)
	}
	j, err := LoadJournal(path)
	if err == nil {
		t.Fatal("expected a load error for corrupt JSON")
	}
	if j == nil {
		t.Fatal("corrupt file must still return a usable journal")
	}
	// The journal recovers: an append overwrites the corrupt file.
	if err := j.Append(journalEntry(time.Now().UTC(), KeySaves, KeySaves, "hi")); err != nil {
		t.Fatalf("append after corrupt load: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var entries []RemarkEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatalf("file not valid JSON after recovery append: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
}

func TestJournalLastRemarkedAtAndContains(t *testing.T) {
	now := time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC)
	j := &Journal{entries: []RemarkEntry{
		journalEntry(now.Add(-5*time.Hour), KeyAchievements, `"Moon Presence" in Deadeus`, "wow"),
		journalEntry(now.Add(-1*time.Hour), KeySaves, KeySaves, "save files!"),
	}}
	if got := j.LastRemarkedAt(`"Moon Presence" in Deadeus`); !got.Equal(now.Add(-5 * time.Hour)) {
		t.Fatalf("LastRemarkedAt = %v", got)
	}
	if !j.LastRemarkedAt("never-mentioned").IsZero() {
		t.Fatal("unknown subject must return zero time")
	}
	if !j.Contains(KeySaves) || j.Contains("never-mentioned") {
		t.Fatal("Contains mismatch")
	}
}

func TestJournalPromptBlock(t *testing.T) {
	now := time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC)
	j := &Journal{}
	for i := 0; i < 7; i++ {
		j.entries = append(j.entries, journalEntry(now.Add(time.Duration(i-7)*time.Hour), TopicQuote, "q", "line "+string(rune('A'+i))))
	}
	block := j.PromptBlock(now)
	if !strings.HasPrefix(block, "RECENT REMARKS (") {
		t.Fatalf("missing header: %q", block)
	}
	// Only the last promptRemarks entries appear, oldest first.
	if strings.Contains(block, "line A") || strings.Contains(block, "line B") {
		t.Fatalf("old entries leaked into block: %q", block)
	}
	for _, want := range []string{`"line C"`, `"line G"`, "5 hours ago", "1 hour ago"} {
		if !strings.Contains(block, want) {
			t.Fatalf("block missing %q: %q", want, block)
		}
	}
	if idxC, idxG := strings.Index(block, "line C"), strings.Index(block, "line G"); idxC > idxG {
		t.Fatal("entries not oldest-first")
	}
}

func TestJournalPromptBlockEmpty(t *testing.T) {
	if got := (&Journal{}).PromptBlock(time.Now()); got != "" {
		t.Fatalf("empty journal must render no block, got %q", got)
	}
}

func TestJournalNilSafe(t *testing.T) {
	var j *Journal
	if err := j.Append(RemarkEntry{}); err != nil {
		t.Fatal("nil Append must be a no-op")
	}
	if j.Recent(5) != nil || !j.LastRemarkedAt("x").IsZero() || j.Contains("x") || j.PromptBlock(time.Now()) != "" {
		t.Fatal("nil journal methods must return zero values")
	}
}
```

Note: `TestJournalPromptBlock` relies on `RelTime` from `internal/devctx/reltime.go:12` rendering "5 hours ago" / "1 hour ago". If the exact phrasing differs, check `reltime_test.go` for the canonical strings and adjust the `want` list — do not change `RelTime`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/devctx/ -run 'TestJournal|TestLoadJournal' -v`
Expected: FAIL — `undefined: RemarkEntry`, `undefined: LoadJournal`, etc.

- [ ] **Step 3: Implement the journal**

Create `internal/devctx/journal.go`:

```go
package devctx

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// journalCap bounds the remark journal: old entries fall off so the file
// stays a few KB and quote dedup self-regulates against the journal window.
const journalCap = 20

// promptRemarks is how many recent entries PromptBlock renders.
const promptRemarks = 5

// TopicQuote marks verbatim quote entries in the journal.
const TopicQuote = "quote"

// RemarkEntry is one spoken proactive remark (or verbatim quote).
type RemarkEntry struct {
	When    time.Time `json:"when"`
	Topic   string    `json:"topic"`
	Subject string    `json:"subject"`
	Reply   string    `json:"reply"`
}

// Journal is BMO's short-term memory of what he recently said out loud,
// consumed by the nudge picker (cooldown dedup) and the system prompt
// (RECENT REMARKS block). It persists as one small JSON file written
// atomically once per spoken remark; a missing or corrupt file is never
// fatal — BMO just starts with an empty memory.
type Journal struct {
	mu      sync.Mutex
	path    string
	entries []RemarkEntry
}

// LoadJournal reads the journal at path. It always returns a usable
// journal; a non-nil error only reports why an existing file could not be
// used (e.g. corrupt JSON after a hard power-off) so the caller can log it.
func LoadJournal(path string) (*Journal, error) {
	j := &Journal{path: path}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return j, nil
		}
		return j, err
	}
	var entries []RemarkEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return j, err
	}
	if len(entries) > journalCap {
		entries = entries[len(entries)-journalCap:]
	}
	j.entries = entries
	return j, nil
}

// Append records a spoken remark and persists the journal via temp-file +
// rename so a hard power-off never leaves a half-written file in place.
func (j *Journal) Append(e RemarkEntry) error {
	if j == nil {
		return nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	j.entries = append(j.entries, e)
	if len(j.entries) > journalCap {
		j.entries = j.entries[len(j.entries)-journalCap:]
	}
	return j.saveLocked()
}

func (j *Journal) saveLocked() error {
	data, err := json.Marshal(j.entries)
	if err != nil {
		return err
	}
	tmp := j.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, j.path)
}

// Recent returns up to n of the most recent entries, oldest first.
func (j *Journal) Recent(n int) []RemarkEntry {
	if j == nil || n <= 0 {
		return nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	start := len(j.entries) - n
	if start < 0 {
		start = 0
	}
	if len(j.entries) == 0 {
		return nil
	}
	out := make([]RemarkEntry, len(j.entries)-start)
	copy(out, j.entries[start:])
	return out
}

// LastRemarkedAt returns when subject was last remarked about, or the zero
// time if it never was (within the journal window).
func (j *Journal) LastRemarkedAt(subject string) time.Time {
	if j == nil {
		return time.Time{}
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	for i := len(j.entries) - 1; i >= 0; i-- {
		if j.entries[i].Subject == subject {
			return j.entries[i].When
		}
	}
	return time.Time{}
}

// Contains reports whether subject appears anywhere in the journal window.
func (j *Journal) Contains(subject string) bool {
	return !j.LastRemarkedAt(subject).IsZero()
}

// PromptBlock renders the RECENT REMARKS system prompt segment, or "" when
// the journal is empty.
func (j *Journal) PromptBlock(now time.Time) string {
	entries := j.Recent(promptRemarks)
	if len(entries) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("RECENT REMARKS (things you already said out loud recently; never repeat them — vary your angle or build on them, and never re-announce news you already announced):\n")
	for _, e := range entries {
		sb.WriteString("- " + RelTime(e.When, now) + ": " + strconv.Quote(e.Reply) + "\n")
	}
	return strings.TrimSpace(sb.String())
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/devctx/ -v`
Expected: all PASS (new journal tests plus all pre-existing devctx tests).

- [ ] **Step 5: Lint and commit**

Run: `gofmt -l ./internal && golangci-lint run ./internal/devctx/`
Expected: no output / no new findings.

```bash
git add internal/devctx/journal.go internal/devctx/journal_test.go
git commit -m "feat: remark journal — BMO's persisted short-term memory of spoken remarks"
```

---

### Task 2: `SpeakRemark` callback + DEBUG prompt logging + `SpeakVerbatim`

**Files:**
- Modify: `internal/assistant/voice.go:256-315` (SpeakRemark), add SpeakVerbatim below it
- Modify: `internal/assistant/voice_test.go` (call sites, captureLogger.Debugf, new tests)
- Modify: `cmd/bmo-pak/main_sdl.go:455` and `cmd/bmo-pak/main_fb.go:395` (call sites — add `nil` arg)

- [ ] **Step 1: Make `captureLogger` record debug lines**

In `internal/assistant/voice_test.go`, replace the empty `Debugf` (currently `func (l *captureLogger) Debugf(format string, args ...any) {}`):

```go
func (l *captureLogger) Debugf(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, fmt.Sprintf(format, args...))
}
```

(Existing assertions are all `strings.Contains`, so extra captured lines cannot break them.)

- [ ] **Step 2: Write the failing tests**

Append to `internal/assistant/voice_test.go`:

```go
func TestSpeakRemarkLogsPromptContext(t *testing.T) {
	m := NewMachine()
	m.SetMode("ai")
	chat := &fakeProvider{reply: "daebak!"}
	tts := &fakeProvider{speech: []byte{1, 2, 3, 4}}
	pipe := NewVoicePipeline(m, &fakeWriter{}, &fakeProvider{}, chat, tts, "", "gpt-4o-mini", "tts-1", "alloy", "", 16000, 1)
	pipe.SetSystemPromptSource(func() string { return "persona\n\nDEVICE AWARENESS: stuff" })
	logger := &captureLogger{}
	pipe.SetLogger(logger)

	if err := pipe.SpeakRemark(context.Background(), "(nudge about achievements)", nil); err != nil {
		t.Fatalf("speak remark: %v", err)
	}
	logs := logger.joined()
	if !strings.Contains(logs, `remark nudge: "(nudge about achievements)"`) {
		t.Errorf("nudge not logged: %q", logs)
	}
	if !strings.Contains(logs, "remark system prompt:") || !strings.Contains(logs, "DEVICE AWARENESS: stuff") {
		t.Errorf("system prompt not logged: %q", logs)
	}
}

func TestSpeakRemarkInvokesOnSpoken(t *testing.T) {
	m := NewMachine()
	m.SetMode("ai")
	chat := &fakeProvider{reply: "what a save file!"}
	tts := &fakeProvider{speech: []byte{1, 2, 3, 4}}
	pipe := NewVoicePipeline(m, &fakeWriter{}, &fakeProvider{}, chat, tts, "", "gpt-4o-mini", "tts-1", "alloy", "", 16000, 1)

	var spoken []string
	if err := pipe.SpeakRemark(context.Background(), "(nudge)", func(reply string) { spoken = append(spoken, reply) }); err != nil {
		t.Fatalf("speak remark: %v", err)
	}
	if len(spoken) != 1 || spoken[0] != "what a save file!" {
		t.Fatalf("onSpoken calls = %v, want one call with the reply", spoken)
	}
}

func TestSpeakRemarkOnSpokenSkippedOnFailure(t *testing.T) {
	m := NewMachine()
	m.SetMode("ai")
	called := 0
	onSpoken := func(string) { called++ }

	// Chat failure: callback must not fire.
	chatFail := &fakeProvider{err: fmt.Errorf("boom")}
	pipe := NewVoicePipeline(m, &fakeWriter{}, &fakeProvider{}, chatFail, &fakeProvider{}, "", "gpt-4o-mini", "", "", "", 16000, 1)
	if err := pipe.SpeakRemark(context.Background(), "(nudge)", onSpoken); err == nil {
		t.Fatal("expected chat error")
	}
	// Empty reply: callback must not fire.
	m2 := NewMachine()
	m2.SetMode("ai")
	pipe2 := NewVoicePipeline(m2, &fakeWriter{}, &fakeProvider{}, &fakeProvider{reply: "  "}, &fakeProvider{}, "", "gpt-4o-mini", "", "", "", 16000, 1)
	if err := pipe2.SpeakRemark(context.Background(), "(nudge)", onSpoken); err != nil {
		t.Fatalf("speak remark: %v", err)
	}
	// TTS failure: callback must not fire.
	m3 := NewMachine()
	m3.SetMode("ai")
	pipe3 := NewVoicePipeline(m3, &fakeWriter{}, &fakeProvider{}, &fakeProvider{reply: "hi"}, &fakeProvider{err: fmt.Errorf("tts boom")}, "", "gpt-4o-mini", "tts-1", "alloy", "", 16000, 1)
	if err := pipe3.SpeakRemark(context.Background(), "(nudge)", onSpoken); err == nil {
		t.Fatal("expected tts error")
	}
	if called != 0 {
		t.Fatalf("onSpoken fired %d times on failure paths, want 0", called)
	}
}

func TestSpeakVerbatimSkipsChat(t *testing.T) {
	m := NewMachine()
	m.SetMode("ai")
	writer := &fakeWriter{}
	chat := &fakeProvider{reply: "must never be used"}
	tts := &fakeProvider{speech: []byte{1, 2, 3, 4}}
	pipe := NewVoicePipeline(m, writer, &fakeProvider{}, chat, tts, "", "gpt-4o-mini", "tts-1", "alloy", "", 16000, 1)

	var spoken []string
	if err := pipe.SpeakVerbatim(context.Background(), "Who wants to play video games?", func(s string) { spoken = append(spoken, s) }); err != nil {
		t.Fatalf("speak verbatim: %v", err)
	}
	if chat.lastChat.Model != "" {
		t.Error("chat provider must not be called for verbatim speech")
	}
	if tts.lastSpeech.Input != "Who wants to play video games?" {
		t.Errorf("tts input = %q", tts.lastSpeech.Input)
	}
	if len(spoken) != 1 || spoken[0] != "Who wants to play video games?" {
		t.Fatalf("onSpoken = %v", spoken)
	}
	if writer.totalBytes() == 0 {
		t.Error("expected PCM written to playback")
	}
	if got := m.State(); got != StateIdle {
		t.Errorf("state after verbatim = %v, want idle", got)
	}
}

func TestSpeakVerbatimSkippedWhenNotIdle(t *testing.T) {
	m := NewMachine()
	m.SetMode("ai")
	m.Transition(EventListen)
	tts := &fakeProvider{speech: []byte{1, 2}}
	pipe := NewVoicePipeline(m, &fakeWriter{}, &fakeProvider{}, &fakeProvider{}, tts, "", "", "tts-1", "alloy", "", 16000, 1)
	if err := pipe.SpeakVerbatim(context.Background(), "quote", nil); err != nil {
		t.Fatalf("speak verbatim: %v", err)
	}
	if tts.lastSpeech.Input != "" {
		t.Error("tts must not be called while not idle")
	}
}
```

Also update the six existing `SpeakRemark` call sites in `voice_test.go` (in `TestSpeakRemarkHappyPath`, `TestSpeakRemarkSkippedOutsideAIMode`, `TestSpeakRemarkSkippedWhenNotIdle`, `TestSpeakRemarkEmptyReplyReturnsToIdle`, `TestSpeakRemarkChatFailureEntersErrorState`, `TestRemarkLogsTokenUsage`) to pass `nil` as the new third argument, e.g.:

```go
if err := pipe.SpeakRemark(context.Background(), "(BMO says something about achievements)", nil); err != nil {
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/assistant/ -run 'TestSpeakRemark|TestSpeakVerbatim' -v`
Expected: FAIL — wrong argument count for SpeakRemark, `undefined: pipe.SpeakVerbatim`.

- [ ] **Step 4: Implement the voice.go changes**

In `internal/assistant/voice.go`, replace `SpeakRemark` (lines 250–315) with:

```go
// SpeakRemark generates and speaks a spontaneous proactive remark. The
// nudge is a stage direction sent as the user message (there is no real
// user speech); the reply flows through the normal TTS → playback path, so
// PTT interruption, amplitude-driven mouth, and state transitions all
// behave exactly like a normal utterance. No-op outside AI mode or when
// BMO is not idle — a remark must never barge into a conversation.
// onSpoken, when non-nil, is invoked with the reply text once TTS has
// succeeded (i.e. the remark will actually be heard), so the caller can
// record it in the remark journal.
func (p *VoicePipeline) SpeakRemark(ctx context.Context, nudge string, onSpoken func(reply string)) error {
	if p == nil || !p.aiModeEnabled() {
		return nil
	}
	nudge = strings.TrimSpace(nudge)
	if nudge == "" {
		return nil
	}
	if p.machine != nil {
		// EventRemark only succeeds from idle, so a PTT press racing this
		// call cannot be hijacked: if EventListen landed first, the
		// transition is refused and the remark is silently dropped.
		if p.machine.Transition(EventRemark) != StateThinking {
			return nil
		}
	}

	systemPrompt := p.currentSystemPrompt()
	if p.logger != nil {
		p.logger.Debugf("remark nudge: %q", nudge)
		p.logger.Debugf("remark system prompt: %q", systemPrompt)
	}
	chatStart := time.Now()
	chat, err := p.chat.Reply(ctx, providers.ChatRequest{
		Model:        p.chatModel,
		Messages:     []providers.Message{{Role: "user", Content: nudge}},
		SystemPrompt: systemPrompt,
	})
	if err != nil {
		return p.fail(err, EventFail)
	}
	if p.logger != nil {
		p.logger.Infof("remark Chat: %dms | tokens: %s",
			time.Since(chatStart).Milliseconds(), usageString(chat.Usage))
	}
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
		return p.fail(err, EventFail)
	}
	if p.logger != nil {
		p.logger.Infof("remark TTS: %dms (%d bytes) | input: %d chars",
			time.Since(ttsStart).Milliseconds(), len(speech), len(reply))
	}
	if onSpoken != nil {
		onSpoken(reply)
	}
	speech = audio.ResampleS16LE(speech, ttsPCMSampleRate, p.sampleRate, p.channels)

	return p.speak(ctx, speech)
}

// SpeakVerbatim speaks text exactly as given — no chat call, no paraphrase
// risk, zero chat tokens. Used for the curated-quote fallback when every
// real remark topic is on cooldown. Same idle-only gating and playback
// path as SpeakRemark; onSpoken fires once TTS has succeeded.
func (p *VoicePipeline) SpeakVerbatim(ctx context.Context, text string, onSpoken func(spoken string)) error {
	if p == nil || !p.aiModeEnabled() {
		return nil
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if p.machine != nil {
		if p.machine.Transition(EventRemark) != StateThinking {
			return nil
		}
	}
	if p.logger != nil {
		p.logger.Debugf("remark quote: %q", text)
	}

	ttsStart := time.Now()
	speech, err := p.tts.Speak(ctx, providers.SpeechRequest{
		Model:        p.ttsModel,
		Voice:        p.ttsVoice,
		Input:        text,
		Format:       "pcm",
		Instructions: p.currentTTSInstructions(),
	})
	if err != nil {
		return p.fail(err, EventFail)
	}
	if p.logger != nil {
		p.logger.Infof("remark TTS: %dms (%d bytes) | input: %d chars",
			time.Since(ttsStart).Milliseconds(), len(speech), len(text))
	}
	if onSpoken != nil {
		onSpoken(text)
	}
	speech = audio.ResampleS16LE(speech, ttsPCMSampleRate, p.sampleRate, p.channels)

	return p.speak(ctx, speech)
}
```

- [ ] **Step 5: Fix the two main call sites (compile only)**

In `cmd/bmo-pak/main_sdl.go:455` and `cmd/bmo-pak/main_fb.go:395`, the line inside the proactive goroutine becomes (full wiring lands in Task 5):

```go
if err := remarkPipeline.SpeakRemark(ctx, nudge, nil); err != nil {
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/assistant/ -v && go build ./...`
Expected: all PASS, build clean.

- [ ] **Step 7: Lint and commit**

Run: `gofmt -l ./internal ./cmd && golangci-lint run ./...`
Expected: no output / no new findings.

```bash
git add internal/assistant/voice.go internal/assistant/voice_test.go cmd/bmo-pak/main_sdl.go cmd/bmo-pak/main_fb.go
git commit -m "feat: remark onSpoken hook, SpeakVerbatim path, DEBUG prompt-context logging"
```

---

### Task 3: Nudge struct, subjects, and 6 h cooldown in the picker

**Files:**
- Modify: `internal/devctx/devctx.go:21-26` (Section gains Subject)
- Modify: `internal/devctx/snapshot.go:24,57` (reminisce signature, journal field, SetJournal)
- Modify: `internal/devctx/achievements.go:199-255` (Section.Subject, RandomPastUnlock subject)
- Modify: `internal/devctx/nudge.go` (Nudge struct, cooldown filtering)
- Test: `internal/devctx/nudge_test.go`, `internal/devctx/achievements_test.go:177-192`

- [ ] **Step 1: Write the failing tests**

In `internal/devctx/nudge_test.go`, update the helper for the new reminisce signature and update the four existing tests to the struct return; then add cooldown tests. The full new file content:

```go
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
	now := time.Date(2026, 6, 11, 16, 45, 0, 0, time.UTC)
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
	now := time.Date(2026, 6, 11, 16, 45, 0, 0, time.UTC)
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
	b.SetJournal(&Journal{entries: []RemarkEntry{
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
	b.SetJournal(&Journal{entries: []RemarkEntry{
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
	b.SetJournal(&Journal{entries: []RemarkEntry{
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
	b.SetJournal(&Journal{entries: []RemarkEntry{
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
	b.SetJournal(&Journal{entries: []RemarkEntry{
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
```

In `internal/devctx/achievements_test.go:177-192`, update `TestRandomPastUnlock`:

```go
func TestRandomPastUnlock(t *testing.T) {
	now := time.Date(2026, 6, 11, 16, 0, 0, 0, time.UTC)
	c := fixtureRA(t, now, "1")
	memory, subject, ok := c.RandomPastUnlock(now)
	if !ok {
		t.Fatal("expected a past unlock")
	}
	for _, want := range []string{`"Reach Stage 7"`, "Alleyway", "2 hours ago"} {
		if !strings.Contains(memory, want) {
			t.Errorf("memory missing %q: %q", want, memory)
		}
	}
	if subject != `"Reach Stage 7" in Alleyway` {
		t.Errorf("subject = %q", subject)
	}
	c2 := fixtureRA(t, now, "0")
	if _, _, ok := c2.RandomPastUnlock(now); ok {
		t.Fatal("expected no reminisce when RA disabled")
	}
}
```

Also add to `achievements_test.go` (uses the same `fixtureRA` helper) an assertion that `Collect` sets the subject — extend the existing Collect test (find the test asserting on `Section.Body`; add):

```go
	if section.Subject != `"Reach Stage 7" in Alleyway` {
		t.Errorf("section subject = %q", section.Subject)
	}
```

(Adjust the expected title/game to the fixture's newest unlock — `"Reach Stage 7"` in Alleyway per the existing fixture; if the fixture's newest unlock differs, use that one. The rule under test: Subject = newest unlock formatted `%q in %s`.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/devctx/ -v`
Expected: FAIL — compile errors (`SetJournal` undefined, `Nudge` undefined, reminisce signature mismatch).

- [ ] **Step 3: Implement**

`internal/devctx/devctx.go` — add Subject to Section (after Body):

```go
// Section is one category's contribution to the context block.
type Section struct {
	Key      string
	Title    string    // uppercase header, e.g. "GAME LIBRARY"
	Body     string    // formatted plain-text, no markdown
	Subject  string    // the specific news item, e.g. the newest unlock; "" = the category itself
	Freshest time.Time // most recent event in the category; zero = evergreen
}
```

`internal/devctx/snapshot.go` — change the reminisce field type (line 24) and add journal/quotes fields to Builder:

```go
	reminisce func(now time.Time) (memory, subject string, ok bool)
	journal   *Journal
	quotes    func() []string
```

Update `SetReminisce` to the new type, and add (next to it):

```go
// SetReminisce installs the reminisce source used by ProactiveNudge.
func (b *Builder) SetReminisce(fn func(time.Time) (string, string, bool)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.reminisce = fn
}

// SetJournal installs the remark journal consulted for cooldown dedup.
// A nil journal disables dedup (every candidate is always eligible).
func (b *Builder) SetJournal(j *Journal) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.journal = j
}
```

(`quotes` stays nil until Task 4; `SetQuotes` arrives there.)

`internal/devctx/achievements.go:234` — set the subject on the returned section (newest unlock is `recent[0]`):

```go
	return Section{
		Key:      KeyAchievements,
		Title:    "RETROACHIEVEMENTS",
		Body:     body,
		Subject:  fmt.Sprintf("%q in %s", recent[0].title, recent[0].game),
		Freshest: unlocks[0].when,
	}, nil
```

`internal/devctx/achievements.go:240-255` — RandomPastUnlock returns the stable subject in the same format:

```go
// RandomPastUnlock returns a one-line description of a randomly chosen past
// unlock for reminisce-style proactive remarks ("remember when..."), plus
// the stable subject used for journal cooldown dedup, or false when RA is
// disabled or nothing is unlocked.
func (c AchievementsCollector) RandomPastUnlock(now time.Time) (memory, subject string, ok bool) {
	if c.Rng == nil || !c.raEnabled() {
		return "", "", false
	}
	unlocks, err := c.load()
	if err != nil || len(unlocks) == 0 {
		return "", "", false
	}
	u := unlocks[c.Rng.Intn(len(unlocks))]
	subject = fmt.Sprintf("%q in %s", u.title, u.game)
	memory = fmt.Sprintf("the time the player unlocked %s (%s), %s",
		subject, u.description, RelTime(u.when, now))
	if tag := difficultyTag(u.points, u.rarity, u.achType); tag != "" {
		memory += " — " + tag
	}
	return memory, subject, true
}
```

`internal/devctx/nudge.go` — full new content:

```go
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
```

Note: `b.journal.LastRemarkedAt` / `b.journal.Contains` are nil-safe (Task 1), so no nil checks are needed at the call sites. `quoteNudge` exists now (Task 4 only wires `SetQuotes`); with `b.quotes == nil` it always returns false, which is what `TestProactiveNudgeAllOnCooldownIsSilent` asserts.

- [ ] **Step 4: Fix the call sites in both mains**

In `cmd/bmo-pak/main_sdl.go:163` and the matching line in `main_fb.go`, `SetReminisce(achievementsCollector.RandomPastUnlock)` still compiles (the method satisfies the new signature) — verify, no edit expected.

In `cmd/bmo-pak/main_sdl.go:452-458` and `cmd/bmo-pak/main_fb.go:392-398`, ProactiveNudge now returns a struct (full journal wiring lands in Task 5):

```go
			if n, ok := deviceCtx.ProactiveNudge(); ok {
				remarkPipeline := audioPipeline
				go func() {
					if err := remarkPipeline.SpeakRemark(ctx, n.Text, nil); err != nil {
						logger.Warnf("proactive remark failed: %v", err)
					}
				}()
			}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/devctx/ ./internal/assistant/ -v && go build ./...`
Expected: all PASS, build clean.

- [ ] **Step 6: Lint and commit**

Run: `gofmt -l ./internal ./cmd && golangci-lint run ./...`
Expected: no output / no new findings.

```bash
git add internal/devctx/ cmd/bmo-pak/
git commit -m "feat: 6h remark cooldown — nudge picker skips recently-covered subjects"
```

---

### Task 4: Curated quotes — asset, loader, picker fallback

**Files:**
- Create: `internal/config/quotes.go`
- Create: `internal/devctx/quotes.go`
- Test: `internal/devctx/quotes_test.go`, extend `internal/devctx/nudge_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/devctx/quotes_test.go`:

```go
package devctx

import (
	"reflect"
	"testing"
	"time"
)

func TestParseQuotes(t *testing.T) {
	content := "Who wants to play video games?\n\n# comment line\n  Check, please!  \n"
	got := ParseQuotes(content)
	want := []string{"Who wants to play video games?", "Check, please!"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseQuotes = %#v, want %#v", got, want)
	}
	if ParseQuotes("") != nil {
		t.Fatal("empty content must yield no quotes")
	}
}

func TestProactiveNudgeQuoteFallback(t *testing.T) {
	now := time.Now().UTC()
	saves := Section{Key: KeySaves, Title: "SAVE FILES", Body: "y", Freshest: now.Add(-1 * time.Hour)}
	b := nudgeBuilder(t, nil, saves)
	b.SetJournal(&Journal{entries: []RemarkEntry{
		{When: now.Add(-30 * time.Minute), Topic: KeySaves, Subject: KeySaves, Reply: "saves!"},
		{When: now.Add(-20 * time.Minute), Topic: TopicQuote, Subject: "Check, please!", Reply: "Check, please!"},
	}})
	b.SetQuotes(func() []string {
		return []string{"Who wants to play video games?", "Check, please!"}
	})
	sawQuote, sawSilence := false, false
	for i := 0; i < 300; i++ {
		n, ok := b.ProactiveNudge()
		if !ok {
			sawSilence = true
			continue
		}
		sawQuote = true
		if !n.Verbatim || n.Topic != TopicQuote {
			t.Fatalf("expected a verbatim quote nudge, got %+v", n)
		}
		// "Check, please!" sits in the journal window: never re-picked.
		if n.Text != "Who wants to play video games?" {
			t.Fatalf("journaled quote re-picked: %+v", n)
		}
		if n.Subject != n.Text {
			t.Fatalf("quote subject must equal its text: %+v", n)
		}
	}
	if !sawQuote || !sawSilence {
		t.Fatalf("expected both quotes and silence over 300 runs (quote=%v silence=%v)", sawQuote, sawSilence)
	}
}

func TestProactiveNudgeQuotesExhausted(t *testing.T) {
	now := time.Now().UTC()
	saves := Section{Key: KeySaves, Title: "SAVE FILES", Body: "y", Freshest: now.Add(-1 * time.Hour)}
	b := nudgeBuilder(t, nil, saves)
	b.SetJournal(&Journal{entries: []RemarkEntry{
		{When: now.Add(-30 * time.Minute), Topic: KeySaves, Subject: KeySaves, Reply: "saves!"},
		{When: now.Add(-20 * time.Minute), Topic: TopicQuote, Subject: "Check, please!", Reply: "Check, please!"},
	}})
	b.SetQuotes(func() []string { return []string{"Check, please!"} })
	for i := 0; i < 100; i++ {
		if n, ok := b.ProactiveNudge(); ok {
			t.Fatalf("every quote is journaled: expected silence, got %+v", n)
		}
	}
}

func TestProactiveNudgeRealTopicsBeatQuotes(t *testing.T) {
	now := time.Now().UTC()
	saves := Section{Key: KeySaves, Title: "SAVE FILES", Body: "y", Freshest: now.Add(-1 * time.Hour)}
	b := nudgeBuilder(t, nil, saves)
	b.SetJournal(&Journal{})
	b.SetQuotes(func() []string { return []string{"Check, please!"} })
	for i := 0; i < 50; i++ {
		n, ok := b.ProactiveNudge()
		if !ok || n.Verbatim {
			t.Fatalf("real topic must always beat the quote fallback: ok=%v n=%+v", ok, n)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/devctx/ -run 'TestParseQuotes|TestProactiveNudgeQuote|TestProactiveNudgeRealTopics' -v`
Expected: FAIL — `undefined: ParseQuotes`, `undefined: b.SetQuotes`.

- [ ] **Step 3: Implement the devctx side**

Create `internal/devctx/quotes.go`:

```go
package devctx

import "strings"

// ParseQuotes splits quotes.txt content into one quote per line, skipping
// blank lines and #-comments.
func ParseQuotes(content string) []string {
	var out []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}

// SetQuotes installs the source of verbatim fallback quotes used when every
// real remark topic is on cooldown. The source is consulted at pick time so
// edits to quotes.txt apply without a restart.
func (b *Builder) SetQuotes(fn func() []string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.quotes = fn
}
```

- [ ] **Step 4: Add the embedded default quotes**

Create `internal/config/quotes.go`. The list is curated for standalone delivery (no scene context needed); users edit `quotes.txt` freely. **Implementer: use exactly this list — do not invent additional quotes.**

```go
package config

import "path/filepath"

// QuotesPath returns the location of the verbatim quotes file.
func QuotesPath(homeDir string) string {
	return filepath.Join(homeDir, "quotes.txt")
}

// DefaultQuotes seeds quotes.txt: curated standalone BMO one-liners from
// the Adventure Time series, one per line, spoken verbatim by the
// proactive-quote fallback. Lines starting with # are ignored.
const DefaultQuotes = `Who wants to play video games?
Football needs my help!
Check, please!
Dance with me, you fool!
I just blew my own mind!
Yay! I sure do love being alive!
Time to mash them buttons!
Do you want to see my new dance?
I am a real living boy!
Hi-ho, neighbor!
Let us all go to the movies!
Shh. This is the good part.
I have stories in me!
Beep boop beep boop!
Sweet babies!
I am a tough little champ!
I am the prettiest robot.
Please do not touch my buttons without washing your hands!
You are my best friend in the whole world.
Today is a good day to play!
Be careful, little one.
I will protect you with my robot body!
My battery is full of love.
Hello, friend of BMO!
Press start to have fun!
Game on!
My circuits are tingling!
High five!
I read you loud and clear, captain!
Initiating party mode!
Ooh, this makes my fans spin!
I dreamed I was a real boy again.
Victory is delicious!
Do not worry. BMO is here.
Let us make a wish together!
I am small, but I am mighty!`
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/devctx/ ./internal/config/ -v && go build ./...`
Expected: all PASS, build clean.

- [ ] **Step 6: Lint and commit**

Run: `gofmt -l ./internal && golangci-lint run ./...`
Expected: no output / no new findings.

```bash
git add internal/devctx/quotes.go internal/devctx/quotes_test.go internal/config/quotes.go
git commit -m "feat: curated BMO quotes fallback — verbatim spice when every topic is on cooldown"
```

---

### Task 5: Wire it all up in both mains

**Files:**
- Modify: `cmd/bmo-pak/ptt_shared.go:36-47` (variadic systemPromptWithContext)
- Modify: `cmd/bmo-pak/main_sdl.go` (~lines 163, 194-196, 450-460)
- Modify: `cmd/bmo-pak/main_fb.go` (same blocks; line numbers differ — locate by searching for the identical code)
- Test: `cmd/bmo-pak/ptt_shared_test.go`

- [ ] **Step 1: Make systemPromptWithContext variadic (failing test first)**

Add to `cmd/bmo-pak/ptt_shared_test.go`:

```go
func TestSystemPromptWithContextThreeSegments(t *testing.T) {
	got := systemPromptWithContext("persona", "device", "remarks")
	if got != "persona\n\ndevice\n\nremarks" {
		t.Fatalf("three segments joined wrong: %q", got)
	}
	if got := systemPromptWithContext("persona", "", "remarks"); got != "persona\n\nremarks" {
		t.Fatalf("empty middle segment not skipped: %q", got)
	}
	if got := systemPromptWithContext("", "", ""); got != "" {
		t.Fatalf("all-empty must be empty: %q", got)
	}
}
```

Run: `go test ./cmd/bmo-pak/ -run TestSystemPromptWithContextThreeSegments -v` — Expected: FAIL (too many arguments).

In `cmd/bmo-pak/ptt_shared.go:34-47`, replace the function:

```go
// systemPromptWithContext joins the persona prompt, the device-awareness
// block, and the recent-remarks block; empty segments are skipped.
func systemPromptWithContext(parts ...string) string {
	var nonEmpty []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			nonEmpty = append(nonEmpty, p)
		}
	}
	return strings.Join(nonEmpty, "\n\n")
}
```

Run: `go test ./cmd/bmo-pak/ -v` — Expected: PASS (existing two-arg tests still compile against variadic).

- [ ] **Step 2: Wire journal + quotes in main_sdl.go**

After `deviceCtx.SetReminisce(achievementsCollector.RandomPastUnlock)` (`main_sdl.go:163`), insert:

```go
	// Short-term memory: the remark journal feeds both the nudge picker
	// (6h cooldown dedup) and the RECENT REMARKS prompt block. A corrupt
	// file (hard power-off mid-write) just means starting empty.
	journalPath := filepath.Join(homeDir, "remarks.json")
	journal, jerr := devctx.LoadJournal(journalPath)
	if jerr != nil {
		logger.Warnf("remark journal unreadable, starting empty: %v", jerr)
	}
	deviceCtx.SetJournal(journal)
	quotesPath := config.QuotesPath(homeDir)
	quotesContent, qerr := config.EnsurePromptFile(quotesPath, config.DefaultQuotes)
	if qerr != nil {
		logger.Warnf("ensure quotes file: %v", qerr)
	}
	deviceCtx.SetQuotes(func() []string {
		content := readPromptFile(quotesPath)
		if content == "" {
			content = quotesContent
		}
		return devctx.ParseQuotes(content)
	})
```

- [ ] **Step 3: Extend the system prompt source (main_sdl.go:194-196)**

```go
			audioPipeline.SetSystemPromptSource(func() string {
				return systemPromptWithContext(readPromptFile(personaPath), deviceCtx.Snapshot(), journal.PromptBlock(time.Now().UTC()))
			})
```

- [ ] **Step 4: Record spoken remarks in the journal (main_sdl.go:450-460)**

Replace the proactive block:

```go
			if audioPipeline != nil && proactive.Due(now) {
				proactive.Reschedule(now)
				if n, ok := deviceCtx.ProactiveNudge(); ok {
					remarkPipeline := audioPipeline
					go func() {
						record := func(reply string) {
							if err := journal.Append(devctx.RemarkEntry{When: time.Now().UTC(), Topic: n.Topic, Subject: n.Subject, Reply: reply}); err != nil {
								logger.Warnf("remark journal save: %v", err)
							}
						}
						var err error
						if n.Verbatim {
							err = remarkPipeline.SpeakVerbatim(ctx, n.Text, record)
						} else {
							err = remarkPipeline.SpeakRemark(ctx, n.Text, record)
						}
						if err != nil {
							logger.Warnf("proactive remark failed: %v", err)
						}
					}()
				}
			}
```

- [ ] **Step 5: Mirror Steps 2–4 in main_fb.go**

Apply the exact same three blocks to `cmd/bmo-pak/main_fb.go`: the journal/quotes wiring after its `SetReminisce` call, the three-segment `SetSystemPromptSource`, and the `record`-callback proactive block (around line 390). The code is identical to Steps 2–4.

- [ ] **Step 6: Build, test, verify**

Run: `go build ./... && go test ./...`
Expected: build clean, all packages PASS.

If the repo's build tags make only one main compile by default, also run the other tag explicitly (check the build tag at the top of `main_fb.go`, e.g.): `go build -tags fb ./...`

- [ ] **Step 7: Lint and commit**

Run: `gofmt -l ./cmd ./internal && golangci-lint run ./...`
Expected: no output / no new findings.

```bash
git add cmd/bmo-pak/
git commit -m "feat: wire remark journal, RECENT REMARKS prompt block, and quote fallback into the pak"
```

---

### Task 6: Final verification

- [ ] **Step 1: Full suite**

Run: `gofmt -l ./internal ./cmd && go vet ./... && go test ./... && golangci-lint run ./...`
Expected: gofmt silent, vet clean, all tests PASS, no new lint findings.

- [ ] **Step 2: Manual smoke (optional, desktop)**

With `BMO_DATA_ROOT` pointed at a scratch dir and `BMO_LOG_LEVEL=debug`, run the pak and confirm the log now shows `remark nudge:` and `remark system prompt:` lines before each `remark Chat:`, and that `remarks.json` appears in `$BMO_DATA_ROOT/BMO/` after the first remark with valid JSON content.

- [ ] **Step 3: Commit any stragglers and stop**

No further commits expected; if verification produced fixes, commit them with a `fix:` message. Do NOT merge or push — integration is decided with the user (superpowers:finishing-a-development-branch).
