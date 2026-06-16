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
			{80, 78, dark[0], dark[1], dark[2], "left eye"},
			{199, 78, dark[0], dark[1], dark[2], "right eye"},
			{0, 0, body[0], body[1], body[2], "top-left corner"},
			{140, 105, screen[0], screen[1], screen[2], "screen center"},
			// (13,11) is inside screen bounds but outside the screen's top-left
			// corner arc (rx=ry=12, arc centre at (24,22), distance ≈15.6 > 12)
			// → must be body not screen.
			{13, 11, body[0], body[1], body[2], "screen top-left corner cut by rx/ry"},
		},
		ExprBlink: {
			{80, 78, dark[0], dark[1], dark[2], "left blink eye"},
			{199, 78, dark[0], dark[1], dark[2], "right blink eye"},
		},
		ExprSleeping: {
			{80, 78, dark[0], dark[1], dark[2], "left flat eye"},
			{199, 78, dark[0], dark[1], dark[2], "right flat eye"},
		},
		ExprListening: {
			{80, 78, dark[0], dark[1], dark[2], "left tall eye"},
			{199, 78, dark[0], dark[1], dark[2], "right tall eye"},
			{140, 122, 0x1a, 0x78, 0x48, "small mouth interior"},
		},
		ExprThinking: {
			{80, 78, dark[0], dark[1], dark[2], "left dot eye"},
			{199, 74, dark[0], dark[1], dark[2], "right dot eye (raised)"},
			{199, 56, dark[0], dark[1], dark[2], "raised brow"},
		},
		ExprConcerned: {
			{80, 82, dark[0], dark[1], dark[2], "left lowered eye"},
			{199, 82, dark[0], dark[1], dark[2], "right lowered eye"},
			{140, 104, dark[0], dark[1], dark[2], "frown apex"},
			{80, 62, dark[0], dark[1], dark[2], "left worried brow"},
		},
		ExprSmile: {
			{80, 77, dark[0], dark[1], dark[2], "left arc eye"},
			{199, 77, dark[0], dark[1], dark[2], "right arc eye"},
			{140, 106, 0xe4, 0xe4, 0xe4, "teeth"},
			{140, 130, 0x1a, 0x78, 0x48, "mouth interior"},
			{140, 141, 0x16, 0xae, 0x81, "tongue"},
		},
	}
	for _, size := range [][2]int{{1024, 768}, {1280, 720}} {
		w, h := size[0], size[1]
		for name, points := range cases {
			data, ok := defaultBytes(name)
			if !ok {
				t.Fatalf("no embedded SVG for %q", name)
			}
			buf, err := Rasterize(renderRestSVG(data), w, h)
			if err != nil {
				t.Fatalf("rasterize %s at %dx%d: %v", name, w, h, err)
			}
			for _, p := range points {
				assertColor(t, buf, w, h, p.x, p.y, p.r, p.g, p.b, name+": "+p.label)
			}
		}
	}
}

func TestNewFacesGeometry(t *testing.T) {
	d := [3]uint8{0x1a, 0x1a, 0x1a}  // features
	mi := [3]uint8{0x1a, 0x78, 0x48} // mouth interior
	gd := [3]uint8{0xf4, 0xc5, 0x31} // star/sparkle gold
	rd := [3]uint8{0xe8, 0x44, 0x3b} // heart/tongue red
	th := [3]uint8{0xe4, 0xe4, 0xe4} // teeth

	type point struct {
		x, y  float64
		c     [3]uint8
		label string
	}
	cases := map[string][]point{
		ExprSad:       {{80, 78, d, "left dot eye"}, {199, 78, d, "right dot eye"}},
		ExprHappy:     {{80, 78, d, "left dot eye"}, {199, 78, d, "right dot eye"}},
		ExprContent:   {{80, 80, d, "left arc eye"}, {199, 80, d, "right arc eye"}},
		ExprAngry:     {{80, 78, d, "left dot eye"}, {199, 78, d, "right dot eye"}},
		ExprSurprised: {{80, 78, d, "left big eye"}, {199, 78, d, "right big eye"}, {140, 121, mi, "mouth interior"}},
		ExprExcited:   {{80, 78, gd, "left star eye"}, {199, 78, gd, "right star eye"}},
		ExprLove:      {{80, 80, rd, "left heart eye"}, {199, 80, rd, "right heart eye"}},
		ExprShy:       {{80, 78, d, "left dot eye"}, {199, 78, d, "right dot eye"}},
		ExprCrying:    {{80, 76, d, "left arc eye"}, {199, 76, d, "right arc eye"}, {140, 127, mi, "mouth interior"}},
		ExprTeary:     {{80, 78, d, "left big eye"}, {199, 78, d, "right big eye"}},
		ExprGloomy:    {{80, 73, d, "left arc eye"}, {199, 73, d, "right arc eye"}},
		ExprDizzy:     {{89, 76, d, "left spiral"}, {208, 76, d, "right spiral"}},
		ExprUnamused:  {{80, 81, d, "left eye dot"}, {199, 81, d, "right eye dot"}, {140, 118, d, "flat mouth"}},
		ExprAnnoyed:   {{80, 78, d, "left dash eye"}, {199, 78, d, "right dash eye"}, {140, 118, d, "dash mouth"}},
		ExprSkeptical: {{80, 78, d, "left dot eye"}, {199, 81, d, "right eye dot"}},
		ExprPlayful:   {{80, 78, d, "left dot eye"}, {140, 129, rd, "tongue"}},
		ExprKiss:      {{88, 78, d, "left > eye"}, {194, 78, d, "right < eye"}},
		ExprGrimace:   {{80, 78, d, "left dash eye"}, {199, 78, d, "right dash eye"}, {118, 116, th, "teeth"}},
		ExprShout:     {{80, 78, d, "left dot eye"}, {199, 78, d, "right dot eye"}, {140, 130, mi, "mouth interior"}},
		ExprDead:      {{80, 78, d, "left x eye"}, {199, 78, d, "right x eye"}, {140, 118, d, "flat mouth"}},
		ExprGlitch:    {{76, 74, d, "left pixel eye"}, {195, 74, d, "right pixel eye"}, {144, 131, d, "pixel mouth"}},
		ExprDismayed:  {{80, 78, d, "left big eye"}, {199, 78, d, "right big eye"}, {140, 124, mi, "gasp mouth interior"}},
		ExprAdoring:   {{80, 78, d, "left big eye"}, {199, 78, d, "right big eye"}},
		ExprSparkle:   {{80, 78, gd, "left sparkle eye"}, {199, 78, gd, "right sparkle eye"}},
	}
	for _, size := range [][2]int{{1024, 768}, {1280, 720}} {
		w, h := size[0], size[1]
		for name, points := range cases {
			data, ok := defaultBytes(name)
			if !ok {
				t.Fatalf("no embedded SVG for %q", name)
			}
			buf, err := Rasterize(renderRestSVG(data), w, h)
			if err != nil {
				t.Fatalf("rasterize %s at %dx%d: %v", name, w, h, err)
			}
			for _, p := range points {
				assertColor(t, buf, w, h, p.x, p.y, p.c[0], p.c[1], p.c[2], name+": "+p.label)
			}
		}
	}
}
