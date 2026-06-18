# Perf-Testing Capability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add toggleable on-device CPU+memory profiling to bmo-pak — Go pprof plus an in-process RSS/CPU sampler that captures the whole-process footprint (incl. CGO/SDL memory pprof can't see) and survives OOM kills — with a script + skill to collect and analyze it.

**Architecture:** A pure-Go `internal/perf` package provides `/proc`-based RSS/CPU sampling and pprof glue. `cmd/bmo-pak` parses opt-in flags and wires start/stop into the existing graceful-exit path. `launch.sh` injects flags from a `.profile-flags` file; `scripts/debug.sh` toggles that file over ADB; a project skill drives collection and writes a ranked findings doc.

**Tech Stack:** Go (stdlib `flag`, `runtime`, `runtime/pprof`, `net/http/pprof`), `/proc/self/{status,stat}`, ADB, `go tool pprof`.

Spec: `docs/superpowers/specs/2026-06-18-perf-testing-capability-design.md`

---

## File Structure

- `internal/perf/sampler.go` (create) — `Sampler`, `/proc` parsers, CPU% math, CSV row formatting.
- `internal/perf/sampler_test.go` (create) — table-driven tests for parsers/math/formatting.
- `internal/perf/pprof.go` (create) — `StartCPUProfile`, `WriteHeapProfile`, `StartLiveServer`, `Logger` interface.
- `internal/perf/pprof_test.go` (create) — CPU-profile smoke test.
- `cmd/bmo-pak/perf.go` (create) — `perfFlags`, `parsePerfFlags`.
- `cmd/bmo-pak/main.go` (modify, ~after line 175) — wire perf start/stop.
- `launch.sh` (modify) — `.profile-flags` passthrough.
- `scripts/debug.sh` (create) — profiling subcommands over ADB.
- `.claude/skills/bmo-pak-profiling/SKILL.md` (create) — collection + analysis workflow.
- `docs/profiling-findings-TEMPLATE.md` (create) — findings doc template.

**Conventions** (verify before starting):
- Tests: `CGO_ENABLED=1 go test ./...`; the `internal/perf` package is pure-Go and also passes under `CGO_ENABLED=0 go test ./internal/perf/`.
- Lint: `golangci-lint run ./...` after every change; new code must add no findings.
- No `Co-Authored-By` trailer in commits (project rule).

---

## Task 1: `/proc` parsers and CPU% math

**Files:**
- Create: `internal/perf/sampler.go` (parsers + helpers only in this task)
- Test: `internal/perf/sampler_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/perf/sampler_test.go`:

```go
package perf

import (
	"strings"
	"testing"
	"time"
)

func TestParseVmRSSKB(t *testing.T) {
	const status = `VmPeak:	  123456 kB
VmSize:	  120000 kB
VmRSS:	   45678 kB
VmData:	   10000 kB
`
	got, err := parseVmRSSKB(strings.NewReader(status))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 45678 {
		t.Fatalf("VmRSS = %d, want 45678", got)
	}
}

func TestParseVmRSSKBMissing(t *testing.T) {
	if _, err := parseVmRSSKB(strings.NewReader("VmSize:\t100 kB\n")); err == nil {
		t.Fatal("expected error when VmRSS absent")
	}
}

func TestParseProcStatCPU(t *testing.T) {
	// comm "(bmo pak)" deliberately contains a space and parens-adjacent text
	// to exercise last-')' splitting. Fields after state: utime is field 14,
	// stime field 15 (1-indexed in proc(5)).
	stat := "4242 (bmo pak) R 1 4242 4242 0 -1 0 0 0 0 0 " +
		"731 412 0 0 20 0 8 0 99999 1234 567 rest ignored\n"
	utime, stime, err := parseProcStatCPU(strings.NewReader(stat))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if utime != 731 || stime != 412 {
		t.Fatalf("utime,stime = %d,%d want 731,412", utime, stime)
	}
}

func TestCPUPercent(t *testing.T) {
	// 50 ticks over 1s at 100 ticks/s = 0.5 CPU-seconds = 50%.
	got := cpuPercent(100, 150, time.Second)
	if got < 49.9 || got > 50.1 {
		t.Fatalf("cpuPercent = %f, want ~50", got)
	}
	// Zero wall duration must not divide-by-zero.
	if v := cpuPercent(100, 150, 0); v != 0 {
		t.Fatalf("cpuPercent(zero wall) = %f, want 0", v)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `CGO_ENABLED=0 go test ./internal/perf/ -run 'TestParseVmRSSKB|TestParseProcStatCPU|TestCPUPercent' -v`
Expected: FAIL — build error, `undefined: parseVmRSSKB` etc.

- [ ] **Step 3: Write the parsers**

Create `internal/perf/sampler.go`:

```go
// Package perf provides opt-in, on-device CPU and memory profiling for
// bmo-pak: Go pprof glue plus a /proc-based whole-process RSS/CPU sampler.
// The sampler captures memory that Go's heap profiler cannot see (SDL
// textures, the CGO heap) — the gap between VmRSS and Go HeapSys.
package perf

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// clockTicksPerSec is the kernel's USER_HZ (sysconf(_SC_CLK_TCK)). It is 100 on
// every Linux target we ship to (TrimUI tg5040/tg5050 ARM), so we hardcode it
// rather than cgo-call sysconf from this pure-Go package.
const clockTicksPerSec = 100

