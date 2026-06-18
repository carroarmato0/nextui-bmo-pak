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
