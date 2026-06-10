//go:build !cgo

package renderer

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

type FrameState struct {
	Expression      string
	Now             time.Time
	QuotaExhausted  bool
	SleepUntil      time.Time
	IdlePhase       float64
	ReducedMotion   bool
	Speaking        bool
	Listening       bool
	Thinking        bool
	LastInteraction time.Time
	Overlay         *OverlayState
}

type OverlayState struct {
	Visible  bool
	Title    string
	Subtitle []string
	Items    []OverlayItem
	Footer   string
}

type OverlayItem struct {
	Code     string
	Label    string
	Selected bool
	Focused  bool
}

type Layout struct {
	W, H         int32
	Margin       int32
	ScreenInset  int32
	EyeY         int32
	EyeW         int32
	EyeH         int32
	EyeGap       int32
	PupilW       int32
	PupilH       int32
	MouthY       int32
	MouthW       int32
	MouthH       int32
	BrowY        int32
	BrowW        int32
	BrowH        int32
	ClockSize    int32
	ClockInset   int32
	CornerRadius int32
	GlowInset    int32
	MouthOpenH   int32
	MouthLineW   int32
}

func LayoutFor(w, h int32) Layout {
	short := w
	if h < short {
		short = h
	}
	margin := clampInt32(short/18, 12, 48)
	screenInset := clampInt32(short/30, 8, 36)
	eyeW := clampInt32(w/5, 60, 190)
	eyeH := clampInt32(h/4, 50, 170)
	eyeGap := clampInt32(w/14, 18, 110)
	pupilW := clampInt32(eyeW/3, 18, eyeW/2)
	pupilH := clampInt32(eyeH/3, 18, eyeH/2)
	mouthW := clampInt32(w/4, 80, 240)
	mouthH := clampInt32(h/8, 28, 130)
	clockSize := clampInt32(short/14, 22, 48)
	cornerRadius := clampInt32(short/12, 18, 72)
	return Layout{
		W:            w,
		H:            h,
		Margin:       margin,
		ScreenInset:  screenInset,
		EyeY:         h * 38 / 100,
		EyeW:         eyeW,
		EyeH:         eyeH,
		EyeGap:       eyeGap,
		PupilW:       pupilW,
		PupilH:       pupilH,
		MouthY:       h * 67 / 100,
		MouthW:       mouthW,
		MouthH:       mouthH,
		BrowY:        h * 25 / 100,
		BrowW:        clampInt32(w/6, 40, 160),
		BrowH:        clampInt32(h/36, 3, 16),
		ClockSize:    clockSize,
		ClockInset:   clampInt32(margin/3, 4, 12),
		CornerRadius: cornerRadius,
		GlowInset:    clampInt32(short/45, 4, 18),
		MouthOpenH:   clampInt32(h/6, 24, 120),
		MouthLineW:   clampInt32(w/18, 16, 80),
	}
}

type Renderer struct {
	fb      *os.File
	fmt     fbFormat
	pixels  []uint32
	raw     []byte
	W       int32
	H       int32
	stride  int
	fbPath  string
	lastErr error
}

type rgba struct {
	R, G, B, A uint8
}

type fbFormat struct {
	BitsPerPixel int
	RedOffset    int
	RedLength    int
	GreenOffset  int
	GreenLength  int
	BlueOffset   int
	BlueLength   int
	AlphaOffset  int
	AlphaLength  int
}

func NewFullscreen(title string) (*Renderer, error) {
	_ = title
	path := strings.TrimSpace(os.Getenv("BMO_FRAMEBUFFER"))
	if path == "" {
		path = "/dev/fb0"
	}
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("open framebuffer %s: %w", path, err)
	}

	info, err := fbScreenInfo(f)
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("read framebuffer info: %w", err)
	}
	pixels := make([]uint32, int(info.W*info.H))
	r := &Renderer{
		fb:     f,
		fmt:    info.Format,
		pixels: pixels,
		W:      info.W,
		H:      info.H,
		stride: int(info.W),
		fbPath: path,
	}
	return r, nil
}

func (r *Renderer) Close() {
	if r == nil {
		return
	}
	if r.fb != nil {
		_ = r.fb.Close()
	}
}

func (r *Renderer) SyncSize() {
	// Physical display size does not change at runtime; nothing to do.
}

func (r *Renderer) Draw(frame FrameState) error {
	if r == nil || r.fb == nil {
		return fmt.Errorf("renderer is nil")
	}
	r.SyncSize()
	if frame.Now.IsZero() {
		frame.Now = time.Now()
	}
	layout := LayoutFor(r.W, r.H)
	style := styleForExpression(frame.Expression)
	phase := frame.IdlePhase
	if phase == 0 {
		phase = float64(frame.Now.UnixNano()) / 1e9
	}

	r.fillRectColor(0, 0, r.W, r.H, rgba{0x4e, 0xcb, 0xa8, 255}) // body teal
	if frame.Overlay != nil && frame.Overlay.Visible {
		// Hide the face while the settings overlay is open so eye arcs
		// cannot bleed outside the panel regardless of the current expression.
		r.drawOverlay(layout, *frame.Overlay)
	} else {
		r.drawBackdrop(layout, phase)
		r.drawFace(layout, style, frame, phase)
		r.drawCornerClock(layout, frame, style)
	}
	return r.present()
}

