package assistant

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/carroarmato0/nextui-bmo/internal/audio"
	"github.com/carroarmato0/nextui-bmo/internal/providers"
)

type fakeWriter struct {
	mu     sync.Mutex
	writes [][]byte
}

func (f *fakeWriter) WritePCM(pcm []byte) error {
	copyBuf := make([]byte, len(pcm))
	copy(copyBuf, pcm)
	f.mu.Lock()
	f.writes = append(f.writes, copyBuf)
	f.mu.Unlock()
	return nil
}

func (f *fakeWriter) totalBytes() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, w := range f.writes {
		n += len(w)
	}
	return n
}

func (f *fakeWriter) writeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.writes)
}

type fakeProvider struct {
	transcript     string
	reply          string
	speech         []byte
	sttUsage       providers.Usage
	chatUsage      providers.Usage
	err            error
	lastSpeech     providers.SpeechRequest
	lastChat       providers.ChatRequest
	lastTranscribe *providers.TranscriptionRequest
}

func (f *fakeProvider) Models(ctx context.Context) ([]string, error) { return nil, nil }
func (f *fakeProvider) RequiresAuth() bool                           { return false }
func (f *fakeProvider) ClassifyError(err error) providers.ErrorKind {
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "quota") {
		return providers.ErrorKindQuota
	}
	return providers.ErrorKindProvider
}
func (f *fakeProvider) Capabilities() []providers.Capability {
	return []providers.Capability{providers.CapabilitySTT, providers.CapabilityChat, providers.CapabilityTTS}
}
func (f *fakeProvider) Supports(cap providers.Capability) bool {
	for _, supported := range f.Capabilities() {
		if supported == cap {
			return true
		}
	}
	return false
}
func (f *fakeProvider) Transcribe(ctx context.Context, req providers.TranscriptionRequest) (providers.TranscriptionResult, error) {
	f.lastTranscribe = &req
	return providers.TranscriptionResult{Text: f.transcript, Usage: f.sttUsage}, f.err
}
func (f *fakeProvider) Reply(ctx context.Context, req providers.ChatRequest) (providers.ChatResult, error) {
	f.lastChat = req
	return providers.ChatResult{Text: f.reply, Usage: f.chatUsage}, f.err
}
func (f *fakeProvider) Speak(ctx context.Context, req providers.SpeechRequest) ([]byte, error) {
	f.lastSpeech = req
	return f.speech, f.err
}

func TestVoicePipelineHappyPath(t *testing.T) {
	m := NewMachine()
	m.SetMode("ai")
	writer := &fakeWriter{}
	stt := &fakeProvider{transcript: "hello"}
	chat := &fakeProvider{reply: "hi there"}
	tts := &fakeProvider{speech: []byte{1, 2, 3}}
	pipe := NewVoicePipeline(m, writer, stt, chat, tts, "whisper-1", "gpt-4o-mini", "tts-1", "alloy", "be bmo", audio.DefaultSampleRate, 1)

	if err := pipe.ProcessBatch(context.Background(), []byte{0x00, 0x40, 0x00, 0x40}); err != nil {
		t.Fatalf("ProcessBatch() error = %v", err)
	}
	if got := m.State(); got != StateIdle {
		t.Fatalf("state = %v, want idle", got)
	}
	if len(writer.writes) != 1 {
		t.Fatalf("writer writes = %d, want 1", len(writer.writes))
	}
}

