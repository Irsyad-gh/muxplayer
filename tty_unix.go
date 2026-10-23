//go:build linux || darwin

package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

func ioctl(fd, req uintptr, p unsafe.Pointer) {
	syscall.Syscall(syscall.SYS_IOCTL, fd, req, uintptr(p))
}

// rawMode switches the tty to raw input and the alternate screen. The returned
// function restores the previous terminal state.
func rawMode() func() {
	var t syscall.Termios
	ioctl(0, getTermios, unsafe.Pointer(&t))
	old := t
	t.Iflag &^= syscall.ICRNL | syscall.IXON | syscall.INLCR | syscall.BRKINT | syscall.ISTRIP
	t.Lflag &^= syscall.ECHO | syscall.ICANON | syscall.ISIG | syscall.IEXTEN
	t.Cc[syscall.VMIN], t.Cc[syscall.VTIME] = 1, 0
	ioctl(0, setTermios, unsafe.Pointer(&t))
	fmt.Print(enterScreen)
	return func() {
		fmt.Print(leaveScreen)
		ioctl(0, setTermios, unsafe.Pointer(&old))
	}
}

// termSize returns the terminal size in cells and the cell aspect ratio
// (height / width). Terminals that report no pixel size fall back to 2.0.
func termSize() (int, int, float64) {
	var ws [4]uint16
	ioctl(1, syscall.TIOCGWINSZ, unsafe.Pointer(&ws))
	w, h := int(ws[1]), int(ws[0])
	if w == 0 || h == 0 {
		w, h = 100, 30
	}
	return w, h, cellAspect(ws[2], ws[3], w, h)
}
