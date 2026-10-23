// muxplayer — TUI music player (mpv + cava), stdlib-only, Linux native.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"
	"unsafe"
)

// VERSION OF SOFTWARE
var version = "0.1.0"

/* ───────────── screen / terminal ───────────── */

const def int32 = -1
const (
	aBold uint8 = 1 << iota
	aDim
	aUnder
)

type cell struct {
	r      rune
	fg, bg int32
	at     uint8
}
type screen struct {
	w, h int
	c    []cell
}

func (s *screen) text(x, y int, str string, fg, bg int32, at uint8, max int) {
	if y < 0 || y >= s.h {
		return
	}
	for _, r := range str {
		if max == 0 {
			break
		}
		if x >= 0 && x < s.w {
			s.c[y*s.w+x] = cell{r, fg, bg, at}
		}
		x++
		max--
	}
}
func (s *screen) ctr(x, w, y int, str string, fg, bg int32, at uint8) {
	str = trunc(str, w)
	s.text(x+(w-utf8.RuneCountInString(str))/2, y, str, fg, bg, at, -1)
}
func (s *screen) rows() []string {
	out := make([]string, s.h)
	var sb strings.Builder
	for y := 0; y < s.h; y++ {
		sb.Reset()
		var pf, pb int32 = -2, -2
		var pa uint8 = 255
		for x := 0; x < s.w; x++ {
			c := s.c[y*s.w+x]
			if c.fg != pf || c.bg != pb || c.at != pa {
				sb.WriteString("\x1b[0")
				if c.at&aBold != 0 {
					sb.WriteString(";1")
				}
				if c.at&aDim != 0 {
					sb.WriteString(";2")
				}
				if c.at&aUnder != 0 {
					sb.WriteString(";4")
				}
				if c.fg >= 0 {
					fmt.Fprintf(&sb, ";38;2;%d;%d;%d", c.fg>>16&255, c.fg>>8&255, c.fg&255)
				}
				if c.bg >= 0 {
					fmt.Fprintf(&sb, ";48;2;%d;%d;%d", c.bg>>16&255, c.bg>>8&255, c.bg&255)
				}
				sb.WriteByte('m')
				pf, pb, pa = c.fg, c.bg, c.at
			}
			sb.WriteRune(c.r)
		}
		sb.WriteString("\x1b[0m")
		out[y] = sb.String()
	}
	return out
}

func ioctl(fd, req uintptr, p unsafe.Pointer) {
	syscall.Syscall(syscall.SYS_IOCTL, fd, req, uintptr(p))
}
func rawMode() func() {
	var t syscall.Termios
	ioctl(0, syscall.TCGETS, unsafe.Pointer(&t))
	old := t
	t.Iflag &^= syscall.ICRNL | syscall.IXON | syscall.INLCR | syscall.BRKINT | syscall.ISTRIP
	t.Lflag &^= syscall.ECHO | syscall.ICANON | syscall.ISIG | syscall.IEXTEN
	t.Cc[syscall.VMIN], t.Cc[syscall.VTIME] = 1, 0
	ioctl(0, syscall.TCSETS, unsafe.Pointer(&t))
	fmt.Print("\x1b[?1049h\x1b[?25l\x1b[?1000h\x1b[?1002h\x1b[?1006h\x1b[2J")
	return func() {
		fmt.Print("\x1b[?1006l\x1b[?1002l\x1b[?1000l\x1b[?25h\x1b[?1049l")
		ioctl(0, syscall.TCSETS, unsafe.Pointer(&old))
	}
}

// termSize returns the terminal size in cells and the cell aspect ratio
// (cell height / cell width). Terminals that do not report pixel sizes fall
// back to 2.0, the usual 1:2 character cell.
func termSize() (int, int, float64) {
	var ws [4]uint16
	ioctl(1, syscall.TIOCGWINSZ, unsafe.Pointer(&ws))
	w, h := int(ws[1]), int(ws[0])
	if w == 0 || h == 0 {
		w, h = 100, 30
	}
	return w, h, cellAspect(ws[2], ws[3], w, h)
}

// cellAspect derives cell height / cell width from the terminal's pixel size.
// A value outside the sane range (or no pixel data) yields the 2.0 default.
func cellAspect(xpixel, ypixel uint16, w, h int) float64 {
	if xpixel == 0 || ypixel == 0 || w <= 0 || h <= 0 {
		return 2
	}
	cellW := float64(xpixel) / float64(w)
	cellH := float64(ypixel) / float64(h)
	if cellW <= 0 {
		return 2
	}
	asp := cellH / cellW
	if asp < 0.5 || asp > 4 {
		return 2
	}
	return asp
}

