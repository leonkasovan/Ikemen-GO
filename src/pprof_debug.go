//go:build debug

package main

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
)

// ProfilePort is the port the pprof HTTP server listens on in debug builds.
const ProfilePort = 6060

// startProfiler launches a pprof HTTP server on localhost:ProfilePort so you
// can capture heap and CPU profiles with:
//
//	go tool pprof http://localhost:6060/debug/pprof/heap
//	go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30
//
// Only built with -tags debug; release builds stub this out entirely.
func startProfiler() {
	addr := fmt.Sprintf("localhost:%d", ProfilePort)
	LogMessage("[PProf] heap profile server on http://%s/debug/pprof/heap", addr)
	LogMessage("[PProf] run: go tool pprof http://%s/debug/pprof/heap", addr)
	go func() {
		if err := http.ListenAndServe(addr, nil); err != nil {
			LogMessage("[PProf] server error: %v", err)
		}
	}()
}