// parseVmRSSKB extracts the VmRSS value (in kB) from /proc/<pid>/status text.
func parseVmRSSKB(r io.Reader) (int64, error) {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "VmRSS:") {
			continue
		}
		fields := strings.Fields(line) // ["VmRSS:", "45678", "kB"]
		if len(fields) < 2 {
			return 0, fmt.Errorf("perf: malformed VmRSS line %q", line)
		}
		return strconv.ParseInt(fields[1], 10, 64)
	}
	if err := sc.Err(); err != nil {
		return 0, err
	}
	return 0, fmt.Errorf("perf: VmRSS not found in status")
}

// parseProcStatCPU extracts utime and stime (in clock ticks) from
// /proc/<pid>/stat. The comm field (field 2) may contain spaces and parens, so
// we split on the LAST ')': everything after it is whitespace-delimited with
// state as the first token (field 3). utime is field 14, stime field 15.
func parseProcStatCPU(r io.Reader) (utime, stime uint64, err error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return 0, 0, err
	}
	s := string(data)
	rparen := strings.LastIndexByte(s, ')')
	if rparen < 0 {
		return 0, 0, fmt.Errorf("perf: malformed stat (no comm)")
	}
	// tokens[0] == field 3 (state); field N == tokens[N-3].
	tokens := strings.Fields(s[rparen+1:])
	if len(tokens) < 13 { // need up to field 15 -> index 12
		return 0, 0, fmt.Errorf("perf: stat too short (%d fields)", len(tokens))
	}
	if utime, err = strconv.ParseUint(tokens[11], 10, 64); err != nil {
		return 0, 0, err
	}
	if stime, err = strconv.ParseUint(tokens[12], 10, 64); err != nil {
		return 0, 0, err
	}
	return utime, stime, nil
}