type input struct {
	key          string
	mouse, press bool
	btn, mx, my  int
}

func readInput(ch chan<- input) {
	buf := make([]byte, 512)
	arrows := map[byte]string{'A': "up", 'B': "down", 'C': "right", 'D': "left", 'H': "home", 'F': "end"}
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			return
		}
		b := string(buf[:n])
		for len(b) > 0 {
			switch {
			case strings.HasPrefix(b, "\x1b[<"):
				e := strings.IndexAny(b, "Mm")
				if e < 0 {
					b = ""
					continue
				}
				var bt, x, y int
				fmt.Sscanf(b[3:e], "%d;%d;%d", &bt, &x, &y)
				ch <- input{mouse: true, btn: bt, mx: x - 1, my: y - 1, press: b[e] == 'M'}
				b = b[e+1:]
			case strings.HasPrefix(b, "\x1b[") && len(b) >= 3:
				if k, ok := arrows[b[2]]; ok {
					ch <- input{key: k}
					b = b[3:]
				} else if len(b) >= 4 && b[3] == '~' && (b[2] == '5' || b[2] == '6') {
					ch <- input{key: map[byte]string{'5': "pgup", '6': "pgdn"}[b[2]]}
					b = b[4:]
				} else {
					b = b[2:]
				}
			default:
				r, sz := utf8.DecodeRuneInString(b)
				b = b[sz:]
				k := string(r)
				switch r {
				case 13:
					k = "enter"
				case 9:
					k = "tab"
				case 32:
					k = "space"
				case 127, 8:
					k = "backspace"
				case 3:
					k = "ctrl-c"
				case 27:
					k = "esc"
				}
				ch <- input{key: k}
			}
		}
	}
}

/* ───────────── mpv (JSON IPC) & cava ───────────── */

type mpv struct {
	conn net.Conn
	cmd  *exec.Cmd
	sock string
	ev   chan map[string]any
}

func startMPV() (*mpv, error) {
	sock := filepath.Join(os.TempDir(), fmt.Sprintf("muxplayer-%d.sock", os.Getpid()))
	cmd := exec.Command("mpv", "--idle=yes", "--no-video", "--no-terminal", "--force-window=no", "--input-ipc-server="+sock)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	var conn net.Conn
	var err error
	for i := 0; i < 50; i++ {
		if conn, err = net.Dial("unix", sock); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		cmd.Process.Kill()
		return nil, err
	}
	m := &mpv{conn, cmd, sock, make(chan map[string]any, 128)}
	go func() {
		sc := bufio.NewScanner(conn)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for sc.Scan() {
			var e map[string]any
			if json.Unmarshal(sc.Bytes(), &e) == nil && e["event"] != nil {
				m.ev <- e
			}
		}
		close(m.ev)
	}()
	for i, p := range []string{"time-pos", "duration", "pause", "metadata", "volume"} {
		m.do("observe_property", i+1, p)
	}
	return m, nil
}
func (m *mpv) do(args ...any) {
	if m == nil {
		return
	}
	b, _ := json.Marshal(map[string]any{"command": args})
	m.conn.Write(append(b, '\n'))
}

func startCava(ch chan<- []float64) *exec.Cmd {
	if _, err := exec.LookPath("cava"); err != nil {
		return nil
	}
	cfg := filepath.Join(os.TempDir(), fmt.Sprintf("muxplayer-cava-%d.conf", os.Getpid()))
	conf := "[general]\nbars = 64\nframerate = 30\n"
	if src := os.Getenv("MUXPLAYER_CAVA_INPUT"); src != "" { // pulse | pipewire | alsa | ...
		conf += "[input]\nmethod = " + src + "\n"
	}
	conf += "[output]\nmethod = raw\nraw_target = /dev/stdout\ndata_format = ascii\nascii_max_range = 1000\nbar_delimiter = 59\nframe_delimiter = 10\n"
	os.WriteFile(cfg, []byte(conf), 0o600)
	cmd := exec.Command("cava", "-p", cfg)
	out, _ := cmd.StdoutPipe()
	if cmd.Start() != nil {
		return nil
	}
	go func() {
		sc := bufio.NewScanner(out)
		for sc.Scan() {
			parts := strings.Split(strings.TrimRight(sc.Text(), ";"), ";")
			v := make([]float64, len(parts))
			for i, p := range parts {
				n, _ := strconv.Atoi(p)
				v[i] = float64(n) / 1000
			}
			select {
			case ch <- v:
			default:
			}
		}
	}()
	return cmd
}

