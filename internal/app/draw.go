package app

import (
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"muxplayer/internal/cover"
)

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
	acc = cover.Pack(m.accent)
	onAcc = 0x101010
	if cover.Lum(m.accent) < 110 {
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
		*sel = min(max(*sel, 0), n-1)
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
		col := cover.Pack(cover.Mix(m.accent, cover.RGB{255, 255, 255}, v*.45))
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

func (m *model) drawPlayer(s *screen, box layout, acc int32, ty int) {
	s.ctr(box.rx, box.rw, ty, m.nowTitle(), def, def, aBold)
	by := ty + 2
	m.drawProgress(s, box, acc, by)
	m.drawButtons(s, box, acc, by+3)
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
