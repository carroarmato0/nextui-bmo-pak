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

// NavAction is a menu navigation intent decoded from a raw Linux evdev event.
type NavAction uint8

const (
	NavUp      NavAction = iota // D-pad up
	NavDown                     // D-pad down
	NavLeft                     // D-pad left
	NavRight                    // D-pad right
	NavCancel                   // B/East or Select — close overlay or exit
	NavSave                     // Start — save / open settings
	NavMenu                     // Mode/Guide — exit to NextUI
	NavQuote                    // X/West — speak a random verbatim quote
	NavGallery                  // Y/North — step to the next face / animation
	NavConfirm                  // A/East — activate the focused menu item
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

// Auto-repeat tuning for a held up/down direction. EV_ABS hat dpads emit no
// kernel autorepeat, so NavReader synthesises repeats itself: hold the button
// past repeatInitialDelay and it then fires every repeatInterval until release.
const (
	repeatInitialDelay = 350 * time.Millisecond
	repeatInterval     = 90 * time.Millisecond
)

// NavReader reads raw Linux evdev events from an input device and emits NavAction values.
type NavReader struct {
	mu     sync.Mutex
	path   string
	file   *os.File
	events chan NavAction
	once   sync.Once

	// Auto-repeat state for a held up/down direction, guarded by repeatMu.
	// repeatGen is bumped on every state change so a stale repeat loop can
	// detect that its press ended (or was superseded) and bail out. wake nudges
	// the repeater goroutine to re-read the state.
	repeatMu     sync.Mutex
	repeatAction NavAction
	repeatOn     bool
	repeatGen    uint64
	wake         chan struct{}
}

// NewNavReader creates a NavReader for the given input device path.
func NewNavReader(path string) (*NavReader, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("nav reader: device path is required")
	}
	return &NavReader{
		path:   path,
		events: make(chan NavAction, 16),
		wake:   make(chan struct{}, 1),
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
	go r.repeater(ctx)
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

		switch typ {
		case evKey:
			switch value {
			case keyStatePressed:
				if action, ok := navActionForKey(code); ok {
					r.emit(ctx, action)
					r.setRepeat(action)
				}
			case keyStateReleased:
				if action, ok := navActionForKey(code); ok {
					r.clearRepeat(action)
				}
			}
			// keyStateRepeat (kernel autorepeat) is ignored: NavReader drives
			// repeats itself so EV_KEY and EV_ABS dpads behave identically.
		case evAbs:
			if action, ok := navActionForAbs(code, value); ok {
				r.emit(ctx, action)
				r.setRepeat(action)
			} else if value == 0 {
				// Hat axis returned to centre — the held direction was released.
				r.clearRepeatAxis(code)
			}
		}
	}
}

// emit delivers a NavAction to consumers, dropping it if the buffer is full
// (the same back-pressure behaviour as raw presses) or the context is done.
func (r *NavReader) emit(ctx context.Context, action NavAction) {
	select {
	case r.events <- action:
	case <-ctx.Done():
	default:
	}
}

// setRepeat begins (or refreshes) auto-repeat for a held up/down press. Any
// other direction cancels an in-flight repeat.
func (r *NavReader) setRepeat(action NavAction) {
	r.repeatMu.Lock()
	if action == NavUp || action == NavDown {
		r.repeatAction = action
		r.repeatOn = true
	} else {
		r.repeatOn = false
	}
	r.repeatGen++
	r.repeatMu.Unlock()
	r.signalWake()
}

// clearRepeat stops auto-repeat when the matching direction button is released.
func (r *NavReader) clearRepeat(action NavAction) {
	r.repeatMu.Lock()
	if r.repeatOn && r.repeatAction == action {
		r.repeatOn = false
		r.repeatGen++
	}
	r.repeatMu.Unlock()
	r.signalWake()
}

// clearRepeatAxis stops auto-repeat when the vertical hat axis recentres.
func (r *NavReader) clearRepeatAxis(code uint16) {
	if code != absHat0Y {
		return
	}
	r.repeatMu.Lock()
	if r.repeatOn {
		r.repeatOn = false
		r.repeatGen++
	}
	r.repeatMu.Unlock()
	r.signalWake()
}

// signalWake nudges the repeater goroutine to re-read the repeat state.
func (r *NavReader) signalWake() {
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

// repeater synthesises held-button auto-repeat. While an up/down direction is
// held it emits that action every repeatInterval after an initial
// repeatInitialDelay, until the press is released or superseded.
func (r *NavReader) repeater(ctx context.Context) {
	for {
		r.repeatMu.Lock()
		on, action, gen := r.repeatOn, r.repeatAction, r.repeatGen
		r.repeatMu.Unlock()

		if !on {
			select {
			case <-r.wake:
				continue
			case <-ctx.Done():
				return
			}
		}

		// Initial delay; a state change (wake) restarts the evaluation so a new
		// press gets its own full delay rather than inheriting the old timer.
		select {
		case <-time.After(repeatInitialDelay):
		case <-r.wake:
			continue
		case <-ctx.Done():
			return
		}

		for r.repeatStillHeld(gen) {
			r.emit(ctx, action)
			select {
			case <-time.After(repeatInterval):
			case <-r.wake:
			case <-ctx.Done():
				return
			}
		}
	}
}

// repeatStillHeld reports whether the press identified by gen is still active.
func (r *NavReader) repeatStillHeld(gen uint64) bool {
	r.repeatMu.Lock()
	defer r.repeatMu.Unlock()
	return r.repeatOn && r.repeatGen == gen
}

// navActionForKey maps a Linux EV_KEY button code to a NavAction on press.
func navActionForKey(code uint16) (NavAction, bool) {
	switch code {
	case 304: // BTN_SOUTH / physical B — cancel/exit
		return NavCancel, true
	case 305: // BTN_EAST / physical A — confirm/activate the focused menu item
		return NavConfirm, true
	case 315: // BTN_START
		return NavSave, true
	case 316: // BTN_MODE / menu button
		return NavMenu, true
	case 308: // BTN_WEST / physical X — speak a random quote
		return NavQuote, true
	case 307: // BTN_NORTH / physical Y — next face / animation
		return NavGallery, true
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
