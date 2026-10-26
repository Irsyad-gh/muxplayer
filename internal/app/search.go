package app

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"muxplayer/internal/cover"
	"muxplayer/internal/diag"
)

// setQuery updates the explorer search text and refreshes the listing.
func (m *model) setQuery(q string) {
	m.query = q
	m.applyFilter()
}

// filterActive reports whether a name query or a type filter hides any files.
func (m *model) filterActive() bool {
	return strings.TrimSpace(m.query) != "" || m.typeFilter != ""
}

// applyFilter rebuilds the explorer's visible rows. While a query is active it
// searches the whole tree below m.dir and shows matches with paths relative to
// that root. Without a query it filters the immediate listing, keeping
// directories visible so they can be navigated. The selected entry is kept when
// it survives the filter.
func (m *model) applyFilter() {
	prev := ""
	if e, ok := m.selectedEntry(); ok {
		prev = m.entryPath(e)
	}

	if len(m.terms()) > 0 {
		m.entries = m.searchEntries()
		m.view = identity(len(m.entries))
	} else {
		m.entries = m.listing
		m.view = m.browseView()
	}

	m.sel[1] = min(max(m.sel[1], 0), max(len(m.view)-1, 0))
	if prev != "" {
		for row, idx := range m.view {
			if m.entryPath(m.entries[idx]) == prev {
				m.sel[1] = row
				break
			}
		}
	}
	m.lastRow = -1
}

// browseView filters and orders the immediate listing: ".." first, then
// directories, then files.
func (m *model) browseView() []int {
	var parent, dirs, files []int
	for i, e := range m.entries {
		switch {
		case e.name == "..":
			parent = append(parent, i)
		case !m.matchEntry(e):
			continue
		case e.dir:
			dirs = append(dirs, i)
		default:
			files = append(files, i)
		}
	}
	m.sortByName(dirs)
	m.sortByName(files)
	if m.sortOrder == orderZA {
		reverseInts(dirs)
		reverseInts(files)
	}

	view := make([]int, 0, len(m.entries))
	view = append(view, parent...)
	view = append(view, dirs...)
	view = append(view, files...)
	return view
}

// searchEntries walks the tree once per directory, then filters the cached
// index on every keystroke. Results carry a path relative to the search root.
// Extension-only queries list files; folders are only included when a name
// term matches, so a type search does not dump the whole directory tree.
func (m *model) searchEntries() []entry {
	m.buildIndex()
	hasName := false
	for _, term := range m.terms() {
		if !strings.HasPrefix(term, ".") {
			hasName = true
			break
		}
	}

	out := []entry{{name: "..", dir: true}}
	var dirs, files []entry
	for _, e := range m.index {
		if e.dir {
			if hasName && m.matchEntry(e) {
				dirs = append(dirs, e)
			}
			continue
		}
		if m.matchEntry(e) {
			files = append(files, e)
		}
	}
	sortEntriesByRel(dirs)
	sortEntriesByRel(files)
	if m.sortOrder == orderZA {
		reverseEntries(dirs)
		reverseEntries(files)
	}

	out = append(out, dirs...)
	out = append(out, files...)
	return out
}

// buildIndex caches every audio file and directory below m.dir. Hidden entries
// are skipped, matching the normal listing; WalkDir does not follow symlinked
// directories, so cycles are impossible.
func (m *model) buildIndex() {
	if m.index != nil {
		return
	}
	root := m.dir
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	idx := make([]entry, 0, 128)
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || p == root {
			return nil
		}
		name := d.Name()
		if strings.HasPrefix(name, ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return nil
		}
		if d.IsDir() {
			idx = append(idx, entry{name: name, dir: true, rel: rel})
			return nil
		}
		if cover.IsAudio(p) {
			idx = append(idx, entry{name: name, rel: rel})
		}
		return nil
	})
	m.index = idx
}

func (m *model) terms() []string {
	return strings.Fields(strings.ToLower(m.query))
}

