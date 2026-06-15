package clips

import (
	"context"
	"math"
	"sync/atomic"
	"time"

	"github.com/carroarmato0/nextui-bmo/internal/audio"
)

const (
	chunkMs          = 20
	playbackBufferMs = 200
)

// AudioWriter is satisfied by audio.Session.
type AudioWriter interface {
	WritePCM(pcm []byte) error
}

// Player streams pre-recorded PCM clips through an AudioWriter at real-time
// rate. Playback runs in its own goroutine, paced with monotonic sleeps so it
// is fully decoupled from the render loop's frame rate: a slow or stalled
// render loop never starves the audio or collapses the playback window. The
// render loop observes playback state via Playing() and CurrentAmplitude(),
// both backed by atomics.
type Player struct {
	writer     AudioWriter
	sampleRate int
	channels   int
	lib        *Library

	ampl    atomic.Uint32
	playing atomic.Bool
}

func NewPlayer(writer AudioWriter, sampleRate, channels int, lib *Library) *Player {
	if sampleRate <= 0 {
		sampleRate = 16000
	}
	if channels <= 0 {
		channels = 2
	}
	return &Player{writer: writer, sampleRate: sampleRate, channels: channels, lib: lib}
}

// Playing reports whether a clip is currently streaming.
func (p *Player) Playing() bool {
	return p != nil && p.playing.Load()
}

// CurrentAmplitude returns the RMS amplitude [0, 1] of the audio being played.
func (p *Player) CurrentAmplitude() float32 {
	if p == nil {
		return 0
	}
	return math.Float32frombits(p.ampl.Load())
}

// PlaySequence plays the named clips in order in a background goroutine and
// returns a channel that is closed once all clips have finished playing (or
// ctx is cancelled). The returned channel is closed immediately if the player
// is nil/unconfigured or no names are given. Playing() reports true for the
// duration; use the returned channel — not Playing() — to detect completion,
// to avoid the startup race where the goroutine has not yet set the flag.
func (p *Player) PlaySequence(ctx context.Context, names ...string) <-chan struct{} {
	done := make(chan struct{})
	if p == nil || p.writer == nil || len(names) == 0 {
		close(done)
		return done
	}
	// Mark playing synchronously so the render loop sees the speaking state on
	// the very next frame, before the goroutine is scheduled.
	p.playing.Store(true)
	go func() {
		defer close(done)
		defer p.playing.Store(false)
		defer p.ampl.Store(0)
		for _, name := range names {
			if ctx.Err() != nil {
				return
			}
			if err := p.playPaced(ctx, p.lib.Load(name)); err != nil {
				return
			}
		}
	}()
	return done
}

// Play loads and plays a single clip, blocking until it has finished. Returns
// ctx.Err() if cancelled. Provided for tests and non-rendering callers.
func (p *Player) Play(ctx context.Context, name string) error {
	if p == nil || p.writer == nil {
		return nil
	}
	p.playing.Store(true)
	defer p.playing.Store(false)
	defer p.ampl.Store(0)
	return p.playPaced(ctx, p.lib.Load(name))
}

// playPaced streams pcm at real-time rate. A cushion of playbackBufferMs is
// written up front so ALSA never starves; thereafter chunk i is written when
// chunk i-lead becomes audible, and the amplitude tracks the audible chunk.
// Returns once the final chunk has played out, so callers can key completion
// to actual sound output.
func (p *Player) playPaced(ctx context.Context, pcm []byte) error {
	if len(pcm) == 0 {
		return nil
	}
	bytesPerChunk := p.sampleRate * p.channels * 2 * chunkMs / 1000
	if bytesPerChunk <= 0 {
		return p.writer.WritePCM(pcm)
	}
	amps := audio.RMSChunks(pcm, p.sampleRate, p.channels, chunkMs)
	lead := playbackBufferMs / chunkMs
	chunkDur := time.Duration(chunkMs) * time.Millisecond
	nChunks := (len(pcm) + bytesPerChunk - 1) / bytesPerChunk

	setAmp := func(audible int) {
		if audible >= 0 && audible < len(amps) {
			p.ampl.Store(math.Float32bits(amps[audible]))
		}
	}

	start := time.Now()
	for i := 0; i < nChunks; i++ {
		if err := sleepUntil(ctx, start.Add(time.Duration(i-lead)*chunkDur)); err != nil {
			return err
		}
		end := (i + 1) * bytesPerChunk
		if end > len(pcm) {
			end = len(pcm)
		}
		if err := p.writer.WritePCM(pcm[i*bytesPerChunk : end]); err != nil {
			return err
		}
		setAmp(i - lead)
	}
	// Drain: keep the amplitude envelope running while the last lead chunks
	// finish playing out of the buffer.
	for j := nChunks - lead; j < nChunks; j++ {
		if j < 0 {
			continue
		}
		if err := sleepUntil(ctx, start.Add(time.Duration(j)*chunkDur)); err != nil {
			return err
		}
		setAmp(j)
	}
	return sleepUntil(ctx, start.Add(time.Duration(nChunks)*chunkDur))
}

// sleepUntil blocks until t or until ctx is cancelled. Deadlines in the past
// return immediately (with ctx.Err() if the context is already done).
func sleepUntil(ctx context.Context, t time.Time) error {
	d := time.Until(t)
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