/* ───────────── colors & cover ───────────── */

type rgb [3]float64

func pack(c rgb) int32 { return int32(int(c[0])<<16 | int(c[1])<<8 | int(c[2])) }
func mix(a, b rgb, t float64) rgb {
	return rgb{a[0] + (b[0]-a[0])*t, a[1] + (b[1]-a[1])*t, a[2] + (b[2]-a[2])*t}
}
func lum(c rgb) float64 { return .299*c[0] + .587*c[1] + .114*c[2] }
func lift(c rgb) rgb { // make sure the accent stays readable on a dark/transparent bg
	if l := lum(c); l < 120 {
		return mix(c, rgb{255, 255, 255}, (120-l)/120*.85)
	}
	return c
}
func hashColor(s string) rgb {
	h := fnv.New32a()
	h.Write([]byte(s))
	hue := float64(h.Sum32()%360) / 60
	x := 1 - math.Abs(math.Mod(hue, 2)-1)
	r, g, b := 0.0, 0.0, 0.0
	switch int(hue) {
	case 0:
		r, g = 1, x
	case 1:
		r, g = x, 1
	case 2:
		g, b = 1, x
	case 3:
		g, b = x, 1
	case 4:
		r, b = x, 1
	default:
		r, b = 1, x
	}
	return mix(rgb{r * 255, g * 255, b * 255}, rgb{255, 255, 255}, .25)
}

var audioExt = map[string]bool{".mp3": true, ".flac": true, ".ogg": true, ".opus": true, ".wav": true, ".m4a": true, ".aac": true, ".wma": true, ".ape": true}

// coverFill scales the cover down from the largest size that fits the panel,
// leaving a little breathing room around it.
const coverFill = 0.85

// coverSize returns the cover's pixel buffer size in device pixels for a panel
// rw cells wide and h cells tall. The buffer is a pre-distorted square: it is
// stretched by the inverse of the terminal cell aspect so that, once drawn with
// half blocks, it appears as a true 1:1 square on screen.
func coverSize(rw, h int, aspect float64) (pw, ph int) {
	if aspect < 0.5 || aspect > 4 {
		aspect = 2
	}
	maxRows := h - 11
	if maxRows < 4 {
		return 0, 0
	}
	fit := math.Min(float64(rw), float64(maxRows)*aspect)
	pw = int(math.Round(fit * coverFill))
	pw -= pw % 2
	if pw < 8 {
		return 0, 0
	}
	rows := int(math.Round(float64(pw) / aspect))
	if rows < 1 {
		rows = 1
	}
	if rows > maxRows {
		rows = maxRows
	}
	return pw, rows * 2
}

// coverVF builds the ffmpeg filter that centre-crops the artwork to a square
// and then stretches it to the target device-pixel size.
func coverVF(pw, ph int) string {
	return fmt.Sprintf("crop=min(iw\\,ih):min(iw\\,ih),scale=%d:%d", pw, ph)
}

func loadCover(path string, w, h int) []byte {
	try := func(src string) []byte {
		out, err := exec.Command("ffmpeg", "-v", "quiet", "-i", src, "-an", "-frames:v", "1", "-vf", coverVF(w, h), "-f", "rawvideo", "-pix_fmt", "rgb24", "-").Output()
		if err != nil || len(out) != w*h*3 {
			return nil
		}
		return out
	}
	if p := try(path); p != nil {
		return p
	}
	for _, n := range []string{"cover.jpg", "folder.jpg", "cover.png", "folder.png", "front.jpg"} {
		f := filepath.Join(filepath.Dir(path), n)
		if _, e := os.Stat(f); e == nil {
			if p := try(f); p != nil {
				return p
			}
		}
	}
	return nil
}

/* ───────────── model ───────────── */

type rect struct{ x, y, w, h int }

func (r rect) in(x, y int) bool { return x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h }

type entry struct {
	name string
	dir  bool
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
	mp             *mpv
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
	accent, target rgb
	cover          []byte
	coverKey       string
	pw, ph         int
	cellAspect     float64 // cell height / cell width, used to keep the cover square
	aspectOverride float64 // MUXPLAYER_CELL_ASPECT, 0 when unset
	coverCh        chan coverRes
	bars           []float64
	cavaOK         bool
	lastClk        time.Time
	lastRow        int
	drag           bool
	errs           int
	// hit areas
	hTab        [2]rect
	hList, hBar rect
	hBtn        [5]rect
	rightX      int
	listY       int
}

