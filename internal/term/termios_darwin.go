//go:build darwin

package term

// note: darwin's termios layout differs just enough to need its own file

import "syscall"

// termios ioctl requests for macOS and the BSDs.
const (
	getTermios = syscall.TIOCGETA
	setTermios = syscall.TIOCSETA
)