func TestProcessBatchRefusesOutsideAIMode(t *testing.T) {
	m := NewMachine()
	m.SetMode("idle")
	writer := &fakeWriter{}
	stt := &fakeProvider{transcript: "hello"}
	chat := &fakeProvider{reply: "hi"}
	tts := &fakeProvider{speech: make([]byte, 2400)}
	pipe := NewVoicePipeline(m, writer, stt, chat, tts, "whisper-1", "gpt-4o-mini", "tts", "nova", "", 16000, 1)

	if err := pipe.ProcessBatch(context.Background(), []byte{0x00, 0x40, 0x00, 0x40}); err != nil {
		t.Fatalf("ProcessBatch() in idle mode error = %v, want silent no-op", err)
	}
	// No provider may have been called and no audio written.
	if stt.lastTranscribe != nil {
		t.Fatal("STT called in idle mode")
	}
	if chat.lastChat.Model != "" {
		t.Fatal("chat called in idle mode")
	}
	if tts.lastSpeech.Model != "" {
		t.Fatal("TTS called in idle mode")
	}
	if writer.totalBytes() != 0 {
		t.Fatal("audio written in idle mode")
	}
	if got := m.State(); got != StateIdle {
		t.Fatalf("state = %v, want untouched idle", got)
	}
}

func TestSystemPromptSourceReadPerUtterance(t *testing.T) {
	m := NewMachine()
	m.SetMode("ai")
	writer := &fakeWriter{}
	stt := &fakeProvider{transcript: "hello"}
	chat := &fakeProvider{reply: "hi"}
	tts := &fakeProvider{speech: make([]byte, 2400)}
	pipe := NewVoicePipeline(m, writer, stt, chat, tts, "whisper-1", "gpt-4o-mini", "tts", "nova", "static persona", 16000, 1)

	current := "persona one"
	pipe.SetSystemPromptSource(func() string { return current })

	if err := pipe.ProcessBatch(context.Background(), []byte{0x00, 0x40, 0x00, 0x40}); err != nil {
		t.Fatalf("ProcessBatch() error = %v", err)
	}
	if got := chat.lastChat.SystemPrompt; got != "persona one" {
		t.Fatalf("first utterance system prompt = %q, want %q", got, "persona one")
	}

	current = "persona two"
	if err := pipe.ProcessBatch(context.Background(), []byte{0x00, 0x40, 0x00, 0x40}); err != nil {
		t.Fatalf("ProcessBatch() error = %v", err)
	}
	if got := chat.lastChat.SystemPrompt; got != "persona two" {
		t.Fatalf("second utterance system prompt = %q, want %q", got, "persona two")
	}

	// An empty source value falls back to the static prompt.
	current = ""
	if err := pipe.ProcessBatch(context.Background(), []byte{0x00, 0x40, 0x00, 0x40}); err != nil {
		t.Fatalf("ProcessBatch() error = %v", err)
	}
	if got := chat.lastChat.SystemPrompt; got != "static persona" {
		t.Fatalf("empty-source system prompt = %q, want fallback", got)
	}
}

func TestTTSInstructionsSourceReadPerUtterance(t *testing.T) {
	m := NewMachine()
	m.SetMode("ai")
	writer := &fakeWriter{}
	stt := &fakeProvider{transcript: "hello"}
	chat := &fakeProvider{reply: "hi"}
	tts := &fakeProvider{speech: make([]byte, 2400)}
	pipe := NewVoicePipeline(m, writer, stt, chat, tts, "whisper-1", "gpt-4o-mini-tts", "tts", "nova", "", 16000, 1)
	pipe.SetTTSInstructions("static fallback")

	current := "take one"
	pipe.SetTTSInstructionsSource(func() string { return current })

	if err := pipe.ProcessBatch(context.Background(), []byte{0x00, 0x40, 0x00, 0x40}); err != nil {
		t.Fatalf("ProcessBatch() error = %v", err)
	}
	if got := tts.lastSpeech.Instructions; got != "take one" {
		t.Fatalf("first utterance instructions = %q, want %q", got, "take one")
	}

	// The source is consulted again for the next utterance — no restart needed.
	current = "take two"
	if err := pipe.ProcessBatch(context.Background(), []byte{0x00, 0x40, 0x00, 0x40}); err != nil {
		t.Fatalf("ProcessBatch() error = %v", err)
	}
	if got := tts.lastSpeech.Instructions; got != "take two" {
		t.Fatalf("second utterance instructions = %q, want %q", got, "take two")
	}

	// An empty source value falls back to the static instructions.
	current = ""
	if err := pipe.ProcessBatch(context.Background(), []byte{0x00, 0x40, 0x00, 0x40}); err != nil {
		t.Fatalf("ProcessBatch() error = %v", err)
	}
	if got := tts.lastSpeech.Instructions; got != "static fallback" {
		t.Fatalf("empty-source instructions = %q, want fallback", got)
	}
}