// cpuPercent converts a delta in CPU ticks over a wall-clock interval into a
// percentage (100 == one core fully busy). Returns 0 for a non-positive wall.
func cpuPercent(prevTicks, curTicks uint64, wall time.Duration) float64 {
	if wall <= 0 || curTicks < prevTicks {
		return 0
	}
	cpuSeconds := float64(curTicks-prevTicks) / clockTicksPerSec
	return cpuSeconds / wall.Seconds() * 100
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `CGO_ENABLED=0 go test ./internal/perf/ -run 'TestParseVmRSSKB|TestParseProcStatCPU|TestCPUPercent' -v`
Expected: PASS (4 tests).

- [ ] **Step 5: Lint and commit**

```bash
golangci-lint run ./internal/perf/...
git add internal/perf/sampler.go internal/perf/sampler_test.go
git commit -m "feat(perf): add /proc parsers and CPU% math"
```

---

## Task 2: Sampler (CSV writing + lifecycle)

**Files:**
- Modify: `internal/perf/sampler.go` (add `Sampler`, `Logger`, row formatting)
- Test: `internal/perf/sampler_test.go` (add formatting + integration tests)

- [ ] **Step 1: Write the failing tests**

Append to `internal/perf/sampler_test.go`:

```go
func TestFormatRow(t *testing.T) {
	row := formatRow(sampleRow{
		uptimeS:      12.5,
		state:        "speaking",
		vmrssKB:      45678,
		goHeapAllocK: 2048,
		goHeapSysK:   8192,
		goNumGC:      7,
		cpuPct:       33.3,
		goroutines:   42,
	})
	want := "12.50,speaking,45678,2048,8192,7,33.30,42\n"
	if row != want {
		t.Fatalf("formatRow = %q, want %q", row, want)
	}
}

func TestSamplerWritesHeaderAndRows(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "perf-sample.csv")
	s := NewSampler(path, 10*time.Millisecond, func() string { return "idle" }, testLogger{})
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	time.Sleep(45 * time.Millisecond)
	s.Stop()
	s.Stop() // idempotent

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if lines[0] != csvHeader {
		t.Fatalf("header = %q, want %q", lines[0], csvHeader)
	}
	if len(lines) < 3 { // header + first immediate row + at least one tick + final
		t.Fatalf("expected several rows, got %d:\n%s", len(lines), data)
	}
	if !strings.Contains(lines[1], ",idle,") {
		t.Fatalf("row missing state tag: %q", lines[1])
	}
}

type testLogger struct{}

func (testLogger) Infof(string, ...any)  {}
func (testLogger) Errorf(string, ...any) {}
```

Add imports `os`, `path/filepath` to the test file's import block.

- [ ] **Step 2: Run tests to verify they fail**

Run: `CGO_ENABLED=0 go test ./internal/perf/ -run 'TestFormatRow|TestSamplerWrites' -v`
Expected: FAIL — `undefined: formatRow`, `NewSampler`, `csvHeader`, etc.

- [ ] **Step 3: Implement the Sampler**

Append to `internal/perf/sampler.go` (and add `os`, `runtime`, `sync` to its import block):

```go
// Logger is the minimal logging surface perf needs. *observability.Logger
// satisfies it; keeping it an interface avoids coupling perf to that package.
type Logger interface {
	Infof(format string, args ...any)
	Errorf(format string, args ...any)
}

const csvHeader = "uptime_s,state,vmrss_kb,go_heapalloc_kb,go_heapsys_kb,go_numgc,cpu_pct,goroutines"

type sampleRow struct {
	uptimeS      float64
	state        string
	vmrssKB      int64
	goHeapAllocK uint64
	goHeapSysK   uint64
	goNumGC      uint32
	cpuPct       float64
	goroutines   int
}

func formatRow(r sampleRow) string {
	return fmt.Sprintf("%.2f,%s,%d,%d,%d,%d,%.2f,%d\n",
		r.uptimeS, r.state, r.vmrssKB, r.goHeapAllocK, r.goHeapSysK,
		r.goNumGC, r.cpuPct, r.goroutines)
}

// Sampler periodically appends a whole-process resource row to a CSV file. Each
// row is written straight to the file (no userspace buffering) so the data is
// complete up to the last tick even if the process is OOM-killed.
type Sampler struct {
	path     string
	interval time.Duration
	state    func() string
	log      Logger

	f         *os.File
	prevTicks uint64
	prevTime  time.Time
	start     time.Time

	stopCh chan struct{}
	doneCh chan struct{}
	once   sync.Once
}

// NewSampler creates a sampler. state returns the current app state to tag each
// row with (e.g. machine.State()); it may be nil.
func NewSampler(path string, interval time.Duration, state func() string, log Logger) *Sampler {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	return &Sampler{
		path:     path,
		interval: interval,
		state:    state,
		log:      log,
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
	}
}

// Start opens the file, writes the header and an immediate baseline row, then
// samples on an interval in a background goroutine until Stop.
func (s *Sampler) Start() error {
	f, err := os.Create(s.path)
	if err != nil {
		return err
	}
	s.f = f
	if _, err := io.WriteString(f, csvHeader+"\n"); err != nil {
		_ = f.Close()
		return err
	}
	s.start = time.Now()
	s.prevTime = s.start
	s.prevTicks = s.readTicks() // seed so the first CPU% delta is meaningful
	s.writeSample()             // immediate baseline
	go s.loop()
	return nil
}

func (s *Sampler) loop() {
	defer close(s.doneCh)
	t := time.NewTicker(s.interval)
	defer t.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-t.C:
			s.writeSample()
		}
	}
}

// Stop halts sampling, writes a final row, and closes the file. Idempotent.
func (s *Sampler) Stop() {
	s.once.Do(func() {
		close(s.stopCh)
		<-s.doneCh
		s.writeSample() // final row
		if err := s.f.Close(); err != nil {
			s.log.Errorf("perf: close sample file: %v", err)
		}
	})
}

func (s *Sampler) readTicks() uint64 {
	f, err := os.Open("/proc/self/stat")
	if err != nil {
		return s.prevTicks
	}
	defer f.Close()
	utime, stime, err := parseProcStatCPU(f)
	if err != nil {
		return s.prevTicks
	}
	return utime + stime
}

func (s *Sampler) writeSample() {
	now := time.Now()

	var vmrss int64
	if f, err := os.Open("/proc/self/status"); err == nil {
		vmrss, _ = parseVmRSSKB(f)
		_ = f.Close()
	}

	curTicks := s.readTicks()
	cpu := cpuPercent(s.prevTicks, curTicks, now.Sub(s.prevTime))
	s.prevTicks = curTicks
	s.prevTime = now

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	state := ""
	if s.state != nil {
		state = s.state()
	}

	row := formatRow(sampleRow{
		uptimeS:      now.Sub(s.start).Seconds(),
		state:        state,
		vmrssKB:      vmrss,
		goHeapAllocK: ms.HeapAlloc / 1024,
		goHeapSysK:   ms.HeapSys / 1024,
		goNumGC:      ms.NumGC,
		cpuPct:       cpu,
		goroutines:   runtime.NumGoroutine(),
	})
	if _, err := io.WriteString(s.f, row); err != nil {
		s.log.Errorf("perf: write sample: %v", err)
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `CGO_ENABLED=0 go test ./internal/perf/ -v`
Expected: PASS (all parser, format, and sampler tests).

- [ ] **Step 5: Lint and commit**

```bash
golangci-lint run ./internal/perf/...
git add internal/perf/sampler.go internal/perf/sampler_test.go
git commit -m "feat(perf): add state-tagged RSS/CPU CSV sampler"
```

---

## Task 3: pprof glue

**Files:**
- Create: `internal/perf/pprof.go`
- Test: `internal/perf/pprof_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/perf/pprof_test.go`:

```go
package perf

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStartCPUProfileWritesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cpu.prof")
	stop, err := StartCPUProfile(path)
	if err != nil {
		t.Fatalf("StartCPUProfile: %v", err)
	}
	// Burn a little CPU so the profile has at least one sample.
	x := 0
	for i := 0; i < 5_000_000; i++ {
		x += i
	}
	_ = x
	stop()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat profile: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("cpu profile is empty")
	}
}

func TestWriteHeapProfileWritesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "heap.prof")
	if err := WriteHeapProfile(path); err != nil {
		t.Fatalf("WriteHeapProfile: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat profile: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("heap profile is empty")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `CGO_ENABLED=0 go test ./internal/perf/ -run 'TestStartCPUProfile|TestWriteHeapProfile' -v`
Expected: FAIL — `undefined: StartCPUProfile`, `WriteHeapProfile`.

- [ ] **Step 3: Implement pprof glue**

Create `internal/perf/pprof.go`:

```go
package perf

import (
	"net/http"
	_ "net/http/pprof" // registers /debug/pprof handlers on DefaultServeMux
	"os"
	"runtime"
	"runtime/pprof"
)

// StartCPUProfile begins writing a CPU profile to path and returns a stop func
// that ends the profile and closes the file. Call stop exactly once.
func StartCPUProfile(path string) (stop func(), err error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	if err := pprof.StartCPUProfile(f); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() {
		pprof.StopCPUProfile()
		_ = f.Close()
	}, nil
}

// WriteHeapProfile runs a GC then writes a heap profile to path.
func WriteHeapProfile(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	runtime.GC() // get up-to-date statistics
	return pprof.WriteHeapProfile(f)
}

// StartLiveServer serves net/http/pprof on addr in a background goroutine.
// Errors are logged, never fatal.
func StartLiveServer(addr string, log Logger) {
	go func() {
		log.Infof("perf: live pprof listening on %s", addr)
		if err := http.ListenAndServe(addr, nil); err != nil { //nolint:gosec // debug-only, opt-in
			log.Errorf("perf: pprof server: %v", err)
		}
	}()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `CGO_ENABLED=0 go test ./internal/perf/ -run 'TestStartCPUProfile|TestWriteHeapProfile' -v`
Expected: PASS (2 tests).

- [ ] **Step 5: Lint and commit**

```bash
golangci-lint run ./internal/perf/...
git add internal/perf/pprof.go internal/perf/pprof_test.go
git commit -m "feat(perf): add cpu/heap profile + live pprof glue"
```

---

## Task 4: Flag parsing helper

**Files:**
- Create: `cmd/bmo-pak/perf.go`
- Test: `cmd/bmo-pak/perf_test.go`

- [ ] **Step 1: Write the failing test**

Create `cmd/bmo-pak/perf_test.go`:

```go
package main

import (
	"testing"
	"time"
)

func TestParsePerfFlags(t *testing.T) {
	pf := parsePerfFlags([]string{
		"-cpuprofile", "/tmp/cpu.prof",
		"-memprofile", "/tmp/mem.prof",
		"-pprof", ":6060",
		"-perfsample", "/tmp/s.csv",
		"-perfinterval", "1s",
		"somepositional", "-unknownflag",
	})
	if pf.cpuProfile != "/tmp/cpu.prof" {
		t.Errorf("cpuProfile = %q", pf.cpuProfile)
	}
	if pf.memProfile != "/tmp/mem.prof" {
		t.Errorf("memProfile = %q", pf.memProfile)
	}
	if pf.pprofAddr != ":6060" {
		t.Errorf("pprofAddr = %q", pf.pprofAddr)
	}
	if pf.sampleFile != "/tmp/s.csv" {
		t.Errorf("sampleFile = %q", pf.sampleFile)
	}
	if pf.interval != time.Second {
		t.Errorf("interval = %s", pf.interval)
	}
}

func TestParsePerfFlagsDefaults(t *testing.T) {
	pf := parsePerfFlags(nil)
	if pf.cpuProfile != "" || pf.sampleFile != "" || pf.pprofAddr != "" {
		t.Errorf("expected empty paths, got %+v", pf)
	}
	if pf.interval != 2*time.Second {
		t.Errorf("default interval = %s, want 2s", pf.interval)
	}
	if pf.enabled() {
		t.Error("enabled() should be false with no flags")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `CGO_ENABLED=1 go test ./cmd/bmo-pak/ -run TestParsePerfFlags -v`
Expected: FAIL — `undefined: parsePerfFlags`.

- [ ] **Step 3: Implement the flag helper**

Create `cmd/bmo-pak/perf.go`:

```go
package main

import (
	"flag"
	"io"
	"time"
)

// perfFlags holds opt-in profiling options parsed from the command line. Empty
// fields mean that profiling facet is disabled (zero overhead).
type perfFlags struct {
	cpuProfile string
	memProfile string
	pprofAddr  string
	sampleFile string
	interval   time.Duration
}

func (p perfFlags) enabled() bool {
	return p.cpuProfile != "" || p.memProfile != "" || p.pprofAddr != "" || p.sampleFile != ""
}

// parsePerfFlags parses perf flags from args (typically os.Args[1:]). It uses a
// private FlagSet with ContinueOnError and ignores parse errors and unknown/
// positional args: launch.sh places $PROFILE_FLAGS ahead of NextUI's own args,
// and profiling must never abort startup.
func parsePerfFlags(args []string) perfFlags {
	fs := flag.NewFlagSet("bmo-pak-perf", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var p perfFlags
	fs.StringVar(&p.cpuProfile, "cpuprofile", "", "write CPU profile to file")
	fs.StringVar(&p.memProfile, "memprofile", "", "write heap profile to file on exit")
	fs.StringVar(&p.pprofAddr, "pprof", "", "serve live pprof on addr (e.g. :6060)")
	fs.StringVar(&p.sampleFile, "perfsample", "", "write RSS/CPU CSV to file")
	fs.DurationVar(&p.interval, "perfinterval", 2*time.Second, "RSS/CPU sample interval")
	_ = fs.Parse(args)
	return p
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `CGO_ENABLED=1 go test ./cmd/bmo-pak/ -run TestParsePerfFlags -v`
Expected: PASS (2 tests).

- [ ] **Step 5: Lint and commit**

```bash
golangci-lint run ./cmd/bmo-pak/...
git add cmd/bmo-pak/perf.go cmd/bmo-pak/perf_test.go
git commit -m "feat(perf): parse opt-in profiling flags"
```

---

## Task 5: Wire perf into main.go

**Files:**
- Modify: `cmd/bmo-pak/main.go` (import; insert block after the initial-state logging, ~line 175)

No new test (wiring is exercised end-to-end on device; flag parsing and perf internals are unit-tested). This task is verified by build + lint.

- [ ] **Step 1: Add the import**

In `cmd/bmo-pak/main.go`, add to the import block (keep grouping/ordering consistent with neighbors):

```go
	"github.com/carroarmato0/nextui-bmo/internal/perf"
```

- [ ] **Step 2: Insert the wiring block**

In `cmd/bmo-pak/main.go`, immediately AFTER this existing line (~175):

```go
	logger.Debugf("assistant snapshot: %+v", machine.Snapshot())
```

insert:

```go
	// Opt-in profiling. All facets are inert unless their flag is set via the
	// .profile-flags file that launch.sh injects. Stop/flush hooks are deferred
	// so they run on the graceful-exit path (the same path that presents black
	// 3x); a kill -9 loses the final flush, which is why each sampler row is
	// written immediately rather than buffered.
	if pf := parsePerfFlags(os.Args[1:]); pf.enabled() {
		if pf.cpuProfile != "" {
			if stop, err := perf.StartCPUProfile(pf.cpuProfile); err != nil {
				logger.Errorf("cpuprofile: %v", err)
			} else {
				logger.Infof("cpuprofile: writing to %s", pf.cpuProfile)
				defer stop()
			}
		}
		if pf.pprofAddr != "" {
			perf.StartLiveServer(pf.pprofAddr, logger)
		}
		if pf.sampleFile != "" {
			sampler := perf.NewSampler(pf.sampleFile, pf.interval,
				func() string { return string(machine.State()) }, logger)
			if err := sampler.Start(); err != nil {
				logger.Errorf("perfsample: %v", err)
			} else {
				logger.Infof("perfsample: writing to %s every %s", pf.sampleFile, pf.interval)
				defer sampler.Stop()
			}
		}
		if pf.memProfile != "" {
			memProfile := pf.memProfile
			defer func() {
				if err := perf.WriteHeapProfile(memProfile); err != nil {
					logger.Errorf("memprofile: %v", err)
				} else {
					logger.Infof("memprofile: written to %s", memProfile)
				}
			}()
		}
	}
```

- [ ] **Step 3: Build and verify the full suite**

Run: `CGO_ENABLED=1 go build ./... && CGO_ENABLED=1 go test ./... 2>&1 | grep -vE '\[no test files\]|^ok|Cannot process svg element'; echo "(empty above = all pass)"`
Expected: build succeeds; no FAIL lines.

- [ ] **Step 4: Lint and commit**

```bash
golangci-lint run ./...
git add cmd/bmo-pak/main.go
git commit -m "feat(perf): wire profiling into bmo-pak startup/shutdown"
```

---

## Task 6: launch.sh `.profile-flags` passthrough

**Files:**
- Modify: `launch.sh` (the repo copy; `scripts/release.sh` regenerates the `dist/` copies)

- [ ] **Step 1: Edit launch.sh**

In `launch.sh`, replace the final line:

```sh
exec "$PAK_DIR/bin/$PLATFORM/bmo-pak" "$@"
```

with:

```sh
# Opt-in profiling: scripts/debug.sh writes flags here; profile-restore removes
# it. Absent in normal use, so this is a no-op for end users.
PROFILE_FLAGS=""
if [ -f "$PAK_DIR/.profile-flags" ]; then
    PROFILE_FLAGS="$(cat "$PAK_DIR/.profile-flags")"
fi
# shellcheck disable=SC2086 # word-splitting of PROFILE_FLAGS is intentional
exec "$PAK_DIR/bin/$PLATFORM/bmo-pak" $PROFILE_FLAGS "$@"
```

- [ ] **Step 2: Verify the script still parses**

Run: `sh -n launch.sh && echo "syntax OK"`
Expected: `syntax OK`.

- [ ] **Step 3: Commit**

```bash
git add launch.sh
git commit -m "feat(perf): inject .profile-flags from launch.sh"
```

---

## Task 7: scripts/debug.sh

**Files:**
- Create: `scripts/debug.sh`

- [ ] **Step 1: Write the script**

Create `scripts/debug.sh`:

```sh
#!/bin/sh
# On-device profiling helper for bmo-pak over ADB.
#
# Workflow:
#   ./scripts/debug.sh profile          # enable cpu+mem+sample flags
#   # launch BMO via NextUI, exercise workloads, then exit BMO gracefully
#   ./scripts/debug.sh pull-profile     # fetch profiles + CSV to ./debug-profiles/
#   ./scripts/debug.sh profile-restore  # remove flags
#   go tool pprof bin/$PLATFORM/bmo-pak debug-profiles/bmo-cpu.prof
set -e

PLATFORM="${BMO_PLATFORM:-tg5040}"
PAK_DEST="/mnt/SDCARD/Tools/$PLATFORM/BMO.pak"
DEV_TMP="/tmp"
PROF_DIR="$(pwd)/debug-profiles"

CPU_PROF="$DEV_TMP/bmo-cpu.prof"
MEM_PROF="$DEV_TMP/bmo-mem.prof"
SAMPLE_CSV="$DEV_TMP/bmo-perf-sample.csv"

usage() {
    cat <<EOF
Usage: $0 <command>

  profile          Enable CPU+memory+RSS-sample flags; launch via NextUI to record
  profile-cpu      Enable CPU-only profiling
  profile-mem      Enable heap-only profiling
  profile-sample   Enable RSS/CPU CSV sampling only
  profile-live     Enable live pprof (HTTP :6060 via ADB forward)
  profile-restore  Remove profiling flags (restores normal launch)
  pull-profile     Pull recorded profiles + CSV to ./debug-profiles/

Platform defaults to tg5040; override with BMO_PLATFORM=tg5050.
EOF
}

write_flags() {
    adb shell "printf '%s' '$1' > $PAK_DEST/.profile-flags"
    echo "==> wrote flags: $1"
    echo "    Now launch BMO via NextUI, exercise workloads, then exit BMO."
    echo "    Then: $0 pull-profile && $0 profile-restore"
}

case "${1:-}" in
    profile)
        write_flags "-cpuprofile $CPU_PROF -memprofile $MEM_PROF -perfsample $SAMPLE_CSV"
        ;;
    profile-cpu)
        write_flags "-cpuprofile $CPU_PROF"
        ;;
    profile-mem)
        write_flags "-memprofile $MEM_PROF"
        ;;
    profile-sample)
        write_flags "-perfsample $SAMPLE_CSV"
        ;;
    profile-live)
        adb shell "printf '%s' '-pprof :6060' > $PAK_DEST/.profile-flags"
        adb forward tcp:6060 tcp:6060
        echo "==> live pprof enabled; forwarded localhost:6060 -> device:6060"
        echo "    Launch BMO via NextUI, then from the host:"
        echo "      go tool pprof 'http://localhost:6060/debug/pprof/profile?seconds=30'"
        echo "      go tool pprof http://localhost:6060/debug/pprof/heap"
        echo "    Then: $0 profile-restore"
        ;;
    profile-restore)
        adb shell "rm -f $PAK_DEST/.profile-flags"
        adb forward --remove tcp:6060 2>/dev/null || true
        echo "==> removed .profile-flags and any :6060 forward"
        ;;
    pull-profile)
        mkdir -p "$PROF_DIR"
        for f in "$CPU_PROF" "$MEM_PROF" "$SAMPLE_CSV"; do
            if adb shell "[ -f $f ] && echo yes" | grep -q yes; then
                adb pull "$f" "$PROF_DIR/"
            fi
        done
        echo "==> pulled available profiles to $PROF_DIR"
        ls -la "$PROF_DIR"
        ;;
    *)
        usage
        exit 1
        ;;
