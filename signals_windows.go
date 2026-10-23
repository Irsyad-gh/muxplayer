//go:build windows

package main

import (
	"os"
	"os/signal"
)

// watchSignals subscribes to interruption only: Windows has no resize signal,
// so resizing is picked up by the size poll in view.render.
func watchSignals(ch chan<- os.Signal) {
	signal.Notify(ch, os.Interrupt)
}

// isResize is always false on Windows.
func isResize(os.Signal) bool { return false }
