package term

import "testing"

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
