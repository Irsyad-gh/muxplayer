// Package mpv controls an mpv process over its JSON IPC channel.
package mpv

import (
	"bufio"
	"encoding/json"
	"io"
	"os/exec"
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
	cmd := exec.Command("mpv", commandLine(addr)...)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	conn, err := dial(addr)
	if err != nil {
		_ = cmd.Process.Kill()
		return nil, err
	}
	p := &Player{conn: conn, cmd: cmd, sock: addr, ev: make(chan map[string]any, 128)}
	go p.readEvents()
	p.observe()
	return p, nil
}

// Events yields mpv's property-change and playback events.
func (p *Player) Events() <-chan map[string]any { return p.ev }

// Do sends a command to mpv.
func (p *Player) Do(cmd ...any) {
	if p == nil {
		return
	}
	msg, _ := json.Marshal(map[string]any{"command": cmd})
	_, _ = p.conn.Write(append(msg, '\n'))
}

// Close kills mpv and removes the IPC endpoint.
func (p *Player) Close() {
	if p == nil {
		return
	}
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
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
	for sc.Scan() {
		var e map[string]any
		ok := json.Unmarshal(sc.Bytes(), &e) == nil && e["event"] != nil
		if ok {
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
