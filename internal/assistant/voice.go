package assistant

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/carroarmato0/nextui-bmo/internal/audio"
	"github.com/carroarmato0/nextui-bmo/internal/providers"
)

type AudioWriter interface {
	WritePCM([]byte) error
}

type VoicePipeline struct {
	machine *Machine
	writer  AudioWriter

	stt providers.STTProvider
	chat providers.ChatProvider
	tts providers.TTSProvider

	sttModel    string
	chatModel   string
	ttsModel    string
	ttsVoice    string
	systemPrompt string

	sampleRate int
	channels   int
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
	transcript = strings.TrimSpace(transcript)
	if transcript == "" {
		if p.machine != nil {
			p.machine.Transition(EventRest)
		}
		return nil
	}

	if p.machine != nil {
		p.machine.Transition(EventThink)
	}

	reply, err := p.chat.Reply(ctx, providers.ChatRequest{
		Model: p.chatModel,
		Messages: []providers.Message{{Role: "user", Content: transcript}},
		SystemPrompt: p.systemPrompt,
	})
	if err != nil {
		return p.fail(err, EventFail)
	}
	reply = strings.TrimSpace(reply)
	if reply == "" {
		if p.machine != nil {
			p.machine.Transition(EventRest)
		}
		return nil
	}

	if p.machine != nil {
		p.machine.Transition(EventSpeak)
	}

	speech, err := p.tts.Speak(ctx, providers.SpeechRequest{
		Model: p.ttsModel,
		Voice: p.ttsVoice,
		Input: reply,
		Format: "pcm",
	})
if err != nil {
		return p.fail(err, EventFail)
	}
	if len(speech) > 0 && p.writer != nil {
		if err := p.writer.WritePCM(speech); err != nil {
			return p.fail(err, EventFail)
		}
	}

	if p.machine != nil {
		p.machine.RecordInteraction(time.Now().UTC())
		p.machine.Transition(EventRest)
	}
	return nil
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