func TestProcessBatchResamplesTTSToPlaybackRate(t *testing.T) {
	m := NewMachine()
	m.SetMode("ai")
	writer := &fakeWriter{}
	// 250ms of 24kHz mono PCM, as returned by OpenAI's "pcm" speech format.
	speech := make([]byte, 24000*2/4)
	stt := &fakeProvider{transcript: "hello"}
	chat := &fakeProvider{reply: "hi"}
	tts := &fakeProvider{speech: speech}
	pipe := NewVoicePipeline(m, writer, stt, chat, tts, "whisper-1", "gpt-4o-mini", "tts-1", "alloy", "", 16000, 1)

	if err := pipe.ProcessBatch(context.Background(), []byte{0x00, 0x40, 0x00, 0x40}); err != nil {
		t.Fatalf("ProcessBatch() error = %v", err)
	}
	// The same 250ms of audio at the 16kHz playback rate: anything else plays
	// slow/deep (24k at 16k) or fast/squeaky.
	want := 16000 * 2 / 4
	if got := writer.totalBytes(); got != want {
		t.Fatalf("wrote %d bytes, want %d (24kHz TTS must be resampled to the 16kHz playback rate)", got, want)
	}
}

func TestPlayPacedTracksPlaybackClock(t *testing.T) {
	writer := &fakeWriter{}
	pipe := NewVoicePipeline(nil, writer, &fakeProvider{}, &fakeProvider{}, &fakeProvider{}, "", "", "", "", "", 16000, 1)

	// 500ms of constant-amplitude PCM (int16 = 8192 -> RMS ~ 0.25).
	const totalMs = 500
	pcm := make([]byte, 16000*2*totalMs/1000)
	for i := 0; i+1 < len(pcm); i += 2 {
		binary.LittleEndian.PutUint16(pcm[i:i+2], uint16(int16(8192)))
	}

	// Sample the mouth amplitude mid-playback.
	var midAmp float32
	done := make(chan struct{})
	go func() {
		defer close(done)
		time.Sleep(totalMs / 2 * time.Millisecond)
		midAmp = pipe.CurrentAmplitude()
	}()

	start := time.Now()
	if err := pipe.playPaced(context.Background(), pcm); err != nil {
		t.Fatalf("playPaced() error = %v", err)
	}
	elapsed := time.Since(start)
	<-done

	if got := writer.totalBytes(); got != len(pcm) {
		t.Fatalf("wrote %d bytes, want %d", got, len(pcm))
	}
	// The call must return only once the audio has audibly finished playing
	// (~total duration), not when the bytes were handed to the pipe.
	if elapsed < 450*time.Millisecond {
		t.Fatalf("playPaced returned after %v; want >= ~%dms (audio would still be playing)", elapsed, totalMs)
	}
	if elapsed > 1500*time.Millisecond {
		t.Fatalf("playPaced took %v; pacing is running far behind real time", elapsed)
	}
	// Writes must be paced in chunks, not dumped at once.
	if got := writer.writeCount(); got < 10 {
		t.Fatalf("writes = %d, want chunked paced writes", got)
	}
	if midAmp <= 0 {
		t.Fatalf("amplitude mid-playback = %v, want > 0", midAmp)
	}
	if got := pipe.CurrentAmplitude(); got != 0 {
		t.Fatalf("amplitude after playback = %v, want 0", got)
	}
}

