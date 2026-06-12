package renderer

import (
	"fmt"
	"math"
	"strings"
	"time"
	"unsafe"

	"github.com/veandco/go-sdl2/sdl"
)

// The BMO face is rendered entirely into an in-memory ARGB8888 pixel buffer
// using software primitives, then uploaded to an SDL streaming texture and
// presented each frame. This keeps a single, backend-agnostic face
// implementation that displays correctly on every device SDL2 supports —
// fbdev on the TrimUI Brick and pvrsrvkm/EGL on the Smart Pro — without the
// display-buffer ownership problems of writing to /dev/fb0 directly.

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
	SpeakAmplitude  float32 // RMS amplitude [0,1] during TTS playback; drives mouth height
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
	window *sdl.Window
	ren    *sdl.Renderer
	tex    *sdl.Texture
	pixels []uint32
	W      int32
	H      int32
	stride int
}

type rgba struct {
	R, G, B, A uint8
}

func NewFullscreen(title string) (*Renderer, error) {
	return New(title, true)
}

func New(title string, fullscreen bool) (*Renderer, error) {
	if err := sdl.Init(sdl.INIT_VIDEO | sdl.INIT_TIMER); err != nil {
		return nil, fmt.Errorf("sdl init: %w", err)
	}

	flags := uint32(sdl.WINDOW_SHOWN)
	if fullscreen {
		flags |= sdl.WINDOW_FULLSCREEN_DESKTOP | sdl.WINDOW_BORDERLESS
	}

	w, h := int32(640), int32(480)
	if mode, err := sdl.GetCurrentDisplayMode(0); err == nil && mode.W > 0 && mode.H > 0 {
		w, h = mode.W, mode.H
	}

	win, err := sdl.CreateWindow(title, sdl.WINDOWPOS_UNDEFINED, sdl.WINDOWPOS_UNDEFINED, w, h, flags)
	if err != nil {
		sdl.Quit()
		return nil, fmt.Errorf("create window: %w", err)
	}

	ren, err := sdl.CreateRenderer(win, -1, sdl.RENDERER_ACCELERATED|sdl.RENDERER_PRESENTVSYNC)
	if err != nil {
		// Fallback for fbdev/software backends (e.g. TrimUI Brick) that do not
		// support hardware acceleration or vsync.
		ren, err = sdl.CreateRenderer(win, -1, 0)
		if err != nil {
			win.Destroy()
			sdl.Quit()
			return nil, fmt.Errorf("create renderer: %w", err)
		}
	}

	r := &Renderer{window: win, ren: ren}
	ow, oh, err := ren.GetOutputSize()
	if err != nil || ow <= 0 || oh <= 0 {
		ow, oh = w, h
	}
	if err := r.ensureBuffer(ow, oh); err != nil {
		ren.Destroy()
		win.Destroy()
		sdl.Quit()
		return nil, err
	}
	return r, nil
}

// ensureBuffer (re)allocates the pixel buffer and streaming texture whenever
// the output size changes (including first use).
func (r *Renderer) ensureBuffer(w, h int32) error {
	if w <= 0 || h <= 0 {
		return fmt.Errorf("invalid output size %dx%d", w, h)
	}
	if r.tex != nil && w == r.W && h == r.H {
		return nil
	}
	if r.tex != nil {
		_ = r.tex.Destroy()
		r.tex = nil
	}
	tex, err := r.ren.CreateTexture(sdl.PIXELFORMAT_ARGB8888, sdl.TEXTUREACCESS_STREAMING, w, h)
	if err != nil {
		return fmt.Errorf("create texture: %w", err)
	}
	// The face code overwrites pixels (no blending); the alpha channel is
	// cosmetic. Present the texture opaquely so on-screen output matches the
	// raw RGB of the pixel buffer exactly.
	if err := tex.SetBlendMode(sdl.BLENDMODE_NONE); err != nil {
		_ = tex.Destroy()
		return fmt.Errorf("set texture blend mode: %w", err)
	}
	r.tex = tex
	r.W, r.H = w, h
	r.stride = int(w)
	r.pixels = make([]uint32, int(w)*int(h))
	return nil
}

func (r *Renderer) Close() {
	if r == nil {
		return
	}
	if r.tex != nil {
		_ = r.tex.Destroy()
	}
	if r.ren != nil {
		_ = r.ren.Destroy()
	}
	if r.window != nil {
		_ = r.window.Destroy()
	}
	sdl.Quit()
}