func (r *Renderer) present() error {
	if len(r.pixels) == 0 {
		return nil
	}
	if r.fmt.BitsPerPixel == 16 {
		if cap(r.raw) < len(r.pixels)*2 {
			r.raw = make([]byte, len(r.pixels)*2)
		}
		buf := r.raw[:len(r.pixels)*2]
		for i, px := range r.pixels {
			packed := px
			off := i * 2
			buf[off] = byte(packed)
			buf[off+1] = byte(packed >> 8)
		}
		_, err := r.fb.WriteAt(buf, 0)
		return err
	}
	// Default to 32bpp; use a zero-copy byte view over the pixel slice.
	buf := unsafe.Slice((*byte)(unsafe.Pointer(&r.pixels[0])), len(r.pixels)*4)
	_, err := r.fb.WriteAt(buf, 0)
	return err
}

type bmoEyeType uint8

const (
	bmoEyeDot       bmoEyeType = iota // dot: idle, concerned, thinking
	bmoEyePill                         // narrow vertical pill: excited, speaking
	bmoEyePillLarge                    // wider pill + shine: listening
	bmoEyeArc                          // upward ∩ arc: happy/squint
	bmoEyeFlat                         // horizontal line: sleeping
)

type bmoMouthType uint8

const (
	bmoMouthIdleSmile  bmoMouthType = iota // gentle upward curve
	bmoMouthFrown                          // gentle downward curve
	bmoMouthOpenLarge                      // full open with teeth + tongue
	bmoMouthOpenSpeak                      // smaller open, animated for TTS
	bmoMouthOpenSmall                      // tiny 'o': listening
)

type bmoBrowType uint8

const (
	bmoBrowNone        bmoBrowType = iota
	bmoBrowWorried                          // inner corners lower
	bmoBrowRaisedRight                      // one raised brow: thinking
)

type expressionStyle struct {
	Eye        bmoEyeType
	Mouth      bmoMouthType
	Brow       bmoBrowType
	Animated   bool // speaking mouth oscillation
	Sleepy     bool // ZZZ marks
	RightEyeUp bool // thinking: right eye slightly higher
}

func styleForExpression(expr string) expressionStyle {
	switch normalizeExpression(expr) {
	case "listening":
		return expressionStyle{Eye: bmoEyePillLarge, Mouth: bmoMouthOpenSmall}
	case "thinking":
		return expressionStyle{Eye: bmoEyeDot, Mouth: bmoMouthIdleSmile, Brow: bmoBrowRaisedRight, RightEyeUp: true}
	case "speaking":
		return expressionStyle{Eye: bmoEyePill, Mouth: bmoMouthOpenSpeak, Animated: true}
	case "sleeping":
		return expressionStyle{Eye: bmoEyeFlat, Mouth: bmoMouthIdleSmile, Sleepy: true}
	case "concerned":
		return expressionStyle{Eye: bmoEyeDot, Mouth: bmoMouthFrown, Brow: bmoBrowWorried}
	case "smile", "laugh", "excited":
		return expressionStyle{Eye: bmoEyeArc, Mouth: bmoMouthOpenLarge}
	case "blink":
		return expressionStyle{Eye: bmoEyeFlat, Mouth: bmoMouthIdleSmile}
	default: // neutral, idle
		return expressionStyle{Eye: bmoEyeDot, Mouth: bmoMouthIdleSmile}
	}
}

func normalizeExpression(expr string) string {
	switch strings.ToLower(strings.TrimSpace(expr)) {
	case "idle", "neutral":
		return "neutral"
	case "asleep", "sleep", "sleeping":
		return "sleeping"
	case "error", "confused", "angry", "sad":
		return "concerned"
	case "happy":
		return "smile"
	case "excited":
		return "laugh"
	default:
		return strings.ToLower(strings.TrimSpace(expr))
	}
}

func (r *Renderer) drawBackdrop(layout Layout, phase float64) {
	w, h := layout.W, layout.H
	r.fillRectColor(0, 0, w, h, rgba{0x4e, 0xcb, 0xa8, 255}) // body teal #4ECBA8
	for i := int32(0); i < 3; i++ {
		sx := w/5 + i*w/4
		sy := h/5 + int32(math.Sin(phase*0.7+float64(i))*float64(h)/16)
		sz := clampInt32(w/18, 18, 44)
		r.fillCircle(txClamp(sx, sz, w), txClamp(sy, sz, h), sz/2, rgba{255, 255, 255, 8})
	}
}

