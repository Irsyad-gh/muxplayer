package app

import (
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"

	"muxplayer/internal/cover"
	"muxplayer/internal/diag"
)

// rebuild rearranges the play order for the current shuffle mode.
func (m *model) rebuild() {
	m.order = m.buildOrder()
	m.pos = 0
	for i, v := range m.order {
		if v != m.cur {
			continue
		}
		m.pos = i
		if m.shuffle == shuffleBag {
			m.order[0], m.order[i] = m.order[i], m.order[0]
			m.pos = 0
		}
		break
	}
}

func (m *model) buildOrder() []int {
	ord := identity(len(m.list))
	switch m.shuffle {
	case shuffleBag:
		rand.Shuffle(len(ord), func(i, j int) { ord[i], ord[j] = ord[j], ord[i] })
	case shuffleFolder:
		ord = folderOrder(m.list)
	}
	return ord
}

func identity(n int) []int {
	ord := make([]int, n)
	for i := range ord {
		ord[i] = i
	}
	return ord
}

// folderOrder shuffles whole folders, keeping each folder's tracks together.
func folderOrder(list []string) []int {
	groups := map[string][]int{}
	var dirs []string
	for i, p := range list {
		d := filepath.Dir(p)
		if _, ok := groups[d]; !ok {
			dirs = append(dirs, d)
		}
		groups[d] = append(groups[d], i)
	}
	rand.Shuffle(len(dirs), func(i, j int) { dirs[i], dirs[j] = dirs[j], dirs[i] })
	ord := make([]int, 0, len(list))
	for _, d := range dirs {
		ord = append(ord, groups[d]...)
	}
	return ord
}

func (m *model) findPos(idx int) {
	if i := indexOf(m.order, idx); i >= 0 {
		m.pos = i
	}
}

func (m *model) play(idx int) {
	if idx < 0 || idx >= len(m.list) {
		return
	}
	if m.shuffle == shuffleBag {
		m.moveToFront(idx)
	}
	m.playAt(idx)
}

// moveToFront makes idx the first bag entry, so every other song still plays
// before any repeats even when playback starts mid-shuffle.
func (m *model) moveToFront(idx int) {
	for i, v := range m.order {
		if v == idx {
			m.order[0], m.order[i] = m.order[i], m.order[0]
			return
		}
	}
}

// playAt starts idx without touching the bag, so automatic advances keep
// walking the current order.
func (m *model) playAt(idx int) {
	if idx < 0 || idx >= len(m.list) {
		return
	}
	diag.Debugf("player: track %d/%d %s", idx+1, len(m.list), m.list[idx])
	m.cur, m.played, m.tpos, m.meta = idx, 1, 0, nil
	m.findPos(idx)
	m.mp.Do("loadfile", m.list[idx], "replace")
	m.mp.Do("set_property", "pause", false)
	m.target = cover.Lift(cover.Hash(m.list[idx]))
}

// step moves forward/backward. auto means a track ended and repeat rules apply.
func (m *model) step(delta int, auto bool) {
	n := len(m.order)
	if n == 0 {
		return
	}
	if m.cur < 0 {
		m.playAt(m.order[0])
		return
	}
	if auto && m.replay() {
		return
	}
	next := m.pos + delta
	if next < 0 || next >= n {
		if m.stopAtEnd(delta, auto) {
			return
		}
		next = m.wrapNext(delta, next, n)
	}
	m.playAt(m.order[next])
}

// replay handles repeat-one and repeat-N; it reports whether playback restarted.
func (m *model) replay() bool {
	switch {
	case m.repeat == repeatOne:
		m.playAt(m.cur)
		return true
	case m.repeat == repeatN && m.played < m.times:
		p := m.played + 1
		m.playAt(m.cur)
		m.played = p
		return true
	}
	return false
}

// stopAtEnd halts playback when a non-repeating playlist reaches its end.
func (m *model) stopAtEnd(delta int, auto bool) bool {
	if delta <= 0 || !auto || m.repeat != repeatOff {
		return false
	}
	m.mp.Do("stop")
	m.cur = -1
	return true
}

