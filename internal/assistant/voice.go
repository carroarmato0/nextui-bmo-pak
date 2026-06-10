package assistant

import (
	"context"
	"encoding/binary"
	"errors"
	"math"
	"strings"
	"sync/atomic"
	"time"

	"github.com/carroarmato0/nextui-bmo/internal/audio"
	"github.com/carroarmato0/nextui-bmo/internal/providers"
)

// VoiceLogger is satisfied by observability.Logger (and any struct with Infof/Debugf).
type VoiceLogger interface {
	Infof(format string, args ...any)
	Debugf(format string, args ...any)
}

type AudioWriter interface {
	WritePCM([]byte) error
}

type VoicePipeline struct {
	machine *Machine
	writer  AudioWriter
	logger  VoiceLogger

	stt  providers.STTProvider
	chat providers.ChatProvider
	tts  providers.TTSProvider

	sttModel     string
	chatModel    string
	ttsModel     string
	ttsVoice     string
	systemPrompt string

	sampleRate int
	channels   int

	// ampl holds float32 bits of the current RMS amplitude during TTS playback.
	// 0 = silence, 1 = maximum loudness. Updated ~every 20ms by a background goroutine.
	ampl atomic.Uint32
}

func NewVoicePipeline(machine *Machine, writer AudioWriter, stt providers.STTProvider, chat providers.ChatProvider, tts providers.TTSProvider, sttModel, chatModel, ttsModel, ttsVoice, systemPrompt string, sampleRate, channels int) *VoicePipeline {
	if sampleRate <= 0 {
		sampleRate = audio.DefaultSampleRate
	}
	if channels <= 0 {
		channels = audio.DefaultChannels
	}
	return &VoicePipeline{
		machine:      machine,
		writer:       writer,
		stt:          stt,
		chat:         chat,
		tts:          tts,
		sttModel:     strings.TrimSpace(sttModel),
		chatModel:    strings.TrimSpace(chatModel),
		ttsModel:     strings.TrimSpace(ttsModel),
		ttsVoice:     strings.TrimSpace(ttsVoice),
		systemPrompt: strings.TrimSpace(systemPrompt),
		sampleRate:   sampleRate,
		channels:     channels,
	}
}

// SetLogger wires a logger for per-stage timing output.
func (p *VoicePipeline) SetLogger(l VoiceLogger) {
	if p != nil {
		p.logger = l
	}
}

// CurrentAmplitude returns the RMS amplitude [0, 1] of the audio currently
// being played back by WritePCM. Returns 0 when not playing.
func (p *VoicePipeline) CurrentAmplitude() float32 {
	if p == nil {
		return 0
	}
	return math.Float32frombits(p.ampl.Load())
}

