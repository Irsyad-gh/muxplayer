//go:build darwin

package main

import "syscall"

// termios ioctl requests for macOS and the BSDs.
const (
	getTermios = syscall.TIOCGETA
	setTermios = syscall.TIOCSETA
)
