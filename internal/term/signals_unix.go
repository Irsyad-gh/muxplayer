//go:build linux || darwin

package term

import (
	"os"
	"os/signal"
	"syscall"
)

// WatchSignals subscribes to resize and termination signals.
func WatchSignals(ch chan<- os.Signal) {
	signal.Notify(ch, syscall.SIGWINCH, syscall.SIGTERM, syscall.SIGHUP)
}

// IsResize reports whether sig is a terminal resize notification.
func IsResize(sig os.Signal) bool { return sig == syscall.SIGWINCH }
