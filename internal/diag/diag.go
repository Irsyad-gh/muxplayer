// Package diag writes muxplayer's diagnostics to stderr.
// Debug lines stay quiet unless MUXPLAYER_DEBUG is set.
package diag

// note: env is read once at startup, so changing MUXPLAYER_DEBUG later does nothing

import (
	"fmt"
	"os"
	"sync"
)

const (
	levelDebug = "DEBUG"
	levelWarn  = "WARN"
	levelError = "ERROR"
)

var (
	debugOn = os.Getenv("MUXPLAYER_DEBUG") != ""
	logMu   sync.Mutex
)

func logf(level, format string, a ...any) {
	if level == levelDebug && !debugOn {
		return
	}
	logMu.Lock()
	defer logMu.Unlock()
	fmt.Fprintf(os.Stderr, "%-5s muxplayer: %s\n", level, fmt.Sprintf(format, a...))
}

// Debugf logs a diagnostic line only when MUXPLAYER_DEBUG is set.
func Debugf(format string, a ...any) { logf(levelDebug, format, a...) }

// Warnf reports a recoverable problem.
func Warnf(format string, a ...any) { logf(levelWarn, format, a...) }

// Errf reports a fatal problem.
func Errf(format string, a ...any) { logf(levelError, format, a...) }
