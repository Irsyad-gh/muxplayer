//go:build linux || darwin

package main

import (
	"os"
	"os/signal"
	"syscall"
)

// watchSignals subscribes to resize and termination signals.
func watchSignals(ch chan<- os.Signal) {
	signal.Notify(ch, syscall.SIGWINCH, syscall.SIGTERM, syscall.SIGHUP)
}

// isResize reports whether sig is a terminal resize notification.
func isResize(sig os.Signal) bool { return sig == syscall.SIGWINCH }