func base(p string) string { return strings.TrimSuffix(filepath.Base(p), filepath.Ext(p)) }
func trunc(s string, n int) string {
	if n < 0 {
		n = 0
	}
	r := []rune(s)
	if len(r) > n {
		if n > 1 {
			return string(r[:n-1]) + "…"
		}
		return string(r[:n])
	}
	return s
}
func fmtT(s float64) string {
	if s < 0 || math.IsNaN(s) {
		s = 0
	}
	return fmt.Sprintf("%02d:%02d", int(s)/60, int(s)%60)
}
func (m *model) metaGet(k string) string {
	for kk, v := range m.meta {
		if strings.EqualFold(kk, k) {
			s, _ := v.(string)
			return s
		}
	}
	return ""
}

// rebuild rearranges the playback order according to the shuffle mode.
func (m *model) rebuild() {
	n := len(m.list)
	ord := make([]int, n)
	for i := range ord {
		ord[i] = i
	}
	switch m.shuffle {
	case 1:
		rand.Shuffle(n, func(i, j int) { ord[i], ord[j] = ord[j], ord[i] })
	case 2:
		g := map[string][]int{}
		var dirs []string
		for i, p := range m.list {
			d := filepath.Dir(p)
			if _, ok := g[d]; !ok {
				dirs = append(dirs, d)
			}
			g[d] = append(g[d], i)
		}
		rand.Shuffle(len(dirs), func(i, j int) { dirs[i], dirs[j] = dirs[j], dirs[i] })
		ord = ord[:0]
		for _, d := range dirs {
			ord = append(ord, g[d]...)
		}
	}
	m.order, m.pos = ord, 0
	for i, v := range ord {
		if v == m.cur {
			m.pos = i
			if m.shuffle == 1 {
				ord[0], ord[i], m.pos = ord[i], ord[0], 0
			}
			break
		}
	}
}
func (m *model) play(idx int) {
	if idx < 0 || idx >= len(m.list) {
		return
	}
	if m.shuffle == 1 {
		// Start the bag at idx so every other song still gets a turn before
		// any repeats, even when playback begins mid-shuffle.
		for i, v := range m.order {
			if v == idx {
				m.order[0], m.order[i] = m.order[i], m.order[0]
				break
			}
		}
	}
	m.playAt(idx)
}

// playAt starts idx without touching the shuffle order, so automatic advances
// keep walking the current bag.
func (m *model) playAt(idx int) {
	if idx < 0 || idx >= len(m.list) {
		return
	}
	m.cur, m.played, m.tpos, m.meta = idx, 1, 0, nil
	for i, v := range m.order {
		if v == idx {
			m.pos = i
		}
	}
	m.mp.do("loadfile", m.list[idx], "replace")
	m.mp.do("set_property", "pause", false)
	m.target = lift(hashColor(m.list[idx]))
}

