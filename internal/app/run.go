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

	"muxplayer/internal/audiocap"
	"muxplayer/internal/diag"
	"muxplayer/internal/mpris"
	"muxplayer/internal/mpv"
	"muxplayer/internal/term"
	"muxplayer/internal/visualizer"
)

// Run opens dir and runs the TUI until the user quits. A non-empty track is
// added to the playlist and starts playing immediately.
func Run(p *mpv.Player, dir, track string) {
	m := newModel(p)
	m.load(dir)
	if track != "" {
		m.startTrack(track)
	} else if len(m.list) == 0 {
		m.tab = 1
	}
	diag.Debugf("app: starting in %s (%d tracks in playlist)", dir, len(m.list))
	mpr := mpris.Start()
	diag.Debugf("app: MPRIS available: %v", mpr != nil)
	run(m, mpr)
}

// startTrack adds track to the playlist when it is missing and begins playback.
func (m *model) startTrack(track string) {
	if track == "" {
		return
	}
	m.play(m.add(track))
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
		diag.Debugf("terminal: %dx%d cells, cell aspect %.2f", w, h, aspect)
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
	vizCh := make(chan []float64, 4)
	viz, err := visualizer.Start(vizCh, visualizer.Config{
		Bars: 64,
		FPS:  30,
		Audio: audiocap.Options{
			Device: os.Getenv("MUXPLAYER_AUDIO_SINK"),
		},
		OnError: func(err error) {
			diag.Debugf("visualizer: capture failed: %v", err)
		},
	})
	if err != nil {
		diag.Debugf("visualizer disabled: %v", err)
	} else {
		m.vizOK = true
	}
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
			// Drag motion arrives at a high rate; log the click, release and
			// wheel events instead of every intermediate coordinate.
			if !in.mouse || in.btn != 32 {
				diag.Debugf("input: %+v", in)
			}
			if m.handleInput(in) {
				return
			}
		case ev, ok := <-evCh:
			if !ok {
				evCh = nil
				continue
			}
			m.handleEvent(ev)
		case bars := <-vizCh:
			m.bars = bars
		case res := <-m.coverCh:
			m.handleCover(res)
		case a := <-mprisCh:
			diag.Debugf("mpris action: %s %v", a.Kind, a.Arg)
			if m.action(a) {
				return
			}
		case s := <-sigCh:
			if !term.IsResize(s) {
				return
			}
			v.invalidate()
		case <-tick.C:
			// Persist playlist changes as they happen so a crash or an
			// unclean exit cannot lose them.
			m.flushPlaylist()
			mpr.Update(m.snapshot())
			v.render(m)
		}
	}
}

// shutdown centralises process cleanup: terminal, playlist, children, signals.
func shutdown(m *model, mpr *mpris.Server, viz *visualizer.Visualizer, restore func(), sigCh chan os.Signal, tick *time.Ticker) {
	diag.Debugf("app: shutting down")
	restore()
	m.savePlaylist()
	m.mp.Close()
	mpr.Close()
	viz.Close()
	signal.Stop(sigCh)
	tick.Stop()
}

// changed marks the playlist as modified so the next tick writes it to disk.
func (m *model) changed() {
	m.dirty = true
	m.saveAt = time.Time{} // flush on the next tick
}

// flushPlaylist saves the playlist when it changed. After a failed attempt it
// backs off for a second, so a read-only config directory does not retry on
// every frame.
func (m *model) flushPlaylist() {
	if !m.dirty {
		return
	}
	if !m.saveAt.IsZero() && time.Since(m.saveAt) < time.Second {
		return
	}
	m.saveAt = time.Now()
	m.savePlaylist()
}

func (m *model) savePlaylist() {
	path := plPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		diag.Warnf("cannot create config dir: %v", err)
		return
	}
	data := []byte(strings.Join(m.list, "\n"))
	if err := writeFileAtomic(path, data, 0o644); err != nil {
		diag.Warnf("cannot save playlist: %v", err)
		return
	}
	m.dirty = false
}

// writeFileAtomic replaces path with data in one step. The temp file lives in
// the same directory so the rename never crosses file systems; after a crash
// the disk holds either the old playlist or the new one, never a partial one.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp) // no-op once the rename has succeeded

	if err := f.Chmod(perm); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	// Best effort: make the rename itself durable. Directory sync is not
	// supported everywhere, so its failure is not treated as an error.
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}
