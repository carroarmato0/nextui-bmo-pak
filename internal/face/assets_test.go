package face

import "testing"

func TestEmbeddedFacesGeometry(t *testing.T) {
	dark := [3]uint8{0x1a, 0x1a, 0x1a}
	screen := [3]uint8{0x90, 0xe5, 0xc8}
	body := [3]uint8{0x4e, 0xcb, 0xa8}

	type point struct {
		x, y    float64
		r, g, b uint8
		label   string
	}
	cases := map[string][]point{
		ExprNeutral: {
			{63, 78, dark[0], dark[1], dark[2], "left eye"},
			{182, 78, dark[0], dark[1], dark[2], "right eye"},
			{0, 0, body[0], body[1], body[2], "top-left corner"},
			{123, 105, screen[0], screen[1], screen[2], "screen center"},
		},
		ExprBlink: {
			{63, 78, dark[0], dark[1], dark[2], "left blink eye"},
			{182, 78, dark[0], dark[1], dark[2], "right blink eye"},
		},
		ExprSleeping: {
			{63, 78, dark[0], dark[1], dark[2], "left flat eye"},
			{182, 78, dark[0], dark[1], dark[2], "right flat eye"},
		},
		ExprListening: {
			{63, 78, dark[0], dark[1], dark[2], "left tall eye"},
			{182, 78, dark[0], dark[1], dark[2], "right tall eye"},
			{123, 122, 0x1a, 0x78, 0x48, "small mouth interior"},
		},
		ExprThinking: {
			{63, 78, dark[0], dark[1], dark[2], "left dot eye"},
			{182, 74, dark[0], dark[1], dark[2], "right dot eye (raised)"},
			{182, 56, dark[0], dark[1], dark[2], "raised brow"},
		},
		ExprConcerned: {
			{63, 82, dark[0], dark[1], dark[2], "left lowered eye"},
			{182, 82, dark[0], dark[1], dark[2], "right lowered eye"},
			{123, 104, dark[0], dark[1], dark[2], "frown apex"},
			{63, 62, dark[0], dark[1], dark[2], "left worried brow"},
		},
		ExprSmile: {
			{63, 77, dark[0], dark[1], dark[2], "left arc eye"},
			{182, 77, dark[0], dark[1], dark[2], "right arc eye"},
			{123, 106, 0xe4, 0xe4, 0xe4, "teeth"},
			{123, 130, 0x1a, 0x78, 0x48, "mouth interior"},
			{123, 141, 0x16, 0xae, 0x81, "tongue"},
		},
	}
	for _, size := range [][2]int{{1024, 768}, {1280, 720}} {
		w, h := size[0], size[1]
		for name, points := range cases {
			data, ok := defaultBytes(name)
			if !ok {
				t.Fatalf("no embedded SVG for %q", name)
			}
			buf, err := Rasterize(data, w, h)
			if err != nil {
				t.Fatalf("rasterize %s at %dx%d: %v", name, w, h, err)
			}
			for _, p := range points {
				assertColor(t, buf, w, h, p.x, p.y, p.r, p.g, p.b, name+": "+p.label)
			}
		}
	}
}
