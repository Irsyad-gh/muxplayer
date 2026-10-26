package visualizer

import "testing"

func TestRingLatestPadsAndKeepsOrder(t *testing.T) {
	r := newRing(4)
	r.write([]float32{1, 2})

	out := make([]float32, 4)
	r.latest(out)
	want := []float32{0, 0, 1, 2}
	for i := range want {
		if out[i] != want[i] {
			t.Fatalf("latest = %v, want %v", out, want)
		}
	}

	r.write([]float32{3, 4, 5})
	r.latest(out)
	want = []float32{2, 3, 4, 5}
	for i := range want {
		if out[i] != want[i] {
			t.Fatalf("latest = %v, want %v", out, want)
		}
	}
}

func TestRingKeepsNewestSamples(t *testing.T) {
	r := newRing(4)
	r.write([]float32{1, 2, 3, 4, 5, 6})

	out := make([]float32, 4)
	r.latest(out)
	want := []float32{3, 4, 5, 6}
	for i := range want {
		if out[i] != want[i] {
			t.Fatalf("latest = %v, want %v", out, want)
		}
	}
}

func TestRingClear(t *testing.T) {
	r := newRing(4)
	r.write([]float32{1, 2, 3})
	r.clear()
	out := make([]float32, 3)
	r.latest(out)
	for i, v := range out {
		if v != 0 {
			t.Fatalf("out[%d] = %v after clear", i, v)
		}
	}
}