func (r *Renderer) drawFace(layout Layout, style expressionStyle, frame FrameState, phase float64) {
	outer := rectInset(layout.W, layout.H, layout.Margin)
	inner := rectInset(layout.W, layout.H, layout.Margin+layout.ScreenInset)

	// Body (bright teal) and screen background (pale mint).
	r.fillRoundedRect(outer.X, outer.Y, outer.W, outer.H, layout.CornerRadius,
		rgba{0x4e, 0xcb, 0xa8, 255}) // #4ECBA8
	r.fillRoundedRect(inner.X, inner.Y, inner.W, inner.H,
		layout.CornerRadius-layout.ScreenInset/2,
		rgba{0x90, 0xe5, 0xc8, 255}) // #90e5c8

	iw := float64(inner.W)
	ih := float64(inner.H)
	ix := inner.X
	iy := inner.Y

	// Canonical eye positions (bmo-face skill): left=20.3%, right=79.2%, cy=37.4%
	lx := ix + int32(iw*0.203)
	rx := ix + int32(iw*0.792)
	ey := iy + int32(ih*0.374)

	dark := rgba{0x1a, 0x1a, 0x1a, 255}

	// Eyes
	switch style.Eye {
	case bmoEyeDot:
		// Reference: 2.9% wide × 7.2% tall — a narrow vertical pill, not a circle.
		pw := max32(5, int32(iw*0.029))
		ph := max32(10, int32(ih*0.072))
		r.fillRoundedRect(lx-pw/2, ey-ph/2, pw, ph, pw/2, dark)
		if style.RightEyeUp {
			r.fillRoundedRect(rx-pw/2, iy+int32(ih*0.348)-ph/2, pw, ph, pw/2, dark)
		} else {
			r.fillRoundedRect(rx-pw/2, ey-ph/2, pw, ph, pw/2, dark)
		}

	case bmoEyePill:
		pw := max32(5, int32(iw*0.035))
		ph := max32(14, int32(ih*0.129))
		r.fillRoundedRect(lx-pw/2, ey-ph/2, pw, ph, pw/2, dark)
		r.fillRoundedRect(rx-pw/2, ey-ph/2, pw, ph, pw/2, dark)

	case bmoEyePillLarge:
		pw := max32(8, int32(iw*0.059))
		ph := max32(18, int32(ih*0.181))
		r.fillRoundedRect(lx-pw/2, ey-ph/2, pw, ph, pw/2, dark)
		r.fillRoundedRect(rx-pw/2, ey-ph/2, pw, ph, pw/2, dark)
		shR := max32(2, int32(iw*0.015))
		r.fillCircle(lx-pw/4, ey-ph/4, shR, rgba{255, 255, 255, 140})
		r.fillCircle(rx-pw/4, ey-ph/4, shR, rgba{255, 255, 255, 140})

	case bmoEyeArc:
		// ∩ upward arc: endpoints y=41.9%, control y=32.3%, half-width=18.8%
		ahw := int32(iw * 0.094) // center-to-endpoint, not full span
		aey := iy + int32(ih*0.419)
		aqy := iy + int32(ih*0.323)
		thk := max32(3, int32(iw*0.025))
		lArc := quadBezierPoints(point{lx - ahw, aey}, point{lx, aqy}, point{lx + ahw, aey}, 14)
		rArc := quadBezierPoints(point{rx - ahw, aey}, point{rx, aqy}, point{rx + ahw, aey}, 14)
		r.drawBezierThick(lArc, thk, dark)
		r.drawBezierThick(rArc, thk, dark)

	case bmoEyeFlat:
		fhw := max32(10, int32(iw*0.074))
		fh := max32(3, int32(ih*0.032))
		r.fillRectColor(lx-fhw, ey-fh/2, fhw*2, fh, dark)
		r.fillRectColor(rx-fhw, ey-fh/2, fhw*2, fh, dark)
	}

	// Brows
	browR := max32(2, int32(ih*0.016))
	switch style.Brow {
	case bmoBrowWorried:
		lox := ix + int32(iw*0.109)
		lix := ix + int32(iw*0.287)
		rix := ix + int32(iw*0.713)
		rox := ix + int32(iw*0.891)
		byOuter := iy + int32(ih*0.226)
		byInner := iy + int32(ih*0.323)
		r.drawThickLine(lox, byOuter, lix, byInner, browR, dark)
		r.drawThickLine(rix, byInner, rox, byOuter, browR, dark)
	case bmoBrowRaisedRight:
		rix := ix + int32(iw*0.713)
		rox := ix + int32(iw*0.891)
		byRaised := iy + int32(ih*0.194)
		byBase := iy + int32(ih*0.258)
		r.drawThickLine(rix, byBase, rox, byRaised, browR, dark)
	}

	// Mouth
	cx := ix + inner.W/2
	slx := ix + int32(iw*0.381)
	srx := ix + int32(iw*0.600)
	sey := iy + int32(ih*0.587)
	sqy := iy + int32(ih*0.665)
	fqy := iy + int32(ih*0.510)
	mouthSW := max32(3, int32(ih*0.026))
	mx := ix + int32(iw*0.292)
	mw := int32(iw * 0.416)
	mty := iy + int32(ih*0.523)
	mh := int32(ih * 0.277)
	mr := int32(float64(mh) * 0.48)
	tth := int32(float64(mh) * 0.28)
	teeth := rgba{0xe4, 0xe4, 0xe4, 255}
	interior := rgba{0x1a, 0x78, 0x48, 255}
	tongue := rgba{0x16, 0xae, 0x81, 255}
	trx := int32(float64(mw/2) * 0.69)
	try := int32(float64(mh) * 0.16)
	_ = mty + tth + int32(float64(mh-tth)*0.67) // tcy no longer used; tongue is at bottom

	switch style.Mouth {
	case bmoMouthIdleSmile:
		smilePts := quadBezierPoints(point{slx, sey}, point{cx, sqy}, point{srx, sey}, 14)
		r.drawBezierThick(smilePts, mouthSW, dark)

	case bmoMouthFrown:
		frownPts := quadBezierPoints(point{slx, sey}, point{cx, fqy}, point{srx, sey}, 14)
		r.drawBezierThick(frownPts, mouthSW, dark)

	case bmoMouthOpenLarge:
		r.fillRoundedRect(mx, mty, mw, mh, mr, dark)
		// Teeth: rounded rect matching mouth's top corner curve for seamless blending.
		r.fillRoundedRect(mx+3, mty+3, mw-6, tth, mr-3, teeth)
		// Interior green fills below teeth.
		r.fillRoundedRect(mx+3, mty+tth, mw-6, mh-tth-3, mr-2, interior)
		// Tongue sits at the bottom of the mouth following its lower curve.
		r.fillEllipse(cx-trx, mty+mh-try*2-3, trx*2, try*2, tongue)

	case bmoMouthOpenSpeak:
		smx := ix + int32(iw*0.341)
		smw := int32(iw * 0.318)
		smty := iy + int32(ih*0.548)
		smhBase := int32(ih * 0.213)
		smh := smhBase
		if style.Animated {
			smh = int32(float64(smhBase) * (0.50 + 0.30*math.Sin(phase*8.0)))
			if smh < smhBase/4 {
				smh = smhBase / 4
			}
		}
		smr := int32(float64(smh) * 0.48)
		stth := int32(float64(smh) * 0.28)
		_ = smty + stth + int32(float64(smh-stth)*0.67) // stcy no longer used; tongue at bottom
		strx := int32(float64(smw/2) * 0.69)
		stry := int32(float64(smh) * 0.16)
		r.fillRoundedRect(smx, smty, smw, smh, smr, dark)
		r.fillRoundedRect(smx+3, smty+3, smw-6, stth, smr-3, teeth)
		r.fillRoundedRect(smx+3, smty+stth, smw-6, smh-stth-3, smr-2, interior)
		r.fillEllipse(cx-strx, smty+smh-stry*2-3, strx*2, stry*2, tongue)

	case bmoMouthOpenSmall:
		soRX := max32(8, int32(iw*0.074))
		soRY := max32(5, int32(ih*0.065))
		soCy := iy + int32(ih*0.665)
		r.fillEllipse(cx-soRX, soCy-soRY, soRX*2, soRY*2, dark)
		r.fillEllipse(cx-soRX*3/4, soCy-soRY*3/4, soRX*3/2, soRY*3/2, interior)
	}

	if style.Sleepy {
		r.drawSleepMarks(layout, phase)
	}
}

