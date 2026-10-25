//go:build windows

package mpv

import (
	"fmt"
	"io"
	"os"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// Windows serialises I/O on a synchronous handle: while a Read is blocked
// waiting for mpv's next event, a Write on the same handle cannot start. The
// pipe must therefore be opened with FILE_FLAG_OVERLAPPED so reads and writes
// can be in flight at the same time. Only the standard library is used.

const (
	errHandleEOF        syscall.Errno = 38
	errBrokenPipe       syscall.Errno = 109
	errMoreData         syscall.Errno = 234
	errPipeNotConnected syscall.Errno = 233
	errIOPending        syscall.Errno = 997
	errOpAborted        syscall.Errno = 995
)

var (
	kernel32                = syscall.NewLazyDLL("kernel32.dll")
	procCreateEventW        = kernel32.NewProc("CreateEventW")
	procGetOverlappedResult = kernel32.NewProc("GetOverlappedResult")
	procCancelIoEx          = kernel32.NewProc("CancelIoEx")
)

// addr returns the named pipe mpv listens on for JSON IPC.
func addr() string {
	return fmt.Sprintf(`\\.\pipe\muxplayer-%d`, os.Getpid())
}

// pipeConn is an overlapped named-pipe client. One goroutine may Read while
// another Writes. Buffers and OVERLAPPED structs live in this heap-allocated
// struct so they never move while the kernel holds pointers to them.
type pipeConn struct {
	h syscall.Handle

	rmu  sync.Mutex
	rov  syscall.Overlapped
	rbuf [4096]byte

	wmu  sync.Mutex
	wov  syscall.Overlapped
	wbuf []byte

	closeOnce sync.Once
}

func newEvent() (syscall.Handle, error) {
	// manual-reset, initially non-signalled, unnamed
	r, _, e := procCreateEventW.Call(0, 1, 0, 0)
	if r == 0 {
		return 0, e
	}
	return syscall.Handle(r), nil
}

// wait blocks until the overlapped operation finishes and returns its byte count.
func (c *pipeConn) wait(ov *syscall.Overlapped) (uint32, error) {
	var n uint32
	r, _, e := procGetOverlappedResult.Call(
		uintptr(c.h), uintptr(unsafe.Pointer(ov)), uintptr(unsafe.Pointer(&n)), 1)
	if r == 0 {
		return n, e
	}
	return n, nil
}

func (c *pipeConn) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	c.rmu.Lock()
	defer c.rmu.Unlock()

	size := len(p)
	if size > len(c.rbuf) {
		size = len(c.rbuf)
	}
	var n uint32
	err := syscall.ReadFile(c.h, c.rbuf[:size], &n, &c.rov)
	if err == errIOPending {
		n, err = c.wait(&c.rov)
	}
	switch err {
	case nil, errMoreData:
		// ok: n bytes arrived
	case errBrokenPipe, errPipeNotConnected, errHandleEOF, errOpAborted:
		return 0, io.EOF
	default:
		return 0, err
	}
	copy(p, c.rbuf[:n])
	return int(n), nil
}

func (c *pipeConn) Write(p []byte) (int, error) {
	c.wmu.Lock()
	defer c.wmu.Unlock()

	c.wbuf = append(c.wbuf[:0], p...)
	written := 0
	for written < len(c.wbuf) {
		var n uint32
		err := syscall.WriteFile(c.h, c.wbuf[written:], &n, &c.wov)
		if err == errIOPending {
			n, err = c.wait(&c.wov)
		}
		if err != nil {
			return written, err
		}
		written += int(n)
	}
	return written, nil
}

func (c *pipeConn) Close() error {
	var err error
	c.closeOnce.Do(func() {
		// Wake any blocked Read/Write, then drop the pipe. The two event
		// handles are released when the process exits.
		procCancelIoEx.Call(uintptr(c.h), 0)
		err = syscall.CloseHandle(c.h)
	})
	return err
}

func open(addr string) (*pipeConn, error) {
	name, err := syscall.UTF16PtrFromString(addr)
	if err != nil {
		return nil, err
	}
	h, err := syscall.CreateFile(name,
		syscall.GENERIC_READ|syscall.GENERIC_WRITE,
		0, nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_OVERLAPPED, 0)
	if err != nil {
		return nil, err
	}
	c := &pipeConn{h: h}
	if c.rov.HEvent, err = newEvent(); err != nil {
		syscall.CloseHandle(h)
		return nil, err
	}
	if c.wov.HEvent, err = newEvent(); err != nil {
		syscall.CloseHandle(c.rov.HEvent)
		syscall.CloseHandle(h)
		return nil, err
	}
	return c, nil
}

// dial retries until mpv has created its named pipe or the budget expires.
func dial(addr string) (io.ReadWriteCloser, error) {
	var err error
	for i := 0; i < 50; i++ {
		var c *pipeConn
		if c, err = open(addr); err == nil {
			return c, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil, err
}

// cleanup is a no-op: Windows frees the named pipe when mpv exits.
func cleanup(addr string) {}