func TestInterruptSpeechCutsPlaybackShort(t *testing.T) {
	m := NewMachine()
	m.SetMode("ai")
	writer := &fakeWriter{}
	// 2s of constant-amplitude speech at 16kHz mono.
	speech := make([]byte, 16000*2*2)
	for i := 0; i+1 < len(speech); i += 2 {
		binary.LittleEndian.PutUint16(speech[i:i+2], uint16(int16(8192)))
	}
	stt := &fakeProvider{transcript: "hello"}
	chat := &fakeProvider{reply: "hi"}
	tts := &fakeProvider{speech: speech}
	pipe := NewVoicePipeline(m, writer, stt, chat, tts, "whisper-1", "gpt-4o-mini", "tts-1", "alloy", "", 16000, 1)

	result := make(chan error, 1)
	go func() { result <- pipe.ProcessBatch(context.Background(), []byte{0x00, 0x40, 0x00, 0x40}) }()

	// Wait until playback is under way.
	deadline := time.Now().Add(2 * time.Second)
	for m.State() != StateSpeaking && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if m.State() != StateSpeaking {
		t.Fatal("pipeline never reached speaking state")
	}
	time.Sleep(100 * time.Millisecond) // let a few chunks play

	start := time.Now()
	if !pipe.InterruptSpeech() {
		t.Fatal("InterruptSpeech() = false, want true while speaking")
	}
	wait := time.Since(start)

	// The interrupter must observe the machine already back in idle: the
	// call blocks until the post-speech transitions have run.
	if got := m.State(); got != StateIdle {
		t.Fatalf("state after interrupt = %v, want idle (not error/concerned)", got)
	}
	if wait > 500*time.Millisecond {
		t.Fatalf("InterruptSpeech blocked for %v, want prompt return", wait)
	}
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("ProcessBatch() after interrupt = %v, want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("ProcessBatch did not return after interrupt")
	}
	if got := writer.totalBytes(); got >= len(speech) {
		t.Fatalf("interrupt wrote all %d bytes; playback was not cut short", got)
	}
	if got := pipe.CurrentAmplitude(); got != 0 {
		t.Fatalf("amplitude after interrupt = %v, want 0", got)
	}
}

func TestInterruptSpeechIdleReturnsFalse(t *testing.T) {
	pipe := NewVoicePipeline(nil, &fakeWriter{}, &fakeProvider{}, &fakeProvider{}, &fakeProvider{}, "", "", "", "", "", 16000, 1)
	done := make(chan bool, 1)
	go func() { done <- pipe.InterruptSpeech() }()
	select {
	case got := <-done:
		if got {
			t.Fatal("InterruptSpeech() = true with no active speech")
		}
	case <-time.After(time.Second):
		t.Fatal("InterruptSpeech blocked with no active speech")
	}
}

func TestVoicePipelineIgnoresSilentAudio(t *testing.T) {
	m := NewMachine()
	m.SetMode("ai")
	writer := &fakeWriter{}
	pipe := NewVoicePipeline(m, writer, &fakeProvider{}, &fakeProvider{}, &fakeProvider{}, "whisper-1", "gpt-4o-mini", "tts-1", "alloy", "", audio.DefaultSampleRate, 1)
	if err := pipe.ProcessBatch(context.Background(), []byte{0x00, 0x00, 0x00, 0x00}); err != nil {
		t.Fatalf("ProcessBatch() error = %v", err)
	}
	if len(writer.writes) != 0 {
		t.Fatalf("silent batch produced writes: %#v", writer.writes)
	}
}

func TestVoicePipelineQuotaFailure(t *testing.T) {
	m := NewMachine()
	m.SetMode("ai")
	writer := &fakeWriter{}
	stt := &fakeProvider{err: errors.New("quota exceeded")}
	pipe := NewVoicePipeline(m, writer, stt, &fakeProvider{}, &fakeProvider{}, "whisper-1", "gpt-4o-mini", "tts-1", "alloy", "", audio.DefaultSampleRate, 1)
	if err := pipe.ProcessBatch(context.Background(), []byte{0x00, 0x40, 0x00, 0x40}); err == nil {
		t.Fatal("ProcessBatch() error = nil, want error")
	}
	if got := m.State(); got != StateSleeping {
		t.Fatalf("state = %v, want sleeping on quota failure", got)
	}
}

