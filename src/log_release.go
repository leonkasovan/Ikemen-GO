//go:build !debug

package main

// logWrite is a no-op in release builds. Build with -tags debug to enable.
func logWrite(level LogLevel, calldepth int, format string, a ...any) {}
