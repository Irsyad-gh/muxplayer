package app

// note: this is the third version of the playlist config and we're still not happy

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"muxplayer/internal/cover"
	"muxplayer/internal/mpv"
	"muxplayer/internal/term"
)

const (
	shuffleOff = iota
	shuffleBag
	shuffleFolder
)

const (
	repeatOff = iota
	repeatAll
	repeatOne
	repeatN
)

const (
	orderAZ = iota
	orderZA
)

type rect struct{ x, y, w, h int }

func (r rect) in(x, y int) bool {
	return x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h
}

type entry struct {
	name string
	dir  bool
	rel  string // path relative to the explorer root; set for recursive matches
}

type row struct {
	text    string
	playing bool
}

type coverRes struct {
	key string
	pix []byte
}

type model struct {
	scr            *screen
	mp             *mpv.Player
	tab            int
	list           []string
	cur, pos       int
	order          []int
	shuffle        int // 0 off · 1 bag (no repeats) · 2 folder (shuffle folder order)
	repeat         int // 0 off · 1 all · 2 one · 3 N×
	times          int
	played         int
	sel, top       [2]int
	dir            string
	entries        []entry
	tpos, dur, vol float64
	paused         bool
	meta           map[string]any
	accent, target cover.RGB
	cover          []byte
	coverKey       string
	pw, ph         int
	cellAspect     float64 // cell height / cell width, keeps the cover square
	aspectOverride float64 // MUXPLAYER_CELL_ASPECT, 0 when unset
	coverCh        chan coverRes
	bars           []float64
	vizOK          bool
	lastClk        time.Time
	lastRow        int
	drag           bool
	errs           int
	dirty          bool      // playlist changed and still needs to be written
	saveAt         time.Time // last save attempt, used to back off on failure
	// explorer search and filter
	query      string
	searching  bool
	typeFilter string  // lowercase extension such as ".flac"; empty means any
	sortOrder  int     // orderAZ or orderZA
	view       []int   // explorer row -> entries index
	listing    []entry // immediate entries of dir, kept for non-search browsing
	index      []entry // recursive scan of dir, built lazily for search
	hSearch    rect
	hType      rect
	hOrder     rect
	// hit areas
	hTab        [2]rect
	hList, hBar rect
	hBtn        [5]rect
	rightX      int
	listY       int
}

func base(p string) string {
	return strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
}

func fmtT(s float64) string {
	if s < 0 || math.IsNaN(s) {
		s = 0
	}
	return fmt.Sprintf("%02d:%02d", int(s)/60, int(s)%60)
}

func (m *model) metaGet(k string) string {
	for key, v := range m.meta {
		if strings.EqualFold(key, k) {
			s, ok := v.(string)
			if ok {
				return s
			}
			return ""
		}
	}
	return ""
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func plPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "muxplayer", "playlist.txt")
}

// envCellAspect reads MUXPLAYER_CELL_ASPECT (cell height / width) so the cover
// can be squared by hand on terminals that report no pixel size.
func envCellAspect() float64 {
	v := os.Getenv("MUXPLAYER_CELL_ASPECT")
	if v == "" {
		return 0
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f < term.MinAspect || f > term.MaxAspect {
		return 0
	}
	return f
}

func newModel(mp *mpv.Player) *model {
	m := &model{
		mp: mp, cur: -1, times: 3, vol: 100,
		accent: cover.RGB{200, 208, 220}, target: cover.RGB{200, 208, 220},
		coverCh: make(chan coverRes, 2), lastRow: -1,
		aspectOverride: envCellAspect(),
	}
	m.loadPlaylist()
	m.rebuild()
	return m
}

func (m *model) loadPlaylist() {
	b, err := os.ReadFile(plPath())
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(b), "\n") {
		if line != "" && exists(line) {
			m.list = append(m.list, line)
		}
	}
}
