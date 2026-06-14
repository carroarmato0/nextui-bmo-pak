# Pre-recorded Clips, Timeout & Cancel Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add seven pre-recorded BMO in-character audio clips, a lightweight always-on clip player, a configurable AI request timeout with audio fallback, and silent B-button cancel for in-flight AI requests.

**Architecture:** A new `internal/clips` package (Library + Player) operates independently of AI mode using only an AudioWriter. The VoicePipeline gains a per-batch cancellable context with timeout, playing embedded PCM clips on timeout or network error instead of entering the error state. `audio.Config` is split into separate capture (mono) and playback (stereo) channel counts to match the TrimUI hardware.

**Tech Stack:** Go stdlib (`encoding/xml`, `context`, `embed`, `sync`), existing `internal/audio`, `internal/providers`, `internal/config` packages, `aplay`/`arecord` via existing `audio.Session`.

---

## File Map

| File | Action |
|---|---|
| `internal/audio/audio.go` | Add `PlaybackChannels int` to Config; update normalize/PlaybackArgs/Summary |
| `internal/audio/audio_test.go` | Add tests for channel split |
| `internal/config/config.go` | Add `RequestTimeout int`; normalize clamps to 15 if outside [15,60] |
| `internal/config/config_test.go` | Add RequestTimeout normalization tests |
| `internal/config/prompts.go` | Add `CheckOverrides(homeDir string) []error` |
| `internal/config/prompts_test.go` | Add CheckOverrides tests |
| `cmd/generate-audio/main.go` | New: tool that calls Chat+TTS API and writes `internal/clips/assets/audio/*.pcm` |
| `internal/clips/assets/audio/*.pcm` | New: 7 generated PCM files (16 kHz stereo S16LE) |
| `internal/clips/embed.go` | New: `//go:embed assets/audio/*.pcm` |
| `internal/clips/library.go` | New: Library (override → embedded → nil) |
| `internal/clips/library_test.go` | New: Library tests |
| `internal/clips/player.go` | New: Player with playPaced |
| `internal/clips/player_test.go` | New: Player tests |
| `internal/assistant/voice.go` | Add batch cancel, timeout context, clip fallback, rename channels→captureChannels+playbackChannels |
| `internal/assistant/voice_test.go` | Add cancel/timeout/error-clip tests |
| `internal/ui/settings_menu.go` | Insert `request_timeout` item at index 13; update count 15→16 |
| `internal/ui/settings_menu_test.go` | Update 15→16 items; add timeout cycle test |
| `cmd/bmo-pak/main.go` | Always start audio session; wire clipPlayer, hello/mod_error/goodbye clips, CancelBatch, SetRequestTimeout, stereo playback channels |

---

## Task 1: Split audio.Config into capture and playback channels

**Files:**
- Modify: `internal/audio/audio.go`
- Modify: `internal/audio/audio_test.go`

- [ ] **Step 1: Add PlaybackChannels field and update normalize**

In `internal/audio/audio.go`, add `PlaybackChannels int` to `Config` and update `normalize()`:

```go
type Config struct {
	CaptureDevice   string
	PlaybackDevice  string
	CaptureTool     string
	PlaybackTool    string
	SampleRate      int
	Channels        int // capture (microphone) — stays 1
	PlaybackChannels int // playback (speakers) — defaults to 2
	Format          string
}
```

Update `normalize()` — add after the existing `Channels` normalization:

```go
if c.PlaybackChannels <= 0 {
    c.PlaybackChannels = 2
}
```

Update `PlaybackArgs()` — change `-c` to use `PlaybackChannels`:

```go
func (c Config) PlaybackArgs() []string {
	c = c.normalize()
	return []string{"-q", "-D", c.PlaybackDevice, "-f", c.Format,
		"-c", fmt.Sprintf("%d", c.PlaybackChannels),
		"-r", fmt.Sprintf("%d", c.SampleRate), "-t", "raw",
		fmt.Sprintf("--buffer-time=%d", PlaybackBufferMs*1000)}
}
```

Update `Summary()`:

```go
func (c Config) Summary() string {
	c = c.normalize()
	return fmt.Sprintf("capture=%s via %s (%dch), playback=%s via %s (%dch), %dHz %s",
		c.CaptureDevice, c.CaptureTool, c.Channels,
		c.PlaybackDevice, c.PlaybackTool, c.PlaybackChannels,
		c.SampleRate, c.Format)
}
```

- [ ] **Step 2: Write failing tests**

Add to `internal/audio/audio_test.go`:

```go
func TestDefaultConfigPlaybackChannelsIs2(t *testing.T) {
	cfg := DefaultConfig(hardware.Profile{}).normalize()
	if cfg.PlaybackChannels != 2 {
		t.Fatalf("PlaybackChannels = %d, want 2", cfg.PlaybackChannels)
	}
}

func TestPlaybackArgsUseStereo(t *testing.T) {
	cfg := DefaultConfig(hardware.Profile{}).normalize()
	args := cfg.PlaybackArgs()
	for i, a := range args {
		if a == "-c" && i+1 < len(args) {
			if args[i+1] != "2" {
				t.Fatalf("PlaybackArgs -c = %q, want 2", args[i+1])
			}
			return
		}
	}
	t.Fatal("no -c flag in PlaybackArgs")
}

func TestCaptureArgsUseMono(t *testing.T) {
	cfg := DefaultConfig(hardware.Profile{}).normalize()
	args := cfg.CaptureArgs()
	for i, a := range args {
		if a == "-c" && i+1 < len(args) {
			if args[i+1] != "1" {
				t.Fatalf("CaptureArgs -c = %q, want 1", args[i+1])
			}
			return
		}
	}
	t.Fatal("no -c flag in CaptureArgs")
}

func TestNormalizeZeroPlaybackChannelsDefaultsTo2(t *testing.T) {
	cfg := Config{}.normalize()
	if cfg.PlaybackChannels != 2 {
		t.Fatalf("PlaybackChannels = %d, want 2", cfg.PlaybackChannels)
	}
}
```

- [ ] **Step 3: Run tests and confirm they pass**

```bash
CGO_ENABLED=0 go test github.com/carroarmato0/nextui-bmo/internal/audio
```

Expected: all pass (including new tests).

- [ ] **Step 4: Lint**

```bash
golangci-lint run ./internal/audio/...
```

Expected: no new findings.

- [ ] **Step 5: Commit**

```bash
git add internal/audio/audio.go internal/audio/audio_test.go
git commit -m "feat(audio): split Channels into capture (mono) and PlaybackChannels (stereo)"
```

---

## Task 2: Add config.RequestTimeout

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

- [ ] **Step 1: Write failing tests**

