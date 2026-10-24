//go:build darwin

package term

import "syscall"

// termios ioctl requests for macOS and the BSDs.
const (
	getTermios = syscall.TIOCGETA
	setTermios = syscall.TIOCSETA
)