esac
```

- [ ] **Step 2: Make executable and syntax-check**

```bash
chmod +x scripts/debug.sh
sh -n scripts/debug.sh && echo "syntax OK"
```
Expected: `syntax OK`.

- [ ] **Step 3: Verify usage prints**

Run: `./scripts/debug.sh 2>&1 | head -3`
Expected: prints the usage block (exit 1 is fine).

- [ ] **Step 4: Commit**

```bash
git add scripts/debug.sh
git commit -m "feat(perf): add scripts/debug.sh profiling helper"
```

---

## Task 8: Profiling skill

**Files:**
- Create: `.claude/skills/bmo-pak-profiling/SKILL.md`

- [ ] **Step 1: Write the skill**

Create `.claude/skills/bmo-pak-profiling/SKILL.md`:

````markdown
---
name: bmo-pak-profiling
description: Use when profiling bmo-pak CPU/memory on a live TrimUI device — enabling on-device profiling flags, exercising BMO's workloads, pulling pprof + RSS/CPU samples, and writing a ranked findings doc. BMO's recurring failure is OOM, and the RSS sample exposes the SDL/CGO memory that Go's pprof cannot see.
---

# BMO Pak — On-Device Profiling

Toggleable profiling lives behind a `.profile-flags` file that `launch.sh`
injects. `scripts/debug.sh` toggles it over ADB. The binary supports:
`-cpuprofile`, `-memprofile`, `-pprof :addr`, `-perfsample <csv>`,
`-perfinterval <dur>`.

**Key insight:** Go's heap profile only sees Go allocations. The RSS sampler's
`vmrss_kb` minus `go_heapsys_kb` is the SDL-texture/CGO footprint — usually the
real driver of BMO's OOM. Always read the CSV, not just the heap profile.

## Prerequisites

- Device connected via ADB (`adb devices` lists it). ADB enabled in NextUI
  Settings → Developer.
- A current build deployed: `./scripts/deploy.sh`.

## Collection workflow

1. Enable profiling (pick one):
   - `./scripts/debug.sh profile`        — CPU + heap + RSS sample (default)
   - `./scripts/debug.sh profile-sample` — RSS/CPU CSV only (lowest overhead)
   - `./scripts/debug.sh profile-live`   — live pprof on :6060
2. **Launch BMO via NextUI** (not a manual ADB launch — a manual launch reads a
   phantom config path and won't reflect real behaviour).
3. Exercise workloads in sequence, noting wall-clock times so CSV rows can be
   segmented by state:
   - idle (default face), ~30s
   - idle long enough to trigger idle animations (whistle / look_around)
   - listening + thinking (push-to-talk a query)
   - speaking (let a TTS reply play fully)
4. **Exit BMO gracefully** (B/BTN_SOUTH to exit) so the heap profile and final
   CSV row flush. A `kill -9` loses the heap profile and the last sample.
5. `./scripts/debug.sh pull-profile`  — pulls to `./debug-profiles/`.
6. `./scripts/debug.sh profile-restore`.

## Analysis

- CPU: `go tool pprof -top -nodecount=20 bin/tg5040/bmo-pak debug-profiles/bmo-cpu.prof`
  (use `-cum` for cumulative). `-list <func>` to see hot lines.
- Heap: `go tool pprof -top -sample_index=inuse_space bin/tg5040/bmo-pak debug-profiles/bmo-mem.prof`.
- RSS over time: read `debug-profiles/bmo-perf-sample.csv`. Columns:
  `uptime_s,state,vmrss_kb,go_heapalloc_kb,go_heapsys_kb,go_numgc,cpu_pct,goroutines`.
  - Plot/scan `vmrss_kb` per `state`: which state drives peak RSS?
  - Compute `vmrss_kb - go_heapsys_kb` = non-Go (SDL/CGO) memory. A large or
    growing gap points at textures/surfaces, not Go allocations.
  - Rising `vmrss_kb` across repeated cycles of the same state = a leak.

## Output

Write `docs/profiling-findings-<YYYY-MM-DD>.md` from
`docs/profiling-findings-TEMPLATE.md`. Fill every section. End with an
**Action Items** list ranked by impact (highest-RSS/CPU win first), each with
the evidence (function, line, or CSV observation) that justifies it.

**Stop after the findings doc.** Do not implement fixes automatically — present
the ranked action items and offer to implement the top ones as separate,
user-approved changes.
````

- [ ] **Step 2: Commit**

```bash
git add .claude/skills/bmo-pak-profiling/SKILL.md
git commit -m "feat(perf): add bmo-pak-profiling skill"
```

---

## Task 9: Findings doc template

**Files:**
- Create: `docs/profiling-findings-TEMPLATE.md`

- [ ] **Step 1: Write the template**

Create `docs/profiling-findings-TEMPLATE.md`:

```markdown
# Profiling Findings — bmo-pak (YYYY-MM-DD)

