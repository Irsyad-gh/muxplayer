package app

// note: cover cache is tiny and dumb, but decoding every frame was worse

import (
	"fmt"

	"muxplayer/internal/cover"
)

// drawCover paints the square cover and returns the y row for the player block.
func (m *model) drawCover(s *screen, box layout, acc, onAcc int32) int {
	pw, ph := cover.Size(box.rw, box.h, m.cellAspect)
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
	go func() { m.coverCh <- coverRes{key, cover.Load(path, pw, ph)} }()
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
