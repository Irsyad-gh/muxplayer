//go:build windows

package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

// Windows console mode flags (kernel32).
const (
	enableProcessedOutput = 0x0001
	enableVTProcessing    = 0x0004

	enableProcessedInput = 0x0001
	enableLineInput      = 0x0002
	enableEchoInput      = 0x0004
	enableVTInput        = 0x0200
)

var (
	kernel32                       = syscall.NewLazyDLL("kernel32.dll")
	procGetConsoleMode             = kernel32.NewProc("GetConsoleMode")
	procSetConsoleMode             = kernel32.NewProc("SetConsoleMode")
	procGetConsoleScreenBufferInfo = kernel32.NewProc("GetConsoleScreenBufferInfo")
)

type coord struct{ x, y int16 }

type smallRect struct {
	left, top, right, bottom int16
}

// consoleInfo mirrors the Windows CONSOLE_SCREEN_BUFFER_INFO structure.
type consoleInfo struct {
	size          coord
	cursor        coord
	attributes    uint16
	window        smallRect
	maximumWindow coord
}

func consoleMode(handle uintptr) uint32 {
	var mode uint32
	ret, _, _ := procGetConsoleMode.Call(handle, uintptr(unsafe.Pointer(&mode)))
	if ret == 0 {
		return 0
	}
	return mode
}

func setConsoleMode(handle uintptr, mode uint32) {
	_, _, _ = procSetConsoleMode.Call(handle, uintptr(mode))
}

// rawMode enables VT sequences and disables line/echo input. The returned
// function restores the previous console modes.
func rawMode() func() {
	in, out := uintptr(syscall.Stdin), uintptr(syscall.Stdout)
	oldIn := consoleMode(in)
	oldOut := consoleMode(out)

	inMode := oldIn | enableVTInput
	inMode &^= enableEchoInput | enableLineInput | enableProcessedInput
	setConsoleMode(in, inMode)
	setConsoleMode(out, oldOut|enableVTProcessing|enableProcessedOutput)

	fmt.Print(enterScreen)
	return func() {
		fmt.Print(leaveScreen)
		setConsoleMode(in, oldIn)
		setConsoleMode(out, oldOut)
	}
}

// termSize reads the console window size. Windows exposes no cell pixel size,
// so the aspect ratio uses the portable default.
func termSize() (int, int, float64) {
	var info consoleInfo
	ret, _, _ := procGetConsoleScreenBufferInfo.Call(
		uintptr(syscall.Stdout), uintptr(unsafe.Pointer(&info)))
	if ret == 0 {
		return 100, 30, cellAspect(0, 0, 0, 0)
	}
	w := int(info.window.right - info.window.left + 1)
	h := int(info.window.bottom - info.window.top + 1)
	if w <= 0 || h <= 0 {
		w, h = 100, 30
	}
	return w, h, cellAspect(0, 0, w, h)
}
