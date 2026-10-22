package main

import "testing"

func newTestModel() *model {
	m := &model{cur: -1, times: 3, vol: 80, accent: rgb{200, 208, 220}, target: rgb{200, 208, 220},
		coverCh: make(chan coverRes, 4), cavaOK: true, bars: []float64{.1, .5, .9, 1, .3, .7}}
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
