package providers

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
)

const defaultTTSVoice = "alloy"

func (c *OpenAICompatibleClient) Transcribe(ctx context.Context, req TranscriptionRequest) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	req = normalizeTranscriptionRequest(c.cfg, req)
	if strings.TrimSpace(c.cfg.BaseURL) == "" {
		return "", fmt.Errorf("transcribe: base url is required")
	}

	body, contentType, err := buildTranscriptionBody(req)
	if err != nil {
		return "", err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+"/audio/transcriptions", body)
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", contentType)
	c.applyAuth(httpReq)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", &HTTPError{StatusCode: resp.StatusCode, Body: strings.TrimSpace(string(body))}
	}

	var payload struct {
		Text string `json:"text"`
	}
	if err := jsonDecode(resp.Body, &payload); err != nil {
		return "", err
	}
	return strings.TrimSpace(payload.Text), nil
}

func (c *OpenAICompatibleClient) Reply(ctx context.Context, req ChatRequest) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	req = normalizeChatRequest(c.cfg, req)
	if strings.TrimSpace(c.cfg.BaseURL) == "" {
		return "", fmt.Errorf("reply: base url is required")
	}

	payload := struct {
		Model    string    `json:"model"`
		Messages []Message `json:"messages"`
	}{
		Model:    req.Model,
		Messages: req.Messages,
	}
	if req.SystemPrompt != "" {
		payload.Messages = append([]Message{{Role: "system", Content: req.SystemPrompt}}, payload.Messages...)
	}

	buf, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+"/chat/completions", bytes.NewReader(buf))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	c.applyAuth(httpReq)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", &HTTPError{StatusCode: resp.StatusCode, Body: strings.TrimSpace(string(body))}
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := jsonDecode(resp.Body, &result); err != nil {
		return "", err
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("reply: no choices returned")
	}
	return strings.TrimSpace(result.Choices[0].Message.Content), nil
}

func (c *OpenAICompatibleClient) Speak(ctx context.Context, req SpeechRequest) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	req = normalizeSpeechRequest(c.cfg, req)
	if strings.TrimSpace(c.cfg.BaseURL) == "" {
		return nil, fmt.Errorf("speak: base url is required")
	}

	payload := struct {
		Model  string `json:"model"`
		Voice  string `json:"voice"`
		Input  string `json:"input"`
		Format string `json:"response_format,omitempty"`
	}{
		Model:  req.Model,
		Voice:  req.Voice,
		Input:  req.Input,
		Format: req.Format,
	}
	if payload.Format == "" {
		payload.Format = "wav"
	}

	buf, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+"/audio/speech", bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	c.applyAuth(httpReq)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, &HTTPError{StatusCode: resp.StatusCode, Body: strings.TrimSpace(string(body))}
	}

	return io.ReadAll(resp.Body)
}

func (c *OpenAICompatibleClient) applyAuth(req *http.Request) {
	if key := strings.TrimSpace(c.cfg.APIKey); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
}

func normalizeTranscriptionRequest(cfg Config, req TranscriptionRequest) TranscriptionRequest {
	if strings.TrimSpace(req.Model) == "" {
		req.Model = defaultModel(cfg)
	}
	if req.SampleRate <= 0 {
		req.SampleRate = 16000
	}
	if req.Channels <= 0 {
		req.Channels = 1
	}
	if strings.TrimSpace(req.Format) == "" {
		req.Format = "wav"
	}
	return req
}

func normalizeChatRequest(cfg Config, req ChatRequest) ChatRequest {
	if strings.TrimSpace(req.Model) == "" {
		req.Model = defaultModel(cfg)
	}
	return req
}

func normalizeSpeechRequest(cfg Config, req SpeechRequest) SpeechRequest {
	if strings.TrimSpace(req.Model) == "" {
		req.Model = defaultModel(cfg)
	}
	if strings.TrimSpace(req.Voice) == "" {
		req.Voice = defaultTTSVoice
	}
	if strings.TrimSpace(req.Format) == "" {
		req.Format = "pcm"
	}
	return req
}

func defaultModel(_ Config) string {
	if models := defaultModelSet(); len(models) > 0 {
		return models[0]
	}
	return ""
}

func buildTranscriptionBody(req TranscriptionRequest) (io.Reader, string, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	if err := writer.WriteField("model", req.Model); err != nil {
		return nil, "", err
	}
	if req.SampleRate > 0 {
		if err := writer.WriteField("sample_rate", fmt.Sprintf("%d", req.SampleRate)); err != nil {
			return nil, "", err
		}
	}
	if req.Channels > 0 {
		if err := writer.WriteField("channels", fmt.Sprintf("%d", req.Channels)); err != nil {
			return nil, "", err
		}
	}
	if err := writer.WriteField("response_format", "json"); err != nil {
		return nil, "", err
	}

	part, err := writer.CreateFormFile("file", filepath.Base("bmo.wav"))
	if err != nil {
		return nil, "", err
	}
	if _, err := part.Write(wavBytes(req.Audio, req.SampleRate, req.Channels)); err != nil {
		return nil, "", err
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return &buf, writer.FormDataContentType(), nil
}

func wavBytes(pcm []byte, sampleRate, channels int) []byte {
	if sampleRate <= 0 {
		sampleRate = 16000
	}
	if channels <= 0 {
		channels = 1
	}
	bitsPerSample := 16
	blockAlign := channels * bitsPerSample / 8
	byteRate := sampleRate * blockAlign
	subchunk2Size := len(pcm)
	riffSize := 36 + subchunk2Size

	buf := bytes.NewBuffer(make([]byte, 0, 44+len(pcm)))
	buf.WriteString("RIFF")
	_ = binary.Write(buf, binary.LittleEndian, uint32(riffSize))
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	_ = binary.Write(buf, binary.LittleEndian, uint32(16))
	_ = binary.Write(buf, binary.LittleEndian, uint16(1))
	_ = binary.Write(buf, binary.LittleEndian, uint16(channels))
	_ = binary.Write(buf, binary.LittleEndian, uint32(sampleRate))
	_ = binary.Write(buf, binary.LittleEndian, uint32(byteRate))
	_ = binary.Write(buf, binary.LittleEndian, uint16(blockAlign))
	_ = binary.Write(buf, binary.LittleEndian, uint16(bitsPerSample))
	buf.WriteString("data")
	_ = binary.Write(buf, binary.LittleEndian, uint32(subchunk2Size))
	buf.Write(pcm)
	return buf.Bytes()
}
