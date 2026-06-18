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