func (r *Renderer) drawCornerClock(layout Layout, frame FrameState, style expressionStyle) {
	show := frame.QuotaExhausted // clock only appears when AI quota is exhausted
	if !show {
		return
	}
	cx := layout.W - layout.ClockInset - layout.ClockSize/2
	cy := layout.ClockInset + layout.ClockSize/2
	r.fillCircle(cx, cy, layout.ClockSize/2, rgba{214, 235, 227, 255})
	r.fillCircle(cx, cy, layout.ClockSize/2-3, rgba{17, 68, 76, 255})
	r.fillCircle(cx, cy, layout.ClockSize/2-8, rgba{214, 235, 227, 255})
	for i := 0; i < 12; i++ {
		angle := float64(i) * (math.Pi / 6)
		r1 := float64(layout.ClockSize) * 0.34
		r2 := float64(layout.ClockSize) * 0.42
		x1 := cx + int32(math.Cos(angle)*r1)
		y1 := cy + int32(math.Sin(angle)*r1)
		x2 := cx + int32(math.Cos(angle)*r2)
		y2 := cy + int32(math.Sin(angle)*r2)
		r.drawLine(x1, y1, x2, y2, rgba{17, 68, 76, 220})
	}
	minuteAngle := -math.Pi / 2
	hourAngle := -math.Pi / 2
	if !frame.SleepUntil.IsZero() && !frame.Now.IsZero() {
		remaining := frame.SleepUntil.Sub(frame.Now)
		if remaining > 0 {
			minuteAngle = -math.Pi/2 + (remaining.Minutes()/60.0)*2*math.Pi
			hourAngle = -math.Pi/2 + (remaining.Hours()/12.0)*2*math.Pi
		}
	}
	r.drawLine(cx, cy, cx+int32(math.Cos(hourAngle)*float64(layout.ClockSize)*0.18), cy+int32(math.Sin(hourAngle)*float64(layout.ClockSize)*0.18), rgba{17, 68, 76, 255})
	r.drawLine(cx, cy, cx+int32(math.Cos(minuteAngle)*float64(layout.ClockSize)*0.28), cy+int32(math.Sin(minuteAngle)*float64(layout.ClockSize)*0.28), rgba{17, 68, 76, 255})
	r.drawSleepCap(cx, cy-layout.ClockSize/2-2)
}