func TestSpeakRemarkHappyPath(t *testing.T) {
	m := NewMachine()
	m.SetMode("ai")
	writer := &fakeWriter{}
	chat := &fakeProvider{reply: "You reached stage 7! Daebak!"}
	tts := &fakeProvider{speech: []byte{1, 2, 3, 4}}
	pipe := NewVoicePipeline(m, writer, &fakeProvider{}, chat, tts, "whisper-1", "gpt-4o-mini", "tts-1", "alloy", "be bmo", 16000, 1)
	pipe.SetSystemPromptSource(func() string { return "persona plus device context" })

	if err := pipe.SpeakRemark(context.Background(), "(BMO says something about achievements)", nil); err != nil {
		t.Fatalf("speak remark: %v", err)
	}
	if chat.lastChat.SystemPrompt != "persona plus device context" {
		t.Errorf("system prompt = %q", chat.lastChat.SystemPrompt)
	}
	if len(chat.lastChat.Messages) != 1 || chat.lastChat.Messages[0].Content != "(BMO says something about achievements)" {
		t.Errorf("nudge not sent as user message: %+v", chat.lastChat.Messages)
	}
	if tts.lastSpeech.Input != "You reached stage 7! Daebak!" {
		t.Errorf("tts input = %q", tts.lastSpeech.Input)
	}
	if writer.totalBytes() == 0 {
		t.Error("expected PCM written to playback")
	}
	if got := m.State(); got != StateIdle {
		t.Errorf("state after remark = %v, want idle", got)
	}
}

func TestSpeakRemarkSkippedOutsideAIMode(t *testing.T) {
	m := NewMachine() // idle mode
	chat := &fakeProvider{reply: "should never be called"}
	pipe := NewVoicePipeline(m, &fakeWriter{}, &fakeProvider{}, chat, &fakeProvider{}, "", "gpt-4o-mini", "", "", "", 16000, 1)
	if err := pipe.SpeakRemark(context.Background(), "(nudge)", nil); err != nil {
		t.Fatalf("speak remark: %v", err)
	}
	if chat.lastChat.Model != "" {
		t.Error("chat provider must not be called outside AI mode")
	}
}

func TestSpeakRemarkSkippedWhenNotIdle(t *testing.T) {
	m := NewMachine()
	m.SetMode("ai")
	m.Transition(EventListen) // user is mid-conversation
	chat := &fakeProvider{reply: "should never be called"}
	pipe := NewVoicePipeline(m, &fakeWriter{}, &fakeProvider{}, chat, &fakeProvider{}, "", "gpt-4o-mini", "", "", "", 16000, 1)
	if err := pipe.SpeakRemark(context.Background(), "(nudge)", nil); err != nil {
		t.Fatalf("speak remark: %v", err)
	}
	if chat.lastChat.Model != "" {
		t.Error("chat provider must not be called while not idle")
	}
}

func TestSpeakRemarkEmptyReplyReturnsToIdle(t *testing.T) {
	m := NewMachine()
	m.SetMode("ai")
	pipe := NewVoicePipeline(m, &fakeWriter{}, &fakeProvider{}, &fakeProvider{reply: "  "}, &fakeProvider{}, "", "gpt-4o-mini", "", "", "", 16000, 1)
	if err := pipe.SpeakRemark(context.Background(), "(nudge)", nil); err != nil {
		t.Fatalf("speak remark: %v", err)
	}
	if got := m.State(); got != StateIdle {
		t.Errorf("state = %v, want idle", got)
	}
}

func TestSpeakRemarkChatFailureEntersErrorState(t *testing.T) {
	m := NewMachine()
	m.SetMode("ai")
	chat := &fakeProvider{err: fmt.Errorf("boom")}
	pipe := NewVoicePipeline(m, &fakeWriter{}, &fakeProvider{}, chat, &fakeProvider{}, "", "gpt-4o-mini", "", "", "", 16000, 1)
	if err := pipe.SpeakRemark(context.Background(), "(nudge)", nil); err == nil {
		t.Fatal("expected error")
	}
	if got := m.State(); got != StateError {
		t.Errorf("state = %v, want error", got)
	}
}

