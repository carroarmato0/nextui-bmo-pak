package input

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	evKey = 0x01

	keyStateReleased = 0
	keyStatePressed  = 1
	keyStateRepeat   = 2
)

var defaultPTTButtons = []string{"BTN_TL", "BTN_TR"}

type Event struct {
	Button string
	Code   uint16
	Held   bool
	At     time.Time
}

type Watcher struct {
	mu     sync.RWMutex
	path   string
	codes  map[uint16]string
	active map[uint16]bool
	held   bool
	file   *os.File
	events chan Event
	once   sync.Once
}

func NewWatcher(path string, buttonNames ...string) (*Watcher, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("ptt watcher: device path is required")
	}
	if len(buttonNames) == 0 {
		buttonNames = defaultPTTButtons
	}

	codes := make(map[uint16]string, len(buttonNames))
	active := make(map[uint16]bool, len(buttonNames))
	for _, name := range buttonNames {
		code, ok := ParseButtonCode(name)
		if !ok {
			return nil, fmt.Errorf("ptt watcher: unknown button %q", name)
		}
		codes[code] = NormalizeButtonName(name)
		active[code] = false
	}

	return &Watcher{
		path:   path,
		codes:  codes,
		active: active,
		events: make(chan Event, 8),
	}, nil
}

func (w *Watcher) Start(ctx context.Context) error {
	if w == nil {
		return errors.New("nil ptt watcher")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	w.mu.Lock()
	if w.file != nil {
		w.mu.Unlock()
		return errors.New("ptt watcher already started")
	}
	f, err := os.Open(w.path)
	if err != nil {
		w.mu.Unlock()
		return fmt.Errorf("open ptt device %s: %w", w.path, err)
	}
	w.file = f
	w.mu.Unlock()

	go w.run(ctx, f)
	return nil
}

func (w *Watcher) Close() error {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	f := w.file
	w.file = nil
	w.mu.Unlock()
	if f != nil {
		return f.Close()
	}
	return nil
}

func (w *Watcher) Events() <-chan Event {
	if w == nil {
		return nil
	}
	return w.events
}

func (w *Watcher) Held() bool {
	if w == nil {
		return false
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.held
}

func (w *Watcher) run(ctx context.Context, f *os.File) {
	defer w.once.Do(func() { close(w.events) })
	defer func() {
		_ = f.Close()
		w.mu.Lock()
		if w.file == f {
			w.file = nil
		}
		w.mu.Unlock()
	}()

	buf := make([]byte, 24)
	for {
		if _, err := io.ReadFull(f, buf); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, os.ErrClosed) || errors.Is(err, context.Canceled) {
				return
			}
			select {
			case w.events <- Event{Button: "error", Held: w.Held(), At: time.Now().UTC()}:
			default:
			}
			return
		}

		if ctx.Err() != nil {
			return
		}

		typ := binary.LittleEndian.Uint16(buf[16:18])
		if typ != evKey {
			continue
		}
		code := binary.LittleEndian.Uint16(buf[18:20])
		value := int32(binary.LittleEndian.Uint32(buf[20:24]))
		if value == keyStateRepeat {
			continue
		}
		if held, changed, buttonName := w.update(code, value == keyStatePressed); changed {
			select {
			case w.events <- Event{Button: buttonName, Code: code, Held: held, At: time.Now().UTC()}:
			case <-ctx.Done():
				return
			}
		}
	}
}

func (w *Watcher) update(code uint16, pressed bool) (held bool, changed bool, buttonName string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	buttonName, ok := w.codes[code]
	if !ok {
		return w.held, false, ""
	}
	if current, ok := w.active[code]; ok && current == pressed {
		return w.held, false, buttonName
	}
	w.active[code] = pressed
	held = false
	for _, active := range w.active {
		if active {
			held = true
			break
		}
	}
	changed = held != w.held
	w.held = held
	return held, changed, buttonName
}

type Buffer struct {
	mu       sync.Mutex
	held     bool
	maxBytes int
	pcm      []byte
}

func NewBuffer(maxBytes int) *Buffer {
	if maxBytes <= 0 {
		maxBytes = 0
	}
	return &Buffer{maxBytes: maxBytes}
}

func (b *Buffer) Begin() {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.held = true
	b.pcm = b.pcm[:0]
}

func (b *Buffer) Append(pcm []byte) {
	if b == nil || len(pcm) == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.held {
		return
	}
	if b.maxBytes > 0 {
		if len(pcm) >= b.maxBytes {
			b.pcm = append(b.pcm[:0], pcm[len(pcm)-b.maxBytes:]...)
			return
		}
		if overflow := len(b.pcm) + len(pcm) - b.maxBytes; overflow > 0 {
			if overflow >= len(b.pcm) {
				b.pcm = b.pcm[:0]
			} else {
				copy(b.pcm, b.pcm[overflow:])
				b.pcm = b.pcm[:len(b.pcm)-overflow]
			}
		}
	}
	b.pcm = append(b.pcm, pcm...)
}

func (b *Buffer) End() []byte {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.held {
		return nil
	}
	b.held = false
	if len(b.pcm) == 0 {
		return nil
	}
	out := make([]byte, len(b.pcm))
	copy(out, b.pcm)
	b.pcm = b.pcm[:0]
	return out
}

func (b *Buffer) Held() bool {
	if b == nil {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.held
}

func ParseButtonCode(name string) (uint16, bool) {
	switch NormalizeButtonName(name) {
	case "BTN_SOUTH":
		return 304, true
	case "BTN_EAST":
		return 305, true
	case "BTN_C":
		return 306, true
	case "BTN_NORTH":
		return 307, true
	case "BTN_WEST":
		return 308, true
	case "BTN_TL":
		return 310, true
	case "BTN_TR":
		return 311, true
	case "BTN_TL2":
		return 312, true
	case "BTN_TR2":
		return 313, true
	case "BTN_SELECT":
		return 314, true
	case "BTN_START":
		return 315, true
	case "BTN_MODE":
		return 316, true
	case "BTN_THUMBL":
		return 317, true
	case "BTN_THUMBR":
		return 318, true
	default:
		return 0, false
	}
}

func NormalizeButtonName(name string) string {
	return strings.ToUpper(strings.TrimSpace(name))
}

func ButtonLabel(name string) string {
	switch NormalizeButtonName(name) {
	case "BTN_SOUTH":
		return "South"
	case "BTN_EAST":
		return "East"
	case "BTN_C":
		return "C"
	case "BTN_NORTH":
		return "North"
	case "BTN_WEST":
		return "West"
	case "BTN_TL":
		return "Left trigger"
	case "BTN_TR":
		return "Right trigger"
	case "BTN_TL2":
		return "Left shoulder"
	case "BTN_TR2":
		return "Right shoulder"
	case "BTN_SELECT":
		return "Select"
	case "BTN_START":
		return "Start"
	case "BTN_MODE":
		return "Mode"
	case "BTN_THUMBL":
		return "Left stick"
	case "BTN_THUMBR":
		return "Right stick"
	default:
		return NormalizeButtonName(name)
	}
}
