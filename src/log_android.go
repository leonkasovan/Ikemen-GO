//go:build debug && android

package main

/*
#cgo LDFLAGS: -llog
#include <android/log.h>
*/
import "C"
import (
	"fmt"
	"unsafe"
)

// logWrite writes a formatted log message to Android's logcat with appropriate priority.
func logWrite(level LogLevel, calldepth int, format string, a ...any) {
	msg := fmt.Sprintf(format, a...)

	var androidPrio C.int
	switch level {
	case LevelDebug:
		androidPrio = C.ANDROID_LOG_DEBUG
	case LevelInfo:
		androidPrio = C.ANDROID_LOG_INFO
	case LevelWarn:
		androidPrio = C.ANDROID_LOG_WARN
	case LevelError:
		androidPrio = C.ANDROID_LOG_ERROR
	default:
		androidPrio = C.ANDROID_LOG_INFO
	}

	cs := C.CString(msg)
	tag := C.CString("ikemen")
	C.__android_log_write(androidPrio, tag, cs)
	C.free(unsafe.Pointer(cs))
	C.free(unsafe.Pointer(tag))
}
