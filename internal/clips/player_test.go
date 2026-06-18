package clips

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type fakeWriter struct {
	mu    sync.Mutex
	total int
}

func (f *fakeWriter) WritePCM(pcm []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.total += len(pcm)
	return nil
}

func (f *fakeWriter) totalBytes() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.total
}

func TestPlayerPlayWritesPCMForHello(t *testing.T) {
	lib := NewLibrary(t.TempDir())
	w := &fakeWriter{}
	p := NewPlayer(w, 16000, 2, lib)
	if err := p.Play(context.Background(), "hello"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w.totalBytes() == 0 {
		t.Fatal("expected PCM writes for hello clip")
	}
}

func TestPlayerPlayCancelledContextReturnsError(t *testing.T) {
	lib := NewLibrary(t.TempDir())
	w := &fakeWriter{}
	p := NewPlayer(w, 16000, 2, lib)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := p.Play(ctx, "hello")
	if err == nil {
		t.Fatal("expected error for pre-cancelled context, got nil")
	}
}

func TestNilPlayerPlayIsNoop(t *testing.T) {
	var p *Player
	if err := p.Play(context.Background(), "hello"); err != nil {
		t.Fatalf("nil Player.Play should be no-op, got error: %v", err)
	}
}

func TestPlaySequenceMarksPlayingSynchronouslyAndClosesDone(t *testing.T) {
	lib := NewLibrary(t.TempDir())
	w := &fakeWriter{}
	p := NewPlayer(w, 16000, 2, lib)

	done := p.PlaySequence(context.Background(), "hello")
	// Playing must be observable immediately, before the goroutine is
	// scheduled, so the render loop shows the speaking face on the next frame.
	if !p.Playing() {
		t.Fatal("expected Playing() true synchronously after PlaySequence")
	}

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("PlaySequence did not finish in time")
	}
	if p.Playing() {
		t.Fatal("expected Playing() false after done")
	}
	if w.totalBytes() == 0 {
		t.Fatal("expected PCM writes for hello clip")
	}
}

func TestPlaySequenceNilPlayerClosesDone(t *testing.T) {
	var p *Player
	done := p.PlaySequence(context.Background(), "hello")
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("nil player PlaySequence should close done immediately")
	}
}

// concWriter records whether two clips ever wrote to the audio output at the
// same time (which would mix/garble on the device).
type concWriter struct {
	mu         sync.Mutex
	active     int
	overlapped bool
}

func (w *concWriter) WritePCM(pcm []byte) error {
	w.mu.Lock()
	w.active++
	if w.active > 1 {
		w.overlapped = true
	}
	w.mu.Unlock()
	time.Sleep(time.Millisecond)
	w.mu.Lock()
	w.active--
	w.mu.Unlock()
	return nil
}

func (w *concWriter) didOverlap() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.overlapped
}

func TestPlaySequenceInterruptsPrevious(t *testing.T) {
	dir := t.TempDir()
	audioDir := filepath.Join(dir, "audio")
	if err := os.MkdirAll(audioDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// ~0.33s clips so the first is still playing when the second starts.
	for _, n := range []string{"a", "b"} {
		if err := os.WriteFile(filepath.Join(audioDir, n+".pcm"), make([]byte, 64000/3), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	w := &concWriter{}
	p := NewPlayer(w, 16000, 2, NewLibrary(dir))

	done1 := p.PlaySequence(context.Background(), "a")
	time.Sleep(20 * time.Millisecond) // let "a" actually start writing
	done2 := p.PlaySequence(context.Background(), "b")

	// Starting "b" must have interrupted "a": its done is already closed.
	select {
	case <-done1:
	default:
		t.Error("a new clip should interrupt and close the previous clip's done")
	}
	<-done2

	if w.didOverlap() {
		t.Error("two clips wrote to the audio output concurrently (audio would mix)")
	}
}

func TestClipDuration(t *testing.T) {
	dir := t.TempDir()
	audioDir := filepath.Join(dir, "audio")
	if err := os.MkdirAll(audioDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// 64000 bytes at 16 kHz * 2 channels * 2 bytes/sample = exactly 1 second.
	if err := os.WriteFile(filepath.Join(audioDir, "myclip.pcm"), make([]byte, 64000), 0o644); err != nil {
		t.Fatal(err)
	}
	p := NewPlayer(&fakeWriter{}, 16000, 2, NewLibrary(dir))

	if got := p.ClipDuration("myclip"); got != time.Second {
		t.Errorf("ClipDuration(myclip) = %v, want 1s", got)
	}
	if got := p.ClipDuration("nonexistent-zzz"); got != 0 {
		t.Errorf("ClipDuration(missing) = %v, want 0", got)
	}
	var nilP *Player
	if got := nilP.ClipDuration("myclip"); got != 0 {
		t.Errorf("nil ClipDuration = %v, want 0", got)
	}
}
