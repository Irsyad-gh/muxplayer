//go:build linux || darwin

package mpv

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"
)

// addr returns the Unix socket mpv listens on for JSON IPC.
func addr() string {
	return filepath.Join(os.TempDir(), fmt.Sprintf("muxplayer-%d.sock", os.Getpid()))
}

// dial retries until mpv has created its socket or the budget expires.
func dial(addr string) (io.ReadWriteCloser, error) {
	var err error
	for i := 0; i < 50; i++ {
		var conn net.Conn
		if conn, err = net.Dial("unix", addr); err == nil {
			return conn, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil, err
}

func cleanup(addr string) { _ = os.Remove(addr) }
