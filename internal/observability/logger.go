package observability

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

func ParseLevel(value string) Level {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return LevelDebug
	case "warn":
		return LevelWarn
	case "error":
		return LevelError
	default:
		return LevelInfo
	}
}

type Logger struct {
	mu      sync.Mutex
	level   Level
	out     io.Writer
	file    *os.File
	secrets []string
}

func NewLogger(path string, level Level, out io.Writer) (*Logger, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}
	if out == nil {
		out = os.Stdout
	}
	// Console output is best-effort: on device stdout is a pipe to NextUI/minui,
	// and if that consumer stops draining, a direct blocking write would hold the
	// log mutex forever and deadlock the render loop (black screen). Forward it
	// through a non-blocking writer that drops when its buffer is full so a
	// stalled console can never freeze the app. The file below is the source of
	// truth and is always written.
	return &Logger{level: level, out: newAsyncWriter(out, 1024), file: file}, nil
}

// Sync flushes pending best-effort console output. Intended for tests that
// assert on a synchronous console writer; production code never needs it.
func (l *Logger) Sync() {
	if a, ok := l.out.(*asyncWriter); ok {
		a.sync()
	}
}

// ConsoleWriter returns the best-effort, non-blocking console sink. Useful for
// redirecting third-party loggers (e.g. oksvg's standard logger) so their output
// can never block on a stalled stdout pipe.
func (l *Logger) ConsoleWriter() io.Writer {
	return l.out
}

// asyncWriter forwards writes to an underlying writer from a background
// goroutine, dropping them when its buffer is full. This decouples callers from
// a slow or stalled sink (an undrained pipe) so logging never blocks.
type asyncWriter struct {
	ch chan asyncChunk
}

// asyncChunk is one queued write; done (when non-nil) is closed after the chunk
// is processed, forming a flush barrier for sync().
type asyncChunk struct {
	data []byte
	done chan struct{}
}

func newAsyncWriter(w io.Writer, depth int) *asyncWriter {
	a := &asyncWriter{ch: make(chan asyncChunk, depth)}
	go func() {
		for c := range a.ch {
			if c.data != nil {
				_, _ = w.Write(c.data)
			}
			if c.done != nil {
				close(c.done)
			}
		}
	}()
	return a
}

func (a *asyncWriter) Write(p []byte) (int, error) {
	b := make([]byte, len(p)) // caller may reuse p; copy before queuing
	copy(b, p)
	select {
	case a.ch <- asyncChunk{data: b}:
	default: // buffer full (sink stalled): drop rather than block
	}
	return len(p), nil
}

// sync blocks until every write queued before it has been flushed to the
// underlying writer. Intended for tests; production logging never calls it.
func (a *asyncWriter) sync() {
	done := make(chan struct{})
	a.ch <- asyncChunk{done: done}
	<-done
}

func (l *Logger) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	return l.file.Close()
}

func (l *Logger) RegisterSecret(value string) {
	if l == nil || value == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, existing := range l.secrets {
		if existing == value {
			return
		}
	}
	l.secrets = append(l.secrets, value)
}

func (l *Logger) SetLevel(level Level) {
	if l == nil {
		return
	}
	l.mu.Lock()
	l.level = level
	l.mu.Unlock()
}

func (l *Logger) Debugf(format string, args ...any) { l.logf(LevelDebug, format, args...) }
func (l *Logger) Infof(format string, args ...any)  { l.logf(LevelInfo, format, args...) }
func (l *Logger) Warnf(format string, args ...any)  { l.logf(LevelWarn, format, args...) }
func (l *Logger) Errorf(format string, args ...any) { l.logf(LevelError, format, args...) }

func (l *Logger) logf(level Level, format string, args ...any) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if level < l.level {
		return
	}

	msg := fmt.Sprintf(format, args...)

	for _, secret := range l.secrets {
		if secret != "" {
			msg = strings.ReplaceAll(msg, secret, "[REDACTED]")
		}
	}

	line := fmt.Sprintf("%s [%s] %s\n", time.Now().Format(time.RFC3339), level.String(), msg)
	// File first: it is the reliable sink. Console is best-effort and async, so a
	// stalled stdout pipe on device can never block this (and the log mutex).
	_, _ = io.WriteString(l.file, line)
	_, _ = io.WriteString(l.out, line)
}

func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}