Add to `internal/config/config_test.go` (create the file if it doesn't exist, keeping the `package config` header):

```go
func TestRequestTimeoutDefaultsTo15(t *testing.T) {
	cfg := Default()
	cfg.Normalize()
	if cfg.RequestTimeout != 15 {
		t.Fatalf("want 15, got %d", cfg.RequestTimeout)
	}
}

func TestRequestTimeoutClampedWhenTooLow(t *testing.T) {
	cfg := Default()
	cfg.RequestTimeout = 5
	cfg.Normalize()
	if cfg.RequestTimeout != 15 {
		t.Fatalf("want 15 (clamped from 5), got %d", cfg.RequestTimeout)
	}
}

func TestRequestTimeoutClampedWhenTooHigh(t *testing.T) {
	cfg := Default()
	cfg.RequestTimeout = 90
	cfg.Normalize()
	if cfg.RequestTimeout != 15 {
		t.Fatalf("want 15 (clamped from 90), got %d", cfg.RequestTimeout)
	}
}

func TestRequestTimeoutPreservesValidValue(t *testing.T) {
	cfg := Default()
	cfg.RequestTimeout = 30
	cfg.Normalize()
	if cfg.RequestTimeout != 30 {
		t.Fatalf("want 30, got %d", cfg.RequestTimeout)
	}
}
```

- [ ] **Step 2: Run tests and confirm they fail**

```bash
CGO_ENABLED=0 go test github.com/carroarmato0/nextui-bmo/internal/config
```

Expected: new tests FAIL with "field RequestTimeout not found" or similar.

- [ ] **Step 3: Add the field and normalization**

In `internal/config/config.go`, add to the `Config` struct (after `LogSystemPrompt`):

```go
RequestTimeout int `json:"request_timeout,omitempty"` // seconds; 0 or out of [15,60] → 15
```

Add a helper and the SupportedRequestTimeouts function after the `SupportedProactiveTalkLevels` function:

```go
// SupportedRequestTimeouts returns the cycle values shown in the settings menu.
func SupportedRequestTimeouts() []int {
	return []int{15, 20, 25, 30, 45, 60}
}
```

In `Normalize()`, add after the `LibraryDetail` block:

```go
if c.RequestTimeout < 15 || c.RequestTimeout > 60 {
    c.RequestTimeout = 15
}
```

- [ ] **Step 4: Run tests and confirm they pass**

```bash
CGO_ENABLED=0 go test github.com/carroarmato0/nextui-bmo/internal/config
```

Expected: all pass.

- [ ] **Step 5: Lint**

```bash
golangci-lint run ./internal/config/...
```

- [ ] **Step 6: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): add RequestTimeout field (default 15s, range 15-60s)"
```

---

## Task 3: Add config.CheckOverrides

**Files:**
- Modify: `internal/config/prompts.go`
- Modify: `internal/config/prompts_test.go`

- [ ] **Step 1: Write failing tests**

Add to `internal/config/prompts_test.go`:

```go
func TestCheckOverridesNoOverridesReturnsNil(t *testing.T) {
	errs := CheckOverrides(t.TempDir())
	if len(errs) != 0 {
		t.Fatalf("empty dir: want no errors, got %v", errs)
	}
}

func TestCheckOverridesEmptyPersonaReturnsError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "persona.txt"), []byte("  \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	errs := CheckOverrides(dir)
	if len(errs) == 0 {
		t.Fatal("want error for blank persona.txt, got none")
	}
}

func TestCheckOverridesValidPersonaReturnsNoError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "persona.txt"), []byte("I am BMO"), 0o600); err != nil {
		t.Fatal(err)
	}
	errs := CheckOverrides(dir)
	if len(errs) != 0 {
		t.Fatalf("want no errors for valid persona.txt, got %v", errs)
	}
}

func TestCheckOverridesInvalidSVGReturnsError(t *testing.T) {
	dir := t.TempDir()
	facesDir := filepath.Join(dir, "faces")
	if err := os.MkdirAll(facesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(facesDir, "neutral.svg"), []byte("not xml!!!"), 0o600); err != nil {
		t.Fatal(err)
	}
	errs := CheckOverrides(dir)
	if len(errs) == 0 {
		t.Fatal("want error for invalid SVG, got none")
	}
}

