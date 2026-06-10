package hardware

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	DefaultPlatform = "tg5040"

	DefaultFramebufferPath = "/dev/fb0"
	DefaultInputEventPath  = "/dev/input/event3"
	DefaultInputJoyPath    = "/dev/input/js0"
	DefaultAudioCapture    = "/dev/snd/pcmC0D0c"
	DefaultAudioPlayback   = "/dev/snd/pcmC0D0p"
)

type Profile struct {
	Platform string

	DeviceTreeModel      string
	DeviceTreeCompatible string

	FramebufferPath string
	DisplayWidth    int32
	DisplayHeight   int32
	BitsPerPixel    int
	Stride          int

	InputEvent    string
	InputJoystick string
	AudioCapture  string
	AudioPlayback string
	AudioALSAName string
}

func Detect(platformHint string) Profile {
	model := readText("/proc/device-tree/model")
	compatible := readText("/proc/device-tree/compatible")
	platform := DetectPlatformFromMetadata(platformHint, model, compatible)

	width, height := parseFramebufferSize(readText("/sys/class/graphics/fb0/virtual_size"), 1024, 768)
	bits := readInt("/sys/class/graphics/fb0/bits_per_pixel", 32)
	stride := readInt("/sys/class/graphics/fb0/stride", int(width*4))
	inputEvent, inputJoy := parseTrimUIInputDevices(readText("/proc/bus/input/devices"))
	if inputEvent == "" {
		inputEvent = DefaultInputEventPath
	}
	if inputJoy == "" {
		inputJoy = DefaultInputJoyPath
	}

	return Profile{
		Platform:             platform,
		DeviceTreeModel:      model,
		DeviceTreeCompatible: compatible,
		FramebufferPath:      DefaultFramebufferPath,
		DisplayWidth:         width,
		DisplayHeight:        height,
		BitsPerPixel:         bits,
		Stride:               stride,
		InputEvent:           inputEvent,
		InputJoystick:        inputJoy,
		AudioCapture:         DefaultAudioCapture,
		AudioPlayback:        DefaultAudioPlayback,
		AudioALSAName:        "hw:0,0",
	}
}

func DetectPlatformFromMetadata(platformHint, model, compatible string) string {
	if hint := strings.ToLower(strings.TrimSpace(platformHint)); hint != "" {
		return hint
	}

	haystack := strings.ToUpper(strings.TrimSpace(model) + " " + strings.TrimSpace(compatible))
	switch {
	case strings.Contains(haystack, "TG5050"), strings.Contains(haystack, "SMART PRO S"):
		return "tg5050"
	case strings.Contains(haystack, "TG5040"), strings.Contains(haystack, "BRICK"), strings.Contains(haystack, "SMART PRO"):
		return "tg5040"
	default:
		return DefaultPlatform
	}
}

func (p Profile) Summary() string {
	return fmt.Sprintf(
		"platform=%s fb=%s %dx%d@%dbpp stride=%d input=%s/%s audio=%s/%s alsa=%s",
		p.Platform,
		p.FramebufferPath,
		p.DisplayWidth,
		p.DisplayHeight,
		p.BitsPerPixel,
		p.Stride,
		p.InputEvent,
		p.InputJoystick,
		p.AudioCapture,
		p.AudioPlayback,
		p.AudioALSAName,
	)
}

func (p Profile) FramebufferAvailable() bool {
	return deviceExists(p.FramebufferPath)
}

func (p Profile) InputAvailable() bool {
	return deviceExists(p.InputEvent) || deviceExists(p.InputJoystick)
}

func (p Profile) AudioAvailable() bool {
	return deviceExists(p.AudioCapture) && deviceExists(p.AudioPlayback)
}

func parseTrimUIInputDevices(data string) (eventNode, joystickNode string) {
	lines := strings.Split(data, "\n")
	matched := false
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "N: Name=") {
			name := strings.Trim(line[len("N: Name="):], "\"")
			matched = strings.EqualFold(strings.TrimSpace(name), "TRIMUI Player1")
			continue
		}
		if !matched || !strings.HasPrefix(line, "H: Handlers=") {
			continue
		}
		for _, handler := range strings.Fields(strings.TrimSpace(line[len("H: Handlers="):])) {
			switch {
			case strings.HasPrefix(handler, "event") && eventNode == "":
				eventNode = "/dev/input/" + handler
			case strings.HasPrefix(handler, "js") && joystickNode == "":
				joystickNode = "/dev/input/" + handler
			}
		}
		if eventNode != "" && joystickNode != "" {
			return eventNode, joystickNode
		}
	}
	return eventNode, joystickNode
}

func parseFramebufferSize(raw string, fallbackW, fallbackH int32) (int32, int32) {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "\x00", ""))
	if raw == "" {
		return fallbackW, fallbackH
	}
	parts := strings.Split(raw, ",")
	if len(parts) != 2 {
		return fallbackW, fallbackH
	}
	w, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return fallbackW, fallbackH
	}
	h, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return fallbackW, fallbackH
	}
	if w <= 0 || h <= 0 {
		return fallbackW, fallbackH
	}
	return int32(w), int32(h)
}

func readText(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.ReplaceAll(string(data), "\x00", ""))
}

func readInt(path string, fallback int) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return fallback
	}
	value, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return fallback
	}
	return value
}

func deviceExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
