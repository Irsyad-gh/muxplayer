package app

// note: mode changes are rare, so they can be written on the next UI tick

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"muxplayer/internal/diag"
)

// modeState is the persistent playback mode selection.
type modeState struct {
	Shuffle int `json:"shuffle"`
	Repeat  int `json:"repeat"`
	Times   int `json:"times"`
}

func modesPath() string { return configPath("modes.json") }

// loadModes returns the saved shuffle/repeat selection. Missing or malformed
// files leave the caller's defaults in place.
func loadModes() (modeState, bool) {
	b, err := os.ReadFile(modesPath())
	if err != nil {
		return modeState{}, false
	}
	var st modeState
	if err := json.Unmarshal(b, &st); err != nil {
		return modeState{}, false
	}
	st.Shuffle = min(2, max(0, st.Shuffle))
	st.Repeat = min(3, max(0, st.Repeat))
	if st.Times < 2 || st.Times > 99 {
		st.Times = 3
	}
	return st, true
}

func (m *model) flushModes() {
	if !m.modesDirty {
		return
	}
	if !m.modesSaveAt.IsZero() && time.Since(m.modesSaveAt) < time.Second {
		return
	}
	if err := m.saveModes(); err != nil {
		m.modesSaveAt = time.Now()
		diag.Warnf("%v", err)
		return
	}
	m.modesSaveAt = time.Time{}
}

func (m *model) saveModes() error {
	data, err := json.Marshal(modeState{Shuffle: m.shuffle, Repeat: m.repeat, Times: m.times})
	if err != nil {
		return err
	}
	path := modesPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	if err := writeFileAtomic(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("save modes: %w", err)
	}
	m.modesDirty = false
	return nil
}