func (r *Renderer) drawSleepCap(cx, topY int32) {
	r.drawLine(cx-6, topY, cx+6, topY, rgba{214, 235, 227, 255})
	r.drawLine(cx-4, topY-4, cx+4, topY-4, rgba{214, 235, 227, 190})
}

func (r *Renderer) drawSleepMarks(layout Layout, phase float64) {
	baseX := layout.W/2 + layout.MouthW/2
	baseY := layout.MouthY - layout.MouthOpenH/2 - layout.ScreenInset
	for i := 0; i < 3; i++ {
		ox := int32(float64(i*22) + math.Sin(phase+float64(i))*4)
		oy := int32(float64(-i*18) + math.Cos(phase*0.8+float64(i))*3)
		sz := int32(8 + i*4)
		r.drawZ(baseX+ox, baseY+oy, sz, rgba{214, 235, 227, 170 - uint8(i*25)})
	}
}

func (r *Renderer) drawZ(x, y, size int32, c rgba) {
	r.drawLine(x, y, x+size, y, c)
	r.drawLine(x+size, y, x, y+size, c)
	r.drawLine(x, y+size, x+size, y+size, c)
}

var glyphs = map[rune][7]uint8{
	' ': {0, 0, 0, 0, 0, 0, 0},
	'!': {4, 4, 4, 4, 4, 0, 4},
	',': {0, 0, 0, 0, 0, 4, 8},
	'-': {0, 0, 0, 31, 0, 0, 0},
	'.': {0, 0, 0, 0, 0, 0, 4},
	'/': {1, 2, 4, 8, 16, 0, 0},
	':': {0, 4, 0, 0, 4, 0, 0},
	'0': {14, 17, 19, 21, 25, 17, 14},
	'1': {4, 12, 4, 4, 4, 4, 14},
	'2': {14, 17, 1, 2, 4, 8, 31},
	'3': {30, 1, 1, 14, 1, 1, 30},
	'4': {2, 6, 10, 18, 31, 2, 2},
	'5': {31, 16, 30, 1, 1, 17, 14},
	'6': {6, 8, 16, 30, 17, 17, 14},
	'7': {31, 1, 2, 4, 8, 8, 8},
	'8': {14, 17, 17, 14, 17, 17, 14},
	'9': {14, 17, 17, 15, 1, 2, 12},
	'A': {14, 17, 17, 31, 17, 17, 17},
	'B': {30, 17, 17, 30, 17, 17, 30},
	'C': {14, 17, 16, 16, 16, 17, 14},
	'D': {30, 17, 17, 17, 17, 17, 30},
	'E': {31, 16, 16, 30, 16, 16, 31},
	'F': {31, 16, 16, 30, 16, 16, 16},
	'G': {14, 17, 16, 23, 17, 17, 15},
	'H': {17, 17, 17, 31, 17, 17, 17},
	'I': {14, 4, 4, 4, 4, 4, 14},
	'J': {7, 2, 2, 2, 18, 18, 12},
	'K': {17, 18, 20, 24, 20, 18, 17},
	'L': {16, 16, 16, 16, 16, 16, 31},
	'M': {17, 27, 21, 21, 17, 17, 17},
	'N': {17, 25, 21, 19, 17, 17, 17},
	'O': {14, 17, 17, 17, 17, 17, 14},
	'P': {30, 17, 17, 30, 16, 16, 16},
	'Q': {14, 17, 17, 17, 21, 18, 13},
	'R': {30, 17, 17, 30, 20, 18, 17},
	'S': {15, 16, 16, 14, 1, 1, 30},
	'T': {31, 4, 4, 4, 4, 4, 4},
	'U': {17, 17, 17, 17, 17, 17, 14},
	'V': {17, 17, 17, 17, 17, 10, 4},
	'W': {17, 17, 17, 21, 21, 21, 10},
	'X': {17, 17, 10, 4, 10, 17, 17},
	'Y': {17, 17, 10, 4, 4, 4, 4},
	'Z': {31, 1, 2, 4, 8, 16, 31},
}