// DebugInfo reports SDL's active video driver, display dimensions, and
// renderer backend. Logged once at startup — valuable because this pak runs
// across multiple display backends (Mali/GLES2 on the Brick, pvrsrvkm on the
// Smart Pro) where a size or backend mismatch otherwise manifests as a silent
// black screen.
func (r *Renderer) DebugInfo() string {
	driver, _ := sdl.GetCurrentVideoDriver()
	var dmW, dmH int32 = -1, -1
	if mode, err := sdl.GetCurrentDisplayMode(0); err == nil {
		dmW, dmH = mode.W, mode.H
	}
	var winW, winH int32 = -1, -1
	if r.window != nil {
		winW, winH = r.window.GetSize()
	}
	rendName := "?"
	if r.ren != nil {
		if info, err := r.ren.GetInfo(); err == nil {
			rendName = info.Name
		}
	}
	return fmt.Sprintf("driver=%s displayMode=%dx%d window=%dx%d output=%dx%d renderer=%s",
		driver, dmW, dmH, winW, winH, r.W, r.H, rendName)
}

func (r *Renderer) SyncSize() {
	if r == nil || r.ren == nil {
		return
	}
	if w, h, err := r.ren.GetOutputSize(); err == nil && w > 0 && h > 0 {
		_ = r.ensureBuffer(w, h)
	}
}

func (r *Renderer) Draw(frame FrameState) error {
	if r == nil || r.ren == nil {
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
		r.drawCornerClock(layout, frame)
	}
	return r.present()
}

func (r *Renderer) present() error {
	if len(r.pixels) == 0 || r.tex == nil {
		return nil
	}
	if err := r.tex.Update(nil, unsafe.Pointer(&r.pixels[0]), r.stride*4); err != nil {
		return fmt.Errorf("texture update: %w", err)
	}
	if err := r.ren.Clear(); err != nil {
		return fmt.Errorf("clear: %w", err)
	}
	if err := r.ren.Copy(r.tex, nil, nil); err != nil {
		return fmt.Errorf("copy texture: %w", err)
	}
	r.ren.Present()
	return nil
}

type bmoEyeType uint8

const (
	bmoEyeDot       bmoEyeType = iota // dot: idle, concerned, thinking
	bmoEyePill                        // narrow vertical pill: excited, speaking
	bmoEyePillLarge                   // wider pill + shine: listening
	bmoEyeArc                         // upward ∩ arc: happy/squint
	bmoEyeFlat                        // horizontal line: sleeping
)

type bmoMouthType uint8

const (
	bmoMouthIdleSmile bmoMouthType = iota // gentle upward curve
	bmoMouthFrown                         // gentle downward curve
	bmoMouthOpenLarge                     // full open with teeth + tongue
	bmoMouthOpenSpeak                     // smaller open, animated for TTS
	bmoMouthOpenSmall                     // tiny 'o': listening
)

type bmoBrowType uint8

