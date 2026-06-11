package providers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

type Capability string

const (
	CapabilitySTT  Capability = "stt"
	CapabilityChat Capability = "chat"
	CapabilityTTS  Capability = "tts"
)

type ErrorKind string

const (
	ErrorKindAuth     ErrorKind = "auth"
	ErrorKindQuota    ErrorKind = "quota"
	ErrorKindNetwork  ErrorKind = "network"
	ErrorKindProvider ErrorKind = "provider"
)

type HTTPError struct {
	StatusCode int
	Body       string
}

func (e *HTTPError) Error() string {
	if e == nil {
		return ""
	}
	if e.Body == "" {
		return fmt.Sprintf("http error: %d", e.StatusCode)
	}
	return fmt.Sprintf("http error: %d: %s", e.StatusCode, e.Body)
}

type Config struct {
	BaseURL string
	APIKey  string
	Timeout time.Duration
}

type ModelLister interface {
	Models(ctx context.Context) ([]string, error)
}

type AuthAware interface {
	RequiresAuth() bool
}

type ErrorClassifier interface {
	ClassifyError(err error) ErrorKind
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type TranscriptionRequest struct {
	Model      string
	Audio      []byte
	SampleRate int
	Channels   int
	Format     string
}

type ChatRequest struct {
	Model        string
	Messages     []Message
	SystemPrompt string
}

type SpeechRequest struct {
	Model  string
	Voice  string
	Input  string
	Format string
	// Instructions steer the speaking style on instruction-capable TTS
	// models (e.g. gpt-4o-mini-tts). Ignored by tts-1 family models.
	Instructions string
}

type STTProvider interface {
	ModelLister
	AuthAware
	ErrorClassifier
	Capabilities() []Capability
	Supports(Capability) bool
	Transcribe(ctx context.Context, req TranscriptionRequest) (string, error)
}

type ChatProvider interface {
	ModelLister
	AuthAware
	ErrorClassifier
	Capabilities() []Capability
	Supports(Capability) bool
	Reply(ctx context.Context, req ChatRequest) (string, error)
}

type TTSProvider interface {
	ModelLister
	AuthAware
	ErrorClassifier
	Capabilities() []Capability
	Supports(Capability) bool
	Speak(ctx context.Context, req SpeechRequest) ([]byte, error)
}

type OpenAICompatibleClient struct {
	cfg    Config
	client *http.Client
}

func NewOpenAICompatibleClient(cfg Config, client *http.Client) *OpenAICompatibleClient {
	if client == nil {
		client = http.DefaultClient
	}
	if cfg.Timeout > 0 {
		clone := *client
		clone.Timeout = cfg.Timeout
		client = &clone
	}
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	return &OpenAICompatibleClient{cfg: cfg, client: client}
}

func (c *OpenAICompatibleClient) Capabilities() []Capability {
	return []Capability{CapabilitySTT, CapabilityChat, CapabilityTTS}
}

func (c *OpenAICompatibleClient) Supports(cap Capability) bool {
	for _, supported := range c.Capabilities() {
		if supported == cap {
			return true
		}
	}
	return false
}

func (c *OpenAICompatibleClient) RequiresAuth() bool {
	return strings.TrimSpace(c.cfg.APIKey) == ""
}

func (c *OpenAICompatibleClient) ClassifyError(err error) ErrorKind {
	if err == nil {
		return ErrorKindProvider
	}

	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		switch {
		case httpErr.StatusCode == http.StatusUnauthorized || httpErr.StatusCode == http.StatusForbidden:
			return ErrorKindAuth
		case httpErr.StatusCode == http.StatusTooManyRequests:
			return ErrorKindQuota
		case httpErr.StatusCode >= 500:
			return ErrorKindNetwork
		default:
			return ErrorKindProvider
		}
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, io.EOF) {
		return ErrorKindNetwork
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return ErrorKindNetwork
	}

	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "unauthorized"), strings.Contains(msg, "api key"), strings.Contains(msg, "forbidden"):
		return ErrorKindAuth
	case strings.Contains(msg, "rate limit"), strings.Contains(msg, "quota"):
		return ErrorKindQuota
	default:
		return ErrorKindProvider
	}
}

func (c *OpenAICompatibleClient) Models(ctx context.Context) ([]string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(c.cfg.BaseURL) == "" {
		return defaultModelSet(), nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.BaseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	if key := strings.TrimSpace(c.cfg.APIKey); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, &HTTPError{StatusCode: resp.StatusCode, Body: strings.TrimSpace(string(body))}
	}

	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := jsonDecode(resp.Body, &payload); err != nil {
		return nil, err
	}

	models := make([]string, 0, len(payload.Data))
	for _, item := range payload.Data {
		if strings.TrimSpace(item.ID) != "" {
			models = append(models, item.ID)
		}
	}
	return models, nil
}

func defaultModelSet() []string {
	return []string{"gpt-4o-mini", "whisper-1", "tts-1"}
}