func (r *Renderer) drawText(x, y, scale int32, c rgba, text string) {
	if scale <= 0 {
		scale = 1
	}
	cursorX := x
	for _, ch := range strings.ToUpper(text) {
		glyph, ok := glyphs[ch]
		if !ok {
			glyph = glyphs[' ']
		}
		for row := 0; row < len(glyph); row++ {
			bits := glyph[row]
			for col := 0; col < 5; col++ {
				if bits&(1<<(4-col)) == 0 {
					continue
				}
				rx := cursorX + int32(col)*scale
				ry := y + int32(row)*scale
				r.fillRectColor(rx, ry, scale, scale, c)
			}
		}
		cursorX += 6 * scale
	}
}

func (r *Renderer) drawOverlay(layout Layout, overlay OverlayState) {
	panelW := clampInt32(layout.W*78/100, 360, layout.W-2*layout.Margin)
	panelH := clampInt32(layout.H*76/100, 260, layout.H-2*layout.Margin)
	panelX := (layout.W - panelW) / 2
	panelY := (layout.H - panelH) / 2
	r.fillRoundedRect(panelX, panelY, panelW, panelH, clampInt32(layout.CornerRadius/2, 12, 48), rgba{10, 29, 39, 255})
	r.fillRoundedRect(panelX+4, panelY+4, panelW-8, panelH-8, clampInt32(layout.CornerRadius/2, 10, 40), rgba{22, 53, 62, 255})

	top := panelY + 22
	left := panelX + 22
	r.drawText(left, top, 4, rgba{214, 235, 227, 255}, overlay.Title)
	top += 40 // title is 28px (7 rows × 4px); 12px breathing room below
	for _, line := range overlay.Subtitle {
		r.drawText(left, top, 2, rgba{176, 213, 206, 255}, line)
		top += 24 // subtitle line is 14px (7 rows × 2px); 10px gap between lines
	}
	top += 18
	for _, item := range overlay.Items {
		boxColor := rgba{79, 139, 141, 255}
		if item.Selected {
			boxColor = rgba{170, 232, 183, 255}
		}
		if item.Focused {
			boxColor = rgba{255, 241, 145, 255}
		}
		r.fillRectColor(left, top+3, 10, 10, boxColor)
		if item.Selected {
			r.drawLine(left+2, top+8, left+4, top+11, rgba{16, 49, 56, 255})
			r.drawLine(left+4, top+11, left+8, top+3, rgba{16, 49, 56, 255})
		}
		labelColor := rgba{214, 235, 227, 255}
		if item.Focused {
			labelColor = rgba{255, 241, 145, 255}
		}
		r.drawText(left+20, top, 2, labelColor, item.Label)
		top += 26
	}
	if strings.TrimSpace(overlay.Footer) != "" {
		r.drawText(left, panelY+panelH-28, 2, rgba{176, 213, 206, 255}, strings.ToUpper(overlay.Footer))
	}
}

func (r *Renderer) drawLine(x1, y1, x2, y2 int32, c rgba) {
	dx := absInt32(x2 - x1)
	sy := int32(-1)
	if y1 < y2 {
		sy = 1
	}
	dy := -absInt32(y2 - y1)
	err := dx + dy
	for {
		r.setPixel(x1, y1, c)
		if x1 == x2 && y1 == y2 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x1 += signInt32(x2 - x1)
		}
		if e2 <= dx {
			err += dx
			y1 += sy
		}
	}
}

func (r *Renderer) drawPolyline(points []point, c rgba) {
	if len(points) < 2 {
		return
	}
	for i := 1; i < len(points); i++ {
		r.drawLine(points[i-1].X, points[i-1].Y, points[i].X, points[i].Y, c)
	}
}

func (r *Renderer) fillRectColor(x, y, w, h int32, c rgba) {
	if w <= 0 || h <= 0 {
		return
	}
	// Clamp to pixel buffer bounds once, avoiding per-pixel bounds checks.
	if x < 0 {
		w += x
		x = 0
	}
	if y < 0 {
		h += y
		y = 0
	}
	if x+w > r.W {
		w = r.W - x
	}
	if y+h > r.H {
		h = r.H - y
	}
	if w <= 0 || h <= 0 {
		return
	}
	px := r.packColor(c)
	// Fill the first row via a range slice — Go eliminates bounds checks here.
	off0 := int(y)*r.stride + int(x)
	row0 := r.pixels[off0 : off0+int(w)]
	for i := range row0 {
		row0[i] = px
	}
	// Copy that row to the remaining rows; copy() compiles down to SIMD memcpy.
	for yy := int32(1); yy < h; yy++ {
		off := int(y+yy)*r.stride + int(x)
		copy(r.pixels[off:], row0)
	}
}

