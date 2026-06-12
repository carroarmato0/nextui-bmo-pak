package main

import (
	"context"
	"os"
	"strings"

	"github.com/carroarmato0/nextui-bmo/internal/assistant"
	"github.com/carroarmato0/nextui-bmo/internal/audio"
	"github.com/carroarmato0/nextui-bmo/internal/config"
	"github.com/carroarmato0/nextui-bmo/internal/hardware"
	"github.com/carroarmato0/nextui-bmo/internal/input"
)

// readPromptFile returns the trimmed content of a prompt file, or "" on any
// error so the pipeline falls back to the content loaded at startup.
func readPromptFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// restorePromptDefaults rewrites both prompt files with their built-in
// defaults; wired to the settings menu RESTORE DEFAULTS item.
func restorePromptDefaults(personaPath, voicePath string) error {
	if err := config.WritePromptFile(personaPath, config.DefaultSystemPrompt); err != nil {
		return err
	}
	return config.WritePromptFile(voicePath, config.DefaultTTSInstructions)
}

// systemPromptWithContext joins the persona prompt, the device-awareness
// block, and the recent-remarks block; empty segments are skipped.
func systemPromptWithContext(parts ...string) string {
	var nonEmpty []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			nonEmpty = append(nonEmpty, p)
		}
	}
	return strings.Join(nonEmpty, "\n\n")
}

type pttLogger interface {
	Infof(string, ...any)
	Warnf(string, ...any)
	Debugf(string, ...any)
}

func startPushToTalk(ctx context.Context, logger pttLogger, machine *assistant.Machine, cfg config.Config, profile hardware.Profile, router *audio.CaptureRouter, pipeline *assistant.VoicePipeline, sampleRate, channels int) func() {
	if ctx == nil || logger == nil || machine == nil || router == nil || pipeline == nil {
		return func() {}
	}
	if !cfg.UsesAI() || cfg.InputTrigger != config.InputTriggerPTT {
		return func() {}
	}

	watcher, err := input.NewWatcher(profile.InputEvent, cfg.PTTButtons...)
	if err != nil {
		logger.Warnf("PTT disabled: %v", err)
		return func() {}
	}
	if err := watcher.Start(ctx); err != nil {
		logger.Warnf("PTT disabled: %v", err)
		return func() {}
	}

	buffer := input.NewBuffer(audio.BytesPerSecond(sampleRate, channels, audio.BytesPerSampleS16LE) * 15)
	utterances := make(chan []byte, 1)

	logger.Infof("PTT ready: device=%s buttons=%v", profile.InputEvent, cfg.PTTButtons)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case utterance := <-utterances:
				if len(utterance) == 0 {
					continue
				}
				if err := pipeline.ProcessBatch(ctx, utterance); err != nil {
					logger.Warnf("voice pipeline error: %v", err)
				}
			}
		}
	}()

	go func() {
		for batch := range router.Batches() {
			if buffer.Held() {
				buffer.Append(batch)
			}
		}
	}()

	go func() {
		for ev := range watcher.Events() {
			if ev.Held {
				// Outside AI mode BMO does not react to push-to-talk at
				// all: no listening state, no recording, no API traffic.
				if !machine.AIEnabled() {
					logger.Debugf("PTT ignored: AI mode disabled")
					continue
				}
				// A while BMO is talking cuts the speech short; the call
				// blocks until the machine is back in idle, so the regular
				// PTT press path below behaves as if pressed from idle.
				if pipeline.InterruptSpeech() {
					logger.Infof("speech interrupted by PTT press")
				}
				snap := machine.Snapshot()
				if snap.Current == assistant.StateSleeping || snap.Current == assistant.StateError {
					machine.Transition(assistant.EventWake)
				}
				machine.Transition(assistant.EventListen)
				buffer.Begin()
				logger.Debugf("PTT pressed: %s (%d)", ev.Button, ev.Code)
				logger.Infof("PTT recording started")
				continue
			}

			payload := buffer.End()
			if machine.State() == assistant.StateListening {
				machine.Transition(assistant.EventRest)
			}
			logger.Debugf("PTT released: %s (%d)", ev.Button, ev.Code)
			logger.Infof("PTT recording stopped: %d bytes captured", len(payload))
			if len(payload) == 0 {
				continue
			}
			select {
			case utterances <- payload:
			case <-ctx.Done():
				return
			default:
				logger.Warnf("dropping PTT utterance: pipeline busy")
			}
		}
	}()

	return func() {
		_ = watcher.Close()
	}
}
