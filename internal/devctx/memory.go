package devctx

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// memoryCap bounds the memory: old entries fall off so the file
// stays a few KB and quote dedup self-regulates against the memory window.
const memoryCap = 20

// promptRemarks is how many recent entries PromptBlock renders.
const promptRemarks = 5

// TopicQuote marks verbatim quote entries in the memory.
const TopicQuote = "quote"

// MemoryEntry is one spoken proactive remark (or verbatim quote).
type MemoryEntry struct {
	When    time.Time `json:"when"`
	Topic   string    `json:"topic"`
	Subject string    `json:"subject"`
	Reply   string    `json:"reply"`
}

// Memory is BMO's short-term memory of what he recently said out loud,
// consumed by the nudge picker (cooldown dedup) and the system prompt
// (RECENT REMARKS block). It persists as one small JSON file written
// atomically once per spoken remark; a missing or corrupt file is never
// fatal — BMO just starts with an empty memory.
type Memory struct {
	mu      sync.Mutex
	path    string
	entries []MemoryEntry
}

// LoadMemory reads the memory at path. It always returns a usable
// memory; a non-nil error only reports why an existing file could not be
// used (e.g. corrupt JSON after a hard power-off) so the caller can log it.
func LoadMemory(path string) (*Memory, error) {
	m := &Memory{path: path}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return m, nil
		}
		return m, err
	}
	var entries []MemoryEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return m, err
	}
	if len(entries) > memoryCap {
		entries = entries[len(entries)-memoryCap:]
	}
	m.entries = entries
	return m, nil
}

// Append records a spoken remark and persists the memory via temp-file +
// rename so a hard power-off never leaves a half-written file in place.
func (m *Memory) Append(e MemoryEntry) error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = append(m.entries, e)
	if len(m.entries) > memoryCap {
		m.entries = m.entries[len(m.entries)-memoryCap:]
	}
	return m.saveLocked()
}

func (m *Memory) saveLocked() error {
	data, err := json.Marshal(m.entries)
	if err != nil {
		return err
	}
	tmp := m.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, m.path)
}

// Recent returns up to n of the most recent entries, oldest first.
func (m *Memory) Recent(n int) []MemoryEntry {
	if m == nil || n <= 0 {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	start := len(m.entries) - n
	if start < 0 {
		start = 0
	}
	if len(m.entries) == 0 {
		return nil
	}
	out := make([]MemoryEntry, len(m.entries)-start)
	copy(out, m.entries[start:])
	return out
}

// LastRemarkedAt returns when subject was last remarked about, or the zero
// time if it never was (within the memory window).
func (m *Memory) LastRemarkedAt(subject string) time.Time {
	if m == nil {
		return time.Time{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := len(m.entries) - 1; i >= 0; i-- {
		if m.entries[i].Subject == subject {
			return m.entries[i].When
		}
	}
	return time.Time{}
}

// Contains reports whether subject appears anywhere in the memory window.
func (m *Memory) Contains(subject string) bool {
	return !m.LastRemarkedAt(subject).IsZero()
}

// PromptBlock renders the RECENT REMARKS system prompt segment, or "" when
// the memory is empty.
func (m *Memory) PromptBlock(now time.Time) string {
	entries := m.Recent(promptRemarks)
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
