// Package cava streams bar heights from cava, the console audio visualizer.
package cava

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"muxplayer/internal/diag"
)

const barCount = 64

// Visualizer is a running cava process.
type Visualizer struct{ cmd *exec.Cmd }

// Start launches cava and sends each frame's bar heights (0..1) to ch. It
// returns nil when cava is not installed or cannot be started.
func Start(ch chan<- []float64) *Visualizer {
	bin, err := find()
	if err != nil {
		diag.Debugf("cava not found; visualizer disabled")
		return nil
	}
	cfg := filepath.Join(os.TempDir(), fmt.Sprintf("muxplayer-cava-%d.conf", os.Getpid()))
	if err := os.WriteFile(cfg, []byte(conf()), 0o600); err != nil {
		diag.Warnf("cannot write cava config: %v", err)
		return nil
	}
	cmd := exec.Command(bin, "-p", cfg)
	out, err := cmd.StdoutPipe()
	if err == nil {
		err = cmd.Start()
	}
	if err != nil {
		diag.Debugf("cannot start cava: %v", err)
		return nil
	}
	go func() {
		scan(out, ch)
		_ = cmd.Wait() // reap cava after it exits or is killed
	}()
	return &Visualizer{cmd: cmd}
}

// Close stops the visualizer.
func (v *Visualizer) Close() {
	if v != nil && v.cmd != nil && v.cmd.Process != nil {
		_ = v.cmd.Process.Kill()
	}
}

// conf renders the config cava is launched with. inputConf is
// platform-specific: Linux/macOS can pick a backend, Windows cannot.
func conf() string {
	return fmt.Sprintf("[general]\nbars = %d\nframerate = 30\n", barCount) +
		inputConf() +
		"[output]\nmethod = raw\nraw_target = /dev/stdout\n" +
		"data_format = ascii\nascii_max_range = 1000\n" +
		"bar_delimiter = 59\nframe_delimiter = 10\n"
}

func scan(out io.Reader, ch chan<- []float64) {
	sc := bufio.NewScanner(out)
	for sc.Scan() {
		v := parseBars(sc.Text())
		select {
		case ch <- v:
		default:
		}
	}
}

func parseBars(line string) []float64 {
	parts := strings.Split(strings.TrimRight(line, ";"), ";")
	v := make([]float64, len(parts))
	for i, p := range parts {
		n, _ := strconv.Atoi(p)
		v[i] = float64(n) / 1000
	}
	return v
}
