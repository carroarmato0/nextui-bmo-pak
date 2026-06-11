package devctx

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/carroarmato0/nextui-bmo/internal/config"
)

// fakeCollector counts calls and returns a canned section or error.
type fakeCollector struct {
	key     string
	section Section
	err     error
	calls   int
}

func (f *fakeCollector) Key() string { return f.key }
func (f *fakeCollector) Collect(now time.Time) (Section, error) {
	f.calls++
	return f.section, f.err
}

func allEnabled() config.DeviceContext { return config.DefaultDeviceContext() }

func testBuilder(collectors ...Collector) (*Builder, *time.Time) {
	b := NewBuilder(collectors, 30*time.Second, 1)
	b.SetEnabled(allEnabled())
	now := time.Date(2026, 6, 11, 16, 45, 0, 0, time.UTC)
	b.SetClock(func() time.Time { return now })
	return b, &now
}

func TestSnapshotFormatsSectionsWithClockAnchor(t *testing.T) {
	lib := &fakeCollector{key: KeyLibrary, section: Section{Key: KeyLibrary, Title: "GAME LIBRARY", Body: "4 games."}}
	b, _ := testBuilder(lib)
	got := b.Snapshot()
	if !strings.Contains(got, "DEVICE AWARENESS") {
		t.Errorf("missing header: %q", got)
	}
	if !strings.Contains(got, "It is Thursday, 2026-06-11 16:45.") {
		t.Errorf("missing clock anchor: %q", got)
	}
	if !strings.Contains(got, "GAME LIBRARY: 4 games.") {
		t.Errorf("missing section: %q", got)
	}
}

func TestSnapshotOmitsDisabledAndFailingCollectors(t *testing.T) {
	lib := &fakeCollector{key: KeyLibrary, section: Section{Key: KeyLibrary, Title: "GAME LIBRARY", Body: "4 games."}}
	saves := &fakeCollector{key: KeySaves, err: fmt.Errorf("boom")}
	b, _ := testBuilder(lib, saves)
	dc := allEnabled()
	dc.Library = false
	b.SetEnabled(dc)
	got := b.Snapshot()
	if strings.Contains(got, "GAME LIBRARY") {
		t.Errorf("disabled section leaked: %q", got)
	}
	if lib.calls != 0 {
		t.Errorf("disabled collector was invoked %d times", lib.calls)
	}
	if !strings.Contains(got, "It is ") {
		t.Errorf("worst case must still carry the clock anchor: %q", got)
	}
}

func TestSnapshotCachesWithinTTL(t *testing.T) {
	lib := &fakeCollector{key: KeyLibrary, section: Section{Key: KeyLibrary, Title: "GAME LIBRARY", Body: "4 games."}}
	b, now := testBuilder(lib)
	b.Snapshot()
	b.Snapshot()
	if lib.calls != 1 {
		t.Fatalf("expected 1 collect within TTL, got %d", lib.calls)
	}
	*now = now.Add(31 * time.Second)
	b.Snapshot()
	if lib.calls != 2 {
		t.Fatalf("expected recollect after TTL, got %d", lib.calls)
	}
}

func TestSetEnabledInvalidatesCache(t *testing.T) {
	lib := &fakeCollector{key: KeyLibrary, section: Section{Key: KeyLibrary, Title: "GAME LIBRARY", Body: "4 games."}}
	b, _ := testBuilder(lib)
	b.Snapshot()
	b.SetEnabled(allEnabled()) // settings touched: cache must drop
	b.Snapshot()
	if lib.calls != 2 {
		t.Fatalf("expected recollect after SetEnabled, got %d", lib.calls)
	}
}
