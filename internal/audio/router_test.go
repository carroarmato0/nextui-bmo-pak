package audio

import (
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeSource struct {
	frames    chan []byte
	startErr  error
	closeErr  error
	started   int
	closed    int
	writeSeen [][]byte
	mu        sync.Mutex
}

func newFakeSource(buffer int) *fakeSource {
	if buffer <= 0 {
		buffer = 4
	}
	return &fakeSource{frames: make(chan []byte, buffer)}
}

func (f *fakeSource) Start() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.started++
	return f.startErr
}

func (f *fakeSource) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed++
	return f.closeErr
}

func (f *fakeSource) Frames() <-chan []byte { return f.frames }

func (f *fakeSource) WritePCM(pcm []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	copyBuf := make([]byte, len(pcm))
	copy(copyBuf, pcm)
	f.writeSeen = append(f.writeSeen, copyBuf)
	return nil
}

func TestCaptureRouterEmitsBatchesAndLevels(t *testing.T) {
	src := newFakeSource(8)
	router := NewCaptureRouter(src, 4)
	if err := router.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	src.frames <- []byte{0x00, 0x40}
	src.frames <- []byte{0x00, 0x40}
	close(src.frames)

	select {
	case batch, ok := <-router.Batches():
		if !ok {
			t.Fatal("Batches() closed before data was emitted")
		}
		if len(batch) != 4 {
			t.Fatalf("batch length = %d, want 4", len(batch))
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for batch")
	}

	select {
	case level := <-router.Levels():
		if level <= 0 {
			t.Fatalf("level = %v, want > 0", level)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for level")
	}

	if err := router.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	<-router.Done()
}

func TestCaptureRouterStartError(t *testing.T) {
	src := newFakeSource(1)
	src.startErr = errors.New("boom")
	router := NewCaptureRouter(src, 4)
	if err := router.Start(); err == nil {
		t.Fatal("Start() error = nil, want error")
	}
}

func TestCaptureRouterWritePCMPassesThrough(t *testing.T) {
	src := newFakeSource(1)
	router := NewCaptureRouter(src, 4)
	if err := router.WritePCM(nil); err != nil {
		t.Fatalf("WritePCM(nil) error = %v, want nil", err)
	}
	if err := router.WritePCM([]byte{1, 2}); err != nil {
		t.Fatalf("WritePCM() error = %v, want nil", err)
	}
	if len(src.writeSeen) != 2 {
		t.Fatalf("write count = %d, want 2", len(src.writeSeen))
	}
}
