package main

import (
	"context"
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/carroarmato0/nextui-bmo/internal/audio"
	"github.com/carroarmato0/nextui-bmo/internal/config"
	"github.com/carroarmato0/nextui-bmo/internal/providers"
)

type clipDef struct {
	name  string
	nudge string
}

var clipDefs = []clipDef{
	{"hello", "Give a single short, excited in-character greeting to user. One sentence only. Do not use punctuation spoken aloud oddly."},
	{"mod_error", "Give a short in-character message warning that one of your customisation files seems broken and you have fallen back to your defaults. One or two sentences only."},
	{"timeout", "Give a short in-character apology for not being able to think of an answer now. Ask the user to try again. One or two sentences only."},
	{"error", "Give a short in-character message saying you cannot reach anyone right now, suggest the user checks the connection. One or two sentences only."},
	{"goodbye", "Give a short, warm in-character farewell to the user. One sentence only."},
	{"sleep", "Give a short in-character message for when you are about to go to sleep. One sentence only."},
	{"wake", "Give a short in-character message for when you have just woken up. One sentence only."},
}

func main() {
	key := flag.String("key", "", "OpenAI API key (overrides env/file)")
	baseURL := flag.String("base-url", "https://api.openai.com/v1", "API base URL")
	chatModel := flag.String("chat-model", "gpt-4o-mini", "Chat model")
	ttsModel := flag.String("tts-model", "tts-1", "TTS model")
	voice := flag.String("voice", "alloy", "TTS voice")
	instructions := flag.String("instructions", config.DefaultTTSInstructions, "TTS style instructions")
	outDir := flag.String("out", "internal/clips/assets/audio", "Output directory for PCM files")
	flag.Parse()

	resolvedKey := strings.TrimSpace(*key)
	if resolvedKey == "" {
		resolvedKey = strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	}
	if resolvedKey == "" {
		data, _ := os.ReadFile(".env")
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if after, ok := strings.CutPrefix(line, "OPENAI_API_KEY="); ok {
				resolvedKey = strings.Trim(strings.TrimSpace(after), `"'`)
				break
			}
		}
	}
	if resolvedKey == "" {
		log.Fatal("no API key found: add OPENAI_API_KEY=sk-... to .env in project root, set OPENAI_API_KEY env var, or use -key flag")
	}

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		log.Fatalf("create output dir: %v", err)
	}

	client := providers.NewOpenAICompatibleClient(providers.Config{
		BaseURL: *baseURL,
		APIKey:  resolvedKey,
	}, http.DefaultClient)

	ctx := context.Background()

	for _, clip := range clipDefs {
		log.Printf("generating %s ...", clip.name)

		chatResp, err := client.Reply(ctx, providers.ChatRequest{
			Model:        *chatModel,
			Messages:     []providers.Message{{Role: "user", Content: clip.nudge}},
			SystemPrompt: config.DefaultSystemPrompt,
		})
		if err != nil {
			log.Fatalf("chat %s: %v", clip.name, err)
		}
		text := strings.TrimSpace(chatResp.Text)
		if text == "" {
			log.Fatalf("empty chat response for %s", clip.name)
		}
		log.Printf("  text: %q", text)

		speech, err := client.Speak(ctx, providers.SpeechRequest{
			Model:        *ttsModel,
			Voice:        *voice,
			Input:        text,
			Format:       "pcm",
			Instructions: *instructions,
		})
		if err != nil {
			log.Fatalf("tts %s: %v", clip.name, err)
		}

		// TTS returns 24kHz mono S16LE; resample to 16kHz mono then upmix to stereo.
		mono16 := audio.ResampleS16LE(speech, 24000, audio.DefaultSampleRate, 1)
		stereo := monoToStereo(mono16)

		outPath := filepath.Join(*outDir, clip.name+".pcm")
		if err := os.WriteFile(outPath, stereo, 0o644); err != nil {
			log.Fatalf("write %s: %v", outPath, err)
		}
		log.Printf("  wrote %d bytes -> %s", len(stereo), outPath)
	}

	fmt.Println("done")
}

func monoToStereo(mono []byte) []byte {
	stereo := make([]byte, len(mono)*2)
	for i := 0; i+1 < len(mono); i += 2 {
		s := binary.LittleEndian.Uint16(mono[i : i+2])
		j := i * 2
		binary.LittleEndian.PutUint16(stereo[j:j+2], s)   // L
		binary.LittleEndian.PutUint16(stereo[j+2:j+4], s) // R
	}
	return stereo
}
