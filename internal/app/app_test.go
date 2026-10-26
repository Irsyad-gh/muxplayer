package app

import (
	"testing"

	"muxplayer/internal/cover"
)

func newTestModel() *model {
	m := &model{cur: -1, times: 3, vol: 80, accent: cover.RGB{200, 208, 220}, target: cover.RGB{200, 208, 220},
		coverCh: make(chan coverRes, 4), vizOK: true, bars: []float64{.1, .5, .9, 1, .3, .7}}
	for _, p := range []string{"/a/1.mp3", "/a/2.mp3", "/b/3.mp3", "/b/4.mp3", "/c/5.mp3"} {
		m.list = append(m.list, p)
	}
	m.rebuild()
	m.load("/tmp")
	return m
}

func TestDrawSizes(t *testing.T) {
	m := newTestModel()
	m.play(1)
	for w := 40; w <= 220; w += 7 {
		for h := 10; h <= 70; h += 5 {
			m.draw(w, h)
			m.scr.rows()
		}
	}
}

func TestVisualizerBarsAreMirrored(t *testing.T) {
	m := newTestModel()
	m.bars = make([]float64, 64)
	for i := range m.bars {
		m.bars[i] = float64((i*7)%11+1) / 11
	}

	const w, h = 100, 30
	m.draw(w, h)
	box := m.layout(w, h)
	slots, offset := barSlots(box.lw + 1)
	if slots < 5 {
		t.Fatalf("not enough bar slots: %d", slots)
	}

	left := offset
	right := box.lw + 1 - (slots*barWidth - 1) - offset
	if left < 0 || right < 0 || left-right > 1 || right-left > 1 {
		t.Fatalf("visualizer margins are not balanced: left=%d right=%d", left, right)
	}

	for i := 0; i < slots/2; i++ {
		j := slots - 1 - i
		for r := 0; r < box.vh; r++ {
			y := box.h - 2 - r
			for c := 0; c < 2; c++ {
				a := m.scr.c[y*w+box.lx+offset+i*barWidth+c]
				b := m.scr.c[y*w+box.lx+offset+j*barWidth+c]
				if a != b {
					t.Fatalf("bar %d and %d differ at row %d", i, j, r)
				}
			}
		}
	}
}

func TestBarSlotsAreBalanced(t *testing.T) {
	for width := 2; width < 300; width++ {
		slots, offset := barSlots(width)
		content := slots*barWidth - 1
		right := width - content - offset
		if slots < 1 || content > width || offset < 0 || right < 0 {
			t.Fatalf("width %d: slots=%d offset=%d content=%d right=%d", width, slots, offset, content, right)
		}
		if offset-right > 1 || right-offset > 1 {
			t.Fatalf("width %d: unbalanced margins left=%d right=%d", width, offset, right)
		}
	}
}

func TestMirrorBandsAreSymmetric(t *testing.T) {
	for slots := 1; slots <= 101; slots++ {
		for slot := 0; slot < slots; slot++ {
			lo, hi := mirrorBands(slot, slots, 64)
			lo2, hi2 := mirrorBands(slots-1-slot, slots, 64)
			if lo != lo2 || hi != hi2 {
				t.Fatalf("slots=%d slot=%d maps to %d:%d but mirror maps to %d:%d", slots, slot, lo, hi, lo2, hi2)
			}
			if lo < 0 || lo >= 64 || hi <= lo || hi > 64 {
				t.Fatalf("slots=%d slot=%d has invalid band range %d:%d", slots, slot, lo, hi)
			}
		}
	}
}

func TestShuffleRepeat(t *testing.T) {
	m := newTestModel()
	// shuffle off + repeat off: stops at the end
	m.play(4)
	m.step(1, true)
	if m.cur != -1 {
		t.Fatal("should stop at the end")
	}
	// repeat N×: the same track plays 3 times
	m.repeat = 3
	m.play(0)
	for i := 0; i < 2; i++ {
		m.step(1, true)
		if m.cur != 0 {
			t.Fatal("should repeat track 0")
		}
	}
	m.step(1, true)
	if m.cur != 1 {
		t.Fatal("should advance to track 1, got", m.cur)
	}
	// bag shuffle: every track plays once before any repeats
	m.repeat, m.shuffle = 1, 1
	m.rebuild()
	m.play(2)
	seen := map[int]bool{2: true}
	for i := 0; i < 4; i++ {
		m.step(1, true)
		if seen[m.cur] {
			t.Fatal("track repeated before the bag was exhausted")
		}
		seen[m.cur] = true
	}
	// folder shuffle: tracks from the same folder stay together
	m.shuffle = 2
	m.rebuild()
	for i := 0; i+1 < len(m.order); i++ {
		a, b := m.order[i], m.order[i+1]
		if (a == 0 && b != 1 && b != 2 && b != 4) || (a == 2 && b != 3 && b != 0 && b != 4) {
			t.Fatal("folder order broken", m.order)
		}
	}
	m.remove(0)
	if len(m.list) != 4 {
		t.Fatal("remove failed")
	}
}
