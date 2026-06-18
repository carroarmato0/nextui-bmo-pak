// Package perf provides opt-in, on-device CPU and memory profiling for
// bmo-pak: Go pprof glue plus a /proc-based whole-process RSS/CPU sampler.
// The sampler captures memory that Go's heap profiler cannot see (SDL
// textures, the CGO heap) — the gap between VmRSS and Go HeapSys.
package perf

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
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
