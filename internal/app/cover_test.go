package app

// note: aspect parsing accepts junk on purpose, the env var is best effort

import (
	"fmt"
	"testing"
)

// coverExtent reports how many rows and columns of the screen are painted with
// the cover's half-block glyphs.
func coverExtent(m *model) (rows, cols int) {
	seenRows := map[int]bool{}
	seenCols := map[int]bool{}
	for y := 0; y < m.scr.h; y++ {
		for x := 0; x < m.scr.w; x++ {
			if m.scr.c[y*m.scr.w+x].r == '▀' {
				seenRows[y] = true
				seenCols[x] = true
			}
		}
	}
	return len(seenRows), len(seenCols)
}

func TestEnvCellAspect(t *testing.T) {
	t.Setenv("MUXPLAYER_CELL_ASPECT", "1.5")
	if got := envCellAspect(); got != 1.5 {
		t.Errorf("valid override = %v, want 1.5", got)
	}
	for _, bad := range []string{"abc", "0", "9", "-1", ""} {
		t.Setenv("MUXPLAYER_CELL_ASPECT", bad)
		if got := envCellAspect(); got != 0 {
			t.Errorf("override %q = %v, want 0", bad, got)
		}
	}
}

func TestCoverSquareRegion(t *testing.T) {
	m := &model{cur: -1, times: 3, vol: 80, coverCh: make(chan coverRes, 4)}
	m.list = []string{"/a/1.mp3", "/a/2.mp3"}
	m.cur = 0
	m.rebuild()

	m.cellAspect = 2 // classic terminal
	m.draw(100, 30)
	if m.pw != m.ph {
		t.Errorf("2:1 aspect: buffer %dx%d, want square", m.pw, m.ph)
	}

	m.cellAspect = 1 // square-cell terminal
	m.coverKey = ""
	m.draw(100, 30)
	if m.ph != 2*m.pw {
		t.Errorf("1:1 aspect: buffer %dx%d, want pre-distorted 2:1", m.pw, m.ph)
	}
}

func TestCoverRenderedSquareFootprint(t *testing.T) {
	const path = "/a/1.mp3"
	m := &model{cur: 0, times: 3, vol: 80, coverCh: make(chan coverRes, 4)}
	m.list = []string{path}
	m.rebuild()

	// 2:1 cells: a square buffer drawn over half as many rows.
	m.cellAspect = 2
	m.draw(100, 30)
	pw, ph := m.pw, m.ph
	if pw != ph {
		t.Fatalf("2:1 cells: unexpected buffer %dx%d", pw, ph)
	}
	m.cover = make([]byte, pw*ph*3)
	for i := range m.cover {
		m.cover[i] = 180
	}
	m.coverKey = fmt.Sprintf("%s|%d|%d", path, pw, ph)
	m.draw(100, 30)
	rows, cols := coverExtent(m)
	if cols != pw || rows != ph/2 {
		t.Errorf("2:1 cells: drawn footprint %dx%d, want %dx%d", cols, rows, pw, ph/2)
	}

	// Square cells: same artwork, pre-distorted buffer that still paints square.
	m2 := &model{cur: 0, times: 3, vol: 80, coverCh: make(chan coverRes, 4)}
	m2.list = []string{path}
	m2.rebuild()
	m2.cellAspect = 1
	m2.draw(100, 30)
	pw2, ph2 := m2.pw, m2.ph
	if ph2 != 2*pw2 {
		t.Fatalf("1:1 cells: unexpected buffer %dx%d", pw2, ph2)
	}
	m2.cover = make([]byte, pw2*ph2*3)
	for i := range m2.cover {
		m2.cover[i] = 180
	}
	m2.coverKey = fmt.Sprintf("%s|%d|%d", path, pw2, ph2)
	m2.draw(100, 30)
	rows2, cols2 := coverExtent(m2)
	if cols2 != pw2 || rows2 != pw2 {
		t.Errorf("1:1 cells: drawn footprint %dx%d, want square %dx%d", cols2, rows2, pw2, pw2)
	}
}
