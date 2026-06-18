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