// wrapNext resolves an out-of-range index, reshuffling the bag when needed.
func (m *model) wrapNext(delta, next, n int) int {
	if delta > 0 && m.shuffle != shuffleOff {
		m.rebuild()
		next = m.pos + 1
	}
	return (next%n + n) % n
}

/* ───────────── playlist & browser ───────────── */

func (m *model) add(p string) int {
	if i := indexOfString(m.list, p); i >= 0 {
		return i
	}
	m.list = append(m.list, p)
	m.rebuild()
	m.changed()
	return len(m.list) - 1
}

func (m *model) addDir(dir string) int {
	n := 0
	walk := func(p string, e os.DirEntry, err error) error {
		if err == nil && !e.IsDir() && cover.IsAudio(p) {
			m.add(p)
			n++
		}
		return nil
	}
	_ = filepath.WalkDir(dir, walk)
	diag.Debugf("playlist: added %d track(s) from %s", n, dir)
	return n
}

func (m *model) remove(i int) {
	if i < 0 || i >= len(m.list) {
		return
	}
	diag.Debugf("playlist: removing %s", m.list[i])
	m.list = append(m.list[:i], m.list[i+1:]...)
	switch {
	case i < m.cur:
		m.cur--
	case i == m.cur:
		m.cur = -1
		m.mp.Do("stop")
	}
	m.rebuild()
	m.changed()
}

func (m *model) load(dir string) {
	diag.Debugf("explorer: opening %s", dir)
	m.dir = dir
	m.index = nil
	m.view = nil // old row indexes must not be applied to the new listing

	listing := []entry{{name: "..", dir: true}}
	es, _ := os.ReadDir(dir)
	var dirs, files []entry
	for _, e := range es {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if entryIsDir(dir, name, e) {
			dirs = append(dirs, entry{name: name, dir: true})
			continue
		}
		if cover.IsAudio(name) {
			files = append(files, entry{name: name})
		}
	}
	listing = append(listing, dirs...)
	listing = append(listing, files...)
	m.listing = listing
	m.entries = listing
	m.sel[1], m.top[1] = 0, 0
	m.applyFilter()
}

// entryPath resolves an explorer entry to its absolute path. Recursive search
// results carry a path relative to the search root; browsing entries do not.
func (m *model) entryPath(e entry) string {
	p := e.name
	if e.rel != "" {
		p = e.rel
	}
	return filepath.Join(m.dir, p)
}

func entryIsDir(dir, name string, e os.DirEntry) bool {
	if e.IsDir() {
		return true
	}
	if e.Type()&os.ModeSymlink == 0 {
		return false
	}
	st, err := os.Stat(filepath.Join(dir, name))
	return err == nil && st.IsDir()
}

func (m *model) listRows() []row {
	if m.tab == 0 {
		return playlistRows(m.list, m.cur)
	}
	return m.explorerRows()
}

func playlistRows(list []string, cur int) []row {
	out := make([]row, len(list))
	for i, p := range list {
		prefix := "  "
		if i == cur {
			prefix = "▶ "
		}
		out[i] = row{prefix + base(p), i == cur}
	}
	return out
}

func (m *model) explorerRows() []row {
	out := make([]row, 0, len(m.view))
	for _, idx := range m.view {
		e := m.entries[idx]
		name := e.rel
		if name == "" {
			name = e.name
		}
		label := "  " + name
		if e.dir {
			label = "▸ " + name + "/"
		}
		playing := !e.dir && m.cur >= 0 && m.cur < len(m.list) &&
			m.list[m.cur] == m.entryPath(e)
		out = append(out, row{label, playing})
	}
	return out
}

func (m *model) activate() {
	if m.tab == 0 {
		m.play(m.sel[0])
		return
	}
	e, ok := m.selectedEntry()
	if !ok {
		return
	}
	p := m.entryPath(e)
	if e.dir {
		m.load(p)
		return
	}
	m.play(m.add(p))
}

func (m *model) seekFrac(x int) {
	frac := float64(x-m.hBar.x) / float64(max(1, m.hBar.w-1))
	m.mp.Do("seek", math.Max(0, math.Min(1, frac))*m.dur, "absolute")
}
