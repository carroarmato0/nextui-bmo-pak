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
)

// NavAction is a menu navigation intent decoded from a raw Linux evdev event.
type NavAction uint8

const (
	NavUp     NavAction = iota // D-pad up
	NavDown                     // D-pad down
	NavLeft                     // D-pad left
	NavRight                    // D-pad right
	NavCancel                   // B/East or Select — close overlay or exit
	NavSave                     // Start — save / open settings
	NavMenu                     // Mode/Guide — exit to NextUI
	NavProvider                 // Y/North — open AI setup menu
)

const (
	evAbs    uint16 = 0x03
	absHat0X uint16 = 16
	absHat0Y uint16 = 17

	// BTN_DPAD_* key codes (Linux input.h gamepad extension).
	btnDpadUp    uint16 = 0x220
	btnDpadDown  uint16 = 0x221
	btnDpadLeft  uint16 = 0x222
	btnDpadRight uint16 = 0x223
)

// NavReader reads raw Linux evdev events from an input device and emits NavAction values.
type NavReader struct {
	mu     sync.Mutex
	path   string
	file   *os.File
	events chan NavAction
	once   sync.Once
}

// NewNavReader creates a NavReader for the given input device path.
func NewNavReader(path string) (*NavReader, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("nav reader: device path is required")
	}
	return &NavReader{
		path:   path,
		events: make(chan NavAction, 16),
	}, nil
}

// Start opens the device and begins reading events in a background goroutine.
func (r *NavReader) Start(ctx context.Context) error {
	if r == nil {
		return errors.New("nil nav reader")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	r.mu.Lock()
	if r.file != nil {
		r.mu.Unlock()
		return errors.New("nav reader already started")
	}
	f, err := os.Open(r.path)
	if err != nil {
		r.mu.Unlock()
		return fmt.Errorf("open nav device %s: %w", r.path, err)
	}
	r.file = f
	r.mu.Unlock()

	go r.run(ctx, f)
	return nil
}

// Close stops the reader and releases the device file.
func (r *NavReader) Close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	f := r.file
	r.file = nil
	r.mu.Unlock()
	if f != nil {
		return f.Close()
	}
	return nil
}

// Events returns the channel that emits decoded NavAction values.
func (r *NavReader) Events() <-chan NavAction {
	if r == nil {
		return nil
	}
	return r.events
}

func (r *NavReader) run(ctx context.Context, f *os.File) {
	defer r.once.Do(func() { close(r.events) })
	defer func() {
		_ = f.Close()
		r.mu.Lock()
		if r.file == f {
			r.file = nil
		}
		r.mu.Unlock()
	}()

	buf := make([]byte, 24)
	for {
		if _, err := io.ReadFull(f, buf); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, os.ErrClosed) || errors.Is(err, context.Canceled) {
				return
			}
			return
		}
		if ctx.Err() != nil {
			return
		}

		typ := binary.LittleEndian.Uint16(buf[16:18])
		code := binary.LittleEndian.Uint16(buf[18:20])
		value := int32(binary.LittleEndian.Uint32(buf[20:24]))

		var (
			action NavAction
			ok     bool
		)
		switch typ {
		case evKey:
			if value != keyStatePressed {
				continue
			}
			action, ok = navActionForKey(code)
		case evAbs:
			action, ok = navActionForAbs(code, value)
		default:
			continue
		}
		if !ok {
			continue
		}
		select {
		case r.events <- action:
		case <-ctx.Done():
			return
		default:
		}
	}
}

// navActionForKey maps a Linux EV_KEY button code to a NavAction on press.
func navActionForKey(code uint16) (NavAction, bool) {
	switch code {
	case 304: // BTN_SOUTH / physical B — cancel/exit
		return NavCancel, true
	case 307: // BTN_NORTH / physical Y — open AI setup
		return NavProvider, true
	case 315: // BTN_START
		return NavSave, true
	case 316: // BTN_MODE / menu button
		return NavMenu, true
	case btnDpadUp:
		return NavUp, true
	case btnDpadDown:
		return NavDown, true
	case btnDpadLeft:
		return NavLeft, true
	case btnDpadRight:
		return NavRight, true
	default:
		return 0, false
	}
}

// navActionForAbs maps an EV_ABS hat axis event (code+value) to a NavAction.
func navActionForAbs(code uint16, value int32) (NavAction, bool) {
	if value == 0 {
		return 0, false
	}
	switch code {
	case absHat0X:
		if value < 0 {
			return NavLeft, true
		}
		return NavRight, true
	case absHat0Y:
		if value < 0 {
			return NavUp, true
		}
		return NavDown, true
	default:
		return 0, false
	}
}
