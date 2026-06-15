package clips

import (
	"context"
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