const (
	bmoBrowNone        bmoBrowType = iota
	bmoBrowWorried                  // inner corners lower
	bmoBrowRaisedRight              // one raised brow: thinking
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
	case exprSleeping:
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
	case "idle", exprNeutral:
		return exprNeutral
	case "asleep", "sleep", exprSleeping:
		return exprSleeping
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

	// Mouth — shared variables used by smile, frown, and open-mouth cases.
	cx := ix + inner.W/2
	slx := ix + int32(iw*0.381)
	srx := ix + int32(iw*0.600)
	sey := iy + int32(ih*0.587)
	sqy := iy + int32(ih*0.665)
	fqy := iy + int32(ih*0.510)
	mouthSW := max32(3, int32(ih*0.026))
	// Large-mouth geometry (used by bmoMouthOpenLarge).
	mw := int32(iw * 0.416)
	mty := iy + int32(ih*0.523)
	mh := int32(ih * 0.277)
	teeth := rgba{0xe4, 0xe4, 0xe4, 255}
	interior := rgba{0x1a, 0x78, 0x48, 255}
	tongue := rgba{0x16, 0xae, 0x81, 255}

	switch style.Mouth {
	case bmoMouthIdleSmile:
		smilePts := quadBezierPoints(point{slx, sey}, point{cx, sqy}, point{srx, sey}, 14)
		r.drawBezierThick(smilePts, mouthSW, dark)

	case bmoMouthFrown:
		frownPts := quadBezierPoints(point{slx, sey}, point{cx, fqy}, point{srx, sey}, 14)
		r.drawBezierThick(frownPts, mouthSW, dark)

	case bmoMouthOpenLarge:
		r.drawMouthFilled(cx, mty, mw, mh, teeth, interior, tongue)

	case bmoMouthOpenSpeak:
		smw := int32(iw * 0.318)
		smty := iy + int32(ih*0.548)
		smhBase := int32(ih * 0.213)
		smh := smhBase
		if frame.SpeakAmplitude > 0 {
			// Amplitude-driven: sqrt gives more visible response at low levels.
			openFactor := math.Sqrt(float64(frame.SpeakAmplitude))
			smh = max32(smhBase/8, int32(float64(smhBase)*openFactor))
		} else if style.Animated {
			// Fallback sin-wave when no amplitude data is available.
			smh = int32(float64(smhBase) * (0.45 + 0.35*math.Sin(phase*8.0)))
			if smh < smhBase/6 {
				smh = smhBase / 6
			}
		}
		r.drawMouthFilled(cx, smty, smw, smh, teeth, interior, tongue)

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

func (r *Renderer) drawCornerClock(layout Layout, frame FrameState) {
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

// drawMouthFilled draws a rounded-rectangle open mouth (flat centre, rounded corners).
// Teeth fill the top 28%, interior fills the rest, tongue sits in the lower interior.
func (r *Renderer) drawMouthFilled(cx, mty, mw, mh int32, teeth, interior, tongue rgba) {
	if mw <= 0 || mh <= 0 {
		return
	}
	// Dark outline: slightly larger rounded rect underneath.
	border := max32(2, mw/90)
	mr := int32(float64(mh) * 0.42) // corner radius
	r.fillRoundedRect(cx-mw/2-border, mty-border, mw+2*border, mh+2*border, mr+border, rgba{0x1a, 0x1a, 0x1a, 255})

	// Per-scanline fill with rounded-rect clip.
	// Corners are rounded; the centre section is full width.
	tth := int32(float64(mh) * 0.28) // teeth height
	// Tongue: ellipse centred on the bottom edge of the opening so the visible
	// dome reads as rising from behind the lower lip. It is rendered inside
	// this scanline loop so the opening clips it — it never overlaps the lip
	// or escapes the mouth (spec §6.2).
	tgy, tgw, tgh := tongueGeometry(mty, mw, mh)
	tcy := float64(tgy) + float64(tgh)/2
	trx := float64(tgw) / 2
	try := float64(tgh) / 2
	for dy := int32(0); dy < mh; dy++ {
		var xOff int32
		if dy < mr {
			yInCorner := float64(mr - dy)
			xOff = int32(float64(mr) - math.Sqrt(float64(mr)*float64(mr)-yInCorner*yInCorner))
		} else if dy >= mh-mr {
			yInCorner := float64(dy - (mh - mr))
			xOff = int32(float64(mr) - math.Sqrt(float64(mr)*float64(mr)-yInCorner*yInCorner))
		}
		lineW := mw - 2*xOff
		if lineW <= 0 {
			continue
		}
		c := interior
		if dy < tth {
			c = teeth
		}
		r.fillRectColor(cx-mw/2+xOff, mty+dy, lineW, 1, c)
		// Tongue segment on this scanline, clipped to the opening width.
		ny := (float64(mty+dy) - tcy) / try
		if ny*ny < 1 {
			halfW := int32(trx * math.Sqrt(1-ny*ny))
			lx := max32(cx-halfW, cx-mw/2+xOff)
			rxEnd := min32(cx+halfW, cx-mw/2+xOff+lineW)
			if rxEnd > lx {
				r.fillRectColor(lx, mty+dy, rxEnd-lx, 1, tongue)
			}
		}
	}
}

// tongueGeometry returns the tongue ellipse bounds for a mouth at (cx, mty)
// of size mw x mh. The ellipse is centred on the mouth's bottom edge so its
// root sits below the opening and only the upper dome is visible once the
// scanline clip is applied — the tongue rises from behind the lower lip and
// never overlaps it (spec §6.2).
func tongueGeometry(mty, mw, mh int32) (y, w, h int32) {
	trx := max32(4, int32(float64(mw)*0.28))
	try := max32(2, int32(float64(mh)*0.18))
	tcy := mty + mh
	return tcy - try, trx * 2, try * 2
}

func max32(a, b int32) int32 {
	if a > b {
		return a
	}
	return b
}

func min32(a, b int32) int32 {
	if a < b {
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
	r.pixels[int(y)*r.stride+int(x)] = r.packColor(c)
}

// packColor packs an rgba into a native uint32 matching SDL_PIXELFORMAT_ARGB8888
// (0xAARRGGBB). Output is presented opaquely, so the alpha byte is cosmetic.
func (r *Renderer) packColor(c rgba) uint32 {
	return uint32(c.A)<<24 | uint32(c.R)<<16 | uint32(c.G)<<8 | uint32(c.B)
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
