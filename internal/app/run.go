// Package app implements muxplayer's terminal UI: the player model, playlist,
// drawing, and input handling.
package app

import (
	"bytes"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"muxplayer/internal/cava"
	"muxplayer/internal/diag"
	"muxplayer/internal/mpris"
	"muxplayer/internal/mpv"
	"muxplayer/internal/term"
)

// Run opens dir and runs the TUI until the user quits.
func Run(p *mpv.Player, dir string) {
	m := newModel(p)
	m.load(dir)
	if len(m.list) == 0 {
		m.tab = 1
	}
	mpr := mpris.Start()
	run(m, mpr)
}

// view tracks the previous frame for diffed rendering and resize handling.
type view struct {
	prev []string
	w, h int
}

func (v *view) invalidate() { v.prev = nil }

func (v *view) render(m *model) {
	w, h, aspect := term.Size()
	if m.aspectOverride > 0 {
		aspect = m.aspectOverride
	}
	m.cellAspect = aspect
	if w != v.w || h != v.h {
		v.w, v.h, v.prev = w, h, nil
		_, _ = os.Stdout.WriteString("\x1b[2J")
	}
	m.draw(w, h)
	v.flush(m.scr.rows())
}

func (v *view) flush(rows []string) {
	var buf bytes.Buffer
	for i, row := range rows {
		if i < len(v.prev) && v.prev[i] == row {
			continue
		}
		fmt.Fprintf(&buf, "\x1b[%d;1H%s", i+1, row)
	}
	v.prev = rows
	_, _ = os.Stdout.Write(buf.Bytes())
}

func run(m *model, mpr *mpris.Server) {
	restore := term.Raw()
	inCh := make(chan input, 64)
	go readInput(inCh)
	cavaCh := make(chan []float64, 4)
	viz := cava.Start(cavaCh)
	m.cavaOK = viz != nil
	sigCh := make(chan os.Signal, 1)
	term.WatchSignals(sigCh)
	tick := time.NewTicker(33 * time.Millisecond)

	defer shutdown(m, mpr, viz, restore, sigCh, tick)

	evCh := m.mp.Events()
	v := &view{}
	// A nil server (no session bus, Windows) makes these channels block, so
	// controller wiring is simply inert when unavailable.
	mprisCh := mpr.Actions()
	for {
		select {
		case in := <-inCh:
			if m.handleInput(in) {
				return
			}
		case ev, ok := <-evCh:
			if !ok {
				evCh = nil
				continue
			}
			m.handleEvent(ev)
		case bars := <-cavaCh:
			m.bars = bars
		case res := <-m.coverCh:
			m.handleCover(res)
		case a := <-mprisCh:
			if m.action(a) {
				return
			}
		case s := <-sigCh:
			if !term.IsResize(s) {
				return
			}
			v.invalidate()
		case <-tick.C:
			mpr.Update(m.snapshot())
			v.render(m)
		}
	}
}

// shutdown centralises process cleanup: terminal, playlist, children, signals.
func shutdown(m *model, mpr *mpris.Server, viz *cava.Visualizer, restore func(), sigCh chan os.Signal, tick *time.Ticker) {
	restore()
	m.savePlaylist()
	m.mp.Close()
	mpr.Close()
	if viz != nil {
		viz.Close()
	}
	signal.Stop(sigCh)
	tick.Stop()
}

func (m *model) savePlaylist() {
	path := plPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		diag.Warnf("cannot create config dir: %v", err)
		return
	}
	data := []byte(strings.Join(m.list, "\n"))
	if err := os.WriteFile(path, data, 0o644); err != nil {
		diag.Warnf("cannot save playlist: %v", err)
	}
}