// step moves forward/backward; auto=true means a track ended and the repeat
// rules apply.
func (m *model) step(d int, auto bool) {
	n := len(m.order)
	if n == 0 {
		return
	}
	if m.cur < 0 {
		m.playAt(m.order[0])
		return
	}
	if auto {
		if m.repeat == 2 {
			m.playAt(m.cur)
			return
		}
		if m.repeat == 3 && m.played < m.times {
			p := m.played + 1
			m.playAt(m.cur)
			m.played = p
			return
		}
	}
	np := m.pos + d
	if np >= n || np < 0 {
		if d > 0 && auto && m.repeat == 0 {
			m.mp.do("stop")
			m.cur = -1
			return
		}
		if d > 0 && m.shuffle != 0 {
			m.rebuild()
			np = m.pos + 1
		}
		np = (np%n + n) % n
	}
	m.playAt(m.order[np])
}
func (m *model) add(p string) int {
	for i, q := range m.list {
		if q == p {
			return i
		}
	}
	m.list = append(m.list, p)
	m.rebuild()
	return len(m.list) - 1
}
func (m *model) addDir(d string) int {
	n := 0
	filepath.WalkDir(d, func(p string, e os.DirEntry, err error) error {
		if err == nil && !e.IsDir() && audioExt[strings.ToLower(filepath.Ext(p))] {
			m.add(p)
			n++
		}
		return nil
	})
	return n
}
func (m *model) remove(i int) {
	if i < 0 || i >= len(m.list) {
		return
	}
	m.list = append(m.list[:i], m.list[i+1:]...)
	if i < m.cur {
		m.cur--
	} else if i == m.cur {
		m.cur = -1
		m.mp.do("stop")
	}
	m.rebuild()
}
func (m *model) load(dir string) {
	m.dir, m.entries = dir, []entry{{"..", true}}
	es, _ := os.ReadDir(dir)
	var ds, fs []entry
	for _, e := range es {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		isDir := e.IsDir()
		if e.Type()&os.ModeSymlink != 0 {
			if st, err := os.Stat(filepath.Join(dir, e.Name())); err == nil {
				isDir = st.IsDir()
			}
		}
		if isDir {
			ds = append(ds, entry{e.Name(), true})
		} else if audioExt[strings.ToLower(filepath.Ext(e.Name()))] {
			fs = append(fs, entry{e.Name(), false})
		}
	}
	m.entries = append(append(m.entries, ds...), fs...)
	m.sel[1], m.top[1] = 0, 0
}
func (m *model) rows() []row {
	var out []row
	if m.tab == 0 {
		for i, p := range m.list {
			pre := "  "
			if i == m.cur {
				pre = "▶ "
			}
			out = append(out, row{pre + base(p), i == m.cur})
		}
		return out
	}
	for _, e := range m.entries {
		t := "  " + e.name
		if e.dir {
			t = "▸ " + e.name + "/"
		}
		out = append(out, row{t, !e.dir && m.cur >= 0 && m.list[m.cur] == filepath.Join(m.dir, e.name)})
	}
	return out
}
func (m *model) activate() {
	if m.tab == 0 {
		m.play(m.sel[0])
		return
	}
	if m.sel[1] >= len(m.entries) {
		return
	}
	e := m.entries[m.sel[1]]
	p := filepath.Join(m.dir, e.name)
	if e.dir {
		m.load(p)
	} else {
		m.play(m.add(p))
	}
}
func (m *model) seekFrac(x int) {
	f := float64(x-m.hBar.x) / float64(max(1, m.hBar.w-1))
	m.mp.do("seek", math.Max(0, math.Min(1, f))*m.dur, "absolute")
}

/* ───────────── drawing ───────────── */

