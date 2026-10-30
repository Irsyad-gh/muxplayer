package app

// note: mpv sends numbers as float64, so these type assertions are load-bearing

import (
	"muxplayer/internal/cover"
	"muxplayer/internal/diag"
)

func (m *model) handleEvent(e map[string]any) {
	switch e["event"] {
	case "property-change":
		m.handleProperty(e)
	case "file-loaded":
		m.errs = 0
	case "end-file":
		m.handleEndFile(e)
	}
}

func (m *model) handleProperty(e map[string]any) {
	d := e["data"]
	switch e["name"] {
	case "time-pos":
		if v, ok := d.(float64); ok {
			m.tpos = v
		}
	case "duration":
		if v, ok := d.(float64); ok {
			m.dur = v
		}
	case "pause":
		if v, ok := d.(bool); ok {
			m.paused = v
		}
	case "volume":
		if v, ok := d.(float64); ok {
			m.vol = v
		}
	case "metadata":
		if v, ok := d.(map[string]any); ok {
			m.meta = v
		}
	}
}

func (m *model) handleEndFile(e map[string]any) {
	switch e["reason"] {
	case "eof":
		m.step(1, true)
	case "error":
		m.errs++
		if m.errs <= len(m.list) {
			m.step(1, true)
		}
	}
}

func (m *model) handleCover(res coverRes) {
	if res.key != m.coverKey {
		return
	}
	m.cover = res.pix
	if res.pix != nil {
		m.target = cover.Lift(cover.AvgColor(res.pix))
		diag.Debugf("cover: loaded %s", res.key)
	} else {
		diag.Debugf("cover: none for %s", res.key)
	}
}
