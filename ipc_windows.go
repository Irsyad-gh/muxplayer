//go:build windows

package main

import (
	"fmt"
	"io"
	"os"
	"time"
)

// mpvAddr is the named pipe mpv listens on for JSON IPC.
func mpvAddr() string {
	return fmt.Sprintf(`\\.\pipe\muxplayer-%d`, os.Getpid())
}

// dialMPV retries until mpv has created its named pipe or the budget expires.
func dialMPV(addr string) (io.ReadWriteCloser, error) {
	var err error
	for i := 0; i < 50; i++ {
		var f *os.File
		if f, err = os.OpenFile(addr, os.O_RDWR, 0); err == nil {
			return f, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil, err
}

// cleanupMPV is a no-op: Windows frees the named pipe when mpv exits.
func cleanupMPV(addr string) {}
