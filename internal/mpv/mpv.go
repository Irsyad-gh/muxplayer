// Package mpv drives an mpv process over its JSON IPC socket.
package mpv

// note: mpv's IPC is line-delimited JSON, and yes, the IDs are a global counter

import (
	"bufio"
	"encoding/json"
	"io"
	"os/exec"
	"time"

	"muxplayer/internal/diag"
	"muxplayer/internal/proc"
)

// Player is a running mpv instance plus its event stream.
type Player struct {
	conn io.ReadWriteCloser
	cmd  *exec.Cmd
	sock string
	ev   chan map[string]any
}

// Start launches mpv in idle mode with a private IPC endpoint and connects.
func Start() (*Player, error) {
	addr := addr()
	diag.Debugf("mpv: starting with IPC endpoint %s", addr)
	cmd := exec.Command("mpv", commandLine(addr)...)
	if err := cmd.Start(); err != nil {
		diag.Debugf("mpv: start error: %v", err)
		return nil, err
	}
	proc.KillOnExit(cmd) // mpv must not outlive muxplayer
	conn, err := dial(addr)
	if err != nil {
		diag.Debugf("mpv: IPC connect error: %v", err)
		cmd.Process.Kill() // ignore error, mpv may have exited
		return nil, err
	}
	diag.Debugf("mpv: connected")
	p := &Player{conn: conn, cmd: cmd, sock: addr, ev: make(chan map[string]any, 128)}
	go p.readEvents()
	p.observe()
	return p, nil
}

func (p *Player) Events() <-chan map[string]any { return p.ev }

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
	p.conn.Write(append(msg, '\n')) // ignore error, event loop will notice
}

func (p *Player) Close() {
	if p == nil {
		return
	}
	diag.Debugf("mpv: closing")
	if p.cmd != nil && p.cmd.Process != nil {
		p.cmd.Process.Kill() // ignore error, likely already gone
	}
	cleanup(p.sock)
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
	close(p.ev)
}

func (p *Player) observe() {
	props := []string{"time-pos", "duration", "pause", "metadata", "volume"}
	for i, prop := range props {
		p.Do("observe_property", i+1, prop)
	}
}
