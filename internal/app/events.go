package app

import (
	"muxplayer/internal/cover"
	"muxplayer/internal/diag"
)

func (m *model) handleInput(in input) bool {
	if in.mouse {
		m.onMouse(in)
		return false
	}
	return m.onKey(in.key)
}

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
		m.tpos, _ = d.(float64)
	case "duration":
		m.dur, _ = d.(float64)
	case "pause":
		m.paused, _ = d.(bool)
	case "volume":
		m.vol, _ = d.(float64)
	case "metadata":
		m.meta, _ = d.(map[string]any)
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
