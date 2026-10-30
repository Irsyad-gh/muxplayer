package visualizer

// note: the fake source sleeps 1ms per read to look like a real audio callback

import (
	"math"
	"sync"
	"testing"
	"time"

	"muxplayer/internal/audiocap"
)

type fakeSource struct {
	rate, channels int
	freq           float64
	pos            int
	closed         chan struct{}
	once           sync.Once
}

func newFakeSource() *fakeSource {
	return &fakeSource{rate: 48000, channels: 2, freq: 440, closed: make(chan struct{})}
}

func (f *fakeSource) Format() audiocap.Format {
	return audiocap.Format{SampleRate: f.rate, Channels: f.channels}
}

func (f *fakeSource) Read(dst []float32) (int, error) {
	select {
	case <-f.closed:
		return 0, audiocap.ErrClosed
	default:
	}
	frames := len(dst) / f.channels
	for i := 0; i < frames; i++ {
		v := float32(math.Sin(2 * math.Pi * f.freq * float64(f.pos+i) / float64(f.rate)))
		for c := 0; c < f.channels; c++ {
			dst[i*f.channels+c] = v
		}
	}
	f.pos += frames
	time.Sleep(time.Millisecond)
	return frames * f.channels, nil
}

func (f *fakeSource) Close() error {
	f.once.Do(func() { close(f.closed) })
	return nil
}

func TestVisualizerFrames(t *testing.T) {
	src := newFakeSource()
	frames := make(chan []float64, 8)
	v, err := Start(frames, Config{Bars: 32, FPS: 30, FFTSize: 4096, Source: src})
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()

	deadline := time.After(2 * time.Second)
	var got []float64
	for got == nil {
		select {
		case got = <-frames:
		case <-deadline:
			t.Fatal("no visualizer frame received")
		}
	}
	if len(got) != 32 {
		t.Fatalf("frame has %d bars, want 32", len(got))
	}
	best := 0.0
	for _, v := range got {
		if v > best {
			best = v
		}
	}
	if best <= 0 {
		t.Fatalf("all bars are zero: %v", got)
	}
}

func TestVisualizerInvalidConfig(t *testing.T) {
	if _, err := Start(make(chan []float64), Config{Bars: -1, Source: newFakeSource()}); err == nil {
		t.Fatal("negative bar count accepted")
	}
	if _, err := Start(make(chan []float64), Config{FPS: -1, Source: newFakeSource()}); err == nil {
		t.Fatal("negative frame rate accepted")
	}
}
