package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
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

func TestCellAspect(t *testing.T) {
	cases := []struct {
		name           string
		xpixel, ypixel uint16
		w, h           int
		want           float64
	}{
		{"typical 2:1 cells", 800, 480, 100, 30, 2},
		{"square cells", 300, 90, 100, 30, 1},
		{"tall cells", 800, 900, 100, 30, 3.75},
		{"no pixel info", 0, 0, 100, 30, 2},
		{"missing height", 800, 0, 100, 30, 2},
		{"insane ratio", 100, 2000, 10, 10, 2},
	}
	for _, c := range cases {
		if got := cellAspect(c.xpixel, c.ypixel, c.w, c.h); got != c.want {
			t.Errorf("%s: cellAspect = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestCoverSize(t *testing.T) {
	// A 2:1 cell aspect keeps the classic square buffer, scaled down slightly.
	if pw, ph := coverSize(36, 30, 2); pw != 30 || ph != 30 {
		t.Errorf("2:1 aspect: got %dx%d, want 30x30", pw, ph)
	}
	// Square cells: the buffer is pre-stretched tall so it still looks square.
	if pw, ph := coverSize(36, 30, 1); pw != 16 || ph != 32 {
		t.Errorf("1:1 aspect: got %dx%d, want 16x32", pw, ph)
	}
	// The region must never exceed the vertical space (h-11 cells).
	for _, asp := range []float64{0.5, 0.8, 1, 1.3, 1.6, 2, 2.4, 3, 4} {
		pw, ph := coverSize(200, 30, asp)
		if ph/2 > 19 {
			t.Errorf("aspect %v: cover is %d cells tall, max is 19", asp, ph/2)
		}
		if pw%2 != 0 || ph%2 != 0 {
			t.Errorf("aspect %v: buffer %dx%d must be even", asp, pw, ph)
		}
	}
	// Too narrow to be worth drawing.
	if pw, ph := coverSize(6, 30, 2); pw != 0 || ph != 0 {
		t.Errorf("tiny panel: got %dx%d, want 0x0", pw, ph)
	}
	// Out-of-range aspects fall back to the default.
	if pw, ph := coverSize(36, 30, 99); pw != 30 || ph != 30 {
		t.Errorf("invalid aspect: got %dx%d, want 30x30", pw, ph)
	}
}

func TestCoverVF(t *testing.T) {
	vf := coverVF(18, 36)
	if !strings.Contains(vf, "crop=min(iw\\,ih):min(iw\\,ih)") {
		t.Errorf("filter should centre-crop to a square: %q", vf)
	}
	if !strings.HasSuffix(vf, "scale=18:36") {
		t.Errorf("filter should stretch to the buffer size: %q", vf)
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

func TestLoadCoverDimensions(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	// A deliberately non-square source to exercise the square crop.
	src := filepath.Join(dir, "art.png")
	if err := exec.Command("ffmpeg", "-v", "quiet", "-f", "lavfi",
		"-i", "testsrc=size=200x120:duration=1", "-frames:v", "1", src).Run(); err != nil {
		t.Skipf("could not generate test image: %v", err)
	}

	for _, size := range [][2]int{{18, 18}, {18, 36}, {36, 18}} {
		pix := loadCover(src, size[0], size[1])
		if len(pix) != size[0]*size[1]*3 {
			t.Errorf("loadCover(%d, %d) returned %d bytes, want %d",
				size[0], size[1], len(pix), size[0]*size[1]*3)
		}
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