func (m *model) draw(w, h int) {
	if m.scr == nil || m.scr.w != w || m.scr.h != h {
		m.scr = &screen{w: w, h: h, c: make([]cell, w*h)}
	}
	s := m.scr
	for i := range s.c {
		s.c[i] = cell{' ', def, def, 0}
	}
	if w < 60 || h < 18 {
		s.text(1, 1, "terminal too small (min 60x18)", def, def, 0, -1)
		return
	}
	for i := range m.accent {
		m.accent[i] += (m.target[i] - m.accent[i]) * .12
	}
	acc := pack(m.accent)
	onAcc := int32(0x101010)
	if lum(m.accent) < 110 {
		onAcc = 0xf0f0f0
	}
	lx, ly := 2, 3
	lw := (w - 7) * 62 / 100
	rx := lx + lw + 3
	rw := w - rx - 2
	vh := max(5, h/3)
	sepY := h - vh - 2
	lh := sepY - ly - 1
	m.rightX, m.listY = rx, ly

	// tabs
	x := lx
	for i, t := range []string{" Playlist ", " Explorer "} {
		if i == m.tab {
			s.text(x, 1, t, onAcc, acc, aBold, -1)
		} else {
			s.text(x, 1, t, def, def, aDim, -1)
		}
		m.hTab[i] = rect{x, 1, len(t), 1}
		x += len(t) + 1
	}
	info := fmt.Sprintf("%d songs", len(m.list))
	if m.tab == 1 {
		info = m.dir
	}
	info = trunc(info, lw-24)
	s.text(lx+lw-utf8.RuneCountInString(info), 1, info, def, def, aDim, -1)

	// list
	rows := m.rows()
	n := len(rows)
	sel, top := &m.sel[m.tab], &m.top[m.tab]
	*sel = max(0, min(*sel, n-1))
	if *sel < *top {
		*top = *sel
	}
	if *sel >= *top+lh {
		*top = *sel - lh + 1
	}
	*top = max(0, *top)
	m.hList = rect{lx, ly, lw, lh}
	for i := 0; i < lh && *top+i < n; i++ {
		idx, r, y := *top+i, rows[*top+i], ly+i
		fg, bg, at := def, def, uint8(0)
		if r.playing {
			fg, at = acc, aBold
		}
		if idx == *sel {
			fg, bg = onAcc, acc
			for xx := lx; xx < lx+lw; xx++ {
				s.c[y*w+xx] = cell{' ', def, acc, 0}
			}
		}
		s.text(lx+1, y, r.text, fg, bg, at, lw-2)
	}
	if n > lh && lh > 0 {
		th := max(1, lh*lh/n)
		ty := *top * (lh - th) / max(1, n-lh)
		for i := 0; i < lh; i++ {
			if i >= ty && i < ty+th {
				s.text(lx+lw, ly+i, "┃", acc, def, 0, 1)
			} else {
				s.text(lx+lw, ly+i, "│", def, def, aDim, 1)
			}
		}
	}

	// separator + cava visualizer
	s.text(lx, sepY, strings.Repeat("─", lw+1), acc, def, aDim, -1)
	if !m.cavaOK {
		s.ctr(lx, lw, sepY+vh/2, "cava not found — visualizer disabled", def, def, aDim)
	} else if len(m.bars) > 0 {
		nb := lw / 3
		for b := 0; b < nb; b++ {
			lo, hi := b*len(m.bars)/nb, (b+1)*len(m.bars)/nb
			v := 0.0
			for k := lo; k < max(hi, lo+1) && k < len(m.bars); k++ {
				v = math.Max(v, m.bars[k])
			}
			eighths := int(math.Min(1, v) * float64(vh*8))
			col := pack(mix(m.accent, rgb{255, 255, 255}, v*.45))
			for r := 0; r < vh; r++ {
				y := h - 2 - r
				var ch rune
				switch e := eighths - r*8; {
				case e >= 8:
					ch = '█'
				case e > 0:
					ch = []rune("▁▂▃▄▅▆▇")[e-1]
				}
				if ch != 0 {
					s.text(lx+b*3, y, string([]rune{ch, ch}), col, def, 0, -1)
				}
			}
		}
	}

	// right panel: cover (kept square on screen; see coverSize)
	pw, ph := coverSize(rw, h, m.cellAspect)
	cw, chh := pw, ph/2
	ty := 1
	if cw >= 8 {
		cx := rx + (rw-cw)/2
		if m.cur >= 0 {
			if key := fmt.Sprintf("%s|%d|%d", m.list[m.cur], pw, ph); key != m.coverKey {
				m.coverKey, m.cover, m.pw, m.ph = key, nil, pw, ph
				go func(p string) { m.coverCh <- coverRes{key, loadCover(p, pw, ph)} }(m.list[m.cur])
			}
		}
		for r := 0; r < chh; r++ {
			for c := 0; c < cw; c++ {
				if m.cover != nil && m.pw == pw && m.ph == ph {
					t, b := (2*r*pw+c)*3, ((2*r+1)*pw+c)*3
					s.c[(1+r)*w+cx+c] = cell{'▀', int32(int(m.cover[t])<<16 | int(m.cover[t+1])<<8 | int(m.cover[t+2])),
						int32(int(m.cover[b])<<16 | int(m.cover[b+1])<<8 | int(m.cover[b+2])), 0}
				} else {
					s.c[(1+r)*w+cx+c] = cell{' ', def, acc, 0}
				}
			}
		}
		if m.cover == nil {
			s.ctr(cx, cw, 1+chh/2, "music cover", onAcc, acc, 0)
		}
		ty = 1 + chh + 1
	}
	title := "muxplayer"
	if m.cur >= 0 {
		title = m.metaGet("title")
		if title == "" {
			title = base(m.list[m.cur])
		}
		if a := m.metaGet("artist"); a != "" {
			title += " – " + a
		}
	}
	s.ctr(rx, rw, ty, title, def, def, aBold)
	by := ty + 2
	m.hBar = rect{rx, by, rw, 1}
	pos := 0
	if m.dur > 0 {
		pos = int(math.Min(1, m.tpos/m.dur) * float64(rw-1))
	}
	for i := 0; i < rw; i++ {
		switch {
		case i == pos:
			s.text(rx+i, by, "●", def, def, aBold, 1)
		case i < pos:
			s.text(rx+i, by, "━", acc, def, 0, 1)
		default:
			s.text(rx+i, by, "━", def, def, aDim, 1)
		}
	}
	s.text(rx, by+1, fmtT(m.tpos), def, def, aDim, -1)
	s.text(rx+rw-5, by+1, fmtT(m.dur), def, def, aDim, -1)

	// buttons
	rl := []string{"↻ off", "↻ all", "↻ one", fmt.Sprintf("↻ %d×", m.times)}[m.repeat]
	sl := []string{"⇄ off", "⇄ bag", "⇄ folder"}[m.shuffle]
	pl := "▶"
	if m.cur >= 0 && !m.paused {
		pl = "‖"
	}
	bl := []string{rl, "|◀", pl, "▶|", sl}
	bx := []int{rx, rx + rw*3/10 - 1, rx + rw/2 - 1, rx + rw*7/10 - 1, rx + rw - utf8.RuneCountInString(sl)}
	for i, t := range bl {
		fg, at := def, aBold
		if (i == 0 && m.repeat > 0) || (i == 4 && m.shuffle > 0) {
			fg = acc
		}
		s.text(bx[i], by+3, t, fg, def, at, -1)
		m.hBtn[i] = rect{bx[i] - 1, by + 3, utf8.RuneCountInString(t) + 2, 1}
	}
	s.ctr(rx, rw, by+5, fmt.Sprintf("vol %d%%", int(m.vol)), def, def, aDim)
	s.text(1, h-1, trunc(" Tab:panel  Enter:play  Space:pause  n/p:track  s:shuffle  r:repeat  [ ]:N×  ←→:seek  +/-:vol  a:add  d:delete  q:quit", w-2), def, def, aDim, -1)
}

