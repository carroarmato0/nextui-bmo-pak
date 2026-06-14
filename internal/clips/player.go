package clips

import (
	"context"
	"time"
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
// rate, paced to match the hardware playback buffer.
type Player struct {
	writer     AudioWriter
	sampleRate int
	channels   int
	lib        *Library
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

// Play loads the named clip and streams it at real-time rate. Returns nil if
// the clip is not found or writer is nil.
func (p *Player) Play(ctx context.Context, name string) error {
	if p == nil || p.writer == nil {
		return nil
	}
	pcm := p.lib.Load(name)
	if len(pcm) == 0 {
		return nil
	}
	return p.playPaced(ctx, pcm)
}

func (p *Player) playPaced(ctx context.Context, pcm []byte) error {
	bytesPerChunk := p.sampleRate * p.channels * 2 * chunkMs / 1000
	if bytesPerChunk <= 0 {
		return p.writer.WritePCM(pcm)
	}
	nChunks := (len(pcm) + bytesPerChunk - 1) / bytesPerChunk
	lead := playbackBufferMs / chunkMs
	chunkDur := time.Duration(chunkMs) * time.Millisecond

	start := time.Now()
	for i := 0; i < nChunks-lead; i++ {
		if err := sleepUntil(ctx, start.Add(time.Duration(i)*chunkDur)); err != nil {
			return err
		}
		end := (i + 1) * bytesPerChunk
		if end > len(pcm) {
			end = len(pcm)
		}
		if err := p.writer.WritePCM(pcm[i*bytesPerChunk : end]); err != nil {
			return err
		}
	}
	for j := nChunks - lead; j < nChunks; j++ {
		if j < 0 {
			continue
		}
		if err := sleepUntil(ctx, start.Add(time.Duration(j)*chunkDur)); err != nil {
			return err
		}
		end := (j + 1) * bytesPerChunk
		if end > len(pcm) {
			end = len(pcm)
		}
		if err := p.writer.WritePCM(pcm[j*bytesPerChunk : end]); err != nil {
			return err
		}
	}
	return sleepUntil(ctx, start.Add(time.Duration(nChunks)*chunkDur))
}

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
