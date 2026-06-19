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

func TestRouterFanOutDeliversToAllSubscribers(t *testing.T) {
	src := newFakeSource(8)
	r := NewCaptureRouter(src, 4)
	if err := r.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	subA, cancelA := r.Subscribe()
	subB, cancelB := r.Subscribe()
	defer cancelA()
	defer cancelB()

	src.frames <- []byte{1, 2}
	src.frames <- []byte{3, 4}

	for i, sub := range []<-chan []byte{subA, subB} {
		select {
		case b := <-sub:
			if len(b) == 0 {
				t.Fatalf("subscriber %d: empty batch", i)
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d did not receive batch", i)
		}
	}

	close(src.frames)
	<-r.Done()
}

func TestRouterSubscribeAfterCancelStopsDelivery(t *testing.T) {
	src := newFakeSource(8)
	r := NewCaptureRouter(src, 4)
	if err := r.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	sub, cancel := r.Subscribe()
	cancel()
	// Second cancel must be a safe no-op.
	cancel()
	src.frames <- []byte{1, 2}
	src.frames <- []byte{3, 4}
	select {
	case _, ok := <-sub:
		if ok {
			t.Fatalf("expected closed channel after cancel")
		}
	case <-time.After(300 * time.Millisecond):
		// no delivery on a cancelled subscriber is also acceptable
	}
}

func TestRouterSubscribeAfterCloseReturnsClosedChannel(t *testing.T) {
	src := newFakeSource(1)
	r := NewCaptureRouter(src, 4)
	if err := r.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	close(src.frames)
	<-r.Done()
	sub, cancel := r.Subscribe()
	defer cancel()
	if _, ok := <-sub; ok {
		t.Fatalf("expected closed channel when subscribing after router stopped")
	}
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