func (r *Renderer) fillRoundedRect(x, y, w, h, radius int32, c rgba) {
	if w <= 0 || h <= 0 {
		return
	}
	if radius <= 0 {
		r.fillRectColor(x, y, w, h, c)
		return
	}
	if radius*2 > w {
		radius = w / 2
	}
	if radius*2 > h {
		radius = h / 2
	}
	r.fillRectColor(x+radius, y, w-2*radius, h, c)
	r.fillRectColor(x, y+radius, radius, h-2*radius, c)
	r.fillRectColor(x+w-radius, y+radius, radius, h-2*radius, c)
	r.fillQuarterCircle(x+radius, y+radius, radius, 2, c)
	r.fillQuarterCircle(x+w-radius-1, y+radius, radius, 1, c)
	r.fillQuarterCircle(x+radius, y+h-radius-1, radius, 3, c)
	r.fillQuarterCircle(x+w-radius-1, y+h-radius-1, radius, 4, c)
}

func (r *Renderer) fillCircle(cx, cy, radius int32, c rgba) {
	for dy := -radius; dy <= radius; dy++ {
		delta := int64(radius)*int64(radius) - int64(dy)*int64(dy)
		if delta < 0 {
			continue
		}
		dx := int32(math.Sqrt(float64(delta)))
		r.drawLine(cx-dx, cy+dy, cx+dx, cy+dy, c)
	}
}

func (r *Renderer) fillEllipse(x, y, w, h int32, c rgba) {
	if w <= 0 || h <= 0 {
		return
	}
	cx := x + w/2
	cy := y + h/2
	rx := float64(w) / 2.0
	ry := float64(h) / 2.0
	if rx < 1 {
		rx = 1
	}
	if ry < 1 {
		ry = 1
	}
	ryi := int32(math.Round(ry))
	for dy := -ryi; dy <= ryi; dy++ {
		norm := 1.0 - (float64(dy)*float64(dy))/(ry*ry)
		if norm < 0 {
			continue
		}
		dx := int32(math.Sqrt(norm) * rx)
		r.drawLine(cx-dx, cy+dy, cx+dx, cy+dy, c)
	}
}

func (r *Renderer) fillQuarterCircle(x, y, radius int32, quadrant int, c rgba) {
	for dy := int32(0); dy <= radius; dy++ {
		dx := int32(math.Sqrt(float64(radius*radius - dy*dy)))
		switch quadrant {
		case 1:
			r.drawLine(x, y-dy, x+dx, y-dy, c)
		case 2:
			r.drawLine(x-dx, y-dy, x, y-dy, c)
		case 3:
			r.drawLine(x-dx, y+dy, x, y+dy, c)
		case 4:
			r.drawLine(x, y+dy, x+dx, y+dy, c)
		}
	}
}

type point struct {
	X, Y int32
}

func arcPoints(cx, cy int32, rx, ry float64, start, end float64, segments int) []point {
	if segments < 2 {
		segments = 2
	}
	pts := make([]point, 0, segments+1)
	for i := 0; i <= segments; i++ {
		t := start + (end-start)*float64(i)/float64(segments)
		x := cx + int32(math.Round(math.Cos(t)*rx))
		y := cy + int32(math.Round(math.Sin(t)*ry))
		pts = append(pts, point{X: x, Y: y})
	}
	return pts
}

func offsetPoints(points []point, dx, dy int32) []point {
	if len(points) == 0 {
		return nil
	}
	out := make([]point, len(points))
	for i, p := range points {
		out[i] = point{X: p.X + dx, Y: p.Y + dy}
	}
	return out
}

// quadBezierPoints samples a quadratic Bezier curve into discrete points.
func quadBezierPoints(p0, p1, p2 point, segments int) []point {
	if segments < 2 {
		segments = 2
	}
	pts := make([]point, 0, segments+1)
	for i := 0; i <= segments; i++ {
		t := float64(i) / float64(segments)
		u := 1 - t
		x := u*u*float64(p0.X) + 2*u*t*float64(p1.X) + t*t*float64(p2.X)
		y := u*u*float64(p0.Y) + 2*u*t*float64(p1.Y) + t*t*float64(p2.Y)
		pts = append(pts, point{X: int32(math.Round(x)), Y: int32(math.Round(y))})
	}
	return pts
}

// drawBezierThick draws a thick curve by stamping a filled circle at each sample point.
func (r *Renderer) drawBezierThick(pts []point, radius int32, c rgba) {
	if radius < 1 {
		radius = 1
	}
	for _, pt := range pts {
		r.fillCircle(pt.X, pt.Y, radius, c)
	}
}

// drawThickLine draws a thick line between two points using filled circles.
func (r *Renderer) drawThickLine(x1, y1, x2, y2, radius int32, c rgba) {
	pts := quadBezierPoints(
		point{x1, y1},
		point{(x1 + x2) / 2, (y1 + y2) / 2},
		point{x2, y2},
		12,
	)
	r.drawBezierThick(pts, radius, c)
}

