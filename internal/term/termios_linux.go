//go:build linux

package term

import "syscall"

// termios ioctl requests for Linux.
const (
	getTermios = syscall.TCGETS
	setTermios = syscall.TCSETS
)
