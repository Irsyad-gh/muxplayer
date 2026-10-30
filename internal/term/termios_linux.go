//go:build linux

package term

// note: linux termios is the easy one, for once

import "syscall"

// termios ioctl requests for Linux.
const (
	getTermios = syscall.TCGETS
	setTermios = syscall.TCSETS
)
