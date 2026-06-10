package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCapabilityDetection(t *testing.T) {
	client := NewOpenAICompatibleClient(Config{BaseURL: "https://example.invalid", APIKey: "secret"}, http.DefaultClient)

	if !client.Supports(CapabilitySTT) || !client.Supports(CapabilityChat) || !client.Supports(CapabilityTTS) {
		t.Fatalf("expected OpenAI-compatible client to support STT, chat, and TTS")
	}
	if client.Supports(Capability("unknown")) {
		t.Fatalf("unexpected capability support for unknown capability")
	}
}

func TestAuthRequirements(t *testing.T) {
	client := NewOpenAICompatibleClient(Config{}, http.DefaultClient)
	if !client.RequiresAuth() {
		t.Fatalf("expected auth to be required when no API key is configured")
	}

	client = NewOpenAICompatibleClient(Config{APIKey: "secret"}, http.DefaultClient)
	if client.RequiresAuth() {
		t.Fatalf("expected auth to be satisfied when API key is configured")
	}
}

func TestErrorClassification(t *testing.T) {
	client := NewOpenAICompatibleClient(Config{}, http.DefaultClient)

	tests := []struct {
		name string
		err  error
		want ErrorKind
	}{
		{name: "auth", err: &HTTPError{StatusCode: http.StatusUnauthorized, Body: "unauthorized"}, want: ErrorKindAuth},
		{name: "quota", err: &HTTPError{StatusCode: http.StatusTooManyRequests, Body: "rate limited"}, want: ErrorKindQuota},
		{name: "provider", err: &HTTPError{StatusCode: http.StatusBadRequest, Body: "bad request"}, want: ErrorKindProvider},
		{name: "network", err: io.EOF, want: ErrorKindNetwork},
		{name: "context", err: context.Canceled, want: ErrorKindNetwork},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := client.ClassifyError(tt.err); got != tt.want {
				t.Fatalf("ClassifyError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestModelListing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Fatalf("missing bearer token, got %q", got)
		}
		if !strings.HasSuffix(r.URL.Path, "/models") {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-4o-mini"},{"id":"whisper-1"},{"id":"tts-1"}]}`))
	}))
	defer server.Close()

	client := NewOpenAICompatibleClient(Config{BaseURL: server.URL, APIKey: "secret"}, server.Client())
	models, err := client.Models(context.Background())
	if err != nil {
		t.Fatalf("Models() error = %v", err)
	}

	if len(models) != 3 || models[0] != "gpt-4o-mini" || models[1] != "whisper-1" || models[2] != "tts-1" {
		t.Fatalf("unexpected models: %#v", models)
	}
}

func TestTranscribeReplyAndSpeak(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/audio/transcriptions":
			if got := r.Header.Get("Authorization"); got != "Bearer secret" {
				t.Fatalf("missing auth header: %q", got)
			}
			reader, err := r.MultipartReader()
			if err != nil {
				t.Fatalf("MultipartReader() error = %v", err)
			}
			fields := map[string]string{}
			for {
				part, err := reader.NextPart()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatalf("NextPart() error = %v", err)
				}
				b, _ := io.ReadAll(part)
				fields[part.FormName()] = string(b)
			}
			if fields["model"] != "whisper-1" || fields["response_format"] != "json" {
				t.Fatalf("unexpected transcription fields: %#v", fields)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"text":"hello bmo"}`))
		case "/chat/completions":
			var payload struct {
				Model    string    `json:"model"`
				Messages []Message `json:"messages"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode chat payload: %v", err)
			}
			if payload.Model != "gpt-4o-mini" || len(payload.Messages) != 2 || payload.Messages[0].Role != "system" {
				t.Fatalf("unexpected chat payload: %#v", payload)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"oh wow"}}]}`))
		case "/audio/speech":
			var payload struct {
				Model  string `json:"model"`
				Voice  string `json:"voice"`
				Input  string `json:"input"`
				Format string `json:"response_format"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode speech payload: %v", err)
			}
			if payload.Model != "tts-1" || payload.Voice != "alloy" || payload.Input != "oh wow" {
				t.Fatalf("unexpected speech payload: %#v", payload)
			}
			_, _ = w.Write([]byte{0x01, 0x02, 0x03})
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewOpenAICompatibleClient(Config{BaseURL: server.URL, APIKey: "secret"}, server.Client())
	text, err := client.Transcribe(context.Background(), TranscriptionRequest{Model: "whisper-1", Audio: []byte{0x00, 0x00, 0x01, 0x00}, SampleRate: 16000, Channels: 1})
	if err != nil {
		t.Fatalf("Transcribe() error = %v", err)
	}
	if text != "hello bmo" {
		t.Fatalf("Transcribe() = %q, want hello bmo", text)
	}

	reply, err := client.Reply(context.Background(), ChatRequest{Model: "gpt-4o-mini", SystemPrompt: "be bmo", Messages: []Message{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatalf("Reply() error = %v", err)
	}
	if reply != "oh wow" {
		t.Fatalf("Reply() = %q, want oh wow", reply)
	}

	speech, err := client.Speak(context.Background(), SpeechRequest{Model: "tts-1", Input: reply})
	if err != nil {
		t.Fatalf("Speak() error = %v", err)
	}
	if !bytes.Equal(speech, []byte{0x01, 0x02, 0x03}) {
		t.Fatalf("Speak() = %#v, want bytes", speech)
	}
}

func TestProviderInterfaceConformance(t *testing.T) {
	var _ STTProvider = NewOpenAICompatibleClient(Config{}, http.DefaultClient)
	var _ ChatProvider = NewOpenAICompatibleClient(Config{}, http.DefaultClient)
	var _ TTSProvider = NewOpenAICompatibleClient(Config{}, http.DefaultClient)
}

func TestClassifyErrorFallback(t *testing.T) {
	client := NewOpenAICompatibleClient(Config{}, http.DefaultClient)
	if got := client.ClassifyError(errors.New("something else")); got != ErrorKindProvider {
		t.Fatalf("fallback classification = %v, want provider", got)
	}
}
