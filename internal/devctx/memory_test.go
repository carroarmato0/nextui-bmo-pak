package devctx

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func memoryEntry(when time.Time, topic, subject, reply string) MemoryEntry {
	return MemoryEntry{When: when, Topic: topic, Subject: subject, Reply: reply}
}

func TestMemoryAppendPersistsAndCaps(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.json")
	m, err := LoadMemory(path)
	if err != nil {
		t.Fatalf("load fresh memory: %v", err)
	}
	base := time.Date(2026, 6, 12, 1, 0, 0, 0, time.UTC)
	for i := 0; i < 25; i++ {
		e := memoryEntry(base.Add(time.Duration(i)*time.Minute), KeyAchievements, "subj", "reply")
		e.Reply = e.When.Format("15:04")
		if err := m.Append(e); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	if got := len(m.Recent(100)); got != memoryCap {
		t.Fatalf("in-memory entries = %d, want %d", got, memoryCap)
	}
	// Reload from disk: capped, newest entries kept, oldest first.
	m2, err := LoadMemory(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	entries := m2.Recent(100)
	if len(entries) != memoryCap {
		t.Fatalf("persisted entries = %d, want %d", len(entries), memoryCap)
	}
	if entries[0].Reply != "01:05" || entries[len(entries)-1].Reply != "01:24" {
		t.Fatalf("wrong window kept: first=%q last=%q", entries[0].Reply, entries[len(entries)-1].Reply)
	}
	// No stray temp file left behind.
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("temp file left behind after rename")
	}
}

func TestLoadMemoryMissingFile(t *testing.T) {
	m, err := LoadMemory(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatalf("missing file must not error: %v", err)
	}
	if got := len(m.Recent(10)); got != 0 {
		t.Fatalf("expected empty memory, got %d entries", got)
	}
}

func TestLoadMemoryCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.json")
	if err := os.WriteFile(path, []byte("{half-written"), 0o600); err != nil {
		t.Fatal(err)
	}
	m, err := LoadMemory(path)
	if err == nil {
		t.Fatal("expected a load error for corrupt JSON")
	}
	if m == nil {
		t.Fatal("corrupt file must still return a usable memory")
	}
	// The memory recovers: an append overwrites the corrupt file.
	if err := m.Append(memoryEntry(time.Now().UTC(), KeySaves, KeySaves, "hi")); err != nil {
		t.Fatalf("append after corrupt load: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var entries []MemoryEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatalf("file not valid JSON after recovery append: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
}

func TestMemoryLastRemarkedAtAndContains(t *testing.T) {
	now := time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC)
	m := &Memory{entries: []MemoryEntry{
		memoryEntry(now.Add(-5*time.Hour), KeyAchievements, `"Moon Presence" in Deadeus`, "wow"),
		memoryEntry(now.Add(-1*time.Hour), KeySaves, KeySaves, "save files!"),
	}}
	if got := m.LastRemarkedAt(`"Moon Presence" in Deadeus`); !got.Equal(now.Add(-5 * time.Hour)) {
		t.Fatalf("LastRemarkedAt = %v", got)
	}
	if !m.LastRemarkedAt("never-mentioned").IsZero() {
		t.Fatal("unknown subject must return zero time")
	}
	if !m.Contains(KeySaves) || m.Contains("never-mentioned") {
		t.Fatal("Contains mismatch")
	}
}

func TestMemoryPromptBlock(t *testing.T) {
	now := time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC)
	m := &Memory{}
	for i := 0; i < 7; i++ {
		m.entries = append(m.entries, memoryEntry(now.Add(time.Duration(i-7)*time.Hour), TopicQuote, "q", "line "+string(rune('A'+i))))
	}
	block := m.PromptBlock(now)
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

func TestMemoryPromptBlockEmpty(t *testing.T) {
	if got := (&Memory{}).PromptBlock(time.Now()); got != "" {
		t.Fatalf("empty memory must render no block, got %q", got)
	}
}

func TestMemoryNilSafe(t *testing.T) {
	var m *Memory
	if err := m.Append(MemoryEntry{}); err != nil {
		t.Fatal("nil Append must be a no-op")
	}
	if m.Recent(5) != nil || !m.LastRemarkedAt("x").IsZero() || m.Contains("x") || m.PromptBlock(time.Now()) != "" {
		t.Fatal("nil memory methods must return zero values")
	}
}
