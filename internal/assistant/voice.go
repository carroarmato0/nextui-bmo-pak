package assistant

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/carroarmato0/nextui-bmo/internal/audio"
	"github.com/carroarmato0/nextui-bmo/internal/face"
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
	logSystemPrompt bool

	// ttsInstructionsSource and systemPromptSource, when set, are consulted
	// before each utterance so the prompts can be tuned at runtime (e.g.
	// re-read from their files) without restarting. An empty result falls
	// back to the corresponding static value.
	ttsInstructionsSource func() string
	systemPromptSource    func() string
	emotionVocabSource    func() []EmotionEntry

	sampleRate       int
	captureChannels  int
	playbackChannels int

	requestTimeout time.Duration
	timeoutClip    []byte
	errorClip      []byte

	// ampl holds float32 bits of the current RMS amplitude during TTS playback.
	// 0 = silence, 1 = maximum loudness. Updated ~every 20ms during paced playback.
	ampl atomic.Uint32

	// playMu guards the interrupt handle for the in-progress TTS playback.
	// playCancel cancels the paced playback; playDone is closed once the
	// post-speech state transitions have run.
	playMu     sync.Mutex
	playCancel context.CancelFunc
	playDone   chan struct{}

	// batchMu guards batchCancel for the in-flight ProcessBatch request.
	batchMu     sync.Mutex
	batchCancel context.CancelFunc
}

func NewVoicePipeline(machine *Machine, writer AudioWriter, stt providers.STTProvider, chat providers.ChatProvider, tts providers.TTSProvider, sttModel, chatModel, ttsModel, ttsVoice, systemPrompt string, sampleRate, captureChannels, playbackChannels int) *VoicePipeline {
	if sampleRate <= 0 {
		sampleRate = audio.DefaultSampleRate
	}
	if captureChannels <= 0 {
		captureChannels = 1
	}
	if playbackChannels <= 0 {
		playbackChannels = 2
	}
	return &VoicePipeline{
		machine:          machine,
		writer:           writer,
		stt:              stt,
		chat:             chat,
		tts:              tts,
		sttModel:         strings.TrimSpace(sttModel),
		chatModel:        strings.TrimSpace(chatModel),
		ttsModel:         strings.TrimSpace(ttsModel),
		ttsVoice:         strings.TrimSpace(ttsVoice),
		systemPrompt:     strings.TrimSpace(systemPrompt),
		sampleRate:       sampleRate,
		captureChannels:  captureChannels,
		playbackChannels: playbackChannels,
	}
}

// SetRequestTimeout sets the per-batch timeout. Zero disables it.
func (p *VoicePipeline) SetRequestTimeout(d time.Duration) {
	if p != nil {
		p.requestTimeout = d
	}
}

// SetTimeoutClip sets the pre-encoded stereo PCM played when a batch times out.
func (p *VoicePipeline) SetTimeoutClip(pcm []byte) {
	if p != nil {
		p.timeoutClip = pcm
	}
}

// SetErrorClip sets the pre-encoded stereo PCM played on network/provider errors.
func (p *VoicePipeline) SetErrorClip(pcm []byte) {
	if p != nil {
		p.errorClip = pcm
	}
}