func TestCheckOverridesValidSVGReturnsNoError(t *testing.T) {
	dir := t.TempDir()
	facesDir := filepath.Join(dir, "faces")
	if err := os.MkdirAll(facesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><rect/></svg>`)
	if err := os.WriteFile(filepath.Join(facesDir, "neutral.svg"), svg, 0o600); err != nil {
		t.Fatal(err)
	}
	errs := CheckOverrides(dir)
	if len(errs) != 0 {
		t.Fatalf("want no errors for valid SVG, got %v", errs)
	}
}
```

- [ ] **Step 2: Run tests and confirm they fail**

```bash
CGO_ENABLED=0 go test github.com/carroarmato0/nextui-bmo/internal/config
```

Expected: FAIL — `CheckOverrides` not defined.

- [ ] **Step 3: Implement CheckOverrides**

Add to `internal/config/prompts.go`. Expand the import block to include `bytes`, `encoding/xml`, `fmt`, `io`, `os`, `path/filepath`:

```go
package config

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// PersonaPath, VoicePath, FacesDir, QuotesPath, LoadPromptFile,
// RemoveOverrides — existing functions below...

// CheckOverrides validates every override file that exists on disk.
// A missing file (user has no override) is silently skipped.
// Returns one descriptive error per failing file.
func CheckOverrides(homeDir string) []error {
	var errs []error

	checkText := func(path, label string) {
		data, err := os.ReadFile(path)
		if err != nil {
			return // file absent — no override, no problem
		}
		if strings.TrimSpace(string(data)) == "" {
			errs = append(errs, fmt.Errorf("%s exists but is blank", label))
		}
	}

	checkText(PersonaPath(homeDir), "persona.txt")
	checkText(VoicePath(homeDir), "voice.txt")
	checkText(QuotesPath(homeDir), "quotes.txt")

	// Validate SVG overrides in the faces/ directory.
	entries, err := os.ReadDir(FacesDir(homeDir))
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, fmt.Errorf("faces dir: %w", err))
		}
		return errs
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".svg" {
			continue
		}
		p := filepath.Join(FacesDir(homeDir), e.Name())
		data, err := os.ReadFile(p)
		if err != nil {
			errs = append(errs, fmt.Errorf("faces/%s: %w", e.Name(), err))
			continue
		}
		if !isValidXML(data) {
			errs = append(errs, fmt.Errorf("faces/%s: not valid XML", e.Name()))
		}
	}
	return errs
}

// isValidXML reports whether data parses as well-formed XML.
func isValidXML(data []byte) bool {
	d := xml.NewDecoder(bytes.NewReader(data))
	for {
		_, err := d.Token()
		if err == io.EOF {
			return true
		}
		if err != nil {
			return false
		}
	}
}
```

- [ ] **Step 4: Run tests and confirm they pass**

```bash
CGO_ENABLED=0 go test github.com/carroarmato0/nextui-bmo/internal/config
```

Expected: all pass.

- [ ] **Step 5: Lint**

```bash
golangci-lint run ./internal/config/...
```

- [ ] **Step 6: Commit**

```bash
git add internal/config/prompts.go internal/config/prompts_test.go
git commit -m "feat(config): add CheckOverrides to detect broken mod assets"
```

---

## Task 4: Write the generate-audio tool

**Files:**
- Create: `cmd/generate-audio/main.go`

This tool calls the Chat+TTS API to generate in-character PCM clips and writes them to `internal/clips/assets/audio/`. Run it whenever the voice or model changes. It does NOT embed anything — it just writes files.

- [ ] **Step 1: Create the tool**

```go
// cmd/generate-audio/main.go
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
	{"hello", "Give a single short, excited in-character greeting to the user. One sentence only. Do not use any punctuation that would sound unnatural when spoken aloud."},
	{"mod_error", "Give a short in-character message warning that one of your customisation files seems broken and you have fallen back to your defaults. One or two sentences only."},
	{"timeout", "Give a short in-character apology for not being able to think of an answer right now. Ask the user to try again. One or two sentences only."},
	{"error", "Give a short in-character message saying you cannot reach anyone right now and suggest the user checks the connection. One or two sentences only."},
	{"goodbye", "Give a short, warm in-character farewell to the user. One sentence only."},
	{"sleep", "Give a short in-character message for when you are about to go to sleep. One sentence only."},
	{"wake", "Give a short in-character message for when you have just woken up. One sentence only."},
}

func main() {
	key := flag.String("key", "", "OpenAI API key (overrides .openai_key file and OPENAI_API_KEY env var)")
	baseURL := flag.String("base-url", "https://api.openai.com/v1", "API base URL")
	chatModel := flag.String("chat-model", "gpt-4o-mini", "Chat model for generating clip text")
	ttsModel := flag.String("tts-model", "gpt-4o-mini-tts", "TTS model")
	voice := flag.String("voice", "alloy", "TTS voice")
	instructions := flag.String("instructions", config.DefaultTTSInstructions, "TTS speaking-style instructions")
	outDir := flag.String("out", "internal/clips/assets/audio", "Output directory for PCM files")
	flag.Parse()

	// Key lookup order: -key flag → .openai_key file → OPENAI_API_KEY env var.
	resolvedKey := strings.TrimSpace(*key)
	if resolvedKey == "" {
		if data, err := os.ReadFile(".openai_key"); err == nil {
			resolvedKey = strings.TrimSpace(string(data))
		}
	}
	if resolvedKey == "" {
		resolvedKey = strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	}
	if resolvedKey == "" {
		log.Fatal("API key required: add .openai_key to project root, set OPENAI_API_KEY, or use -key flag")
	}
	key = &resolvedKey

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		log.Fatalf("create output dir: %v", err)
	}

	client := providers.NewOpenAICompatibleClient(providers.Config{
		BaseURL: *baseURL,
		APIKey:  resolvedKey,
	}, http.DefaultClient)

	ctx := context.Background()

	for _, clip := range clipDefs {
		log.Printf("generating %s...", clip.name)

		chatResp, err := client.Reply(ctx, providers.ChatRequest{
			Model:        *chatModel,
			Messages:     []providers.Message{{Role: "user", Content: clip.nudge}},
			SystemPrompt: config.DefaultSystemPrompt,
		})
		if err != nil {
			log.Fatalf("chat for %s: %v", clip.name, err)
		}
		text := strings.TrimSpace(chatResp.Text)
		if text == "" {
			log.Fatalf("empty reply for %s", clip.name)
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
			log.Fatalf("tts for %s: %v", clip.name, err)
		}

		// TTS returns 24kHz mono S16LE; resample to 16kHz mono then upmix to stereo.
		mono16 := audio.ResampleS16LE(speech, 24000, audio.DefaultSampleRate, 1)
		stereo := monoToStereo(mono16)

		outPath := filepath.Join(*outDir, clip.name+".pcm")
		if err := os.WriteFile(outPath, stereo, 0o644); err != nil {
			log.Fatalf("write %s: %v", outPath, err)
		}
		log.Printf("  wrote %d bytes → %s", len(stereo), outPath)
	}
	fmt.Println("done")
}

// monoToStereo duplicates each S16LE sample to produce interleaved stereo.
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
```

- [ ] **Step 2: Verify it compiles**

```bash
CGO_ENABLED=0 go build ./cmd/generate-audio/...
```

Expected: exits 0 with no output.

- [ ] **Step 3: Lint**

```bash
golangci-lint run ./cmd/generate-audio/...
```

- [ ] **Step 4: Commit**

```bash
git add cmd/generate-audio/main.go
git commit -m "feat: add generate-audio tool for pre-recording BMO clip PCMs"
```

---

## Task 5: Generate and commit PCM clips

> **Note:** This task requires a real OpenAI API key. Place your key in `.openai_key` at the project root (already gitignored), or set `OPENAI_API_KEY`, or pass `-key`. The tool picks them up in that order.

- [ ] **Step 1: Add your API key to .openai_key (if not already set via env)**

```bash
echo "sk-..." > .openai_key
```

- [ ] **Step 2: Run the generate-audio tool from the repo root**

```bash
go run ./cmd/generate-audio/...
```

Expected output (order and exact bytes will vary):

```
generating hello...
  text: "..."
  wrote NNNNN bytes → internal/clips/assets/audio/hello.pcm
generating mod_error...
...
done
```

- [ ] **Step 3: Verify all 7 files were created**

```bash
ls -lh internal/clips/assets/audio/
```

Expected: 7 `.pcm` files, each at least 10 kB.

- [ ] **Step 4: Spot-check one clip on device (optional but recommended)**

```bash
adb push internal/clips/assets/audio/hello.pcm /tmp/hello.pcm
adb shell "aplay -D hw:0,0 -f S16_LE -c 2 -r 16000 -t raw /tmp/hello.pcm"
```

Expected: hear BMO saying hello in character.

- [ ] **Step 5: Commit the generated files**

```bash
git add internal/clips/assets/audio/
git commit -m "feat(clips): add generated BMO pre-recorded audio clips (7 clips, 16kHz stereo)"
```

---

## Task 6: Add internal/clips package

**Files:**
- Create: `internal/clips/embed.go`
- Create: `internal/clips/library.go`
- Create: `internal/clips/library_test.go`
- Create: `internal/clips/player.go`
- Create: `internal/clips/player_test.go`

The PCM files must exist (Task 5) before this task — `go:embed` fails at compile time if the glob matches nothing.

- [ ] **Step 1: Create embed.go**

```go
// internal/clips/embed.go
package clips

import "embed"

//go:embed assets/audio/*.pcm
var embedded embed.FS
```

- [ ] **Step 2: Create library.go**

```go
// internal/clips/library.go
package clips

import (
	"os"
	"path/filepath"
)

// Library resolves clip names to raw S16LE PCM bytes. On-disk overrides
// in <homeDir>/audio/ take precedence over the embedded defaults.
// A missing override file is silently skipped. Load returns nil when
// neither source has the clip; callers treat nil as "skip silently."
type Library struct {
	dir string // <homeDir>/audio; may not exist
}

// NewLibrary returns a Library backed by homeDir/audio for overrides.
func NewLibrary(homeDir string) *Library {
	return &Library{dir: filepath.Join(homeDir, "audio")}
}

// Load returns PCM bytes for the named clip. Lookup order:
//  1. <homeDir>/audio/<name>.pcm  (override)
//  2. embedded assets/audio/<name>.pcm
//  3. nil (unknown clip)
func (l *Library) Load(name string) []byte {
	if l != nil && l.dir != "" {
		path := filepath.Join(l.dir, name+".pcm")
		if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
			return data
		}
	}
	data, _ := embedded.ReadFile("assets/audio/" + name + ".pcm")
	return data // nil if not embedded
}
```

- [ ] **Step 3: Write failing library tests**

```go
// internal/clips/library_test.go
package clips

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLibraryLoadEmbeddedHello(t *testing.T) {
	lib := NewLibrary(t.TempDir())
	data := lib.Load("hello")
	if len(data) == 0 {
		t.Fatal("expected embedded hello.pcm, got nil/empty")
	}
}

func TestLibraryLoadUnknownClipReturnsNil(t *testing.T) {
	lib := NewLibrary(t.TempDir())
	if got := lib.Load("does_not_exist"); got != nil {
		t.Fatalf("expected nil for unknown clip, got %d bytes", len(got))
	}
}

func TestLibraryOverridePreferredOverEmbedded(t *testing.T) {
	dir := t.TempDir()
	audioDir := filepath.Join(dir, "audio")
	if err := os.MkdirAll(audioDir, 0o755); err != nil {
		t.Fatal(err)
	}
	override := []byte{0x01, 0x02, 0x03, 0x04}
	if err := os.WriteFile(filepath.Join(audioDir, "hello.pcm"), override, 0o600); err != nil {
		t.Fatal(err)
	}
	lib := NewLibrary(dir)
	got := lib.Load("hello")
	if len(got) != len(override) || got[0] != override[0] {
		t.Fatalf("expected override bytes, got %v", got)
	}
}

func TestLibraryEmptyOverrideFallsBackToEmbedded(t *testing.T) {
	dir := t.TempDir()
	audioDir := filepath.Join(dir, "audio")
	if err := os.MkdirAll(audioDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Write an empty override file — should fall back to embedded.
	if err := os.WriteFile(filepath.Join(audioDir, "hello.pcm"), []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}
	lib := NewLibrary(dir)
	data := lib.Load("hello")
	if len(data) == 0 {
		t.Fatal("expected embedded fallback for empty override, got nil/empty")
	}
}

func TestLibraryNilSafeLoad(t *testing.T) {
	var lib *Library
	if got := lib.Load("hello"); got != nil {
		t.Fatalf("nil Library.Load should return nil, got %d bytes", len(got))
	}
}
```

Update `Load` to handle nil receiver:

```go
func (l *Library) Load(name string) []byte {
	if l == nil {
		return nil
	}
	// ... rest of the function unchanged
```

- [ ] **Step 4: Create player.go**

```go
// internal/clips/player.go
package clips

import (
	"context"
	"time"
)

// AudioWriter is satisfied by *audio.Session and any test double.
type AudioWriter interface {
	WritePCM([]byte) error
}

const (
	chunkMs          = 20   // pacing granularity in milliseconds
	playbackBufferMs = 200  // prefill to match aplay --buffer-time
)

// Player plays pre-recorded PCM clips through an AudioWriter.
// It does not interact with the state machine or face renderer.
type Player struct {
	writer     AudioWriter
	sampleRate int
	channels   int
	lib        *Library
}

// NewPlayer returns a Player backed by the given writer and library.
func NewPlayer(writer AudioWriter, sampleRate, channels int, lib *Library) *Player {
	if sampleRate <= 0 {
		sampleRate = 16000
	}
	if channels <= 0 {
		channels = 2
	}
	return &Player{writer: writer, sampleRate: sampleRate, channels: channels, lib: lib}
}

// Play loads the clip by name and streams it to the audio writer at real-time
// rate. Returns nil immediately if the clip is not found or the writer is nil.
// The context controls cancellation.
func (p *Player) Play(ctx context.Context, name string) error {
	if p == nil || p.writer == nil {
		return nil
	}
	pcm := p.lib.Load(name)
	if len(pcm) == 0 {
		return nil
	}
	return p.playPaced(ctx, pcm)
}

func (p *Player) playPaced(ctx context.Context, pcm []byte) error {
	bytesPerChunk := p.sampleRate * p.channels * 2 * chunkMs / 1000
	if bytesPerChunk <= 0 {
		return p.writer.WritePCM(pcm)
	}
	nChunks := (len(pcm) + bytesPerChunk - 1) / bytesPerChunk
	lead := playbackBufferMs / chunkMs
	chunkDur := time.Duration(chunkMs) * time.Millisecond

	start := time.Now()
	for i := 0; i < nChunks; i++ {
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
	}
	for j := nChunks - lead; j < nChunks; j++ {
		if j < 0 {
			continue
		}
		if err := sleepUntil(ctx, start.Add(time.Duration(j)*chunkDur)); err != nil {
			return err
		}
	}
	return sleepUntil(ctx, start.Add(time.Duration(nChunks)*chunkDur))
}

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
```

- [ ] **Step 5: Write failing player tests**

```go
// internal/clips/player_test.go
package clips

import (
	"context"
	"sync"
	"testing"
)

type fakeWriter struct {
	mu     sync.Mutex
	chunks [][]byte
	err    error
}

func (f *fakeWriter) WritePCM(pcm []byte) error {
	if f.err != nil {
		return f.err
	}
	cp := make([]byte, len(pcm))
	copy(cp, pcm)
	f.mu.Lock()
	f.chunks = append(f.chunks, cp)
	f.mu.Unlock()
	return nil
}

func (f *fakeWriter) totalBytes() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.chunks {
		n += len(c)
	}
	return n
}

func TestPlayerPlayUnknownClipIsNoop(t *testing.T) {
	lib := NewLibrary(t.TempDir())
	w := &fakeWriter{}
	p := NewPlayer(w, 16000, 2, lib)
	if err := p.Play(context.Background(), "does_not_exist"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w.totalBytes() != 0 {
		t.Fatal("expected no writes for unknown clip")
	}
}

func TestPlayerPlayKnownClipWritesPCM(t *testing.T) {
	lib := NewLibrary(t.TempDir())
	w := &fakeWriter{}
	p := NewPlayer(w, 16000, 2, lib)
	if err := p.Play(context.Background(), "hello"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w.totalBytes() == 0 {
		t.Fatal("expected PCM writes for hello clip")
	}
}

func TestPlayerPlayCancelledContextReturnsError(t *testing.T) {
	lib := NewLibrary(t.TempDir())
	w := &fakeWriter{}
	p := NewPlayer(w, 16000, 2, lib)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel
	err := p.Play(ctx, "hello")
	if err == nil {
		t.Fatal("expected error for pre-cancelled context, got nil")
	}
}

func TestNilPlayerPlayIsNoop(t *testing.T) {
	var p *Player
	if err := p.Play(context.Background(), "hello"); err != nil {
		t.Fatalf("nil Player.Play should be no-op, got error: %v", err)
	}
}
```

- [ ] **Step 6: Run tests**

```bash
CGO_ENABLED=0 go test github.com/carroarmato0/nextui-bmo/internal/clips
```

Expected: all pass.

- [ ] **Step 7: Lint**

```bash
golangci-lint run ./internal/clips/...
```

- [ ] **Step 8: Full build check**

```bash
CGO_ENABLED=0 go build ./...
```

- [ ] **Step 9: Commit**

```bash
git add internal/clips/
git commit -m "feat(clips): add Library and Player for pre-recorded BMO audio clips"
```

---

## Task 7: Refactor VoicePipeline — channels split, batch cancel, timeout, clip fallback

**Files:**
- Modify: `internal/assistant/voice.go`
- Modify: `internal/assistant/voice_test.go`

This is the largest task. Work through the changes in order.

- [ ] **Step 1: Rename channels field and update constructor**

In `internal/assistant/voice.go`, rename `channels int` to `captureChannels int` and add `playbackChannels int` in the struct:

```go
type VoicePipeline struct {
	// ... existing fields ...
	sampleRate       int
	captureChannels  int
	playbackChannels int
	// ... rest unchanged ...
}
```

Update `NewVoicePipeline` signature and body (last two params change):

```go
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
		// ... existing fields ...
		sampleRate:       sampleRate,
		captureChannels:  captureChannels,
		playbackChannels: playbackChannels,
	}
}
```

Update all uses of `p.channels` in the file:
- `TranscriptionRequest{..., Channels: p.captureChannels, ...}` (in `ProcessBatch`)
- `audioSeconds(len(pcm), p.sampleRate, p.captureChannels)` (logging)
- `audio.ResampleS16LE(speech, ttsPCMSampleRate, p.sampleRate, p.playbackChannels)` (TTS output)
- `playPaced` — uses `p.sampleRate` and `p.channels`; rename `p.channels` to `p.playbackChannels`
- `rmsChunks(pcm, p.sampleRate, p.channels, speakChunkMs)` → `p.playbackChannels`

- [ ] **Step 2: Add batch cancel and timeout fields and methods**

Add to the `VoicePipeline` struct (after `playDone chan struct{}`):

```go
// batchMu guards batchCancel for the in-flight STT+Chat request.
batchMu     sync.Mutex
batchCancel context.CancelFunc

requestTimeout time.Duration // 0 → defaults to 15s at use time
timeoutClip    []byte        // pre-loaded timeout.pcm at playback rate
errorClip      []byte        // pre-loaded error.pcm at playback rate
```

Add setter methods after `SetLogSystemPrompt`:

```go
// SetRequestTimeout sets the per-batch timeout for the STT+Chat phase.
// Zero means use the 15s default.
func (p *VoicePipeline) SetRequestTimeout(d time.Duration) {
	if p != nil {
		p.requestTimeout = d
	}
}

// SetTimeoutClip sets the pre-recorded PCM played when a request times out.
func (p *VoicePipeline) SetTimeoutClip(pcm []byte) {
	if p != nil {
		p.timeoutClip = pcm
	}
}

// SetErrorClip sets the pre-recorded PCM played on network/API errors.
func (p *VoicePipeline) SetErrorClip(pcm []byte) {
	if p != nil {
		p.errorClip = pcm
	}
}

// CancelBatch cancels the in-flight STT+Chat request. Returns true if
// a batch was in progress.
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
```

- [ ] **Step 3: Replace ProcessBatch error handling with batch context and clip fallback**

Replace the body of `ProcessBatch` from the `stt.Transcribe` call onward. The new structure:

```go
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

	// Per-batch cancellable context with timeout. The timeout covers only
	// the STT+Chat fetch; TTS and playback use the parent ctx.
	timeout := p.requestTimeout
	if timeout <= 0 {
		timeout = 15 * time.Second
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

	if p.logger != nil && p.logSystemPrompt {
		p.logger.Debugf("pipeline system prompt: %q", p.currentSystemPrompt())
	}

	chatStart := time.Now()
	chat, err := p.chat.Reply(batchCtx, providers.ChatRequest{
		Model:        p.chatModel,
		Messages:     []providers.Message{{Role: "user", Content: transcript}},
		SystemPrompt: p.currentSystemPrompt(),
	})
	if err != nil {
		return p.handleBatchError(ctx, batchCtx, err, true)
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

	if p.logger != nil && p.logSystemPrompt {
		p.logger.Debugf("pipeline TTS instructions: %q", p.currentTTSInstructions())
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
		return p.fail(err)
	}
	if p.logger != nil {
		p.logger.Infof("pipeline TTS: %dms (%d bytes) | input: %d chars | total: %dms",
			time.Since(ttsStart).Milliseconds(), len(speech), len(reply),
			time.Since(totalStart).Milliseconds())
	}

	speech = audio.ResampleS16LE(speech, ttsPCMSampleRate, p.sampleRate, p.playbackChannels)
	return p.speak(ctx, speech)
}
```

Add `handleBatchError` and `playFallbackClip` below `ProcessBatch`:

```go
// handleBatchError dispatches STT/Chat errors per the error table in the spec.
// postSTT is true when the error occurred during Chat (STT already succeeded).
func (p *VoicePipeline) handleBatchError(ctx, batchCtx context.Context, err error, postSTT bool) error {
	// Parent context cancelled → app is shutting down; propagate.
	if ctx.Err() != nil {
		return err
	}
	// Batch context manually cancelled (B-press) → silent return to idle.
	if errors.Is(batchCtx.Err(), context.Canceled) {
		if p.machine != nil {
			p.machine.Transition(EventRest)
		}
		return nil
	}
	// Batch context timed out → play timeout clip only if we reached the Chat phase.
	if errors.Is(batchCtx.Err(), context.DeadlineExceeded) {
		if !postSTT {
			if p.machine != nil {
				p.machine.Transition(EventRest)
			}
			return nil
		}
		return p.playFallbackClip(ctx, p.timeoutClip)
	}
	// Quota exhausted → existing sleep behaviour.
	if classifyQuota(p.stt, err) || classifyQuota(p.chat, err) {
		if p.machine != nil {
			p.machine.Transition(EventQuotaExhausted)
		}
		return err
	}
	// Any other error (network, provider) → play error clip and return to idle.
	return p.playFallbackClip(ctx, p.errorClip)
}

// playFallbackClip plays a pre-recorded clip through the existing speak path.
// If pcm is empty the machine transitions directly to idle.
func (p *VoicePipeline) playFallbackClip(ctx context.Context, pcm []byte) error {
	if len(pcm) == 0 {
		if p.machine != nil {
			p.machine.Transition(EventRest)
		}
		return nil
	}
	return p.speak(ctx, pcm)
}
```

- [ ] **Step 4: Write failing tests for new behaviour**

Add to `internal/assistant/voice_test.go`. First, add a blocking provider type after `fakeProvider`:

```go
// blockingProvider blocks its STT/Chat calls until the context is cancelled.
type blockingProvider struct {
	fakeProvider
}

func (b *blockingProvider) Transcribe(ctx context.Context, req providers.TranscriptionRequest) (providers.TranscriptionResult, error) {
	<-ctx.Done()
	return providers.TranscriptionResult{}, ctx.Err()
}

func (b *blockingProvider) Reply(ctx context.Context, req providers.ChatRequest) (providers.ChatResult, error) {
	<-ctx.Done()
	return providers.ChatResult{}, ctx.Err()
}
```

Then add tests:

```go
func TestCancelBatchReturnsFalseWhenIdle(t *testing.T) {
	p := NewVoicePipeline(nil, &fakeWriter{}, &fakeProvider{}, &fakeProvider{}, &fakeProvider{},
		"", "", "", "", "", 16000, 1, 2)
	if p.CancelBatch() {
		t.Fatal("CancelBatch should return false when nothing is in flight")
	}
}

func TestProcessBatchSilentCancelReturnsMachineToIdle(t *testing.T) {
	machine := NewMachine()
	machine.SetMode(ModeAI)
	w := &fakeWriter{}
	bp := &blockingProvider{fakeProvider: fakeProvider{transcript: "hi"}}
	p := NewVoicePipeline(machine, w, bp, bp, &fakeProvider{},
		"m", "m", "m", "v", "sys", 16000, 1, 2)
	p.SetRequestTimeout(30 * time.Second)

	// PCM with signal: 1 second of noise at 16-bit.
	pcm := make([]byte, 32000)
	for i := range pcm {
		pcm[i] = byte(i % 127)
	}

	done := make(chan error, 1)
	go func() {
		done <- p.ProcessBatch(context.Background(), pcm)
	}()

	// Wait until machine is in listening/thinking state, then cancel.
	for i := 0; i < 100; i++ {
		time.Sleep(5 * time.Millisecond)
		if s := machine.State(); s == StateListening || s == StateThinking {
			break
		}
	}
	p.CancelBatch()

	err := <-done
	if err != nil {
		t.Fatalf("expected nil error after B-cancel, got %v", err)
	}
	if got := machine.State(); got != StateIdle {
		t.Fatalf("machine should be idle after cancel, got %v", got)
	}
	if w.totalBytes() > 0 {
		t.Fatal("expected no PCM writes after B-cancel (no fallback clip)")
	}
}

func TestProcessBatchTimeoutDuringChatPlaysFallback(t *testing.T) {
	machine := NewMachine()
	machine.SetMode(ModeAI)
	w := &fakeWriter{}
	// STT returns immediately; Chat blocks.
	stt := &fakeProvider{transcript: "hello"}
	chat := &blockingProvider{}
	p := NewVoicePipeline(machine, w, stt, chat, &fakeProvider{},
		"m", "m", "m", "v", "sys", 16000, 1, 2)
	p.SetRequestTimeout(50 * time.Millisecond)

	// Fake timeout clip: 4 bytes (2 stereo samples).
	timeoutPCM := []byte{0x01, 0x00, 0x01, 0x00}
	p.SetTimeoutClip(timeoutPCM)

	pcm := make([]byte, 32000)
	for i := range pcm {
		pcm[i] = byte(i % 127)
	}

	if err := p.ProcessBatch(context.Background(), pcm); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w.totalBytes() == 0 {
		t.Fatal("expected timeout clip to be played (PCM written)")
	}
	if got := machine.State(); got != StateIdle {
		t.Fatalf("machine should be idle after timeout, got %v", got)
	}
}

func TestProcessBatchNetworkErrorPlaysErrorClip(t *testing.T) {
	machine := NewMachine()
	machine.SetMode(ModeAI)
	w := &fakeWriter{}
	stt := &fakeProvider{err: fmt.Errorf("connection refused")}
	p := NewVoicePipeline(machine, w, stt, &fakeProvider{}, &fakeProvider{},
		"m", "m", "m", "v", "sys", 16000, 1, 2)
	p.SetRequestTimeout(5 * time.Second)

	errorPCM := []byte{0x02, 0x00, 0x02, 0x00}
	p.SetErrorClip(errorPCM)

	pcm := make([]byte, 32000)
	for i := range pcm {
		pcm[i] = byte(i % 127)
	}

	if err := p.ProcessBatch(context.Background(), pcm); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w.totalBytes() == 0 {
		t.Fatal("expected error clip to be played")
	}
	if got := machine.State(); got != StateIdle {
		t.Fatalf("machine should be idle after network error, got %v", got)
	}
}
```

- [ ] **Step 5: Run tests**

```bash
CGO_ENABLED=0 go test github.com/carroarmato0/nextui-bmo/internal/assistant
```

Expected: all pass.

- [ ] **Step 6: Lint**

```bash
golangci-lint run ./internal/assistant/...
```

- [ ] **Step 7: Full build**

```bash
CGO_ENABLED=0 go build ./...
```

Expected: exits 0. (main.go will have compile errors until Task 9 — fix by temporarily updating the NewVoicePipeline call if needed, or do Task 9 before running the full build check here.)

- [ ] **Step 8: Commit**

```bash
git add internal/assistant/voice.go internal/assistant/voice_test.go
git commit -m "feat(assistant): add batch cancel, request timeout, and clip fallback to VoicePipeline"
```

---

## Task 8: Add Timeout item to settings menu

**Files:**
- Modify: `internal/ui/settings_menu.go`
- Modify: `internal/ui/settings_menu_test.go`

The new `request_timeout` item is inserted at index 13. Items previously at 13 and 14 shift to 14 and 15. Total item count: 16.

- [ ] **Step 1: Update the failing test expectations first**

In `internal/ui/settings_menu_test.go`, update:

```go
func TestSettingsMenuHas16Items(t *testing.T) {  // renamed from Has15Items
	m := NewSettingsMenu(config.Default())
	if got := len(m.Overlay().Items); got != 16 {
		t.Fatalf("expected 16 overlay items, got %d", got)
	}
}

func TestSettingsMenuItemCodes(t *testing.T) {
	m := NewSettingsMenu(config.Default())
	items := m.Overlay().Items
	want := []string{
		"log_level", "log_system_prompt", "mode",
		"stt_status", "chat_status", "tts_status", "voice_status",
		"aware_library", "aware_saves", "aware_playlog",
		"aware_system", "aware_achievements",
		"library_detail", "request_timeout", "proactive_talk", "restore_defaults",
	}
	for i, code := range want {
		if got := items[i].Code; got != code {
			t.Errorf("items[%d].Code = %q, want %q", i, got, code)
		}
	}
}
```

Also update existing navigation tests that assumed 15 items to use 16 where the loop count matters:

```go
func TestSettingsMenuLogSystemPromptNotNavigableOutsideDebug(t *testing.T) {
	cfg := config.Default()
	m := NewSettingsMenu(cfg)
	for range 16 {  // was 15
		m.Move(1)
		if m.Overlay().Items[1].Focused {
			t.Fatal("log_system_prompt item was focused while log level is not debug")
		}
	}
}
```

Add a new test for the timeout cycle:

```go
func TestSettingsMenuTimeoutCycles(t *testing.T) {
	m := NewSettingsMenu(config.Default())
	// Focus on request_timeout (index 13).
	// From start (focus=0), need to move down past 0→2→7→8→9→10→11→12→13.
	for _, delta := range []int{1, 1, 1, 1, 1, 1, 1, 1} {
		m.Move(delta)
	}
	if got := m.Overlay().Items[13].Code; got != "request_timeout" {
		t.Fatalf("expected request_timeout at focus after navigation, got %q", got)
	}
	initial := m.Config().RequestTimeout
	if err := m.ToggleFocused(); err != nil {
		t.Fatalf("ToggleFocused: %v", err)
	}
	if m.Config().RequestTimeout == initial {
		t.Fatal("timeout should have changed after toggle")
	}
}
```

- [ ] **Step 2: Run tests and confirm they fail**

```bash
CGO_ENABLED=0 go test github.com/carroarmato0/nextui-bmo/internal/ui
```

Expected: `TestSettingsMenuHas16Items` fails with "expected 16, got 15"; `TestSettingsMenuItemCodes` fails at index 13.

- [ ] **Step 3: Update settings_menu.go**

In `internal/ui/settings_menu.go`:

Update the `Move` constant from 15 to 16:

```go
func (m *SettingsMenu) Move(delta int) {
	if m == nil {
		return
	}
	const count = 16
	// ... rest unchanged ...
}
```

Update `ToggleFocused` — shift cases 13 and 14, insert new case 13:

```go
case 13: // request_timeout
	timeouts := config.SupportedRequestTimeouts()
	curr := m.cfg.RequestTimeout
	next := timeouts[0]
	for i, t := range timeouts {
		if t == curr {
			next = timeouts[(i+1)%len(timeouts)]
			break
		}
	}
	m.cfg.RequestTimeout = next
case 14: // proactive_talk (was 13)
	levels := config.SupportedProactiveTalkLevels()
	curr := strings.ToLower(strings.TrimSpace(m.cfg.ProactiveTalk))
	next := levels[0]
	for i, l := range levels {
		if l == curr {
			next = levels[(i+1)%len(levels)]
			break
		}
	}
	m.cfg.ProactiveTalk = next
case 15: // restore_defaults (was 14)
	if m.onRestore != nil {
		return m.onRestore()
	}
```

Update `Overlay()` — add the new item and shift focus indices:

```go
{Code: "library_detail", Label: "LIBRARY DETAIL: " + strings.ToUpper(m.cfg.LibraryDetail),
    Selected: true, Focused: m.focus == 12},
{Code: "request_timeout", Label: fmt.Sprintf("TIMEOUT: %ds", m.cfg.RequestTimeout),
    Selected: true, Focused: m.focus == 13},
{Code: "proactive_talk", Label: "PROACTIVE TALK: " + strings.ToUpper(m.cfg.ProactiveTalk),
    Selected: true, Focused: m.focus == 14},
{Code: "restore_defaults", Label: "RESTORE DEFAULTS", Focused: m.focus == 15},
```

- [ ] **Step 4: Run tests**

```bash
CGO_ENABLED=0 go test github.com/carroarmato0/nextui-bmo/internal/ui
```

Expected: all pass.

- [ ] **Step 5: Lint**

```bash
golangci-lint run ./internal/ui/...
```

- [ ] **Step 6: Commit**

```bash
git add internal/ui/settings_menu.go internal/ui/settings_menu_test.go
git commit -m "feat(ui): add request timeout setting to settings menu (16 items)"
```

---

## Task 9: Wire everything in main.go

**Files:**
- Modify: `cmd/bmo-pak/main.go`

This task connects all the new components: always-on audio session for clips, clip player, hello/mod_error/goodbye wiring, B-button batch cancel, and stereo playback channels.

- [ ] **Step 1: Add imports**

Add to the import block in `cmd/bmo-pak/main.go`:

```go
"github.com/carroarmato0/nextui-bmo/internal/clips"
```

- [ ] **Step 2: Always create the audio session (move outside cfg.UsesAI block)**

Replace the current audio setup block. The audio session must be created unconditionally so clips always have a writer. The CaptureRouter is still AI-only.

Find and replace the existing block starting with `var audioSession *audio.Session`:

```go
// Audio session is always started for pre-recorded clip playback.
// In idle mode the capture side runs silently (no CaptureRouter consumer).
audioCfg := audio.DefaultConfig(hardwareProfile)
audioSession := audio.NewSession(audioCfg)
if err := audioSession.Start(); err != nil {
    logger.Warnf("audio session unavailable, clips disabled: %v", err)
    audioSession = nil
} else {
    defer audioSession.Close()
    logger.Infof("audio session ready: %s", audioCfg.Summary())
}

// Clip library and player — always available when audio session is up.
var clipPlayer *clips.Player
if audioSession != nil {
    clipLib := clips.NewLibrary(homeDir)
    clipPlayer = clips.NewPlayer(audioSession, audioCfg.SampleRate, audioCfg.PlaybackChannels, clipLib)
}

var audioRouter *audio.CaptureRouter
var audioPipeline *assistant.VoicePipeline
var stopPTT func()
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

if cfg.UsesAI() && audioSession != nil {
    audioRouter = audio.NewCaptureRouter(audioSession,
        audio.BytesPerSecond(audioCfg.SampleRate, audioCfg.Channels, audio.BytesPerSampleS16LE)/2)
    if err := audioRouter.Start(); err != nil {
        logger.Warnf("capture router unavailable: %v", err)
        audioRouter = nil
    } else {
        defer audioRouter.Close()

        sttClient := providers.NewOpenAICompatibleClient(providers.Config{BaseURL: cfg.STT.BaseURL, APIKey: cfg.STT.APIKey}, http.DefaultClient)
        chatClient := providers.NewOpenAICompatibleClient(providers.Config{BaseURL: cfg.Chat.BaseURL, APIKey: cfg.Chat.APIKey}, http.DefaultClient)
        ttsClient := providers.NewOpenAICompatibleClient(providers.Config{BaseURL: cfg.TTS.BaseURL, APIKey: cfg.TTS.APIKey}, http.DefaultClient)
        audioPipeline = assistant.NewVoicePipeline(machine, audioRouter, sttClient, chatClient, ttsClient,
            cfg.STT.Model, cfg.Chat.Model, cfg.TTS.Model, cfg.TTS.Voice, personaPrompt,
            audioCfg.SampleRate, audioCfg.Channels, audioCfg.PlaybackChannels)
        audioPipeline.SetLogger(logger)
        audioPipeline.SetTTSInstructions(voicePrompt)
        audioPipeline.SetTTSInstructionsSource(func() string {
            return config.LoadPromptFile(voicePath, config.DefaultTTSInstructions)
        })
        audioPipeline.SetSystemPromptSource(func() string {
            return systemPromptWithContext(
                config.LoadPromptFile(personaPath, config.DefaultSystemPrompt),
                deviceCtx.Snapshot(),
                memory.PromptBlock(time.Now().UTC()),
            )
        })
        audioPipeline.SetLogSystemPrompt(cfg.LogSystemPrompt)
        audioPipeline.SetRequestTimeout(time.Duration(cfg.RequestTimeout) * time.Second)
        if clipPlayer != nil {
            lib := clips.NewLibrary(homeDir)
            audioPipeline.SetTimeoutClip(lib.Load("timeout"))
            audioPipeline.SetErrorClip(lib.Load("error"))
        }
        stopPTT = startPushToTalk(ctx, logger, machine, cfg, hardwareProfile, audioRouter, audioPipeline,
            audioCfg.SampleRate, audioCfg.Channels, func() bool { return activeMenu != nil })
    }
}
if stopPTT != nil {
    defer stopPTT()
}
```

- [ ] **Step 3: Add CancelBatch to handleNav**

In the `handleNav` function, insert `CancelBatch` before `InterruptSpeech`:

```go
if action == input.NavCancel {
    if activeMenu != nil {
        setActiveMenu(nil)
        return
    }
    // Cancel an in-flight AI request (B-press during thinking).
    if audioPipeline != nil && audioPipeline.CancelBatch() {
        logger.Infof("AI request cancelled by B press")
        return
    }
    if audioPipeline != nil && audioPipeline.InterruptSpeech() {
        logger.Infof("speech interrupted by B press")
        return
    }
    running = false
    return
}
```

- [ ] **Step 4: Update commitMenu to call SetRequestTimeout**

In `commitMenu`, after `audioPipeline.SetLogSystemPrompt(cfg.LogSystemPrompt)`, add:

```go
audioPipeline.SetRequestTimeout(time.Duration(cfg.RequestTimeout) * time.Second)
```

- [ ] **Step 5: Play hello and mod_error clips before the face loop**

Insert just before `for running {`:

```go
// Play startup clips before entering the main loop.
if clipPlayer != nil {
    startupCtx, startupCancel := context.WithTimeout(ctx, 10*time.Second)
    _ = clipPlayer.Play(startupCtx, "hello")
    overrideErrs := config.CheckOverrides(homeDir)
    for _, e := range overrideErrs {
        logger.Warnf("mod override: %v", e)
    }
    if len(overrideErrs) > 0 {
        _ = clipPlayer.Play(startupCtx, "mod_error")
    }
    startupCancel()
}
```

- [ ] **Step 6: Play goodbye clip after the face loop**

Replace the existing:

```go
logger.Infof("BMO shutting down")
return nil
```

With:

```go
logger.Infof("BMO shutting down")
if clipPlayer != nil {
    goodbyeCtx, goodbyeCancel := context.WithTimeout(context.Background(), 5*time.Second)
    _ = clipPlayer.Play(goodbyeCtx, "goodbye")
    goodbyeCancel()
}
return nil
```

- [ ] **Step 7: Build**

```bash
CGO_ENABLED=0 go build ./...
```

Expected: exits 0.

- [ ] **Step 8: Run all tests**

```bash
CGO_ENABLED=0 go test ./...
```

Expected: all pass.

- [ ] **Step 9: Lint**

```bash
golangci-lint run ./...
```

Expected: no new findings.

- [ ] **Step 10: Commit**

```bash
git add cmd/bmo-pak/main.go
git commit -m "feat(main): wire clip player, stereo playback, hello/goodbye/mod_error clips, batch cancel and timeout"
```

---

## Self-Review

**Spec coverage check:**

| Spec section | Task |
|---|---|
| 7 clips with names and triggers | Tasks 4, 5, 6, 9 |
| 16kHz stereo S16LE storage | Task 4 (monoToStereo) |
| go:embed in internal/clips | Task 6 |
| Override: homeDir/audio/*.pcm | Task 6 (Library) |
| generate-audio tool with flags | Task 4 |
| Chat nudge → TTS → resample → upmix | Task 4 |
| CheckOverrides validates persona/voice/quotes/faces | Task 3 |
| config.RequestTimeout, clamp 15-60 | Task 2 |
| SupportedRequestTimeouts() | Task 2 |
| Settings menu Timeout item at idx 13, 16 total | Task 8 |
| VoicePipeline: captureChannels / playbackChannels | Task 7 |
| VoicePipeline: CancelBatch(), SetRequestTimeout(), SetTimeoutClip(), SetErrorClip() | Task 7 |
| ProcessBatch: batch context with timeout | Task 7 |
| Error dispatch table (cancel/timeout/quota/other) | Task 7 |
| B-button cancel in handleNav | Task 9 |
| Always-on audio session for clips | Task 9 |
| clipPlayer.Play("hello") at startup | Task 9 |
| clipPlayer.Play("mod_error") after bad overrides | Task 9 |
| clipPlayer.Play("goodbye") before deferred cleanup | Task 9 |
| audio.Config PlaybackChannels split | Task 1 |
| sleep/wake clips generated but not yet wired | Tasks 4, 5 |
