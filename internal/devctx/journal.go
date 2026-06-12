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