// CancelBatch cancels the in-flight ProcessBatch request. Returns false when
// no batch is in progress.
func (p *VoicePipeline) CancelBatch() bool {
	if p == nil {
		return false
	}
	p.batchMu.Lock()
	cancel := p.batchCancel
	p.batchMu.Unlock()
	if cancel == nil {
		return false
	}
	cancel()
	return true
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

// SetLogSystemPrompt controls whether system prompt and TTS instructions are
// written to the debug log on each ProcessBatch call.
func (p *VoicePipeline) SetLogSystemPrompt(v bool) {
	if p != nil {
		p.logSystemPrompt = v
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

// SetEmotionVocabularySource installs a function consulted before each utterance
// for the active emotion vocabulary, so a mod switch updates what the model is
// told. An empty result falls back to the built-in emotion set.
func (p *VoicePipeline) SetEmotionVocabularySource(source func() []EmotionEntry) {
	if p != nil {
		p.emotionVocabSource = source
	}
}

// currentEmotionVocab resolves the emotion vocabulary for the next utterance,
// falling back to the built-in emotion faces when no source is installed.
func (p *VoicePipeline) currentEmotionVocab() []EmotionEntry {
	if p.emotionVocabSource != nil {
		if v := p.emotionVocabSource(); len(v) > 0 {
			return v
		}
	}
	return BuildEmotionVocabulary(face.EmotionNames(), nil, nil)
}

// currentSystemPrompt resolves the chat persona for the next utterance and
// appends the emotion protocol so the model can drive BMO's face. With no
// persona configured the protocol is returned on its own.
func (p *VoicePipeline) currentSystemPrompt() string {
	persona := p.systemPrompt
	if p.systemPromptSource != nil {
		if prompt := strings.TrimSpace(p.systemPromptSource()); prompt != "" {
			persona = prompt
		}
	}
	proto := emotionProtocolPrompt(p.currentEmotionVocab())
	if strings.TrimSpace(persona) == "" {
		return proto
	}
	return persona + "\n\n" + proto
}

// CurrentAmplitude returns the RMS amplitude [0, 1] of the audio currently
// being played back by WritePCM. Returns 0 when not playing.
func (p *VoicePipeline) CurrentAmplitude() float32 {
	if p == nil {
		return 0
	}
	return math.Float32frombits(p.ampl.Load())
}

// resolveSpeech parses a chat reply into the text BMO should speak, applying any
// facial emotion directive to the machine as a side effect. ok is false when the
// reply contained no speakable text (it was only a directive); the caller should
// then return to idle without calling TTS. The emotion is cleared automatically
// by the next non-speak transition, so it only needs setting here.
func (p *VoicePipeline) resolveSpeech(reply string) (spoken string, ok bool) {
	spoken, emotion := ParseEmotion(reply, emotionNameSet(p.currentEmotionVocab()))
	if spoken == "" {
		return "", false
	}
	if emotion != "" {
		if p.machine != nil {
			p.machine.SetEmotion(emotion)
		}
		if p.logger != nil {
			p.logger.Debugf("pipeline emotion: %q", emotion)
		}
	}
	return spoken, true
}

func (p *VoicePipeline) ProcessBatch(ctx context.Context, pcm []byte) error {
	if p == nil {
		return errors.New("nil voice pipeline")
	}
	if !p.aiModeEnabled() {
		return nil
	}
	if len(pcm) == 0 || !audio.PCMHasSignal(pcm, 0.01) {
		return nil
	}
	if p.machine != nil {
		p.machine.Transition(EventListen)
	}

	timeout := p.requestTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	batchCtx, batchCancel := context.WithTimeout(ctx, timeout)
	p.batchMu.Lock()
	p.batchCancel = batchCancel
	p.batchMu.Unlock()
	defer func() {
		batchCancel()
		p.batchMu.Lock()
		p.batchCancel = nil
		p.batchMu.Unlock()
	}()

	totalStart := time.Now()

	sttStart := time.Now()
	transcription, err := p.stt.Transcribe(batchCtx, providers.TranscriptionRequest{
		Model:      p.sttModel,
		Audio:      pcm,
		SampleRate: p.sampleRate,
		Channels:   p.captureChannels,
		Format:     "wav",
	})
	if err != nil {
		return p.handleBatchError(ctx, batchCtx, err, false)
	}
	if p.logger != nil {
		p.logger.Infof("pipeline STT: %dms | tokens: %s (%.1fs audio)",
			time.Since(sttStart).Milliseconds(), usageString(transcription.Usage),
			audioSeconds(len(pcm), p.sampleRate, p.captureChannels))
	}
	transcript := strings.TrimSpace(transcription.Text)
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

	systemPrompt := p.currentSystemPrompt()
	if p.logger != nil && p.logSystemPrompt {
		p.logger.Debugf("pipeline system prompt: %q", systemPrompt)
	}

	chatStart := time.Now()
	chat, err := p.chat.Reply(batchCtx, providers.ChatRequest{
		Model:        p.chatModel,
		Messages:     []providers.Message{{Role: "user", Content: transcript}},
		SystemPrompt: systemPrompt,
	})
	if err != nil {
		return p.handleBatchError(ctx, batchCtx, err, false)
	}
	if p.logger != nil {
		p.logger.Infof("pipeline Chat: %dms | tokens: %s",
			time.Since(chatStart).Milliseconds(), usageString(chat.Usage))
	}
	reply := strings.TrimSpace(chat.Text)
	if reply == "" {
		if p.machine != nil {
			p.machine.Transition(EventRest)
		}
		return nil
	}
	if p.logger != nil {
		p.logger.Debugf("pipeline reply: %q", reply)
	}

	spoken, ok := p.resolveSpeech(reply)
	if !ok {
		if p.machine != nil {
			p.machine.Transition(EventRest)
		}
		return nil
	}

	if p.logger != nil && p.logSystemPrompt {
		p.logger.Debugf("pipeline TTS instructions: %q", p.currentTTSInstructions())
	}

	ttsStart := time.Now()
	// Use batchCtx so the request timeout and a B-press cancel cover synthesis
	// too — the thinking face is still showing here. With the parent ctx a slow
	// TTS hung forever and B could not abort it.
	speech, err := p.tts.Speak(batchCtx, providers.SpeechRequest{
		Model:        p.ttsModel,
		Voice:        p.ttsVoice,
		Input:        spoken,
		Format:       "pcm",
		Instructions: p.currentTTSInstructions(),
	})
	if err != nil {
		return p.handleBatchError(ctx, batchCtx, err, false)
	}
	if p.logger != nil {
		p.logger.Infof("pipeline TTS: %dms (%d bytes) | input: %d chars | total: %dms",
			time.Since(ttsStart).Milliseconds(), len(speech), len(spoken),
			time.Since(totalStart).Milliseconds())
	}

	speech = p.resampleTTS(speech)

	// Synthesis is done: we are committed to speaking. Hand B over from
	// CancelBatch (which aborts the request) to InterruptSpeech (which stops
	// playback) by clearing the batch cancel now, so a B press during playback
	// interrupts the audio instead of being swallowed as a no-op batch cancel.
	p.batchMu.Lock()
	p.batchCancel = nil
	p.batchMu.Unlock()

	return p.speak(ctx, speech)
}

// handleBatchError dispatches STT/Chat errors: B-cancel → silent idle,
// timeout → timeout clip, quota → quota state, other → error clip.
func (p *VoicePipeline) handleBatchError(ctx, batchCtx context.Context, err error, _ bool) error {
	// B-button cancel: batchCtx cancelled but outer ctx still alive.
	if errors.Is(batchCtx.Err(), context.Canceled) && ctx.Err() == nil {
		if p.machine != nil {
			p.machine.Transition(EventRest)
		}
		return nil
	}
	// Timeout: batchCtx deadline exceeded but outer ctx still alive.
	if errors.Is(batchCtx.Err(), context.DeadlineExceeded) && ctx.Err() == nil {
		return p.playFallbackClip(ctx, p.timeoutClip)
	}
	// Quota exhausted.
	if classifyQuota(p.stt, err) || classifyQuota(p.chat, err) || classifyQuota(p.tts, err) {
		if p.machine != nil {
			p.machine.Transition(EventQuotaExhausted)
		}
		return err
	}
	// Other network/provider error.
	return p.playFallbackClip(ctx, p.errorClip)
}

// playFallbackClip plays pcm through the writer. If pcm is empty the machine
// transitions to idle silently via EventRest; otherwise speak() handles all
// state transitions.
func (p *VoicePipeline) playFallbackClip(ctx context.Context, pcm []byte) error {
	if len(pcm) == 0 {
		if p.machine != nil {
			p.machine.Transition(EventRest)
		}
		return nil
	}
	// Machine may be in Listening or Thinking; speak() handles EventSpeak/EventRest.
	return p.speak(ctx, pcm)
}

// SpeakRemark generates and speaks a spontaneous proactive remark. The
// nudge is a stage direction sent as the user message (there is no real
// user speech); the reply flows through the normal TTS → playback path, so
// PTT interruption, amplitude-driven mouth, and state transitions all
// behave exactly like a normal utterance. No-op outside AI mode or when
// BMO is not idle — a remark must never barge into a conversation.
// onSpoken, when non-nil, is invoked with the reply text once TTS has
// succeeded (i.e. the remark will actually be heard), so the caller can
// record it in the memory.
// requestCtx bounds ctx by the per-request timeout so a hung chat/TTS cannot
// freeze BMO on the thinking face. The caller must defer the returned cancel.
// This is the proactive-remark counterpart to ProcessBatch's batchCtx.
func (p *VoicePipeline) requestCtx(ctx context.Context) (context.Context, context.CancelFunc) {
	timeout := p.requestTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return context.WithTimeout(ctx, timeout)
}

// remarkFail handles a chat/TTS error during a proactive remark. A timeout or
// cancellation (reqCtx done while the app ctx is alive) abandons the remark
// silently and returns to idle — the user never asked for it, so there is no
// error clip. Anything else goes through the normal fail path.
func (p *VoicePipeline) remarkFail(ctx, reqCtx context.Context, err error) error {
	if reqCtx.Err() != nil && ctx.Err() == nil {
		if p.machine != nil {
			p.machine.Transition(EventRest)
		}
		return nil
	}
	return p.fail(err)
}

func (p *VoicePipeline) SpeakRemark(ctx context.Context, nudge string, onSpoken func(reply string)) error {
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

	// Bound chat + TTS so a stalled provider can't freeze the thinking face for
	// minutes; playback below stays on the parent ctx.
	reqCtx, cancel := p.requestCtx(ctx)
	defer cancel()

	systemPrompt := p.currentSystemPrompt()
	if p.logger != nil {
		p.logger.Debugf("remark nudge: %q", nudge)
		p.logger.Debugf("remark system prompt: %q", systemPrompt)
	}
	chatStart := time.Now()
	chat, err := p.chat.Reply(reqCtx, providers.ChatRequest{
		Model:        p.chatModel,
		Messages:     []providers.Message{{Role: "user", Content: nudge}},
		SystemPrompt: systemPrompt,
	})
	if err != nil {
		return p.remarkFail(ctx, reqCtx, err)
	}
	if p.logger != nil {
		p.logger.Infof("remark Chat: %dms | tokens: %s",
			time.Since(chatStart).Milliseconds(), usageString(chat.Usage))
	}
	reply := strings.TrimSpace(chat.Text)
	if reply == "" {
		if p.machine != nil {
			p.machine.Transition(EventRest)
		}
		return nil
	}
	if p.logger != nil {
		p.logger.Debugf("remark reply: %q", reply)
	}

	spoken, ok := p.resolveSpeech(reply)
	if !ok {
		if p.machine != nil {
			p.machine.Transition(EventRest)
		}
		return nil
	}

	ttsStart := time.Now()
	speech, err := p.tts.Speak(reqCtx, providers.SpeechRequest{
		Model:        p.ttsModel,
		Voice:        p.ttsVoice,
		Input:        spoken,
		Format:       "pcm",
		Instructions: p.currentTTSInstructions(),
	})
	if err != nil {
		return p.remarkFail(ctx, reqCtx, err)
	}
	if p.logger != nil {
		p.logger.Infof("remark TTS: %dms (%d bytes) | input: %d chars",
			time.Since(ttsStart).Milliseconds(), len(speech), len(spoken))
	}
	if onSpoken != nil {
		onSpoken(spoken)
	}
	speech = p.resampleTTS(speech)

	return p.speak(ctx, speech)
}

// SpeakVerbatim speaks text exactly as given — no chat call, no paraphrase
// risk, zero chat tokens. Used for the curated-quote fallback when every
// real remark topic is on cooldown. Same idle-only gating and playback
// path as SpeakRemark; onSpoken fires once TTS has succeeded.
func (p *VoicePipeline) SpeakVerbatim(ctx context.Context, text string, onSpoken func(spoken string)) error {
	if p == nil || !p.aiModeEnabled() {
		return nil
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if p.machine != nil {
		if p.machine.Transition(EventRemark) != StateThinking {
			return nil
		}
	}
	if p.logger != nil {
		p.logger.Debugf("remark quote: %q", text)
	}

	// Bound TTS so a stalled provider can't freeze the thinking face; playback
	// below stays on the parent ctx.
	reqCtx, cancel := p.requestCtx(ctx)
	defer cancel()

	ttsStart := time.Now()
	speech, err := p.tts.Speak(reqCtx, providers.SpeechRequest{
		Model:        p.ttsModel,
		Voice:        p.ttsVoice,
		Input:        text,
		Format:       "pcm",
		Instructions: p.currentTTSInstructions(),
	})
	if err != nil {
		return p.remarkFail(ctx, reqCtx, err)
	}
	if p.logger != nil {
		p.logger.Infof("remark TTS: %dms (%d bytes) | input: %d chars",
			time.Since(ttsStart).Milliseconds(), len(speech), len(text))
	}
	if onSpoken != nil {
		onSpoken(text)
	}
	speech = p.resampleTTS(speech)

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
			return p.fail(err)
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
	bytesPerChunk := p.sampleRate * p.playbackChannels * 2 /* 16-bit */ * speakChunkMs / 1000
	if bytesPerChunk <= 0 {
		return p.writer.WritePCM(pcm)
	}
	amps := audio.RMSChunks(pcm, p.sampleRate, p.playbackChannels, speakChunkMs)
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

// resampleTTS converts the 24kHz mono S16LE output from OpenAI's "pcm" format
// to the device playback rate and channel count. Resampling as mono then
// upmixing avoids the 2× speed regression that results from passing
// playbackChannels directly to ResampleS16LE on mono input.
func (p *VoicePipeline) resampleTTS(pcm []byte) []byte {
	out := audio.ResampleS16LE(pcm, ttsPCMSampleRate, p.sampleRate, 1)
	if p.playbackChannels > 1 {
		out = audio.UpmixMonoToStereo(out)
	}
	return out
}

func (p *VoicePipeline) fail(err error) error {
	if p != nil && p.machine != nil {
		if classifyQuota(p.stt, err) || classifyQuota(p.chat, err) || classifyQuota(p.tts, err) {
			p.machine.Transition(EventQuotaExhausted)
			return err
		}
		p.machine.Transition(EventFail)
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

// usageString renders provider token accounting for the pipeline log
// lines; "n/a" when the provider reported nothing (e.g. whisper-1).
func usageString(u providers.Usage) string {
	if !u.Reported() {
		return "n/a"
	}
	return fmt.Sprintf("%d prompt + %d completion", u.PromptTokens, u.CompletionTokens)
}

// audioSeconds converts a PCM S16LE byte count to seconds of audio, the
// billing unit for STT models that do not report token usage.
func audioSeconds(pcmBytes, sampleRate, channels int) float64 {
	if sampleRate <= 0 || channels <= 0 {
		return 0
	}
	return float64(pcmBytes) / float64(sampleRate*channels*2)
}
