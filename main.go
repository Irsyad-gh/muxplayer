// Command muxplayer is a terminal music player. It drives mpv over its JSON
// IPC socket, draws cava's spectrum, and uses only the Go standard library.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// version is reported by --version. Override it at build time with:
//
//	go build -ldflags "-X main.version=1.2.3"
var version = "0.1.0"

/* ───────────── logging ───────────── */

// The TUI owns stdout, so every diagnostic goes to stderr with a severity.
// DEBUG lines stay silent unless MUXPLAYER_DEBUG is set.
const (
	lvlDebug = "DEBUG"
	lvlWarn  = "WARN"
	lvlError = "ERROR"
)

var debugOn = os.Getenv("MUXPLAYER_DEBUG") != ""

func logMsg(level, msg string) {
	if level != lvlDebug || debugOn {
		fmt.Fprintf(os.Stderr, "%-5s muxplayer: %s\n", level, msg)
	}
}

func logDebugf(format string, a ...any) { logMsg(lvlDebug, fmt.Sprintf(format, a...)) }
func logWarnf(format string, a ...any)  { logMsg(lvlWarn, fmt.Sprintf(format, a...)) }
func logErrf(format string, a ...any)   { logMsg(lvlError, fmt.Sprintf(format, a...)) }

/* ───────────── screen ───────────── */

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
	off := (w - utf8.RuneCountInString(str)) / 2
	s.text(x+off, y, str, fg, bg, at, -1)
}

func (s *screen) rows() []string {
	out := make([]string, s.h)
	for y := 0; y < s.h; y++ {
		out[y] = s.rowStr(y)
	}
	return out
}

// rowStr renders one screen row, emitting an SGR sequence only when the
// attributes or colours change between adjacent cells.
func (s *screen) rowStr(y int) string {
	var b strings.Builder
	fg, bg, at := int32(-2), int32(-2), uint8(255)
	for x := 0; x < s.w; x++ {
		c := s.c[y*s.w+x]
		if c.fg != fg || c.bg != bg || c.at != at {
			b.WriteString(sgr(c))
			fg, bg, at = c.fg, c.bg, c.at
		}
		b.WriteRune(c.r)
	}
	b.WriteString("\x1b[0m")
	return b.String()
}

func sgr(c cell) string {
	var b strings.Builder
	b.WriteString("\x1b[0")
	if c.at&aBold != 0 {
		b.WriteString(";1")
	}
	if c.at&aDim != 0 {
		b.WriteString(";2")
	}
	if c.at&aUnder != 0 {
		b.WriteString(";4")
	}
	if c.fg >= 0 {
		fmt.Fprintf(&b, ";38;2;%d;%d;%d", c.fg>>16&255, c.fg>>8&255, c.fg&255)
	}
	if c.bg >= 0 {
		fmt.Fprintf(&b, ";48;2;%d;%d;%d", c.bg>>16&255, c.bg>>8&255, c.bg&255)
	}
	b.WriteByte('m')
	return b.String()
}

/* ───────────── terminal ───────────── */

const (
	enterScreen = "\x1b[?1049h\x1b[?25l\x1b[?1000h\x1b[?1002h\x1b[?1006h\x1b[2J"
	leaveScreen = "\x1b[?1006l\x1b[?1002l\x1b[?1000l\x1b[?25h\x1b[?1049l"

	minAspect     = 0.5
	maxAspect     = 4.0
	defaultAspect = 2.0
)

// cellAspect derives height / width from the terminal's pixel size. Missing or
// implausible data yields the 2.0 default.
func cellAspect(xpixel, ypixel uint16, w, h int) float64 {
	if xpixel == 0 || ypixel == 0 || w <= 0 || h <= 0 {
		return defaultAspect
	}
	cellW := float64(xpixel) / float64(w)
	cellH := float64(ypixel) / float64(h)
	if cellW <= 0 {
		return defaultAspect
	}
	asp := cellH / cellW
	if asp < minAspect || asp > maxAspect {
		return defaultAspect
	}
	return asp
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

/* ───────────── input ───────────── */

type input struct {
	key          string
	mouse, press bool
	btn, mx, my  int
}

var arrowKeys = map[byte]string{
	'A': "up", 'B': "down", 'C': "right", 'D': "left", 'H': "home", 'F': "end",
}

var pageKeys = map[byte]string{'5': "pgup", '6': "pgdn"}

var controlKeys = map[rune]string{
	13: "enter", 9: "tab", 32: "space", 127: "backspace", 8: "backspace",
	3: "ctrl-c", 27: "esc",
}

func readInput(ch chan<- input) {
	buf := make([]byte, 512)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			return
		}
		rest := string(buf[:n])
		for len(rest) > 0 {
			in, used := parseInput(rest)
			rest = rest[used:]
			if in.mouse || in.key != "" {
				ch <- in
			}
		}
	}
}

