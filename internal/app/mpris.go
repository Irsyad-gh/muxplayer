// TUI wiring for the MPRIS service: the model's view of controller state and
// the player side of controller commands.
package app

import (
	"net/url"
	"path/filepath"

	"muxplayer/internal/cover"
	"muxplayer/internal/mpris"
)

// mprisLoop maps a repeat mode to the MPRIS loop status; N× restarts collide
// with repeat-one, which is what "Track" expresses to a controller.
var mprisLoop = [...]string{
	mpris.LoopNone,     // repeatOff
	mpris.LoopPlaylist, // repeatAll
	mpris.LoopTrack,    // repeatOne
	mpris.LoopTrack,    // repeatN
}

// snapshot converts the model into a state snapshot for the bus.
func (m *model) snapshot() mpris.Snapshot {
	st := mpris.Snapshot{
		Loop:    mprisLoop[m.repeat],
		Shuffle: m.shuffle != shuffleOff,
		Pos:     m.tpos,
		Dur:     m.dur,
		Vol:     m.vol,
	}
	st.HasNext = len(m.list) > 1
	st.HasPrev = st.HasNext
	if m.cur < 0 || m.cur >= len(m.list) {
		return st
	}
	st.Track = m.list[m.cur]
	st.Title = m.metaGet("title")
	if st.Title == "" {
		st.Title = base(st.Track)
	}
	st.Artist = m.metaGet("artist")
	st.Album = m.metaGet("album")
	if art := cover.Artwork(st.Track); art != "" {
		st.ArtURL = (&url.URL{Scheme: "file", Path: art}).String()
	}
	st.Playing = !m.paused
	return st
}

// action runs a controller command and reports whether it asked to quit.
func (m *model) action(a mpris.Action) bool {
	switch a.Kind {
	case mpris.ActQuit:
		return true
	case mpris.ActPlay:
		if m.cur < 0 {
			m.play(m.sel[0])
		} else {
			m.mp.Do("set_property", "pause", false)
		}
	case mpris.ActPause:
		if m.cur >= 0 {
			m.mp.Do("set_property", "pause", true)
		}
	case mpris.ActToggle:
		m.toggle()
	case mpris.ActNext:
		m.step(1, false)
	case mpris.ActPrevious:
		m.prev()
	case mpris.ActStop:
		m.stop()
	case mpris.ActSeekRel:
		if s, ok := a.Arg.(float64); ok {
			m.mp.Do("seek", s, "relative")
		}
	case mpris.ActSeekAbs:
		if s, ok := a.Arg.(float64); ok {
			m.mp.Do("seek", max(0, s), "absolute")
		}
	case mpris.ActVolume:
		if v, ok := a.Arg.(float64); ok {
			m.mp.Do("set_property", "volume", v)
		}
	case mpris.ActLoop:
		m.setLoop(a.Arg)
	case mpris.ActShuffle:
		if b, ok := a.Arg.(bool); ok {
			m.shuffle = 0
			if b {
				m.shuffle = shuffleBag
			}
			m.rebuild()
		}
	case mpris.ActOpen:
		m.openURI(a.Arg)
	}
	return false
}

// setLoop maps a controller loop request onto the repeat mode.
func (m *model) setLoop(arg any) {
	switch s, _ := arg.(string); s {
	case mpris.LoopTrack:
		m.repeat, m.played = repeatOne, 1
	case mpris.LoopPlaylist:
		m.repeat, m.played = repeatAll, 1
	default:
		m.repeat, m.played = repeatOff, 1
	}
}

// openURI plays a controller-supplied file URI by adding it to the playlist.
func (m *model) openURI(arg any) {
	s, ok := arg.(string)
	if !ok {
		return
	}
	if u, err := url.Parse(s); err == nil && u.Scheme != "" {
		if u.Scheme != "file" {
			return
		}
		s = u.Path
	}
	abs, err := filepath.Abs(s)
	if err != nil || !cover.IsAudio(abs) {
		return
	}
	m.play(m.add(abs))
}
