package app

// note: the session is written while playback runs, not just on shutdown

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"slices"
	"time"

	"muxplayer/internal/diag"
)

const sessionSaveInterval = 250 * time.Millisecond

// sessionState is the last playing track and its position in seconds.
type sessionState struct {
	Track string  `json:"track"`
	Pos   float64 `json:"pos"`
}

func sessionPath() string { return configPath("session.json") }

// loadSession returns the saved playback state. Missing, stale, and malformed
// files are ignored rather than treated as fatal.
func loadSession() (sessionState, bool) {
	b, err := os.ReadFile(sessionPath())
	if err != nil {
		return sessionState{}, false
	}
	var st sessionState
	if err := json.Unmarshal(b, &st); err != nil {
		return sessionState{}, false
	}
	if st.Track == "" || st.Pos < 0 || math.IsNaN(st.Pos) || math.IsInf(st.Pos, 0) {
		return sessionState{}, false
	}
	return st, true
}

// restoreSession loads the last track without starting playback, so reopening
// the app does not unexpectedly make sound. The file-loaded event applies the
// saved position once mpv has the track ready.
func (m *model) restoreSession() bool {
	st, ok := loadSession()
	if !ok {
		return false
	}
	idx := slices.Index(m.list, st.Track)
	if idx < 0 || !exists(st.Track) {
		return false
	}
	diag.Debugf("session: resuming %s at %.2fs", base(st.Track), st.Pos)
	m.startAt(idx, true)
	m.sel[0] = idx
	m.resumePos = st.Pos
	m.tpos = st.Pos
	return true
}

// applyResume seeks to the saved position after mpv reports the file loaded.
func (m *model) applyResume() {
	pos := m.resumePos
	m.resumePos = 0
	if pos > 0 {
		m.mp.Do("seek", pos, "absolute")
	}
}

// flushSession persists the current track and position at a bounded rate.
// Playback produces position updates continuously, so writing on every one
// would hammer the disk.
func (m *model) flushSession() {
	if !m.sessionDirty || m.cur < 0 {
		return
	}
	if !m.sessionSaveAt.IsZero() && time.Since(m.sessionSaveAt) < sessionSaveInterval {
		return
	}
	m.sessionSaveAt = time.Now()
	if err := m.saveSession(); err != nil {
		diag.Warnf("%v", err)
	}
}

func (m *model) saveSession() error {
	if m.cur < 0 || m.cur >= len(m.list) {
		m.sessionDirty = false
		return nil
	}
	data, err := json.Marshal(sessionState{Track: m.list[m.cur], Pos: m.tpos})
	if err != nil {
		return err
	}
	path := sessionPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := writeFileAtomic(path, append(data, '\n'), 0o644); err != nil {
		return err
	}
	m.sessionDirty = false
	return nil
}
