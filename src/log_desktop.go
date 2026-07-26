//go:build debug && !android

package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"
)

// logWrite writes a formatted log message to stderr with timestamp, level, and source location.
func logWrite(level LogLevel, calldepth int, format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	now := time.Now().Format("15:04:05")
	_, file, line, ok := runtime.Caller(calldepth)
	prefix := levelPrefix(level)
	if ok {
		// Shorten the file path to just the last component (e.g. "render_gl33.go")
		if idx := strings.LastIndexByte(file, '/'); idx >= 0 {
			file = file[idx+1:]
		}
		fmt.Fprintf(os.Stderr, "[%s] %s %s:%d: %s\n", prefix, now, file, line, msg)
	} else {
		fmt.Fprintf(os.Stderr, "[%s] %s %s\n", prefix, now, msg)
	}
}
