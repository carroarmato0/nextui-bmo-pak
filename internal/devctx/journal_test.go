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
	for _, want := range []string{`"line C"`, `"line G"`, "5 hours ago", "an hour ago"} {
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
