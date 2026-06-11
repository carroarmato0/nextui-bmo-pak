package assistant

import (
	"context"
	"encoding/binary"
	"errors"
	"math"
	"strings"
	"sync"
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

	sttModel        string
	chatModel       string
	ttsModel        string
	ttsVoice        string
	ttsInstructions string
	systemPrompt    string

	// ttsInstructionsSource and systemPromptSource, when set, are consulted
	// before each utterance so the prompts can be tuned at runtime (e.g.
	// re-read from their files) without restarting. An empty result falls
	// back to the corresponding static value.
	ttsInstructionsSource func() string
	systemPromptSource    func() string

	sampleRate int
	channels   int

	// ampl holds float32 bits of the current RMS amplitude during TTS playback.
	// 0 = silence, 1 = maximum loudness. Updated ~every 20ms during paced playback.
	ampl atomic.Uint32

	// playMu guards the interrupt handle for the in-progress TTS playback.
	// playCancel cancels the paced playback; playDone is closed once the
	// post-speech state transitions have run.
	playMu     sync.Mutex
	playCancel context.CancelFunc
	playDone   chan struct{}
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

// SetTTSInstructions sets the speaking-style prompt forwarded to
// instruction-capable TTS models.
func (p *VoicePipeline) SetTTSInstructions(instructions string) {
	if p != nil {
		p.ttsInstructions = strings.TrimSpace(instructions)
	}
}

// SetTTSInstructionsSource installs a function consulted before each
// utterance for the current speaking-style prompt, enabling on-the-fly
// tuning. An empty result falls back to the static SetTTSInstructions value.
func (p *VoicePipeline) SetTTSInstructionsSource(source func() string) {
	if p != nil {
		p.ttsInstructionsSource = source
	}
}

// currentTTSInstructions resolves the speaking-style prompt for the next
// utterance.
func (p *VoicePipeline) currentTTSInstructions() string {
	if p.ttsInstructionsSource != nil {
		if instructions := strings.TrimSpace(p.ttsInstructionsSource()); instructions != "" {
			return instructions
		}
	}
	return p.ttsInstructions
}

// SetSystemPromptSource installs a function consulted before each utterance
// for the current chat persona, enabling on-the-fly tuning. An empty result
// falls back to the static constructor value.
func (p *VoicePipeline) SetSystemPromptSource(source func() string) {
	if p != nil {
		p.systemPromptSource = source
	}
}

// currentSystemPrompt resolves the chat persona for the next utterance.
func (p *VoicePipeline) currentSystemPrompt() string {
	if p.systemPromptSource != nil {
		if prompt := strings.TrimSpace(p.systemPromptSource()); prompt != "" {
			return prompt
		}
	}
	return p.systemPrompt
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
	// Outside AI mode no provider/API traffic may happen at all.
	if !p.aiModeEnabled() {
		return nil
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
		SystemPrompt: p.currentSystemPrompt(),
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

	ttsStart := time.Now()
	speech, err := p.tts.Speak(ctx, providers.SpeechRequest{
		Model:        p.ttsModel,
		Voice:        p.ttsVoice,
		Input:        reply,
		Format:       "pcm",
		Instructions: p.currentTTSInstructions(),
	})
	if err != nil {
		return p.fail(err, EventFail)
	}
	if p.logger != nil {
		p.logger.Infof("pipeline TTS: %dms (%d bytes) | total: %dms",
			time.Since(ttsStart).Milliseconds(), len(speech),
			time.Since(totalStart).Milliseconds())
	}

	// OpenAI's "pcm" speech format is fixed at 24kHz mono S16LE; resample to
	// the device playback rate so BMO's voice plays at natural speed and
	// pitch. (Skipping this step is the planned "funny voice" easter egg:
	// 24kHz played at 16kHz sounds ~1.5x slower and a third deeper.)
	speech = audio.ResampleS16LE(speech, ttsPCMSampleRate, p.sampleRate, p.channels)

	return p.speak(ctx, speech)
}

// SpeakRemark generates and speaks a spontaneous proactive remark. The
// nudge is a stage direction sent as the user message (there is no real
// user speech); the reply flows through the normal TTS → playback path, so
// PTT interruption, amplitude-driven mouth, and state transitions all
// behave exactly like a normal utterance. No-op outside AI mode or when
// BMO is not idle — a remark must never barge into a conversation.
func (p *VoicePipeline) SpeakRemark(ctx context.Context, nudge string) error {
	if p == nil || !p.aiModeEnabled() {
		return nil
	}
	nudge = strings.TrimSpace(nudge)
	if nudge == "" {
		return nil
	}
	if p.machine != nil {
		// EventRemark only succeeds from idle, so a PTT press racing this
		// call cannot be hijacked: if EventListen landed first, the
		// transition is refused and the remark is silently dropped.
		if p.machine.Transition(EventRemark) != StateThinking {
			return nil
		}
	}

	chatStart := time.Now()
	reply, err := p.chat.Reply(ctx, providers.ChatRequest{
		Model:        p.chatModel,
		Messages:     []providers.Message{{Role: "user", Content: nudge}},
		SystemPrompt: p.currentSystemPrompt(),
	})
	if err != nil {
		return p.fail(err, EventFail)
	}
	if p.logger != nil {
		p.logger.Infof("remark Chat: %dms", time.Since(chatStart).Milliseconds())
	}
	reply = strings.TrimSpace(reply)
	if reply == "" {
		if p.machine != nil {
			p.machine.Transition(EventRest)
		}
		return nil
	}
	if p.logger != nil {
		p.logger.Debugf("remark reply: %q", reply)
	}

	ttsStart := time.Now()
	speech, err := p.tts.Speak(ctx, providers.SpeechRequest{
		Model:        p.ttsModel,
		Voice:        p.ttsVoice,
		Input:        reply,
		Format:       "pcm",
		Instructions: p.currentTTSInstructions(),
	})
	if err != nil {
		return p.fail(err, EventFail)
	}
	if p.logger != nil {
		p.logger.Infof("remark TTS: %dms (%d bytes)", time.Since(ttsStart).Milliseconds(), len(speech))
	}
	speech = audio.ResampleS16LE(speech, ttsPCMSampleRate, p.sampleRate, p.channels)

	return p.speak(ctx, speech)
}

// speak plays the synthesized speech and owns the post-speech state
// transitions. It registers an interrupt handle so InterruptSpeech can cut
// the playback short; the handle's waiters are released only after the
// transitions have run, so an interrupter observes the machine back in idle.
func (p *VoicePipeline) speak(ctx context.Context, speech []byte) error {
	if len(speech) > 0 && p.writer != nil {
		if p.machine != nil {
			p.machine.Transition(EventSpeak)
		}

		playCtx, cancel := context.WithCancel(ctx)
		done := make(chan struct{})
		p.playMu.Lock()
		p.playCancel = cancel
		p.playDone = done
		p.playMu.Unlock()

		release := func() {
			p.playMu.Lock()
			p.playCancel = nil
			p.playDone = nil
			p.playMu.Unlock()
			cancel()
			close(done)
		}

		// Stream the PCM at real-time pace so the speaking state and mouth
		// amplitude track the audible playback position; returns once the
		// audio has finished playing or the playback was interrupted.
		err := p.playPaced(playCtx, speech)
		// A user interrupt cancels only playCtx and counts as a normal end
		// of speech; parent-context cancellation keeps the failure path.
		if err != nil && !(errors.Is(err, context.Canceled) && ctx.Err() == nil) {
			release()
			return p.fail(err, EventFail)
		}
		defer release()
	}

	if p.machine != nil {
		p.machine.RecordInteraction(time.Now().UTC())
		p.machine.Transition(EventRest)
	}
	return nil
}

// InterruptSpeech cuts an in-progress TTS playback short. It returns false
// immediately when nothing is playing. Otherwise it cancels the playback and
// blocks until the pipeline has finished its post-speech state transitions
// (speaking -> idle), so the caller can follow up immediately with e.g.
// EventListen for push-to-talk.
func (p *VoicePipeline) InterruptSpeech() bool {
	if p == nil {
		return false
	}
	p.playMu.Lock()
	cancel, done := p.playCancel, p.playDone
	p.playMu.Unlock()
	if cancel == nil {
		return false
	}
	cancel()
	<-done
	return true
}

// speakChunkMs is the pacing granularity for TTS playback: PCM is written in
// windows of this size and the mouth amplitude is updated per window.
const speakChunkMs = 20

// ttsPCMSampleRate is the sample rate of the raw "pcm" response format of the
// OpenAI speech API (24kHz mono S16LE). Raw PCM carries no header, so the
// rate cannot be detected from the payload.
const ttsPCMSampleRate = 24000

// playPaced streams pcm to the playback writer at real-time rate instead of
// dumping it into the pipe at once. Dumping parks seconds of audio in the
// pipe + ALSA buffers, which made the mouth animation and the speaking state
// run ahead of the audible sound and end while sound was still playing.
//
// A cushion of audio.PlaybackBufferMs is written up front so the device never
// starves (matching aplay's --buffer-time); after that, chunk i is written
// when chunk i-lead becomes audible, and the amplitude is set for the audible
// chunk. The call returns once the final chunk has finished playing, so the
// caller can key the speaking state directly to actual sound output.
func (p *VoicePipeline) playPaced(ctx context.Context, pcm []byte) error {
	bytesPerChunk := p.sampleRate * p.channels * 2 /* 16-bit */ * speakChunkMs / 1000
	if bytesPerChunk <= 0 {
		return p.writer.WritePCM(pcm)
	}
	amps := rmsChunks(pcm, p.sampleRate, p.channels, speakChunkMs)
	lead := audio.PlaybackBufferMs / speakChunkMs
	chunkDur := time.Duration(speakChunkMs) * time.Millisecond
	nChunks := (len(pcm) + bytesPerChunk - 1) / bytesPerChunk
	defer p.ampl.Store(0)

	setAmp := func(audible int) {
		if audible >= 0 && audible < len(amps) {
			p.ampl.Store(math.Float32bits(amps[audible]))
		}
	}

	// t=0 is when the prefill has been written, which is when ALSA's buffer
	// fills and playback starts: chunk j becomes audible at ~ t = j*chunkDur.
	start := time.Now()
	for i := 0; i < nChunks; i++ {
		// Chunk i is written `lead` chunks before it is due to play; the
		// first `lead` chunks form the instant prefill.
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
	// The last `lead` chunks are still draining out of the buffers: keep the
	// amplitude envelope running until the final chunk has played out.
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

// aiModeEnabled reports whether AI processing is allowed. With no machine
// attached (tests, headless tools) it defaults to allowed.
func (p *VoicePipeline) aiModeEnabled() bool {
	return p.machine == nil || p.machine.AIEnabled()
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
