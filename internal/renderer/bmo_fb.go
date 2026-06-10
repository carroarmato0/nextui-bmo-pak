//go:build !cgo

package renderer

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

	w, h := fbSize(1280, 720)
	if fw, fh, err := fbVirtualSize(); err == nil && fw > 0 && fh > 0 {
		w, h = fw, fh
	}
	format, err := fbPixelFormat(path)
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	pixels := make([]uint32, int(w*h))
	r := &Renderer{fb: f, fmt: format, pixels: pixels, W: w, H: h, stride: int(w), fbPath: path}
	if format.BitsPerPixel == 0 {
		r.fmt.BitsPerPixel = 32
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
	if r == nil {
		return
	}
	if w, h, err := fbVirtualSize(); err == nil && w > 0 && h > 0 && (w != r.W || h != r.H) {
		r.W, r.H = w, h
		r.stride = int(w)
		r.pixels = make([]uint32, int(w*h))
	}
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

	r.fillRectColor(0, 0, r.W, r.H, rgba{22, 108, 121, 255})
	r.drawBackdrop(layout, phase)
	r.drawFace(layout, style, frame, phase)
	r.drawCornerClock(layout, frame, style)
	if frame.Overlay != nil && frame.Overlay.Visible {
		r.drawOverlay(layout, *frame.Overlay)
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

type expressionStyle struct {
	EyeOpen     float64
	EyeSquint   float64
	Mouth       mouthKind
	BrowTilt    float64
	PupilShiftX float64
	PupilShiftY float64
	PupilScale  float64
	Sleepy      bool
	Talky       bool
	Laughing    bool
	Whistling   bool
	Frown       bool
}

type mouthKind int

const (
	mouthNeutral mouthKind = iota
	mouthSmile
	mouthFrown
	mouthOpen
	mouthWhistle
	mouthLine
)

func styleForExpression(expr string) expressionStyle {
	switch normalizeExpression(expr) {
	case "blink":
		return expressionStyle{EyeOpen: 0.06, Mouth: mouthNeutral}
	case "listening":
		return expressionStyle{EyeOpen: 1.0, Mouth: mouthLine, PupilScale: 1.0}
	case "thinking":
		return expressionStyle{EyeOpen: 0.85, Mouth: mouthLine, BrowTilt: -0.6, PupilScale: 0.9}
	case "speaking":
		return expressionStyle{EyeOpen: 0.95, Mouth: mouthOpen, Talky: true, PupilScale: 1.0}
	case "sleeping":
		return expressionStyle{EyeOpen: 0.02, Mouth: mouthSmile, Sleepy: true}
	case "concerned":
		return expressionStyle{EyeOpen: 0.82, Mouth: mouthFrown, BrowTilt: 0.8, Frown: true}
	case "smile":
		return expressionStyle{EyeOpen: 1.0, Mouth: mouthSmile, PupilScale: 1.0}
	case "laugh":
		return expressionStyle{EyeOpen: 0.45, EyeSquint: 0.55, Mouth: mouthOpen, Laughing: true}
	case "whistle":
		return expressionStyle{EyeOpen: 0.55, Mouth: mouthWhistle, Whistling: true, PupilScale: 0.75}
	case "look_around":
		return expressionStyle{EyeOpen: 1.0, Mouth: mouthNeutral, PupilScale: 1.0}
	default:
		return expressionStyle{EyeOpen: 1.0, Mouth: mouthNeutral, PupilScale: 1.0}
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
	// A subtle vignette and highlight wash to keep the screen lively.
	r.fillRectColor(0, 0, w, h, rgba{22, 108, 121, 255})
	for i := int32(0); i < 3; i++ {
		sx := w/5 + i*w/4
		sy := h/5 + int32(math.Sin(phase*0.7+float64(i))*float64(h)/16)
		sz := clampInt32(w/18, 18, 44)
		r.fillCircle(txClamp(sx, sz, w), txClamp(sy, sz, h), sz/2, rgba{255, 255, 255, 10})
	}
}

func (r *Renderer) drawFace(layout Layout, style expressionStyle, frame FrameState, phase float64) {
	outer := rectInset(layout.W, layout.H, layout.Margin)
	inner := rectInset(layout.W, layout.H, layout.Margin+layout.ScreenInset)

	r.fillRoundedRect(outer.X, outer.Y, outer.W, outer.H, layout.CornerRadius, rgba{18, 88, 97, 255})
	r.fillRoundedRect(inner.X, inner.Y, inner.W, inner.H, layout.CornerRadius-layout.ScreenInset/2, rgba{23, 128, 132, 255})
	r.fillRoundedRect(inner.X+layout.GlowInset, inner.Y+layout.GlowInset, inner.W-2*layout.GlowInset, inner.H-2*layout.GlowInset, layout.CornerRadius/2, rgba{46, 170, 170, 34})

	centerX := layout.W / 2
	leftEyeX := centerX - layout.EyeGap/2 - layout.EyeW
	rightEyeX := centerX + layout.EyeGap/2
	eyeY := layout.EyeY
	pupilXShift := int32(math.Round(style.PupilShiftX * float64(layout.EyeW/4)))
	pupilYShift := int32(math.Round(style.PupilShiftY * float64(layout.EyeH/6)))

	browColor := rgba{16, 57, 68, 255}
	browLift := int32(math.Round(style.BrowTilt * float64(layout.EyeH) / 8))
	r.drawLine(leftEyeX, eyeY-layout.EyeH/2-layout.BrowH, leftEyeX+layout.EyeW, eyeY-layout.EyeH/2-layout.BrowH-browLift, browColor)
	r.drawLine(rightEyeX, eyeY-layout.EyeH/2-layout.BrowH-browLift, rightEyeX+layout.EyeW, eyeY-layout.EyeH/2-layout.BrowH, browColor)

	eyeColor := rgba{13, 48, 62, 255}
	eyeOpen := style.EyeOpen
	if frame.ReducedMotion {
		eyeOpen = minFloat(eyeOpen, 0.9)
	}
	if eyeOpen < 0.12 {
		r.drawEyeClosed(leftEyeX, eyeY, layout.EyeW, layout.EyeH, eyeColor)
		r.drawEyeClosed(rightEyeX, eyeY, layout.EyeW, layout.EyeH, eyeColor)
	} else {
		r.drawEye(leftEyeX, eyeY, layout.EyeW, layout.EyeH, eyeOpen, eyeColor)
		r.drawEye(rightEyeX, eyeY, layout.EyeW, layout.EyeH, eyeOpen, eyeColor)
	}

	expr := normalizeExpression(frame.Expression)
	lookPhase := phase
	if style.PupilShiftX == 0 && style.Mouth == mouthNeutral && !style.Talky && !style.Laughing && !style.Whistling {
		lookPhase *= 0.35
	}
	pupilShiftX, pupilShiftY := pupilXShift, pupilYShift
	if expr == "look_around" {
		pupilShiftX = int32(math.Round(math.Sin(lookPhase*0.9) * float64(layout.EyeW) / 8))
		pupilShiftY = int32(math.Round(math.Sin(lookPhase*1.2+1.1) * float64(layout.EyeH) / 12))
	}
	pupilW := int32(math.Round(float64(layout.PupilW) * style.PupilScale))
	pupilH := int32(math.Round(float64(layout.PupilH) * style.PupilScale))
	if pupilW < 8 {
		pupilW = 8
	}
	if pupilH < 8 {
		pupilH = 8
	}
	pupilColor := rgba{3, 17, 28, 255}
	r.drawPupil(leftEyeX+layout.EyeW/2-pupilW/2+pupilShiftX, eyeY-layout.EyeH/2+pupilH/2+pupilShiftY, pupilW, pupilH, pupilColor)
	r.drawPupil(rightEyeX+layout.EyeW/2-pupilW/2-pupilShiftX, eyeY-layout.EyeH/2+pupilH/2-pupilShiftY, pupilW, pupilH, pupilColor)
	r.drawPupil(leftEyeX+layout.EyeW/2-pupilW/3+pupilShiftX/2, eyeY-layout.EyeH/2+pupilH/3+pupilShiftY/2, pupilW/5, pupilH/5, rgba{255, 255, 255, 80})
	r.drawPupil(rightEyeX+layout.EyeW/2-pupilW/3-pupilShiftX/2, eyeY-layout.EyeH/2+pupilH/3-pupilShiftY/2, pupilW/5, pupilH/5, rgba{255, 255, 255, 80})

	mouthY := layout.MouthY
	mouthColor := rgba{10, 40, 54, 255}
	if style.Laughing {
		mouthY += int32(math.Sin(phase*3.0) * 2)
	}
	switch style.Mouth {
	case mouthNeutral:
		r.drawMouthLine(centerX, mouthY, layout.MouthLineW, mouthColor)
	case mouthSmile:
		r.drawMouthSmile(centerX, mouthY, layout.MouthW, layout.MouthH, mouthColor)
	case mouthFrown:
		r.drawMouthFrown(centerX, mouthY, layout.MouthW, layout.MouthH, mouthColor)
	case mouthOpen:
		openH := layout.MouthOpenH
		if style.Talky {
			openH = int32(float64(layout.MouthOpenH) * (0.45 + 0.25*math.Sin(phase*8.0)))
		}
		if style.Laughing {
			openH = int32(float64(layout.MouthOpenH) * 1.1)
		}
		r.drawMouthOpen(centerX, mouthY, layout.MouthW, openH, mouthColor)
	case mouthWhistle:
		r.drawMouthWhistle(centerX, mouthY, layout.MouthW/4, mouthColor)
	case mouthLine:
		r.drawMouthLine(centerX, mouthY, layout.MouthLineW, mouthColor)
	}
	if style.Talky || expr == "neutral" {
		bob := int32(math.Sin(phase*1.3) * 2)
		r.drawMouthLine(centerX, mouthY+bob, layout.MouthLineW, rgba{7, 34, 46, 120})
	}
	if style.Sleepy {
		r.drawSleepMarks(layout, phase)
	}
}

func (r *Renderer) drawCornerClock(layout Layout, frame FrameState, style expressionStyle) {
	show := frame.QuotaExhausted || style.Sleepy
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

	top := panelY + 18
	left := panelX + 18
	r.drawText(left, top, 4, rgba{214, 235, 227, 255}, overlay.Title)
	top += 28
	for _, line := range overlay.Subtitle {
		r.drawText(left, top, 2, rgba{176, 213, 206, 255}, line)
		top += 16
	}
	top += 8
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
		top += 20
	}
	if strings.TrimSpace(overlay.Footer) != "" {
		r.drawText(left, panelY+panelH-28, 2, rgba{176, 213, 206, 255}, strings.ToUpper(overlay.Footer))
	}
}

func (r *Renderer) drawEye(x, y, w, h int32, open float64, c rgba) {
	visibleH := int32(math.Round(float64(h) * open))
	if visibleH < 4 {
		visibleH = 4
	}
	cy := y - h/2 + visibleH/2
	r.fillRoundedRect(x, cy-visibleH/2, w, visibleH, visibleH/2, c)
}

func (r *Renderer) drawEyeClosed(x, y, w, h int32, c rgba) {
	cy := y - h/2 + h/2
	r.drawLine(x+8, cy, x+w-8, cy, c)
	r.drawLine(x+10, cy+1, x+w-10, cy+1, rgba{c.R, c.G, c.B, 150})
}

func (r *Renderer) drawPupil(x, y, w, h int32, c rgba) {
	r.fillEllipse(x, y, w, h, c)
}

func (r *Renderer) drawMouthLine(cx, cy, halfW int32, c rgba) {
	r.drawLine(cx-halfW, cy, cx+halfW, cy, c)
	r.drawLine(cx-halfW+1, cy+1, cx+halfW-1, cy+1, rgba{c.R, c.G, c.B, 120})
}

func (r *Renderer) drawMouthWhistle(cx, cy, radius int32, c rgba) {
	r.fillEllipse(cx-radius, cy-radius, radius*2, radius*2, c)
	r.fillEllipse(cx-radius/2, cy-radius/2, radius, radius, rgba{214, 235, 227, 80})
}

func (r *Renderer) drawMouthOpen(cx, cy, w, h int32, c rgba) {
	r.fillEllipse(cx-w/2, cy-h/2, w, h, c)
	r.fillEllipse(cx-w/4, cy-h/5, w/2, h/3, rgba{46, 25, 34, 160})
}

func (r *Renderer) drawMouthSmile(cx, cy, w, h int32, c rgba) {
	points := arcPoints(cx, cy, float64(w)/2.0, float64(h)/2.0, math.Pi, 2*math.Pi, 18)
	r.drawPolyline(points, c)
	r.drawPolyline(offsetPoints(points, 0, 1), rgba{c.R, c.G, c.B, 120})
}

func (r *Renderer) drawMouthFrown(cx, cy, w, h int32, c rgba) {
	points := arcPoints(cx, cy+int32(float64(h)*0.2), float64(w)/2.0, float64(h)/2.5, 0, math.Pi, 18)
	r.drawPolyline(points, c)
	r.drawPolyline(offsetPoints(points, 0, 1), rgba{c.R, c.G, c.B, 120})
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
	for yy := int32(0); yy < h; yy++ {
		for xx := int32(0); xx < w; xx++ {
			r.setPixel(x+xx, y+yy, c)
		}
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

func fbPixelFormat(path string) (fbFormat, error) {
	format := fbFormat{BitsPerPixel: 32, RedOffset: 16, RedLength: 8, GreenOffset: 8, GreenLength: 8, BlueOffset: 0, BlueLength: 8, AlphaOffset: 24, AlphaLength: 8}
	base := filepath.Dir(path)
	if v, err := readInt(filepath.Join(base, "bits_per_pixel")); err == nil && v > 0 {
		format.BitsPerPixel = v
	}
	if v, err := readInt(filepath.Join(base, "red", "offset")); err == nil {
		format.RedOffset = v
	}
	if v, err := readInt(filepath.Join(base, "red", "length")); err == nil {
		format.RedLength = v
	}
	if v, err := readInt(filepath.Join(base, "green", "offset")); err == nil {
		format.GreenOffset = v
	}
	if v, err := readInt(filepath.Join(base, "green", "length")); err == nil {
		format.GreenLength = v
	}
	if v, err := readInt(filepath.Join(base, "blue", "offset")); err == nil {
		format.BlueOffset = v
	}
	if v, err := readInt(filepath.Join(base, "blue", "length")); err == nil {
		format.BlueLength = v
	}
	if v, err := readInt(filepath.Join(base, "transp", "offset")); err == nil {
		format.AlphaOffset = v
	}
	if v, err := readInt(filepath.Join(base, "transp", "length")); err == nil {
		format.AlphaLength = v
	}
	if format.BitsPerPixel != 16 && format.BitsPerPixel != 32 {
		format.BitsPerPixel = 32
	}
	return format, nil
}

func fbVirtualSize() (int32, int32, error) {
	data, err := os.ReadFile("/sys/class/graphics/fb0/virtual_size")
	if err != nil {
		return 0, 0, err
	}
	parts := strings.Split(strings.TrimSpace(string(data)), ",")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid framebuffer size: %q", strings.TrimSpace(string(data)))
	}
	w, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, err
	}
	h, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, 0, err
	}
	return int32(w), int32(h), nil
}

func fbSize(defaultW, defaultH int32) (int32, int32) {
	return defaultW, defaultH
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
