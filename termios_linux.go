//go:build linux

package main

import "syscall"

// termios ioctl requests for Linux.
const (
	getTermios = syscall.TCGETS
	setTermios = syscall.TCSETS
)