/* ───────────── input handling ───────────── */

func (m *model) onMouse(in input) {
	x, y := in.mx, in.my
	switch {
	case in.btn == 64 || in.btn == 65:
		d := 1
		if in.btn == 64 {
			d = -1
		}
		if x < m.rightX {
			m.sel[m.tab] += d * 3
			m.top[m.tab] += d * 3
			m.top[m.tab] = max(0, m.top[m.tab])
		} else {
			m.mp.do("add", "volume", -d*5)
		}
	case in.btn == 32 && m.drag:
		m.seekFrac(x)
	case in.btn == 0 && !in.press:
		m.drag = false
	case in.btn == 0 && in.press:
		for i, r := range m.hTab {
			if r.in(x, y) {
				m.tab = i
			}
		}
		if m.hList.in(x, y) {
			i := m.top[m.tab] + y - m.hList.y
			if i < len(m.rows()) {
				m.sel[m.tab] = i
				if i == m.lastRow && time.Since(m.lastClk) < 400*time.Millisecond {
					m.activate()
				}
				m.lastRow, m.lastClk = i, time.Now()
			}
		}
		if m.hBar.in(x, y) && m.dur > 0 {
			m.drag = true
			m.seekFrac(x)
		}
		for i, r := range m.hBtn {
			if r.in(x, y) {
				switch i {
				case 0:
					m.repeat, m.played = (m.repeat+1)%4, 1
				case 1:
					m.prev()
				case 2:
					m.toggle()
				case 3:
					m.step(1, false)
				case 4:
					m.shuffle = (m.shuffle + 1) % 3
					m.rebuild()
				}
			}
		}
	}
}
func (m *model) prev() {
	if m.tpos > 3 {
		m.mp.do("seek", 0, "absolute")
	} else {
		m.step(-1, false)
	}
}
func (m *model) toggle() {
	if m.cur < 0 {
		m.play(m.sel[0])
	} else {
		m.mp.do("cycle", "pause")
	}
}

// onKey returns true when the app should quit.
func (m *model) onKey(k string) bool {
	switch k {
	case "q", "ctrl-c":
		return true
	case "tab":
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
		m.shuffle = (m.shuffle + 1) % 3
		m.rebuild()
	case "r":
		m.repeat, m.played = (m.repeat+1)%4, 1
	case "[":
		m.times = max(2, m.times-1)
	case "]":
		m.times = min(99, m.times+1)
	case "left":
		m.mp.do("seek", -5, "relative")
	case "right":
		m.mp.do("seek", 5, "relative")
	case "+", "=":
		m.mp.do("add", "volume", 5)
	case "-", "_":
		m.mp.do("add", "volume", -5)
	case "backspace":
		if m.tab == 1 {
			m.load(filepath.Dir(m.dir))
		}
	case "a":
		if m.tab == 1 && m.sel[1] < len(m.entries) && m.entries[m.sel[1]].name != ".." {
			e := m.entries[m.sel[1]]
			if p := filepath.Join(m.dir, e.name); e.dir {
				m.addDir(p)
			} else {
				m.add(p)
			}
		}
	case "d":
		if m.tab == 0 {
			m.remove(m.sel[0])
		}
	}
	return false
}

/* ───────────── main ───────────── */

func plPath() string {
	d, _ := os.UserConfigDir()
	return filepath.Join(d, "muxplayer", "playlist.txt")
}

func hasVersionFlag(args []string) bool {
	for _, a := range args {
		if a == "--version" || a == "-version" {
			return true
		}
	}
	return false
}

