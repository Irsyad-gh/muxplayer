package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newExplorer builds a model rooted at a temp directory with folders and audio
// files, so load() and the filter run against a real listing.
func newExplorer(t *testing.T) (*model, string) {
	t.Helper()
	dir := t.TempDir()
	for _, d := range []string{"sub", "Zed"} {
		if err := os.Mkdir(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{"A.mp3", "b.flac", "c.mp3", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(dir, f), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m := newTestModel()
	m.load(dir)
	m.tab = 1
	return m, dir
}

// visible returns the paths currently shown in the explorer, in row order.
// Recursive search results use their relative path, matching the UI.
func visible(m *model) []string {
	out := make([]string, 0, len(m.view))
	for _, i := range m.view {
		e := m.entries[i]
		if e.rel != "" {
			out = append(out, e.rel)
			continue
		}
		out = append(out, e.name)
	}
	return out
}

func equalStrings(a, b []string) bool {
	return strings.Join(a, "\x00") == strings.Join(b, "\x00")
}

func TestExplorerSearchByName(t *testing.T) {
	m, _ := newExplorer(t)
	if got, want := visible(m), []string{"..", "sub", "Zed", "A.mp3", "b.flac", "c.mp3"}; !equalStrings(got, want) {
		t.Fatalf("initial listing = %q, want %q", got, want)
	}

	m.setQuery("mp3")
	if got, want := visible(m), []string{"..", "A.mp3", "c.mp3"}; !equalStrings(got, want) {
		t.Fatalf("name search = %q, want %q", got, want)
	}

	m.setQuery("SUB") // folders match too, case-insensitively
	if got, want := visible(m), []string{"..", "sub"}; !equalStrings(got, want) {
		t.Fatalf("folder search = %q, want %q", got, want)
	}

	m.setQuery("missing")
	if got, want := visible(m), []string{".."}; !equalStrings(got, want) {
		t.Fatalf("no-match view = %q, want %q", got, want)
	}
}

// TestExplorerSearchIsRecursive checks that a query scans subdirectories,
// labels nested matches with paths relative to the search root, orders them,
// applies the type filter and skips hidden folders.
func TestExplorerSearchIsRecursive(t *testing.T) {
	m, dir := newExplorer(t)
	if err := os.MkdirAll(filepath.Join(dir, "sub", "deeper"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, ".hidden"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{
		filepath.Join("sub", "inner.mp3"),
		filepath.Join("sub", "deep.flac"),
		filepath.Join("Zed", "zeta.mp3"),
		filepath.Join(".hidden", "secret.mp3"),
	} {
		if err := os.WriteFile(filepath.Join(dir, f), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m.load(dir)

	m.setQuery("inner")
	if got, want := visible(m), []string{"..", filepath.Join("sub", "inner.mp3")}; !equalStrings(got, want) {
		t.Fatalf("nested name search = %q, want %q", got, want)
	}

	m.setQuery(".flac")
	if got, want := visible(m), []string{"..", "b.flac", filepath.Join("sub", "deep.flac")}; !equalStrings(got, want) {
		t.Fatalf("nested type search = %q, want %q", got, want)
	}

	m.setQuery("secret")
	if got, want := visible(m), []string{".."}; !equalStrings(got, want) {
		t.Fatalf("hidden folder leaked into results: %q", got)
	}

	// A matching folder is shown with its relative path so it stays navigable.
	m.setQuery("deeper")
	if got, want := visible(m), []string{"..", filepath.Join("sub", "deeper")}; !equalStrings(got, want) {
		t.Fatalf("nested folder search = %q, want %q", got, want)
	}

	m.setQuery(".mp3")
	if got, want := visible(m), []string{
		"..", "A.mp3", "c.mp3",
		filepath.Join("sub", "inner.mp3"),
		filepath.Join("Zed", "zeta.mp3"),
	}; !equalStrings(got, want) {
		t.Fatalf("recursive order = %q, want %q", got, want)
	}
	m.toggleOrder()
	if got, want := visible(m), []string{
		"..",
		filepath.Join("Zed", "zeta.mp3"),
		filepath.Join("sub", "inner.mp3"),
		"c.mp3", "A.mp3",
	}; !equalStrings(got, want) {
		t.Fatalf("recursive Z-A order = %q, want %q", got, want)
	}
	m.toggleOrder()

	// The type filter narrows recursive name searches too.
	m.setQuery("a")
	m.typeFilter = ".flac"
	m.applyFilter()
	if got, want := visible(m), []string{"..", "b.flac", filepath.Join("sub", "deep.flac")}; !equalStrings(got, want) {
		t.Fatalf("recursive type filter = %q, want %q", got, want)
	}

	// Opening a nested result plays its absolute path.
	m.typeFilter = ""
	m.setQuery("inner")
	m.sel[1] = 1
	m.activate()
	want := filepath.Join(dir, "sub", "inner.mp3")
	if m.cur < 0 || m.list[m.cur] != want {
		t.Fatalf("playing %q, want %q", m.list[m.cur], want)
	}
}

func TestExplorerSearchByType(t *testing.T) {
	m, _ := newExplorer(t)
	m.setQuery(".flac")
	// Extension searches recurse, so folders are only listed when a name term
	// matches one; file results are narrowed by type.
	if got, want := visible(m), []string{"..", "b.flac"}; !equalStrings(got, want) {
		t.Fatalf(".flac view = %q, want %q", got, want)
	}

	m.setQuery(".MP3")
	if got, want := visible(m), []string{"..", "A.mp3", "c.mp3"}; !equalStrings(got, want) {
		t.Fatalf(".MP3 view = %q, want %q", got, want)
	}

	// Terms combine: a name plus a dot type.
	m.setQuery("a .mp3")
	if got, want := visible(m), []string{"..", "A.mp3"}; !equalStrings(got, want) {
		t.Fatalf("combined search = %q, want %q", got, want)
	}
}

func TestExplorerTypeFilterAndOrder(t *testing.T) {
	m, _ := newExplorer(t)

	if got, want := m.typeChoices(), []string{"", ".flac", ".mp3"}; !equalStrings(got, want) {
		t.Fatalf("type choices = %q, want %q", got, want)
	}
	m.cycleType()
	if m.typeFilter != ".flac" {
		t.Fatalf("first type = %q, want .flac", m.typeFilter)
	}
	if got, want := visible(m), []string{"..", "sub", "Zed", "b.flac"}; !equalStrings(got, want) {
		t.Fatalf("type listing = %q, want %q", got, want)
	}
	m.cycleType()
	if m.typeFilter != ".mp3" {
		t.Fatalf("second type = %q, want .mp3", m.typeFilter)
	}
	m.cycleType()
	if m.typeFilter != "" {
		t.Fatalf("type did not cycle back to any: %q", m.typeFilter)
	}

	m.toggleOrder()
	if got, want := visible(m), []string{"..", "Zed", "sub", "c.mp3", "b.flac", "A.mp3"}; !equalStrings(got, want) {
		t.Fatalf("Z-A listing = %q, want %q", got, want)
	}
	m.toggleOrder()
	if m.sortOrder != orderAZ {
		t.Fatalf("order = %d, want A-Z", m.sortOrder)
	}
}

func TestExplorerFilterKeepsSelection(t *testing.T) {
	m, _ := newExplorer(t)
	// Select b.flac, then narrow the listing while keeping it visible.
	for row, idx := range m.view {
		if m.entries[idx].name == "b.flac" {
			m.sel[1] = row
		}
	}
	m.setQuery("flac")
	e, ok := m.selectedEntry()
	if !ok || e.name != "b.flac" {
		t.Fatalf("selection = %q (ok=%v), want b.flac", e.name, ok)
	}
}

func TestSearchKeyTyping(t *testing.T) {
	m, _ := newExplorer(t)
	in, _ := parseInput("\x15")
	if in.key != "ctrl-u" {
		t.Fatalf("Ctrl-U decoded as %q, want ctrl-u", in.key)
	}
	m.searching = true

	for _, k := range []string{".", "f", "l", "a", "c"} {
		if quit := m.onKey(k); quit {
			t.Fatal("typing in the search bar requested quit")
		}
	}
	if m.query != ".flac" {
		t.Fatalf("query = %q, want .flac", m.query)
	}
	if got, want := visible(m), []string{"..", "b.flac"}; !equalStrings(got, want) {
		t.Fatalf("typed filter view = %q, want %q", got, want)
	}

	m.onKey("space")
	if m.query != ".flac " {
		t.Fatalf("space was not typed: %q", m.query)
	}
	m.onKey("backspace")
	if m.query != ".flac" {
		t.Fatalf("backspace = %q, want .flac", m.query)
	}
	m.onKey("ctrl-u")
	if m.query != "" {
		t.Fatalf("ctrl-u = %q, want empty", m.query)
	}

	m.onKey("b")
	m.onKey("enter")
	if m.searching {
		t.Fatal("enter did not leave the search bar")
	}
	if m.query != "b" {
		t.Fatalf("enter cleared the query: %q", m.query)
	}

	m.searching = true
	m.onKey("esc")
	if m.searching || m.query != "" {
		t.Fatalf("esc left searching=%v query=%q", m.searching, m.query)
	}
}

// TestSearchIsCaseInsensitive drives mixed-case input through the real key
// handler and checks names, extensions and the type filter.
func TestSearchIsCaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"A.mp3", "c.MP3", "Zeta.FLAC"} {
		if err := os.WriteFile(filepath.Join(dir, f), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m := newTestModel()
	m.load(dir)
	m.tab = 1
	m.searching = true

	// An uppercase name matches its entry and is not treated as a shortcut.
	for _, k := range []string{"Z", "E", "T", "A"} {
		m.onKey(k)
	}
	if got, want := visible(m), []string{"..", "Zeta.FLAC"}; !equalStrings(got, want) {
		t.Fatalf("uppercase name search = %q, want %q", got, want)
	}

	// An uppercase dot-prefixed term matches lowercase and uppercase files.
	m.onKey("ctrl-u")
	for _, k := range []string{".", "F", "L", "A", "C"} {
		m.onKey(k)
	}
	if got, want := visible(m), []string{"..", "Zeta.FLAC"}; !equalStrings(got, want) {
		t.Fatalf("uppercase type search = %q, want %q", got, want)
	}

	// The type filter folds extensions too: .mp3 catches both .mp3 and .MP3.
	m.onKey("ctrl-u")
	m.onKey("enter")
	m.onKey("t") // .flac
	m.onKey("t") // .mp3
	if got, want := visible(m), []string{"..", "sub", "A.mp3", "c.MP3"}; !equalStrings(got, want) {
		t.Fatalf("type filter = %q, want %q", got, want)
	}
}

func TestExplorerActivateUsesFilteredView(t *testing.T) {
	m, dir := newExplorer(t)
	m.setQuery("b.flac")
	m.sel[1] = 1 // ".." stays on row 0
	m.activate()
	want := filepath.Join(dir, "b.flac")
	if m.cur < 0 || m.list[m.cur] != want {
		t.Fatalf("playing %q, want %q", m.list[m.cur], want)
	}

	m2, dir2 := newExplorer(t)
	m2.setQuery("sub")
	m2.sel[1] = 1
	m2.activate()
	if m2.dir != filepath.Join(dir2, "sub") {
		t.Fatalf("entered %q, want the sub directory", m2.dir)
	}
	if m2.query != "sub" {
		t.Fatalf("filter was not kept across directories: %q", m2.query)
	}
}

func TestSearchChipsAreClickable(t *testing.T) {
	m, _ := newExplorer(t)
	m.draw(100, 30)
	if m.hType.w == 0 || m.hOrder.w == 0 {
		t.Fatal("search chips have no hit areas")
	}
	m.clickSearch(m.hType.x, m.hType.y)
	if m.typeFilter != ".flac" {
		t.Fatalf("type chip = %q, want .flac", m.typeFilter)
	}
	m.clickSearch(m.hOrder.x, m.hOrder.y)
	if m.sortOrder != orderZA {
		t.Fatalf("order chip = %d, want Z-A", m.sortOrder)
	}

	m.tab = 0
	m.clickSearch(10, 2)
	if m.searching {
		t.Fatal("the search bar should be inert on the playlist tab")
	}
}

func TestDrawExplorerSearch(t *testing.T) {
	m, _ := newExplorer(t)
	for _, q := range []string{"", "b", ".flac", strings.Repeat("long query ", 20)} {
		m.setQuery(q)
		m.searching = q != ""
		for w := 60; w <= 200; w += 7 {
			for h := 18; h <= 50; h += 4 {
				m.draw(w, h)
				m.scr.rows()
			}
		}
	}
}