func max32(a, b int32) int32 {
	if a > b {
		return a
	}
	return b
}

func rectInset(w, h, inset int32) rect {
	if inset < 0 {
		inset = 0
	}
	return rect{X: inset, Y: inset, W: w - inset*2, H: h - inset*2}
}

type rect struct {
	X, Y, W, H int32
}

func (r *Renderer) setPixel(x, y int32, c rgba) {
	if x < 0 || y < 0 || x >= r.W || y >= r.H {
		return
	}
	idx := int(y)*r.stride + int(x)
	r.pixels[idx] = r.packColor(c)
}

func (r *Renderer) packColor(c rgba) uint32 {
	return r.packRGBA(c.R, c.G, c.B, c.A)
}

func (r *Renderer) packRGBA(red, green, blue, alpha uint8) uint32 {
	if r.fmt.BitsPerPixel == 16 {
		var value uint32
		value |= scaleToBits(red, r.fmt.RedLength) << r.fmt.RedOffset
		value |= scaleToBits(green, r.fmt.GreenLength) << r.fmt.GreenOffset
		value |= scaleToBits(blue, r.fmt.BlueLength) << r.fmt.BlueOffset
		value |= scaleToBits(alpha, r.fmt.AlphaLength) << r.fmt.AlphaOffset
		return value
	}
	var value uint32
	value |= uint32(red) << r.fmt.RedOffset
	value |= uint32(green) << r.fmt.GreenOffset
	value |= uint32(blue) << r.fmt.BlueOffset
	value |= uint32(alpha) << r.fmt.AlphaOffset
	return value
}

// fbioGetVScreenInfo is the Linux ioctl number for FBIOGET_VSCREENINFO.
const fbioGetVScreenInfo = 0x4600

type fbScreenInfoResult struct {
	W, H   int32
	Format fbFormat
}

// fbScreenInfo queries the framebuffer device for its physical display
// dimensions and pixel format using the FBIOGET_VSCREENINFO ioctl.
// This is the only reliable way to get xres/yres (physical) as opposed to
// xres_virtual/yres_virtual which may be a large multiple of the actual
// screen height due to multi-page buffering.
func fbScreenInfo(f *os.File) (fbScreenInfoResult, error) {
	// fb_var_screeninfo is 160 bytes. The fields we care about:
	//   [0:4]   xres          (physical width)
	//   [4:8]   yres          (physical height)
	//   [24:28] bits_per_pixel
	//   [32:36] red.offset,   [36:40] red.length
	//   [44:48] green.offset, [48:52] green.length
	//   [56:60] blue.offset,  [60:64] blue.length
	//   [68:72] transp.offset,[72:76] transp.length
	var raw [160]byte
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), fbioGetVScreenInfo, uintptr(unsafe.Pointer(&raw[0]))); errno != 0 {
		return fbScreenInfoResult{}, errno
	}
	u32 := func(off int) int { return int(*(*uint32)(unsafe.Pointer(&raw[off]))) }

	w := int32(u32(0))
	h := int32(u32(4))
	if w <= 0 || h <= 0 {
		return fbScreenInfoResult{}, fmt.Errorf("ioctl returned invalid display size %dx%d", w, h)
	}

	bpp := u32(24)
	if bpp != 16 && bpp != 32 {
		bpp = 32
	}
	format := fbFormat{
		BitsPerPixel: bpp,
		RedOffset:    u32(32), RedLength: u32(36),
		GreenOffset:  u32(44), GreenLength: u32(48),
		BlueOffset:   u32(56), BlueLength: u32(60),
		AlphaOffset:  u32(68), AlphaLength: u32(72),
	}
	// Fall back to sensible defaults if the kernel didn't fill in channel info.
	if format.RedLength == 0 && format.GreenLength == 0 && format.BlueLength == 0 {
		format.RedOffset, format.RedLength = 16, 8
		format.GreenOffset, format.GreenLength = 8, 8
		format.BlueOffset, format.BlueLength = 0, 8
		format.AlphaOffset, format.AlphaLength = 24, 8
	}
	return fbScreenInfoResult{W: w, H: h, Format: format}, nil
}

func readInt(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

func scaleToBits(v uint8, bits int) uint32 {
	if bits <= 0 {
		return 0
	}
	if bits >= 8 {
		return uint32(v)
	}
	max := uint32((1 << bits) - 1)
	return uint32(v) * max / 255
}

func clampInt32(v, lo, hi int32) int32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func minFloat(v, hi float64) float64 {
	if v < hi {
		return v
	}
	return hi
}

func txClamp(v, size, limit int32) int32 {
	if v < size/2 {
		return size / 2
	}
	if v > limit-size/2 {
		return limit - size/2
	}
	return v
}

func absInt32(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}

func signInt32(v int32) int32 {
	if v < 0 {
		return -1
	}
	return 1
}