// captureLogger records Infof lines so tests can assert log content.
type captureLogger struct {
	mu    sync.Mutex
	lines []string
}

func (l *captureLogger) Infof(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, fmt.Sprintf(format, args...))
}
func (l *captureLogger) Debugf(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, fmt.Sprintf(format, args...))
}

func (l *captureLogger) joined() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Join(l.lines, "\n")
}

func TestPipelineLogsTokenUsage(t *testing.T) {
	m := NewMachine()
	m.SetMode("ai")
	stt := &fakeProvider{transcript: "hello"}
	chat := &fakeProvider{
		reply:     "oh wow",
		chatUsage: providers.Usage{PromptTokens: 612, CompletionTokens: 43, TotalTokens: 655},
	}
	tts := &fakeProvider{speech: []byte{1, 2, 3, 4}}
	pipe := NewVoicePipeline(m, &fakeWriter{}, stt, chat, tts, "whisper-1", "gpt-4o-mini", "tts-1", "alloy", "be bmo", 16000, 1)
	logger := &captureLogger{}
	pipe.SetLogger(logger)

	// One second of loud-enough PCM so the signal gate passes.
	pcm := make([]byte, 32000)
	for i := 0; i < len(pcm); i += 2 {
		pcm[i] = 0x00
		pcm[i+1] = 0x40
	}
	if err := pipe.ProcessBatch(context.Background(), pcm); err != nil {
		t.Fatalf("process batch: %v", err)
	}

	logs := logger.joined()
	// whisper-1 reports no usage: STT line falls back to audio seconds.
	if !strings.Contains(logs, "pipeline STT:") || !strings.Contains(logs, "tokens: n/a (1.0s audio)") {
		t.Errorf("STT log missing usage fallback: %q", logs)
	}
	if !strings.Contains(logs, "tokens: 612 prompt + 43 completion") {
		t.Errorf("chat log missing token usage: %q", logs)
	}
	// TTS billing unit is input characters ("oh wow" = 6).
	if !strings.Contains(logs, "input: 6 chars") {
		t.Errorf("TTS log missing input chars: %q", logs)
	}
}

func TestRemarkLogsTokenUsage(t *testing.T) {
	m := NewMachine()
	m.SetMode("ai")
	chat := &fakeProvider{
		reply:     "daebak!",
		chatUsage: providers.Usage{PromptTokens: 705, CompletionTokens: 38, TotalTokens: 743},
	}
	tts := &fakeProvider{speech: []byte{1, 2, 3, 4}}
	pipe := NewVoicePipeline(m, &fakeWriter{}, &fakeProvider{}, chat, tts, "", "gpt-4o-mini", "tts-1", "alloy", "", 16000, 1)
	logger := &captureLogger{}
	pipe.SetLogger(logger)

	if err := pipe.SpeakRemark(context.Background(), "(nudge)", nil); err != nil {
		t.Fatalf("speak remark: %v", err)
	}
	logs := logger.joined()
	if !strings.Contains(logs, "remark Chat:") || !strings.Contains(logs, "tokens: 705 prompt + 38 completion") {
		t.Errorf("remark chat log missing token usage: %q", logs)
	}
	if !strings.Contains(logs, "remark TTS:") || !strings.Contains(logs, "input: 7 chars") {
		t.Errorf("remark TTS log missing input chars: %q", logs)
	}
}

