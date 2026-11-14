// Package mpv drives an mpv process over its JSON IPC socket.
package mpv

// note: mpv's IPC is line-delimited JSON, and yes, the IDs are a global counter

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/Irsyad-gh/muxplayer/internal/diag"
	"github.com/Irsyad-gh/muxplayer/internal/proc"
)

// Player is a running mpv instance plus its event stream.
type Player struct {
	conn io.ReadWriteCloser
	cmd  *exec.Cmd
	sock string
	ev   chan map[string]any
	done chan struct{}
	tail *tailBuffer

	closeOnce sync.Once
	mu        sync.Mutex
	err       error
}

// Start launches mpv in idle mode with a private IPC endpoint and connects.
func Start() (*Player, error) {
	addr := addr()
	cleanup(addr) // a crash can leave an old socket behind
	diag.Debugf("mpv: starting with IPC endpoint %s", addr)
	cmd := exec.Command("mpv", commandLine(addr)...)
	tail := &tailBuffer{max: 8 << 10}
	cmd.Stderr = tail
	if err := cmd.Start(); err != nil {
		diag.Debugf("mpv: start error: %v", err)
		return nil, err
	}
	proc.KillOnExit(cmd) // mpv must not outlive muxplayer
	p := &Player{
		cmd:  cmd,
		sock: addr,
		ev:   make(chan map[string]any, 128),
		done: make(chan struct{}),
		tail: tail,
	}
	exited := make(chan error, 1)
	go p.wait(exited)

	conn, err := dial(addr, exited)
	if err != nil {
		diag.Debugf("mpv: IPC connect error: %v", err)
		cmd.Process.Kill() // ignore error, mpv may have exited already
		<-p.done
		if msg := strings.TrimSpace(tail.String()); msg != "" {
			return nil, fmt.Errorf("%w: %s", err, msg)
		}
		return nil, err
	}
	p.conn = conn
	diag.Debugf("mpv: connected")
	go p.readEvents()
	p.observe()
	return p, nil
}

// wait reaps mpv and records why it left. Dial uses the channel to fail fast
// when mpv dies before its IPC endpoint appears.
func (p *Player) wait(exited chan<- error) {
	err := p.cmd.Wait()
	p.mu.Lock()
	p.err = err
	p.mu.Unlock()
	exited <- err
	close(p.done)
}

func (p *Player) Events() <-chan map[string]any { return p.ev }

// Done closes when mpv is gone. A nil player returns a nil channel, which
// simply blocks forever in a select.
func (p *Player) Done() <-chan struct{} {
	if p == nil {
		return nil
	}
	return p.done
}

// ExitErr describes how mpv ended. It keeps the tail of stderr because mpv
// usually says something useful before falling over.
func (p *Player) ExitErr() error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	err := p.err
	p.mu.Unlock()
	msg := strings.TrimSpace(p.tail.String())
	switch {
	case err != nil && msg != "":
		return fmt.Errorf("mpv exited: %w: %s", err, msg)
	case err != nil:
		return fmt.Errorf("mpv exited: %w", err)
	case msg != "":
		return errors.New("mpv exited: " + msg)
	default:
		return errors.New("mpv exited")
	}
}

func (p *Player) Do(cmd ...any) {
	if p == nil {
		return
	}
	diag.Debugf("mpv command: %v", cmd)
	msg, err := json.Marshal(map[string]any{"command": cmd})
	if err != nil {
		diag.Debugf("mpv: marshal command: %v", err)
		return
	}
	p.conn.Write(append(msg, '\n')) // ignore error, the exit watcher will notice
}

func (p *Player) Close() {
	if p == nil {
		return
	}
	p.closeOnce.Do(func() {
		diag.Debugf("mpv: closing")
		if p.conn != nil {
			p.conn.Close() // ignore error, mpv is going away anyway
		}
		if p.cmd != nil && p.cmd.Process != nil {
			p.cmd.Process.Kill() // ignore error, it may already be gone
		}
		cleanup(p.sock)
	})
}

func commandLine(addr string) []string {
	return []string{
		"--idle=yes", "--no-video", "--no-terminal",
		"--force-window=no", "--input-ipc-server=" + addr,
	}
}

func (p *Player) readEvents() {
	sc := bufio.NewScanner(p.conn)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	lastPos := time.Time{}
	for sc.Scan() {
		var e map[string]any
		ok := json.Unmarshal(sc.Bytes(), &e) == nil && e["event"] != nil
		if ok {
			log := true
			// time-pos changes many times per second; keep the log readable
			// while still showing a position sample every second.
			if e["event"] == "property-change" && e["name"] == "time-pos" {
				if time.Since(lastPos) < time.Second {
					log = false
				} else {
					lastPos = time.Now()
				}
			}
			if log {
				diag.Debugf("mpv event: %v", e)
			}
			p.ev <- e
		}
	}
	if err := sc.Err(); err != nil {
		diag.Debugf("mpv: event stream ended: %v", err)
	}
	close(p.ev)
}

func (p *Player) observe() {
	props := []string{"time-pos", "duration", "pause", "metadata", "volume"}
	for i, prop := range props {
		p.Do("observe_property", i+1, prop)
	}
}

// tailBuffer keeps the last few KB of mpv's stderr without growing forever.
type tailBuffer struct {
	mu  sync.Mutex
	buf []byte
	max int
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if t.max > 0 && len(t.buf) > t.max {
		t.buf = append(t.buf[:0], t.buf[len(t.buf)-t.max:]...)
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.buf)
}