// parseInput decodes the first event in s and returns how many bytes it used.
func parseInput(s string) (input, int) {
	switch {
	case strings.HasPrefix(s, "\x1b[<"):
		return parseMouse(s)
	case strings.HasPrefix(s, "\x1b[") && len(s) >= 3:
		return parseCSI(s)
	default:
		return parseRune(s)
	}
}

func parseMouse(s string) (input, int) {
	end := strings.IndexAny(s, "Mm")
	if end < 0 {
		return input{}, len(s)
	}
	var btn, x, y int
	fmt.Sscanf(s[3:end], "%d;%d;%d", &btn, &x, &y)
	in := input{mouse: true, btn: btn, mx: x - 1, my: y - 1, press: s[end] == 'M'}
	return in, end + 1
}

func parseCSI(s string) (input, int) {
	if k, ok := arrowKeys[s[2]]; ok {
		return input{key: k}, 3
	}
	if len(s) >= 4 && s[3] == '~' {
		if k, ok := pageKeys[s[2]]; ok {
			return input{key: k}, 4
		}
	}
	return input{}, 2
}

func parseRune(s string) (input, int) {
	r, size := utf8.DecodeRuneInString(s)
	if k, ok := controlKeys[r]; ok {
		return input{key: k}, size
	}
	return input{key: string(r)}, size
}

/* ───────────── mpv (JSON IPC) ───────────── */

type mpv struct {
	conn io.ReadWriteCloser
	cmd  *exec.Cmd
	sock string
	ev   chan map[string]any
}

// startMPV launches mpv in idle mode with a private IPC endpoint and connects.
func startMPV() (*mpv, error) {
	addr := mpvAddr()
	cmd := exec.Command("mpv", mpvArgs(addr)...)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	conn, err := dialMPV(addr)
	if err != nil {
		_ = cmd.Process.Kill()
		return nil, err
	}
	m := &mpv{conn: conn, cmd: cmd, sock: addr, ev: make(chan map[string]any, 128)}
	go m.readEvents()
	m.observe()
	return m, nil
}

func mpvArgs(addr string) []string {
	return []string{
		"--idle=yes", "--no-video", "--no-terminal",
		"--force-window=no", "--input-ipc-server=" + addr,
	}
}

func (m *mpv) readEvents() {
	sc := bufio.NewScanner(m.conn)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var e map[string]any
		ok := json.Unmarshal(sc.Bytes(), &e) == nil && e["event"] != nil
		if ok {
			m.ev <- e
		}
	}
	close(m.ev)
}

func (m *mpv) observe() {
	props := []string{"time-pos", "duration", "pause", "metadata", "volume"}
	for i, p := range props {
		m.do("observe_property", i+1, p)
	}
}

func (m *mpv) do(args ...any) {
	if m == nil {
		return
	}
	msg, _ := json.Marshal(map[string]any{"command": args})
	_, _ = m.conn.Write(append(msg, '\n'))
}

/* ───────────── cava ───────────── */

const cavaBars = 64

func startCava(ch chan<- []float64) *exec.Cmd {
	if _, err := exec.LookPath("cava"); err != nil {
		logDebugf("cava not found; visualizer disabled")
		return nil
	}
	cfg := filepath.Join(os.TempDir(), fmt.Sprintf("muxplayer-cava-%d.conf", os.Getpid()))
	if err := os.WriteFile(cfg, []byte(cavaConf()), 0o600); err != nil {
		logWarnf("cannot write cava config: %v", err)
		return nil
	}
	cmd := exec.Command("cava", "-p", cfg)
	out, err := cmd.StdoutPipe()
	if err != nil || cmd.Start() != nil {
		return nil
	}
	go scanBars(out, ch)
	return cmd
}

func cavaConf() string {
	conf := fmt.Sprintf("[general]\nbars = %d\nframerate = 30\n", cavaBars)
	if src := os.Getenv("MUXPLAYER_CAVA_INPUT"); src != "" {
		conf += "[input]\nmethod = " + src + "\n"
	}
	return conf + "[output]\nmethod = raw\nraw_target = /dev/stdout\n" +
		"data_format = ascii\nascii_max_range = 1000\n" +
		"bar_delimiter = 59\nframe_delimiter = 10\n"
}

func scanBars(out io.Reader, ch chan<- []float64) {
	sc := bufio.NewScanner(out)
	for sc.Scan() {
		v := parseBars(sc.Text())
		select {
		case ch <- v:
		default:
		}
	}
}