func (p *VoicePipeline) ProcessBatch(ctx context.Context, pcm []byte) error {
	if p == nil {
		return errors.New("nil voice pipeline")
	}
	if len(pcm) == 0 || !audio.PCMHasSignal(pcm, 0.01) {
		return nil
	}
	if p.machine != nil {
		p.machine.Transition(EventListen)
	}

	totalStart := time.Now()

	sttStart := time.Now()
	transcript, err := p.stt.Transcribe(ctx, providers.TranscriptionRequest{
		Model:      p.sttModel,
		Audio:      pcm,
		SampleRate: p.sampleRate,
		Channels:   p.channels,
		Format:     "wav",
	})
	if err != nil {
		return p.fail(err, EventFail)
	}
	if p.logger != nil {
		p.logger.Infof("pipeline STT: %dms", time.Since(sttStart).Milliseconds())
	}
	transcript = strings.TrimSpace(transcript)
	if transcript == "" {
		if p.machine != nil {
			p.machine.Transition(EventRest)
		}
		return nil
	}
	if p.logger != nil {
		p.logger.Debugf("pipeline transcript: %q", transcript)
	}

	if p.machine != nil {
		p.machine.Transition(EventThink)
	}

	chatStart := time.Now()
	reply, err := p.chat.Reply(ctx, providers.ChatRequest{
		Model:        p.chatModel,
		Messages:     []providers.Message{{Role: "user", Content: transcript}},
		SystemPrompt: p.systemPrompt,
	})
	if err != nil {
		return p.fail(err, EventFail)
	}
	if p.logger != nil {
		p.logger.Infof("pipeline Chat: %dms", time.Since(chatStart).Milliseconds())
	}
	reply = strings.TrimSpace(reply)
	if reply == "" {
		if p.machine != nil {
			p.machine.Transition(EventRest)
		}
		return nil
	}
	if p.logger != nil {
		p.logger.Debugf("pipeline reply: %q", reply)
	}

	if p.machine != nil {
		p.machine.Transition(EventSpeak)
	}

	ttsStart := time.Now()
	speech, err := p.tts.Speak(ctx, providers.SpeechRequest{
		Model:  p.ttsModel,
		Voice:  p.ttsVoice,
		Input:  reply,
		Format: "pcm",
	})
	if err != nil {
		return p.fail(err, EventFail)
	}
	if p.logger != nil {
		p.logger.Infof("pipeline TTS: %dms (%d bytes) | total: %dms",
			time.Since(ttsStart).Milliseconds(), len(speech),
			time.Since(totalStart).Milliseconds())
	}

	if len(speech) > 0 && p.writer != nil {
		// Pre-compute amplitude envelope for mouth animation (RMS per 20ms window).
		// OpenAI TTS PCM is 24 kHz; using p.sampleRate is approximate but close enough.
		chunks := rmsChunks(speech, p.sampleRate, p.channels, 20)
		go func() {
			defer p.ampl.Store(0)
			for _, amp := range chunks {
				if ctx.Err() != nil {
					return
				}
				p.ampl.Store(math.Float32bits(amp))
				time.Sleep(20 * time.Millisecond)
			}
		}()

		if err := p.writer.WritePCM(speech); err != nil {
			p.ampl.Store(0)
			return p.fail(err, EventFail)
		}
		p.ampl.Store(0)
	}

	if p.machine != nil {
		p.machine.RecordInteraction(time.Now().UTC())
		p.machine.Transition(EventRest)
	}
	return nil
}

// rmsChunks splits pcm into chunkMs-millisecond windows and returns the RMS
// amplitude [0, 1] of each window. Used to drive the speaking mouth animation.
func rmsChunks(pcm []byte, sampleRate, channels, chunkMs int) []float32 {
	if sampleRate <= 0 || channels <= 0 || chunkMs <= 0 {
		return nil
	}
	bytesPerChunk := sampleRate * channels * 2 /* 16-bit */ * chunkMs / 1000
	if bytesPerChunk <= 0 {
		return nil
	}
	var out []float32
	for i := 0; i+bytesPerChunk <= len(pcm); i += bytesPerChunk {
		chunk := pcm[i : i+bytesPerChunk]
		var sum float64
		n := 0
		for j := 0; j+1 < len(chunk); j += 2 {
			s := int16(binary.LittleEndian.Uint16(chunk[j : j+2]))
			v := float64(s) / 32767.0
			sum += v * v
			n++
		}
		if n > 0 {
			out = append(out, float32(math.Sqrt(sum/float64(n))))
		} else {
			out = append(out, 0)
		}
	}
	return out
}

func (p *VoicePipeline) fail(err error, event Event) error {
	if p != nil && p.machine != nil {
		if classifyQuota(p.stt, err) || classifyQuota(p.chat, err) || classifyQuota(p.tts, err) {
			p.machine.Transition(EventQuotaExhausted)
			return err
		}
		p.machine.Transition(event)
	}
	return err
}

func classifyQuota(provider any, err error) bool {
	classifier, ok := provider.(providers.ErrorClassifier)
	if !ok || classifier == nil {
		return false
	}
	return classifier.ClassifyError(err) == providers.ErrorKindQuota
}
