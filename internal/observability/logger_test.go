package observability

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
	if strings.Contains(buf.String(), "should be hidden") {
		t.Fatal("debug message printed at info level")
	}

	l.SetLevel(LevelDebug)
	l.Debugf("should be visible")
	if !strings.Contains(buf.String(), "should be visible") {
		t.Fatal("debug message not printed after SetLevel(LevelDebug)")
	}
}