func parseBars(line string) []float64 {
	parts := strings.Split(strings.TrimRight(line, ";"), ";")
	v := make([]float64, len(parts))
	for i, p := range parts {
		n, _ := strconv.Atoi(p)
		v[i] = float64(n) / 1000
	}
	return v
}

/* ───────────── colors & cover ───────────── */

type rgb [3]float64

func pack(c rgb) int32 { return int32(int(c[0])<<16 | int(c[1])<<8 | int(c[2])) }

func mix(a, b rgb, t float64) rgb {
	return rgb{a[0] + (b[0]-a[0])*t, a[1] + (b[1]-a[1])*t, a[2] + (b[2]-a[2])*t}
}

func lum(c rgb) float64 { return .299*c[0] + .587*c[1] + .114*c[2] }

// lift keeps the accent readable on a dark, transparent background.
func lift(c rgb) rgb {
	l := lum(c)
	if l >= 120 {
		return c
	}
	return mix(c, rgb{255, 255, 255}, (120-l)/120*.85)
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

var audioExt = map[string]bool{
	".mp3": true, ".flac": true, ".ogg": true, ".opus": true, ".wav": true,
	".m4a": true, ".aac": true, ".wma": true, ".ape": true,
}

var coverNames = []string{"cover.jpg", "folder.jpg", "cover.png", "folder.png", "front.jpg"}

func isAudio(p string) bool {
	return audioExt[strings.ToLower(filepath.Ext(p))]
}

// coverFill scales the cover down from the largest size that fits, leaving a
// little breathing room around it.
const coverFill = 0.85

// coverSize returns the cover's device-pixel buffer size for a panel rw cells
// wide and h cells tall. The buffer is a pre-distorted square: it is stretched
// by the inverse of the cell aspect so, drawn with half blocks, it appears as
// a true 1:1 square.
func coverSize(rw, h int, aspect float64) (pw, ph int) {
	if aspect < minAspect || aspect > maxAspect {
		aspect = defaultAspect
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
	rows := clamp(int(math.Round(float64(pw)/aspect)), 1, maxRows)
	return pw, rows * 2
}

// coverVF builds the ffmpeg filter that centre-crops the artwork to a square
// and then stretches it to the target device-pixel size.
func coverVF(pw, ph int) string {
	return fmt.Sprintf("crop=min(iw\\,ih):min(iw\\,ih),scale=%d:%d", pw, ph)
}

func loadCover(path string, w, h int) []byte {
	if pix := ffmpegCover(path, w, h); pix != nil {
		return pix
	}
	for _, name := range coverNames {
		p := filepath.Join(filepath.Dir(path), name)
		if !exists(p) {
			continue
		}
		if pix := ffmpegCover(p, w, h); pix != nil {
			return pix
		}
	}
	logDebugf("no cover art for %s", base(path))
	return nil
}

func ffmpegCover(src string, w, h int) []byte {
	args := []string{
		"-v", "quiet", "-i", src, "-an", "-frames:v", "1",
		"-vf", coverVF(w, h), "-f", "rawvideo", "-pix_fmt", "rgb24", "-",
	}
	out, err := exec.Command("ffmpeg", args...).Output()
	if err != nil || len(out) != w*h*3 {
		return nil
	}
	return out
}

/* ───────────── model ───────────── */

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

type rect struct{ x, y, w, h int }

func (r rect) in(x, y int) bool {
	return x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h
}

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
	cellAspect     float64 // cell height / cell width, keeps the cover square
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

func base(p string) string {
	return strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
}

func trunc(s string, n int) string {
	if n < 0 {
		n = 0
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n > 1 {
		return string(r[:n-1]) + "…"
	}
	return string(r[:n])
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
			s, _ := v.(string)
			return s
		}
	}
	return ""
}

func indexOf(xs []int, v int) int {
	for i, x := range xs {
		if x == v {
			return i
		}
	}
	return -1
}

func indexOfString(xs []string, v string) int {
	for i, x := range xs {
		if x == v {
			return i
		}
	}
	return -1
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

/* ───────────── playback order ───────────── */

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
	m.cur, m.played, m.tpos, m.meta = idx, 1, 0, nil
	m.findPos(idx)
	m.mp.do("loadfile", m.list[idx], "replace")
	m.mp.do("set_property", "pause", false)
	m.target = lift(hashColor(m.list[idx]))
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
	m.mp.do("stop")
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
	return len(m.list) - 1
}

func (m *model) addDir(dir string) int {
	n := 0
	walk := func(p string, e os.DirEntry, err error) error {
		if err == nil && !e.IsDir() && isAudio(p) {
			m.add(p)
			n++
		}
		return nil
	}
	_ = filepath.WalkDir(dir, walk)
	return n
}

func (m *model) remove(i int) {
	if i < 0 || i >= len(m.list) {
		return
	}
	m.list = append(m.list[:i], m.list[i+1:]...)
	switch {
	case i < m.cur:
		m.cur--
	case i == m.cur:
		m.cur = -1
		m.mp.do("stop")
	}
	m.rebuild()
}

func (m *model) load(dir string) {
	m.dir = dir
	m.entries = []entry{{"..", true}}
	es, _ := os.ReadDir(dir)
	var dirs, files []entry
	for _, e := range es {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if entryIsDir(dir, name, e) {
			dirs = append(dirs, entry{name, true})
			continue
		}
		if isAudio(name) {
			files = append(files, entry{name, false})
		}
	}
	m.entries = append(m.entries, dirs...)
	m.entries = append(m.entries, files...)
	m.sel[1], m.top[1] = 0, 0
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
	return explorerRows(m.dir, m.entries, m.cur, m.list)
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

func explorerRows(dir string, entries []entry, cur int, list []string) []row {
	out := make([]row, 0, len(entries))
	for _, e := range entries {
		label := "  " + e.name
		if e.dir {
			label = "▸ " + e.name + "/"
		}
		playing := !e.dir && cur >= 0 && list[cur] == filepath.Join(dir, e.name)
		out = append(out, row{label, playing})
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
		return
	}
	m.play(m.add(p))
}

func (m *model) seekFrac(x int) {
	frac := float64(x-m.hBar.x) / float64(max(1, m.hBar.w-1))
	m.mp.do("seek", math.Max(0, math.Min(1, frac))*m.dur, "absolute")
}

/* ───────────── drawing ───────────── */

const (
	barWidth  = 3
	barLevels = "▁▂▃▄▅▆▇"
)

const helpText = " Tab:panel  Enter:play  Space:pause  n/p:track  " +
	"s:shuffle  r:repeat  [ ]:N×  ←→:seek  +/-:vol  " +
	"a:add  d:delete  q:quit"

// layout holds the geometry computed once per frame.
type layout struct {
	w, h       int
	lx, ly, lw int
	rx, rw     int
	sepY, vh   int
	listH      int
}

func (m *model) draw(w, h int) {
	s := m.resetScreen(w, h)
	if w < 60 || h < 18 {
		s.text(1, 1, "terminal too small (min 60x18)", def, def, 0, -1)
		return
	}
	m.easeAccent()
	acc, onAcc := m.colors()
	box := m.layout(w, h)
	m.drawTabs(s, box, acc, onAcc)
	m.drawList(s, box, acc, onAcc)
	m.drawVisualizer(s, box, acc)
	ty := m.drawCover(s, box, acc, onAcc)
	m.drawPlayer(s, box, acc, ty)
	s.text(1, h-1, trunc(helpText, w-2), def, def, aDim, -1)
}

func (m *model) resetScreen(w, h int) *screen {
	if m.scr == nil || m.scr.w != w || m.scr.h != h {
		m.scr = &screen{w: w, h: h, c: make([]cell, w*h)}
	}
	for i := range m.scr.c {
		m.scr.c[i] = cell{' ', def, def, 0}
	}
	return m.scr
}

func (m *model) easeAccent() {
	for i := range m.accent {
		m.accent[i] += (m.target[i] - m.accent[i]) * .12
	}
}

func (m *model) colors() (acc, onAcc int32) {
	acc = pack(m.accent)
	onAcc = 0x101010
	if lum(m.accent) < 110 {
		onAcc = 0xf0f0f0
	}
	return acc, onAcc
}

func (m *model) layout(w, h int) layout {
	lx, ly := 2, 3
	lw := (w - 7) * 62 / 100
	rx := lx + lw + 3
	rw := w - rx - 2
	vh := max(5, h/3)
	sepY := h - vh - 2
	m.rightX, m.listY = rx, ly
	return layout{
		w: w, h: h, lx: lx, ly: ly, lw: lw, rx: rx, rw: rw,
		sepY: sepY, vh: vh, listH: sepY - ly - 1,
	}
}

func (m *model) drawTabs(s *screen, box layout, acc, onAcc int32) {
	x := box.lx
	for i, label := range []string{" Playlist ", " Explorer "} {
		fg, bg, at := def, def, aDim
		if i == m.tab {
			fg, bg, at = onAcc, acc, aBold
		}
		s.text(x, 1, label, fg, bg, at, -1)
		m.hTab[i] = rect{x, 1, len(label), 1}
		x += len(label) + 1
	}
	info := fmt.Sprintf("%d songs", len(m.list))
	if m.tab == 1 {
		info = m.dir
	}
	info = trunc(info, box.lw-24)
	s.text(box.lx+box.lw-utf8.RuneCountInString(info), 1, info, def, def, aDim, -1)
}

func (m *model) drawList(s *screen, box layout, acc, onAcc int32) {
	rows := m.listRows()
	n := len(rows)
	sel, top := &m.sel[m.tab], &m.top[m.tab]
	m.scrollTo(n, sel, top, box.listH)
	m.hList = rect{box.lx, box.ly, box.lw, box.listH}
	for i := 0; i < box.listH && *top+i < n; i++ {
		idx, y := *top+i, box.ly+i
		r := rows[idx]
		fg, bg, at := def, def, uint8(0)
		if r.playing {
			fg, at = acc, aBold
		}
		if idx == *sel {
			fg, bg = onAcc, acc
			fillRow(s, box.lx, y, box.lw, acc)
		}
		s.text(box.lx+1, y, r.text, fg, bg, at, box.lw-2)
	}
	m.drawScrollbar(s, box, *top, n, acc)
}

func (m *model) scrollTo(n int, sel, top *int, listH int) {
	if n > 0 {
		*sel = clamp(*sel, 0, n-1)
	} else {
		*sel = 0
	}
	if *sel < *top {
		*top = *sel
	}
	if *sel >= *top+listH {
		*top = *sel - listH + 1
	}
	*top = max(0, *top)
}

func (m *model) drawScrollbar(s *screen, box layout, top, n int, acc int32) {
	if n <= box.listH || box.listH <= 0 {
		return
	}
	th := max(1, box.listH*box.listH/n)
	ty := top * (box.listH - th) / max(1, n-box.listH)
	for i := 0; i < box.listH; i++ {
		if i >= ty && i < ty+th {
			s.text(box.lx+box.lw, box.ly+i, "┃", acc, def, 0, 1)
			continue
		}
		s.text(box.lx+box.lw, box.ly+i, "│", def, def, aDim, 1)
	}
}

func fillRow(s *screen, x, y, w int, bg int32) {
	for xx := x; xx < x+w; xx++ {
		s.c[y*s.w+xx] = cell{' ', def, bg, 0}
	}
}

func (m *model) drawVisualizer(s *screen, box layout, acc int32) {
	s.text(box.lx, box.sepY, strings.Repeat("─", box.lw+1), acc, def, aDim, -1)
	if !m.cavaOK {
		msg := "cava not found — visualizer disabled"
		s.ctr(box.lx, box.lw, box.sepY+box.vh/2, msg, def, def, aDim)
		return
	}
	if len(m.bars) > 0 {
		m.drawBars(s, box, acc)
	}
}

func (m *model) drawBars(s *screen, box layout, acc int32) {
	nb := box.lw / barWidth
	for b := 0; b < nb; b++ {
		lo, hi := b*len(m.bars)/nb, (b+1)*len(m.bars)/nb
		v := barsMax(m.bars, lo, hi)
		eighths := int(math.Min(1, v) * float64(box.vh*8))
		col := pack(mix(m.accent, rgb{255, 255, 255}, v*.45))
		m.drawBar(s, box, b, eighths, col)
	}
}

func barsMax(bars []float64, lo, hi int) float64 {
	v := 0.0
	for k := lo; k < max(hi, lo+1) && k < len(bars); k++ {
		v = math.Max(v, bars[k])
	}
	return v
}

func (m *model) drawBar(s *screen, box layout, b, eighths int, col int32) {
	for r := 0; r < box.vh; r++ {
		ch := barGlyph(eighths - r*8)
		if ch == 0 {
			continue
		}
		y := box.h - 2 - r
		s.text(box.lx+b*barWidth, y, string([]rune{ch, ch}), col, def, 0, -1)
	}
}

func barGlyph(eighths int) rune {
	switch {
	case eighths >= 8:
		return '█'
	case eighths > 0:
		return []rune(barLevels)[eighths-1]
	}
	return 0
}

// drawCover paints the square cover and returns the y row for the player block.
func (m *model) drawCover(s *screen, box layout, acc, onAcc int32) int {
	pw, ph := coverSize(box.rw, box.h, m.cellAspect)
	if pw < 8 {
		return 1
	}
	cw, ch := pw, ph/2
	cx := box.rx + (box.rw-cw)/2
	m.requestCover(pw, ph)
	m.blitCover(s, cx, cw, ch, pw, acc, onAcc)
	return 1 + ch + 1
}

// requestCover starts an async load when the track or target size changes.
func (m *model) requestCover(pw, ph int) {
	if m.cur < 0 {
		return
	}
	key := fmt.Sprintf("%s|%d|%d", m.list[m.cur], pw, ph)
	if key == m.coverKey {
		return
	}
	m.coverKey, m.cover, m.pw, m.ph = key, nil, pw, ph
	path := m.list[m.cur]
	go func() { m.coverCh <- coverRes{key, loadCover(path, pw, ph)} }()
}

func (m *model) blitCover(s *screen, cx, cw, ch, pw int, acc, onAcc int32) {
	ready := m.cover != nil && m.pw == pw && m.ph == ch*2
	for r := 0; r < ch; r++ {
		for c := 0; c < cw; c++ {
			s.c[(1+r)*s.w+cx+c] = m.coverCell(r, c, pw, acc, ready)
		}
	}
	if !ready {
		s.ctr(cx, cw, 1+ch/2, "music cover", onAcc, acc, 0)
	}
}

func (m *model) coverCell(r, c, pw int, acc int32, ready bool) cell {
	if !ready {
		return cell{' ', def, acc, 0}
	}
	top := m.coverPixel(r*2, c, pw)
	bottom := m.coverPixel(r*2+1, c, pw)
	return cell{'▀', top, bottom, 0}
}

func (m *model) coverPixel(row, col, pw int) int32 {
	i := (row*pw + col) * 3
	r := int32(m.cover[i]) << 16
	g := int32(m.cover[i+1]) << 8
	return r | g | int32(m.cover[i+2])
}

func (m *model) drawPlayer(s *screen, box layout, acc int32, ty int) {
	s.ctr(box.rx, box.rw, ty, m.nowTitle(), def, def, aBold)
	by := ty + 2
	m.drawProgress(s, box, acc, by)
	m.drawButtons(s, box, acc, by)
	s.ctr(box.rx, box.rw, by+5, fmt.Sprintf("vol %d%%", int(m.vol)), def, def, aDim)
}

func (m *model) nowTitle() string {
	if m.cur < 0 {
		return "muxplayer"
	}
	title := m.metaGet("title")
	if title == "" {
		title = base(m.list[m.cur])
	}
	if artist := m.metaGet("artist"); artist != "" {
		title += " – " + artist
	}
	return title
}

func (m *model) drawProgress(s *screen, box layout, acc int32, y int) {
	m.hBar = rect{box.rx, y, box.rw, 1}
	pos := 0
	if m.dur > 0 {
		pos = int(math.Min(1, m.tpos/m.dur) * float64(box.rw-1))
	}
	for i := 0; i < box.rw; i++ {
		ch, col, at := "━", def, aDim
		switch {
		case i == pos:
			ch, at = "●", aBold
		case i < pos:
			ch, col, at = "━", acc, 0
		}
		s.text(box.rx+i, y, ch, col, def, at, 1)
	}
	s.text(box.rx, y+1, fmtT(m.tpos), def, def, aDim, -1)
	s.text(box.rx+box.rw-5, y+1, fmtT(m.dur), def, def, aDim, -1)
}

func (m *model) drawButtons(s *screen, box layout, acc int32, y int) {
	labels := []string{m.repeatLabel(), "|◀", m.playLabel(), "▶|", m.shuffleLabel()}
	xs := m.buttonXs(box, labels[4])
	for i, t := range labels {
		fg, at := def, aBold
		if (i == 0 && m.repeat > 0) || (i == 4 && m.shuffle > 0) {
			fg = acc
		}
		s.text(xs[i], y, t, fg, def, at, -1)
		w := utf8.RuneCountInString(t) + 2
		m.hBtn[i] = rect{xs[i] - 1, y, w, 1}
	}
}

func (m *model) repeatLabel() string {
	labels := []string{"↻ off", "↻ all", "↻ one", fmt.Sprintf("↻ %d×", m.times)}
	return labels[m.repeat]
}

func (m *model) shuffleLabel() string {
	return []string{"⇄ off", "⇄ bag", "⇄ folder"}[m.shuffle]
}

func (m *model) playLabel() string {
	if m.cur >= 0 && !m.paused {
		return "‖"
	}
	return "▶"
}

func (m *model) buttonXs(box layout, last string) []int {
	return []int{
		box.rx,
		box.rx + box.rw*3/10 - 1,
		box.rx + box.rw/2 - 1,
		box.rx + box.rw*7/10 - 1,
		box.rx + box.rw - utf8.RuneCountInString(last),
	}
}

/* ───────────── input handling ───────────── */

func (m *model) onMouse(in input) {
	switch {
	case in.btn == 64 || in.btn == 65:
		m.onWheel(in)
	case in.btn == 32 && m.drag:
		m.seekFrac(in.mx)
	case in.btn == 0 && !in.press:
		m.drag = false
	case in.btn == 0 && in.press:
		m.onClick(in.mx, in.my)
	}
}

func (m *model) onWheel(in input) {
	dir := 1
	if in.btn == 64 {
		dir = -1
	}
	if in.mx >= m.rightX {
		m.mp.do("add", "volume", -dir*5)
		return
	}
	m.sel[m.tab] += dir * 3
	m.top[m.tab] = max(0, m.top[m.tab]+dir*3)
}

func (m *model) onClick(x, y int) {
	m.clickTab(x, y)
	m.clickList(x, y)
	m.clickBar(x, y)
	m.clickButton(x, y)
}

func (m *model) clickTab(x, y int) {
	for i, r := range m.hTab {
		if r.in(x, y) {
			m.tab = i
		}
	}
}

func (m *model) clickList(x, y int) {
	if !m.hList.in(x, y) {
		return
	}
	i := m.top[m.tab] + y - m.hList.y
	if i >= len(m.listRows()) {
		return
	}
	m.sel[m.tab] = i
	if i == m.lastRow && time.Since(m.lastClk) < 400*time.Millisecond {
		m.activate()
	}
	m.lastRow, m.lastClk = i, time.Now()
}

func (m *model) clickBar(x, y int) {
	if m.hBar.in(x, y) && m.dur > 0 {
		m.drag = true
		m.seekFrac(x)
	}
}

func (m *model) clickButton(x, y int) {
	for i, r := range m.hBtn {
		if r.in(x, y) {
			m.activateButton(i)
			return
		}
	}
}

func (m *model) activateButton(i int) {
	switch i {
	case 0:
		m.cycleRepeat()
	case 1:
		m.prev()
	case 2:
		m.toggle()
	case 3:
		m.step(1, false)
	case 4:
		m.cycleShuffle()
	}
}

func (m *model) prev() {
	if m.tpos > 3 {
		m.mp.do("seek", 0, "absolute")
		return
	}
	m.step(-1, false)
}

func (m *model) toggle() {
	if m.cur < 0 {
		m.play(m.sel[0])
		return
	}
	m.mp.do("cycle", "pause")
}

func (m *model) cycleShuffle() {
	m.shuffle = (m.shuffle + 1) % 3
	m.rebuild()
}

func (m *model) cycleRepeat() {
	m.repeat, m.played = (m.repeat+1)%4, 1
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
		m.cycleShuffle()
	case "r":
		m.cycleRepeat()
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
		m.goUpDir()
	case "a":
		m.addSelected()
	case "d":
		m.deleteSelected()
	}
	return false
}

func (m *model) goUpDir() {
	if m.tab == 1 {
		m.load(filepath.Dir(m.dir))
	}
}

func (m *model) addSelected() {
	if m.tab != 1 || m.sel[1] >= len(m.entries) {
		return
	}
	e := m.entries[m.sel[1]]
	if e.name == ".." {
		return
	}
	p := filepath.Join(m.dir, e.name)
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

/* ───────────── main ───────────── */

func plPath() string {
	dir, _ := os.UserConfigDir()
	return filepath.Join(dir, "muxplayer", "playlist.txt")
}

func hasVersionFlag(args []string) bool {
	for _, a := range args {
		if a == "--version" || a == "-version" {
			return true
		}
	}
	return false
}

// envCellAspect reads MUXPLAYER_CELL_ASPECT (cell height / width) so the cover
// can be squared by hand on terminals that report no pixel size.
func envCellAspect() float64 {
	v := os.Getenv("MUXPLAYER_CELL_ASPECT")
	if v == "" {
		return 0
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f < minAspect || f > maxAspect {
		return 0
	}
	return f
}

func newModel(mp *mpv) *model {
	m := &model{
		mp: mp, cur: -1, times: 3, vol: 100,
		accent: rgb{200, 208, 220}, target: rgb{200, 208, 220},
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

// startDir picks the folder to open on launch: a CLI argument if given, else
// ~/Music when it exists, else the home directory.
func startDir(args []string) string {
	home, _ := os.UserHomeDir()
	dir := home
	if music := filepath.Join(home, "Music"); exists(music) {
		dir = music
	}
	if len(args) > 0 {
		if abs, err := filepath.Abs(args[0]); err == nil {
			dir = abs
		}
	}
	return dir
}

func main() {
	if hasVersionFlag(os.Args[1:]) {
		fmt.Printf("muxplayer %s\n", version)
		return
	}
	mp, err := startMPV()
	if err != nil {
		logErrf("failed to start mpv (is it installed?): %v", err)
		os.Exit(1)
	}
	m := newModel(mp)
	m.load(startDir(os.Args[1:]))
	if len(m.list) == 0 {
		m.tab = 1
	}
	run(m)
}

// view tracks the previous frame for diffed rendering and resize handling.
type view struct {
	prev []string
	w, h int
}

func (v *view) invalidate() { v.prev = nil }

func (v *view) render(m *model) {
	w, h, aspect := termSize()
	if m.aspectOverride > 0 {
		aspect = m.aspectOverride
	}
	m.cellAspect = aspect
	if w != v.w || h != v.h {
		v.w, v.h, v.prev = w, h, nil
		_, _ = os.Stdout.WriteString("\x1b[2J")
	}
	m.draw(w, h)
	v.flush(m.scr.rows())
}

func (v *view) flush(rows []string) {
	var buf bytes.Buffer
	for i, row := range rows {
		if i < len(v.prev) && v.prev[i] == row {
			continue
		}
		fmt.Fprintf(&buf, "\x1b[%d;1H%s", i+1, row)
	}
	v.prev = rows
	_, _ = os.Stdout.Write(buf.Bytes())
}

func run(m *model) {
	restore := rawMode()
	inCh := make(chan input, 64)
	go readInput(inCh)
	cavaCh := make(chan []float64, 4)
	cava := startCava(cavaCh)
	m.cavaOK = cava != nil
	sigCh := make(chan os.Signal, 1)
	watchSignals(sigCh)
	tick := time.NewTicker(33 * time.Millisecond)

	defer shutdown(m, cava, restore, sigCh, tick)

	evCh := m.mp.ev
	v := &view{}
	for {
		select {
		case in := <-inCh:
			if m.handleInput(in) {
				return
			}
		case ev, ok := <-evCh:
			if !ok {
				evCh = nil
				continue
			}
			m.handleEvent(ev)
		case bars := <-cavaCh:
			m.bars = bars
		case res := <-m.coverCh:
			m.handleCover(res)
		case s := <-sigCh:
			if !isResize(s) {
				return
			}
			v.invalidate()
		case <-tick.C:
			v.render(m)
		}
	}
}

func (m *model) handleInput(in input) bool {
	if in.mouse {
		m.onMouse(in)
		return false
	}
	return m.onKey(in.key)
}

func (m *model) handleEvent(e map[string]any) {
	switch e["event"] {
	case "property-change":
		m.handleProperty(e)
	case "file-loaded":
		m.errs = 0
	case "end-file":
		m.handleEndFile(e)
	}
}

func (m *model) handleProperty(e map[string]any) {
	d := e["data"]
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
}

func (m *model) handleEndFile(e map[string]any) {
	switch e["reason"] {
	case "eof":
		m.step(1, true)
	case "error":
		m.errs++
		if m.errs <= len(m.list) {
			m.step(1, true)
		}
	}
}

func (m *model) handleCover(res coverRes) {
	if res.key != m.coverKey {
		return
	}
	m.cover = res.pix
	if res.pix != nil {
		m.target = lift(avgColor(res.pix))
	}
}

func avgColor(pix []byte) rgb {
	if len(pix) < 3 {
		return rgb{}
	}
	var r, g, b float64
	for i := 0; i+2 < len(pix); i += 3 {
		r, g, b = r+float64(pix[i]), g+float64(pix[i+1]), b+float64(pix[i+2])
	}
	n := float64(len(pix) / 3)
	return rgb{r / n, g / n, b / n}
}

// shutdown centralises process cleanup: terminal, playlist, children, signals.
func shutdown(m *model, cava *exec.Cmd, restore func(), sigCh chan os.Signal, tick *time.Ticker) {
	restore()
	m.savePlaylist()
	if m.mp.cmd != nil && m.mp.cmd.Process != nil {
		_ = m.mp.cmd.Process.Kill()
	}
	cleanupMPV(m.mp.sock)
	if cava != nil {
		_ = cava.Process.Kill()
	}
	signal.Stop(sigCh)
	tick.Stop()
}

func (m *model) savePlaylist() {
	path := plPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		logWarnf("cannot create config dir: %v", err)
		return
	}
	data := []byte(strings.Join(m.list, "\n"))
	if err := os.WriteFile(path, data, 0o644); err != nil {
		logWarnf("cannot save playlist: %v", err)
	}
}
