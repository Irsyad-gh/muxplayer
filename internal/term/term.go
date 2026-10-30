// Package term handles raw mode, the alternate screen, window size, and signals.
package term

// note: escape sequences are hard-coded; terminfo can wait

const (
	enterScreen = "\x1b[?1049h\x1b[?25l\x1b[?1000h\x1b[?1002h\x1b[?1006h\x1b[2J"
	leaveScreen = "\x1b[?1006l\x1b[?1002l\x1b[?1000l\x1b[?25h\x1b[?1049l"

	// MinAspect and MaxAspect bound plausible cell height/width ratios;
	// DefaultAspect is used when the terminal reports no pixel size.
	MinAspect     = 0.5
	MaxAspect     = 4.0
	DefaultAspect = 2.0
)

// cellAspect derives height / width from the terminal's pixel size. Missing or
// implausible data yields the default.
func cellAspect(xpixel, ypixel uint16, w, h int) float64 {
	if xpixel == 0 || ypixel == 0 || w <= 0 || h <= 0 {
		return DefaultAspect
	}
	cellW := float64(xpixel) / float64(w)
	cellH := float64(ypixel) / float64(h)
	if cellW <= 0 {
		return DefaultAspect
	}
	asp := cellH / cellW
	if asp < MinAspect || asp > MaxAspect {
		return DefaultAspect
	}
	return asp
}
