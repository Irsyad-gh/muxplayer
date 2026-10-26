package app

import "time"

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
		m.mp.Do("add", "volume", -dir*5)
		return
	}
	m.sel[m.tab] += dir * 3
	m.top[m.tab] = max(0, m.top[m.tab]+dir*3)
}

func (m *model) onClick(x, y int) {
	m.clickTab(x, y)
	m.clickSearch(x, y)
	m.clickList(x, y)
	m.clickBar(x, y)
	m.clickButton(x, y)
}

// clickSearch focuses the search bar or flips one of its filter chips.
func (m *model) clickSearch(x, y int) {
	if m.tab != 1 {
		return
	}
	switch {
	case m.hSearch.in(x, y):
		m.searching = true
	case m.hType.in(x, y):
		m.cycleType()
	case m.hOrder.in(x, y):
		m.toggleOrder()
	}
}

func (m *model) clickTab(x, y int) {
	for i, r := range m.hTab {
		if r.in(x, y) {
			m.searching = false
			m.tab = i
		}
	}
}

func (m *model) clickList(x, y int) {
	if !m.hList.in(x, y) {
		return
	}
	m.searching = false
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
