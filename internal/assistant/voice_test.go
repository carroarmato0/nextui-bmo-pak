package assistant

import (
	"context"
	"encoding/binary"
	"errors"
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
	transcript string
	reply      string
	speech     []byte
	err        error
}

func (f *fakeProvider) Models(ctx context.Context) ([]string, error) { return nil, nil }
func (f *fakeProvider) RequiresAuth() bool                           { return false }
func (f *fakeProvider) ClassifyError(err error) providers.ErrorKind  {
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "quota") {
		return providers.ErrorKindQuota
	}
	return providers.ErrorKindProvider
}
func (f *fakeProvider) Capabilities() []providers.Capability         { return []providers.Capability{providers.CapabilitySTT, providers.CapabilityChat, providers.CapabilityTTS} }
func (f *fakeProvider) Supports(cap providers.Capability) bool {
	for _, supported := range f.Capabilities() {
		if supported == cap {
			return true
		}
	}
	return false
}
func (f *fakeProvider) Transcribe(ctx context.Context, req providers.TranscriptionRequest) (string, error) {
	return f.transcript, f.err
}
func (f *fakeProvider) Reply(ctx context.Context, req providers.ChatRequest) (string, error) {
	return f.reply, f.err
}
func (f *fakeProvider) Speak(ctx context.Context, req providers.SpeechRequest) ([]byte, error) {
	return f.speech, f.err
}

func TestVoicePipelineHappyPath(t *testing.T) {
	m := NewMachine()
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
