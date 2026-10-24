//go:build windows

package term

import (
	"os"
	"os/signal"
)

// WatchSignals subscribes to interruption only: Windows has no resize signal,
// so resizing is picked up by the size poll in view.render.
func WatchSignals(ch chan<- os.Signal) {
	signal.Notify(ch, os.Interrupt)
}

// IsResize is always false on Windows.
func IsResize(os.Signal) bool { return false }
