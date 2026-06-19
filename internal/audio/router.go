package audio

import (
	"errors"
	"sync"
)

type PCMSource interface {
	Start() error
	Close() error
	Frames() <-chan []byte
	WritePCM([]byte) error
}

// subscriber is one batch consumer. Each gets every batch; a slow consumer
// drops batches (non-blocking send) rather than stalling capture.
type subscriber struct {
	ch chan []byte
}

type CaptureRouter struct {
	source PCMSource

	batchLimit int

	mu     sync.RWMutex
	subs   map[*subscriber]struct{}
	legacy *subscriber // backs Batches()
	levels chan float64
	errors chan error
	done   chan struct{}
	closed bool
}

func NewCaptureRouter(source PCMSource, batchLimit int) *CaptureRouter {
	if batchLimit <= 0 {
		batchLimit = BytesPerSecond(DefaultSampleRate, DefaultChannels, BytesPerSampleS16LE) / 2
	}
	r := &CaptureRouter{
		source:     source,
		batchLimit: batchLimit,
		subs:       make(map[*subscriber]struct{}),
		levels:     make(chan float64, 8),
		errors:     make(chan error, 4),
		done:       make(chan struct{}),
	}
	// The legacy Batches() subscriber is registered eagerly so batches flushed
	// before the first Batches() call are still buffered (preserving the
	// original single-channel semantics).
	r.legacy = &subscriber{ch: make(chan []byte, 4)}
	r.subs[r.legacy] = struct{}{}
	return r
}

func (r *CaptureRouter) Start() error {
	if r == nil {
		return errors.New("nil capture router")
	}
	if r.source == nil {
		return errors.New("nil capture source")
	}

	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return errors.New("capture router closed")
	}
	r.mu.Unlock()

	if err := r.source.Start(); err != nil {
		return err
	}
	go r.run()
	return nil
}

func (r *CaptureRouter) run() {
	defer close(r.done)
	defer func() {
		r.mu.Lock()
		r.closed = true
		for s := range r.subs {
			close(s.ch)
		}
		r.subs = map[*subscriber]struct{}{}
		r.mu.Unlock()
	}()
	defer close(r.levels)
	defer close(r.errors)

	buffer := make([]byte, 0, r.batchLimit)
	flush := func() {
		if len(buffer) == 0 {
			return
		}
		batch := make([]byte, len(buffer))
		copy(batch, buffer)
		buffer = buffer[:0]
		r.mu.RLock()
		for s := range r.subs {
			select {
			case s.ch <- batch:
			default:
			}
		}
		r.mu.RUnlock()
	}

	for frame := range r.source.Frames() {
		if len(frame) == 0 {
			continue
		}

		level := PCMLevelS16LE(frame)
		select {
		case r.levels <- level:
		default:
		}

		buffer = append(buffer, frame...)
		if len(buffer) >= r.batchLimit {
			flush()
		}
	}
	flush()
}

// Batches returns the legacy single-consumer batch channel. It is one
// permanent subscriber; new consumers should use Subscribe instead.
func (r *CaptureRouter) Batches() <-chan []byte {
	if r == nil {
		return nil
	}
	return r.legacy.ch
}

// Subscribe registers a new batch consumer and returns its channel plus a
// cancel func that unregisters and closes it. Every subscriber receives every
// batch; a slow subscriber drops batches rather than stalling capture. After
// the router has stopped, Subscribe returns an already-closed channel.
func (r *CaptureRouter) Subscribe() (<-chan []byte, func()) {
	if r == nil {
		ch := make(chan []byte)
		close(ch)
		return ch, func() {}
	}
	s := &subscriber{ch: make(chan []byte, 4)}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		close(s.ch)
		return s.ch, func() {}
	}
	r.subs[s] = struct{}{}
	r.mu.Unlock()
	var once sync.Once
	cancel := func() {
		once.Do(func() {
			r.mu.Lock()
			if _, ok := r.subs[s]; ok {
				delete(r.subs, s)
				close(s.ch)
			}
			r.mu.Unlock()
		})
	}
	return s.ch, cancel
}

func (r *CaptureRouter) Levels() <-chan float64 {
	if r == nil {
		return nil
	}
	return r.levels
}

func (r *CaptureRouter) Errors() <-chan error {
	if r == nil {
		return nil
	}
	return r.errors
}

func (r *CaptureRouter) Done() <-chan struct{} {
	if r == nil {
		return nil
	}
	return r.done
}

func (r *CaptureRouter) WritePCM(pcm []byte) error {
	if r == nil {
		return errors.New("nil capture router")
	}
	if r.source == nil {
		return errors.New("nil capture source")
	}
	return r.source.WritePCM(pcm)
}

func (r *CaptureRouter) Close() error {
	if r == nil {
		return nil
	}
	return r.source.Close()
}
