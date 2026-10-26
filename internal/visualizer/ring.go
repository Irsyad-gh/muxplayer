package visualizer

import "sync"

// ring is a small overwrite-on-full sample ring shared by the capture reader
// and the UI frame timer.
type ring struct {
	mu   sync.Mutex
	data []float32
	pos  int // index of the oldest sample
	n    int
}

func newRing(capacity int) *ring {
	if capacity < 1 {
		capacity = 1
	}
	return &ring{data: make([]float32, capacity)}
}

func (r *ring) write(samples []float32) {
	if len(samples) == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(samples) >= len(r.data) {
		copy(r.data, samples[len(samples)-len(r.data):])
		r.pos, r.n = 0, len(r.data)
		return
	}
	if overflow := r.n + len(samples) - len(r.data); overflow > 0 {
		r.pos = (r.pos + overflow) % len(r.data)
		r.n -= overflow
	}
	w := (r.pos + r.n) % len(r.data)
	c := copy(r.data[w:], samples)
	copy(r.data, samples[c:])
	r.n += len(samples)
}

// latest writes the most recent len(dst) samples to dst in chronological order,
// zero-padding at the start when fewer samples have been captured so far.
func (r *ring) latest(dst []float32) {
	clear(dst)
	r.mu.Lock()
	defer r.mu.Unlock()

	take := min(len(dst), r.n)
	if take == 0 {
		return
	}
	start := (r.pos + r.n - take) % len(r.data)
	c := copy(dst[len(dst)-take:], r.data[start:])
	if c < take {
		copy(dst[len(dst)-take+c:], r.data[:take-c])
	}
}

func (r *ring) clear() {
	r.mu.Lock()
	r.pos, r.n = 0, 0
	r.mu.Unlock()
}
