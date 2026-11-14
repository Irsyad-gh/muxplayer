//go:build linux || darwin

package mpv

// note: unix socket paths have a length limit, so the hash keeps them short

import (
	"errors"
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

// dial retries until mpv has created its socket, its IPC budget expires, or
// mpv exits first.
func dial(addr string, exited <-chan error) (io.ReadWriteCloser, error) {
	var err error
	for i := 0; i < 50; i++ {
		var conn net.Conn
		if conn, err = net.Dial("unix", addr); err == nil {
			return conn, nil
		}
		select {
		case waitErr := <-exited:
			if waitErr != nil {
				return nil, fmt.Errorf("mpv died before IPC was ready: %w", waitErr)
			}
			return nil, errors.New("mpv died before IPC was ready")
		case <-time.After(100 * time.Millisecond):
		}
	}
	return nil, err
}

func cleanup(addr string) {
	os.Remove(addr) // ignore error, socket may already be gone
}
