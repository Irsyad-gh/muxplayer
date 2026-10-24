package app

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

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