func TestSpeakRemarkLogsPromptContext(t *testing.T) {
	m := NewMachine()
	m.SetMode("ai")
	chat := &fakeProvider{reply: "daebak!"}
	tts := &fakeProvider{speech: []byte{1, 2, 3, 4}}
	pipe := NewVoicePipeline(m, &fakeWriter{}, &fakeProvider{}, chat, tts, "", "gpt-4o-mini", "tts-1", "alloy", "", 16000, 1)
	pipe.SetSystemPromptSource(func() string { return "persona\n\nDEVICE AWARENESS: stuff" })
	logger := &captureLogger{}
	pipe.SetLogger(logger)

	if err := pipe.SpeakRemark(context.Background(), "(nudge about achievements)", nil); err != nil {
		t.Fatalf("speak remark: %v", err)
	}
	logs := logger.joined()
	if !strings.Contains(logs, `remark nudge: "(nudge about achievements)"`) {
		t.Errorf("nudge not logged: %q", logs)
	}
	if !strings.Contains(logs, "remark system prompt:") || !strings.Contains(logs, "DEVICE AWARENESS: stuff") {
		t.Errorf("system prompt not logged: %q", logs)
	}
}

func TestSpeakRemarkInvokesOnSpoken(t *testing.T) {
	m := NewMachine()
	m.SetMode("ai")
	chat := &fakeProvider{reply: "what a save file!"}
	tts := &fakeProvider{speech: []byte{1, 2, 3, 4}}
	pipe := NewVoicePipeline(m, &fakeWriter{}, &fakeProvider{}, chat, tts, "", "gpt-4o-mini", "tts-1", "alloy", "", 16000, 1)

	var spoken []string
	if err := pipe.SpeakRemark(context.Background(), "(nudge)", func(reply string) { spoken = append(spoken, reply) }); err != nil {
		t.Fatalf("speak remark: %v", err)
	}
	if len(spoken) != 1 || spoken[0] != "what a save file!" {
		t.Fatalf("onSpoken calls = %v, want one call with the reply", spoken)
	}
}

func TestSpeakRemarkOnSpokenSkippedOnFailure(t *testing.T) {
	m := NewMachine()
	m.SetMode("ai")
	called := 0
	onSpoken := func(string) { called++ }

	// Chat failure: callback must not fire.
	chatFail := &fakeProvider{err: fmt.Errorf("boom")}
	pipe := NewVoicePipeline(m, &fakeWriter{}, &fakeProvider{}, chatFail, &fakeProvider{}, "", "gpt-4o-mini", "", "", "", 16000, 1)
	if err := pipe.SpeakRemark(context.Background(), "(nudge)", onSpoken); err == nil {
		t.Fatal("expected chat error")
	}
	// Empty reply: callback must not fire.
	m2 := NewMachine()
	m2.SetMode("ai")
	pipe2 := NewVoicePipeline(m2, &fakeWriter{}, &fakeProvider{}, &fakeProvider{reply: "  "}, &fakeProvider{}, "", "gpt-4o-mini", "", "", "", 16000, 1)
	if err := pipe2.SpeakRemark(context.Background(), "(nudge)", onSpoken); err != nil {
		t.Fatalf("speak remark: %v", err)
	}
	// TTS failure: callback must not fire.
	m3 := NewMachine()
	m3.SetMode("ai")
	pipe3 := NewVoicePipeline(m3, &fakeWriter{}, &fakeProvider{}, &fakeProvider{reply: "hi"}, &fakeProvider{err: fmt.Errorf("tts boom")}, "", "gpt-4o-mini", "tts-1", "alloy", "", 16000, 1)
	if err := pipe3.SpeakRemark(context.Background(), "(nudge)", onSpoken); err == nil {
		t.Fatal("expected tts error")
	}
	if called != 0 {
		t.Fatalf("onSpoken fired %d times on failure paths, want 0", called)
	}
}

