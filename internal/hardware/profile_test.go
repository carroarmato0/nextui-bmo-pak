package hardware

import "testing"

func TestDetectPlatformFromMetadata(t *testing.T) {
	tests := []struct {
		name   string
		hint   string
		model  string
		compat string
		want   string
	}{
		{name: "hint wins", hint: "tg5050", model: "TrimUI Brick", compat: "allwinner,a133", want: "tg5050"},
		{name: "smart pro maps to tg5040", model: "TrimUI Smart Pro", compat: "allwinner,a133", want: "tg5040"},
		{name: "brick maps to tg5040", model: "TrimUI Brick", compat: "allwinner,a133", want: "tg5040"},
		{name: "tg5050 via compatible", model: "unknown", compat: "trimui,tg5050", want: "tg5050"},
		{name: "fallback default", model: "unknown", compat: "unknown", want: DefaultPlatform},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DetectPlatformFromMetadata(tt.hint, tt.model, tt.compat); got != tt.want {
				t.Fatalf("DetectPlatformFromMetadata() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseTrimUIInputDevices(t *testing.T) {
	data := `I: Bus=0003 Vendor=0000 Product=0000 Version=0000
N: Name="TRIMUI Player1"
P: Phys=usb-0000:00:14.0-1/input0
S: Sysfs=/devices/platform/soc/1c6a000.hci1/usb1/1-1/1-1:1.0/0003:0000:0000.0001/input/input3
U: Uniq=
H: Handlers=sysrq kbd event3 js0 
B: PROP=0
`
	event, joy := parseTrimUIInputDevices(data)
	if event != "/dev/input/event3" {
		t.Fatalf("event = %q, want /dev/input/event3", event)
	}
	if joy != "/dev/input/js0" {
		t.Fatalf("joystick = %q, want /dev/input/js0", joy)
	}
}

func TestParseFramebufferSize(t *testing.T) {
	w, h := parseFramebufferSize("1024,768", 640, 480)
	if w != 1024 || h != 768 {
		t.Fatalf("parseFramebufferSize() = %dx%d, want 1024x768", w, h)
	}

	w, h = parseFramebufferSize("bad", 640, 480)
	if w != 640 || h != 480 {
		t.Fatalf("parseFramebufferSize(bad) = %dx%d, want 640x480", w, h)
	}
}
