package main

import (
	"testing"
	"time"

	"github.com/carroarmato0/nextui-bmo/internal/config"
)

func newTestController() *wakeController {
	return &wakeController{
		now:          time.Now(),
		guardTail:    wakeGuardTail,
		maxFollowUps: wakeMaxFollowUp,
	}
}

func TestWakeIgnoredWhileSpeaking(t *testing.T) {
	c := newTestController()
	c.speaking = true
	if c.onDetection(c.now) {
		t.Fatal("must not trigger while BMO is speaking")
	}
}

func TestWakeIgnoredDuringGuardTail(t *testing.T) {
	c := newTestController()
	c.speaking = false
	c.speechEndedAt = c.now // just stopped speaking
	if c.onDetection(c.now.Add(200 * time.Millisecond)) {
		t.Fatal("must not trigger within guard tail after speech")
	}
	if !c.onDetection(c.now.Add(c.guardTail + time.Millisecond)) {
		t.Fatal("should trigger after guard tail elapses")
	}
}

func TestWakeObserveStateRecordsSpeechEnd(t *testing.T) {
	c := newTestController()
	c.observeState(true, c.now)
	end := c.now.Add(time.Second)
	c.observeState(false, end)
	if c.speaking {
		t.Fatal("should no longer be speaking")
	}
	if !c.speechEndedAt.Equal(end) {
		t.Fatalf("speechEndedAt = %v, want %v", c.speechEndedAt, end)
	}
}

func TestContinuedConversationReopensWindow(t *testing.T) {
	c := newTestController()
	c.continuedWindow = 8 * time.Second
	c.onReplyFinished(c.now)
	if !c.windowOpen(c.now.Add(3 * time.Second)) {
		t.Fatal("follow-up window should be open")
	}
	if c.windowOpen(c.now.Add(9 * time.Second)) {
		t.Fatal("window should have expired")
	}
}

func TestContinuedConversationOffNeverOpens(t *testing.T) {
	c := newTestController()
	c.continuedWindow = 0 // off
	c.onReplyFinished(c.now)
	if c.windowOpen(c.now.Add(time.Second)) {
		t.Fatal("window must stay closed when continued conversation is off")
	}
}

func TestFollowUpCapBacksOff(t *testing.T) {
	c := newTestController()
	c.continuedWindow = 8 * time.Second
	c.maxFollowUps = 2
	for i := 0; i < 2; i++ {
		c.onReplyFinished(c.now)
		c.startFollowUp()
	}
	c.onReplyFinished(c.now)
	if c.windowOpen(c.now.Add(time.Second)) {
		t.Fatal("window must stay closed after max follow-ups")
	}
}

func TestContinuedWindowForMapsModes(t *testing.T) {
	if continuedWindowFor(config.ContinuedConvoShort) != 8*time.Second {
		t.Fatal("short window wrong")
	}
	if continuedWindowFor(config.ContinuedConvoLong) != 20*time.Second {
		t.Fatal("long window wrong")
	}
	if continuedWindowFor("off") != 0 {
		t.Fatal("off window should be zero")
	}
}
