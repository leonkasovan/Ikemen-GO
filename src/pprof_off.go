//go:build !debug

package main

// startProfiler is a no-op in release builds. Build with -tags debug to
// enable the pprof HTTP server (see pprof_debug.go).
func startProfiler() {}