func TestSpeakVerbatimSkipsChat(t *testing.T) {
	m := NewMachine()
	m.SetMode("ai")
	writer := &fakeWriter{}
	chat := &fakeProvider{reply: "must never be used"}
	tts := &fakeProvider{speech: []byte{1, 2, 3, 4}}
	pipe := NewVoicePipeline(m, writer, &fakeProvider{}, chat, tts, "", "gpt-4o-mini", "tts-1", "alloy", "", 16000, 1)

	var spoken []string
	if err := pipe.SpeakVerbatim(context.Background(), "Who wants to play video games?", func(s string) { spoken = append(spoken, s) }); err != nil {
		t.Fatalf("speak verbatim: %v", err)
	}
	if chat.lastChat.Model != "" {
		t.Error("chat provider must not be called for verbatim speech")
	}
	if tts.lastSpeech.Input != "Who wants to play video games?" {
		t.Errorf("tts input = %q", tts.lastSpeech.Input)
	}
	if len(spoken) != 1 || spoken[0] != "Who wants to play video games?" {
		t.Fatalf("onSpoken = %v", spoken)
	}
	if writer.totalBytes() == 0 {
		t.Error("expected PCM written to playback")
	}
	if got := m.State(); got != StateIdle {
		t.Errorf("state after verbatim = %v, want idle", got)
	}
}

func TestSpeakVerbatimSkippedWhenNotIdle(t *testing.T) {
	m := NewMachine()
	m.SetMode("ai")
	m.Transition(EventListen)
	tts := &fakeProvider{speech: []byte{1, 2}}
	pipe := NewVoicePipeline(m, &fakeWriter{}, &fakeProvider{}, &fakeProvider{}, tts, "", "", "tts-1", "alloy", "", 16000, 1)
	if err := pipe.SpeakVerbatim(context.Background(), "quote", nil); err != nil {
		t.Fatalf("speak verbatim: %v", err)
	}
	if tts.lastSpeech.Input != "" {
		t.Error("tts must not be called while not idle")
	}
}

func TestProcessBatchDoesNotLogSystemPromptByDefault(t *testing.T) {
	m := NewMachine()
	m.SetMode("ai")
	stt := &fakeProvider{transcript: "hello"}
	chat := &fakeProvider{reply: "hi"}
	tts := &fakeProvider{speech: make([]byte, 2400)}
	pipe := NewVoicePipeline(m, &fakeWriter{}, stt, chat, tts, "whisper-1", "gpt-4o-mini", "tts-1", "nova", "secret persona", 16000, 1)
	logger := &captureLogger{}
	pipe.SetLogger(logger)
	pipe.SetTTSInstructions("secret voice style")

	if err := pipe.ProcessBatch(context.Background(), []byte{0x00, 0x40, 0x00, 0x40}); err != nil {
		t.Fatalf("ProcessBatch() error = %v", err)
	}
	logs := logger.joined()
	if strings.Contains(logs, "secret persona") {
		t.Errorf("system prompt leaked into logs with logSystemPrompt=false: %q", logs)
	}
	if strings.Contains(logs, "secret voice style") {
		t.Errorf("TTS instructions leaked into logs with logSystemPrompt=false: %q", logs)
	}
}

func TestProcessBatchLogsSystemPromptWhenEnabled(t *testing.T) {
	m := NewMachine()
	m.SetMode("ai")
	stt := &fakeProvider{transcript: "hello"}
	chat := &fakeProvider{reply: "hi"}
	tts := &fakeProvider{speech: make([]byte, 2400)}
	pipe := NewVoicePipeline(m, &fakeWriter{}, stt, chat, tts, "whisper-1", "gpt-4o-mini", "tts-1", "nova", "", 16000, 1)
	pipe.SetSystemPromptSource(func() string { return "be bmo, the computer" })
	pipe.SetTTSInstructions("speak like bmo")
	logger := &captureLogger{}
	pipe.SetLogger(logger)
	pipe.SetLogSystemPrompt(true)

	if err := pipe.ProcessBatch(context.Background(), []byte{0x00, 0x40, 0x00, 0x40}); err != nil {
		t.Fatalf("ProcessBatch() error = %v", err)
	}
	logs := logger.joined()
	if !strings.Contains(logs, "pipeline system prompt:") || !strings.Contains(logs, "be bmo, the computer") {
		t.Errorf("system prompt not in logs: %q", logs)
	}
	if !strings.Contains(logs, "pipeline TTS instructions:") || !strings.Contains(logs, "speak like bmo") {
		t.Errorf("TTS instructions not in logs: %q", logs)
	}
}
