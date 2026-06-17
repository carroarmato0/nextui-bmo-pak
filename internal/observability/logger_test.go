package observability

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// blockingWriter blocks every Write until released, simulating a console pipe
// whose consumer (NextUI/minui on device) has stopped draining.
type blockingWriter struct{ release chan struct{} }

func (b *blockingWriter) Write(p []byte) (int, error) {
	<-b.release
	return len(p), nil
}

// TestLoggerDoesNotBlockOnStuckConsole guards the on-device freeze: when the
// console writer (stdout pipe) stalls, logging must not block — otherwise the
// log mutex is held forever and the render loop deadlocks (black screen). The
// file log is the source of truth and must keep receiving lines.
func TestLoggerDoesNotBlockOnStuckConsole(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "BMO.txt")
	stuck := &blockingWriter{release: make(chan struct{})}
	defer close(stuck.release)

	logger, err := NewLogger(path, LevelDebug, stuck)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer logger.Close()

	done := make(chan struct{})
	go func() {
		for i := 0; i < 2000; i++ {
			logger.Infof("line %d with some padding to use buffer space", i)
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("logging blocked on a stuck console writer — render loop would deadlock")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if !strings.Contains(string(data), "line 1999") {
		t.Fatal("file log missing lines — file must remain the reliable sink")
	}
}

func TestLoggerRedactsSecrets(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "BMO.txt")
	var buf bytes.Buffer

	logger, err := NewLogger(path, LevelDebug, &buf)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer logger.Close()

	logger.RegisterSecret("sk-test-123")
	logger.Infof("sending token=%s", "sk-test-123")

	logger.Sync()
	if got := buf.String(); !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("stdout log not redacted: %q", got)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if !strings.Contains(string(data), "[REDACTED]") {
		t.Fatalf("file log not redacted: %q", string(data))
	}
}

func TestLoggerHonorsLevel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "BMO.txt")
	var buf bytes.Buffer

	logger, err := NewLogger(path, LevelInfo, &buf)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer logger.Close()

	logger.Debugf("debug message")
	logger.Infof("info message")

	logger.Sync()
	got := buf.String()
	if strings.Contains(got, "debug message") {
		t.Fatalf("debug message should not be logged at info level: %q", got)
	}
	if !strings.Contains(got, "info message") {
		t.Fatalf("info message missing: %q", got)
	}
}

func TestSetLevel(t *testing.T) {
	var buf bytes.Buffer
	l, err := NewLogger(filepath.Join(t.TempDir(), "test.log"), LevelInfo, &buf)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer l.Close()

	l.Debugf("should be hidden")
	l.Sync()
	if strings.Contains(buf.String(), "should be hidden") {
		t.Fatal("debug message printed at info level")
	}

	l.SetLevel(LevelDebug)
	l.Debugf("should be visible")
	l.Sync()
	if !strings.Contains(buf.String(), "should be visible") {
		t.Fatal("debug message not printed after SetLevel(LevelDebug)")
	}
}
