package audiocap

// note: the buffer blocks forever if nobody writes and nobody closes, which is on purpose

import "sync"

// buffer turns push-based capture into the pull-based Source API. It keeps the
// newest audio and drops old samples when the consumer falls behind.
type buffer struct {
	mu     sync.Mutex
	cond   *sync.Cond
	data   []float32
	r, n   int
	format Format
	err    error
}

func newBuffer(format Format, capacity int) *buffer {
	if capacity < 1 {
		capacity = 1
	}
	b := &buffer{data: make([]float32, capacity), format: format}
	b.cond = sync.NewCond(&b.mu)
	return b
}

func (b *buffer) Format() Format { return b.format }

// push copies samples into the ring. If the ring is full, the oldest samples
// are discarded.
func (b *buffer) push(samples []float32) {
	if len(samples) == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.err != nil {
		return
	}
	if len(samples) >= len(b.data) {
		copy(b.data, samples[len(samples)-len(b.data):])
		b.r, b.n = 0, len(b.data)
		b.cond.Broadcast()
		return
	}
	if overflow := b.n + len(samples) - len(b.data); overflow > 0 {
		b.r = (b.r + overflow) % len(b.data)
		b.n -= overflow
	}
	w := (b.r + b.n) % len(b.data)
	c := copy(b.data[w:], samples)
	copy(b.data, samples[c:])
	b.n += len(samples)
	b.cond.Broadcast()
}

func (b *buffer) Read(dst []float32) (int, error) {
	if len(dst) == 0 {
		return 0, nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for b.n == 0 && b.err == nil {
		b.cond.Wait()
	}
	if b.n == 0 {
		return 0, b.err
	}
	n := min(len(dst), b.n)
	first := min(n, len(b.data)-b.r)
	c := copy(dst, b.data[b.r:b.r+first])
	if c < n {
		copy(dst[c:], b.data[:n-c])
	}
	b.r = (b.r + n) % len(b.data)
	b.n -= n
	return n, nil
}

// close marks the stream finished. Buffered samples are still delivered before
// err is returned.
func (b *buffer) close(err error) {
	if err == nil {
		err = ErrClosed
	}
	b.mu.Lock()
	if b.err == nil {
		b.err = err
	}
	b.mu.Unlock()
	b.cond.Broadcast()
}
