package app

import (
	"path/filepath"

	"muxplayer/internal/diag"
)

func (m *model) prev() {
	if m.tpos > 3 {
		m.mp.Do("seek", 0, "absolute")
		return
	}
	m.step(-1, false)
}

func (m *model) toggle() {
	if m.cur < 0 {
		m.play(m.sel[0])
		return
	}
	m.mp.Do("cycle", "pause")
}

// stop halts playback and resets the player block to its idle state.
func (m *model) stop() {
	m.mp.Do("stop")
	m.cur, m.tpos, m.meta = -1, 0, nil
}

func (m *model) cycleShuffle() {
	m.shuffle = (m.shuffle + 1) % 3
	diag.Debugf("mode: shuffle %d", m.shuffle)
	m.rebuild()
}

func (m *model) cycleRepeat() {
	m.repeat, m.played = (m.repeat+1)%4, 1
	diag.Debugf("mode: repeat %d", m.repeat)
}

// onKey returns true when the app should quit.
func (m *model) onKey(k string) bool {
	if m.typeKey(k) {
		return false
	}
	switch k {
	case "q", "ctrl-c":
		return true
	case "tab":
		m.searching = false
		m.tab = 1 - m.tab
	case "up", "k":
		m.sel[m.tab]--
	case "down", "j":
		m.sel[m.tab]++
	case "pgup":
		m.sel[m.tab] -= 10
	case "pgdn":
		m.sel[m.tab] += 10
	case "home", "g":
		m.sel[m.tab] = 0
	case "end", "G":
		m.sel[m.tab] = 1 << 20
	case "enter":
		m.activate()
	case "space":
		m.toggle()
	case "n":
		m.step(1, false)
	case "p":
		m.prev()
	case "s":
		m.cycleShuffle()
	case "r":
		m.cycleRepeat()
	case "[":
		m.times = max(2, m.times-1)
	case "]":
		m.times = min(99, m.times+1)
	case "left":
		m.mp.Do("seek", -5, "relative")
	case "right":
		m.mp.Do("seek", 5, "relative")
	case "+", "=":
		m.mp.Do("add", "volume", 5)
	case "-", "_":
		m.mp.Do("add", "volume", -5)
	case "backspace":
		m.goUpDir()
	case "a":
		m.addSelected()
	case "d":
		m.deleteSelected()
	case "/":
		if m.tab == 1 {
			m.searching = true
		}
	case "t":
		if m.tab == 1 {
			m.cycleType()
		}
	case "o":
		if m.tab == 1 {
			m.toggleOrder()
		}
	}
	return false
}

func (m *model) goUpDir() {
	if m.tab == 1 {
		m.load(filepath.Dir(m.dir))
	}
}

func (m *model) addSelected() {
	if m.tab != 1 {
		return
	}
	e, ok := m.selectedEntry()
	if !ok || e.name == ".." {
		return
	}
	p := m.entryPath(e)
	if e.dir {
		m.addDir(p)
		return
	}
	m.add(p)
}

func (m *model) deleteSelected() {
	if m.tab == 0 {
		m.remove(m.sel[0])
	}
}
