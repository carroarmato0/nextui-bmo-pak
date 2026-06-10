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

type CaptureRouter struct {
	source PCMSource

	batchLimit int

	mu     sync.RWMutex
	batches chan []byte
	levels  chan float64
	errors  chan error
	done    chan struct{}
	closed  bool
}

func NewCaptureRouter(source PCMSource, batchLimit int) *CaptureRouter {
	if batchLimit <= 0 {
		batchLimit = BytesPerSecond(DefaultSampleRate, DefaultChannels, BytesPerSampleS16LE) / 2
	}
	return &CaptureRouter{
		source:     source,
		batchLimit: batchLimit,
		batches:    make(chan []byte, 4),
		levels:     make(chan float64, 8),
		errors:     make(chan error, 4),
		done:       make(chan struct{}),
	}
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
	defer close(r.batches)
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
		select {
		case r.batches <- batch:
		default:
		}
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

func (r *CaptureRouter) Batches() <-chan []byte {
	if r == nil {
		return nil
	}
	return r.batches
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