// matchEntry reports whether an entry passes the search bar and the file-type
// filter. A term that starts with a dot matches the file extension exactly
// (".flac"); other terms match anywhere in the name, case-insensitively.
func (m *model) matchEntry(e entry) bool {
	name := strings.ToLower(e.name)
	for _, term := range m.terms() {
		if strings.HasPrefix(term, ".") {
			if !e.dir && !strings.EqualFold(filepath.Ext(e.name), term) {
				return false
			}
			continue
		}
		if !strings.Contains(name, term) {
			return false
		}
	}
	return e.dir || m.typeFilter == "" || strings.EqualFold(filepath.Ext(e.name), m.typeFilter)
}

func (m *model) sortByName(rows []int) {
	sort.SliceStable(rows, func(a, b int) bool {
		return strings.ToLower(m.entries[rows[a]].name) < strings.ToLower(m.entries[rows[b]].name)
	})
}

func sortEntriesByRel(es []entry) {
	sort.SliceStable(es, func(a, b int) bool {
		return strings.ToLower(es[a].rel) < strings.ToLower(es[b].rel)
	})
}

func reverseEntries(es []entry) {
	for i, j := 0, len(es)-1; i < j; i, j = i+1, j-1 {
		es[i], es[j] = es[j], es[i]
	}
}

func reverseInts(xs []int) {
	for i, j := 0, len(xs)-1; i < j; i, j = i+1, j-1 {
		xs[i], xs[j] = xs[j], xs[i]
	}
}

// selectedEntry returns the entry under the explorer cursor. The playlist has
// its own index space, so this is explorer-only.
func (m *model) selectedEntry() (entry, bool) {
	if m.sel[1] < 0 || m.sel[1] >= len(m.view) {
		return entry{}, false
	}
	return m.entries[m.view[m.sel[1]]], true
}

// typeChoices lists "any" plus the file types reachable from the current
// directory. During a search it uses the recursive index, otherwise the
// immediate listing. A filter carried in from another folder stays selectable
// so it can be cycled back to "any".
func (m *model) typeChoices() []string {
	src := m.listing
	if len(m.terms()) > 0 {
		m.buildIndex()
		src = m.index
	}

	seen := make(map[string]bool)
	var exts []string
	for _, e := range src {
		if e.dir {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.name))
		if ext == "" || seen[ext] {
			continue
		}
		seen[ext] = true
		exts = append(exts, ext)
	}
	sort.Strings(exts)

	choices := append([]string{""}, exts...)
	if m.typeFilter != "" && !seen[m.typeFilter] {
		choices = append(choices, m.typeFilter)
	}
	return choices
}

func (m *model) cycleType() {
	choices := m.typeChoices()
	next := 0
	for i, ext := range choices {
		if ext == m.typeFilter {
			next = (i + 1) % len(choices)
			break
		}
	}
	m.typeFilter = choices[next]
	diag.Debugf("explorer: type filter %q", m.typeFilter)
	m.applyFilter()
}

func (m *model) toggleOrder() {
	m.sortOrder = orderAZ + orderZA - m.sortOrder
	diag.Debugf("explorer: order %d", m.sortOrder)
	m.applyFilter()
}

// typeKey feeds a key to the search bar and reports whether it belonged to the
// query. Named navigation keys are left for the normal shortcut handler.
func (m *model) typeKey(k string) bool {
	if !m.searching || m.tab != 1 {
		return false
	}
	switch k {
	case "esc":
		m.searching = false
		m.setQuery("")
		return true
	case "enter":
		m.searching = false
		return true
	case "backspace":
		if q := []rune(m.query); len(q) > 0 {
			m.setQuery(string(q[:len(q)-1]))
		}
		return true
	case "ctrl-u":
		m.setQuery("")
		return true
	case "space":
		m.setQuery(m.query + " ")
		return true
	}
	if r := []rune(k); len(r) == 1 && r[0] >= ' ' && r[0] != 127 {
		m.setQuery(m.query + k)
		return true
	}
	return false
}