// envCellAspect reads MUXPLAYER_CELL_ASPECT (cell height / cell width) so the
// cover can be squared by hand on terminals that do not report pixel sizes.
func envCellAspect() float64 {
	v := os.Getenv("MUXPLAYER_CELL_ASPECT")
	if v == "" {
		return 0
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f < 0.5 || f > 4 {
		return 0
	}
	return f
}

func main() {
	if hasVersionFlag(os.Args[1:]) {
		fmt.Printf("muxplayer %s\n", version)
		return
	}
	mp, err := startMPV()
	if err != nil {
		fmt.Fprintln(os.Stderr, "muxplayer: failed to start mpv (is it installed?):", err)
		os.Exit(1)
	}
	m := &model{mp: mp, cur: -1, times: 3, vol: 100, accent: rgb{200, 208, 220}, target: rgb{200, 208, 220},
		coverCh: make(chan coverRes, 2), lastRow: -1, aspectOverride: envCellAspect()}
	if b, err := os.ReadFile(plPath()); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			if _, e := os.Stat(l); l != "" && e == nil {
				m.list = append(m.list, l)
			}
		}
	}
	m.rebuild()
	start, _ := os.UserHomeDir()
	if st := filepath.Join(start, "Music"); func() bool { _, e := os.Stat(st); return e == nil }() {
		start = st
	}
	if len(os.Args) > 1 {
		if a, e := filepath.Abs(os.Args[1]); e == nil {
			start = a
		}
	}
	m.load(start)
	if len(m.list) == 0 {
		m.tab = 1
	}

	restore := rawMode()
	inCh := make(chan input, 64)
	go readInput(inCh)
	cavaCh := make(chan []float64, 4)
	cava := startCava(cavaCh)
	m.cavaOK = cava != nil
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGWINCH, syscall.SIGTERM, syscall.SIGHUP)
	tick := time.NewTicker(33 * time.Millisecond)
	evCh := mp.ev
	var prev []string
	pw, ph := 0, 0

loop:
	for {
		select {
		case in := <-inCh:
			if in.mouse {
				m.onMouse(in)
			} else if m.onKey(in.key) {
				break loop
			}
		case e, ok := <-evCh:
			if !ok {
				evCh = nil
				break
			}
			d := e["data"]
			switch e["event"] {
			case "property-change":
				switch e["name"] {
				case "time-pos":
					m.tpos, _ = d.(float64)
				case "duration":
					m.dur, _ = d.(float64)
				case "pause":
					m.paused, _ = d.(bool)
				case "volume":
					m.vol, _ = d.(float64)
				case "metadata":
					m.meta, _ = d.(map[string]any)
				}
			case "file-loaded":
				m.errs = 0
			case "end-file":
				switch e["reason"] {
				case "eof":
					m.step(1, true)
				case "error":
					if m.errs++; m.errs <= len(m.list) {
						m.step(1, true)
					}
				}
			}
		case v := <-cavaCh:
			m.bars = v
		case c := <-m.coverCh:
			if c.key == m.coverKey {
				m.cover = c.pix
				if c.pix != nil {
					var r, g, b float64
					for i := 0; i+2 < len(c.pix); i += 3 {
						r, g, b = r+float64(c.pix[i]), g+float64(c.pix[i+1]), b+float64(c.pix[i+2])
					}
					n := float64(len(c.pix) / 3)
					m.target = lift(rgb{r / n, g / n, b / n})
				}
			}
		case s := <-sig:
			if s != syscall.SIGWINCH {
				break loop
			}
			prev = nil
		case <-tick.C:
			w, h, asp := termSize()
			if m.aspectOverride > 0 {
				asp = m.aspectOverride
			}
			m.cellAspect = asp
			if w != pw || h != ph {
				pw, ph, prev = w, h, nil
				os.Stdout.WriteString("\x1b[2J")
			}
			m.draw(w, h)
			var buf bytes.Buffer
			rows := m.scr.rows()
			for i, r := range rows {
				if i >= len(prev) || prev[i] != r {
					fmt.Fprintf(&buf, "\x1b[%d;1H%s", i+1, r)
				}
			}
			prev = rows
			os.Stdout.Write(buf.Bytes())
		}
	}

	restore()
	os.MkdirAll(filepath.Dir(plPath()), 0o755)
	os.WriteFile(plPath(), []byte(strings.Join(m.list, "\n")), 0o644)
	mp.cmd.Process.Kill()
	os.Remove(mp.sock)
	if cava != nil {
		cava.Process.Kill()
	}
}