## Environment

- Device / platform: (tg5040 Brick / Smart Pro | tg5050 Smart Pro S)
- Framebuffer resolution:
- Build / commit:
- Flags used:

## Workloads exercised

| Workload            | uptime_s range | Notes |
|---------------------|----------------|-------|
| idle                |                |       |
| idle animations     |                |       |
| listening + thinking|                |       |
| speaking            |                |       |

## CPU findings

(`go tool pprof -top` highlights; hot functions and why.)

## Heap findings (Go allocations)

(`inuse_space` top; notable Go-side allocators.)

## RSS-over-time findings (whole process)

- Peak VmRSS: __ kB during state: __
- Non-Go footprint (VmRSS − HeapSys): __ kB — i.e. SDL textures / CGO.
- Leak check (RSS across repeated same-state cycles): (stable / growing)

## Action Items (ranked by impact)

1. **[highest impact]** — evidence: (func / line / CSV observation). Expected win:
2.
3.
```

- [ ] **Step 2: Commit**

```bash
git add docs/profiling-findings-TEMPLATE.md
git commit -m "docs(perf): add profiling-findings template"
```

---

## Final Verification

- [ ] `CGO_ENABLED=0 go test ./internal/perf/ -v` — all perf tests pass.
- [ ] `CGO_ENABLED=1 go build ./... && CGO_ENABLED=1 go test ./...` — full build + suite green.
- [ ] `golangci-lint run ./...` — no new findings.
- [ ] `sh -n launch.sh && sh -n scripts/debug.sh` — scripts parse.
- [ ] (On device) `./scripts/deploy.sh && ./scripts/debug.sh profile`, launch via NextUI, exercise workloads, exit, `./scripts/debug.sh pull-profile` — `debug-profiles/` contains `bmo-cpu.prof`, `bmo-mem.prof`, `bmo-perf-sample.csv` with a header + state-tagged rows.
```
