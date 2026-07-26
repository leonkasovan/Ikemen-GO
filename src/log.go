package main

// LogLevel represents the severity of a log message.
type LogLevel int

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelWarn
	LevelError
)

// levelPrefix returns the 4-character level tag for log output.
func levelPrefix(l LogLevel) string {
	switch l {
	case LevelDebug:
		return "DEBG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERRO"
	default:
		return "INFO"
	}
}

// Replaces sys.errLog print calls because those only worked on Windows if Ikemen was built with a paired terminal.
// LogMessage writes an info-level message to stderr with source location.
func LogMessage(format string, a ...any) {
	logWrite(LevelInfo, 2, format, a...)
}

// LogDebug writes a debug-level message to stderr.
func LogDebug(format string, a ...any) {
	logWrite(LevelDebug, 2, format, a...)
}

// LogWarn writes a warning-level message to stderr.
func LogWarn(format string, a ...any) {
	logWrite(LevelWarn, 2, format, a...)
}

// LogError writes an error-level message to stderr.
func LogError(format string, a ...any) {
	logWrite(LevelError, 2, format, a...)
}

