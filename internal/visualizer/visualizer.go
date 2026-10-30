// Package visualizer captures live audio and turns it into spectrum bars.
// Works with any audiocap.Source; if nobody reads fast we just drop frames.
package visualizer

import (
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"muxplayer/internal/audiocap"
)

const (
	DefaultBars    = 64
	DefaultFPS     = 30
	DefaultFFTSize = 8192
)

// Config controls a Visualizer; the zero value is fine.
type Config struct {
	Bars    int // bar count per frame; DefaultBars when 0
	FPS     int // frames per second; DefaultFPS when 0
	FFTSize int // analysis window in samples; DefaultFFTSize when 0, must be a power of two ≥ 256

	// Audio selects the capture device; ignored when Source is set.
	Audio audiocap.Options
	// Source supplies samples from a custom backend; the Visualizer owns and
	// closes it.
	Source audiocap.Source
	// OnError is called from the capture goroutine when capture fails after
	// Start returned. It must not call Close.
	OnError func(error)
}

// Visualizer captures audio, analyses it, and sends bar frames to a channel.
type Visualizer struct {
	src    audiocap.Source
	ch     chan<- []float64
	ring   *ring
	an     *Analyzer
	fps    int
	onErr  func(error)
	done   chan struct{}
	closed sync.Once
	wg     sync.WaitGroup

	samples    []float32
	bars       []float64
	captureEnd chan struct{}
}

// Start produces one frame per tick: a fresh []float64 copy of the bar levels,
// so callers can hang on to it if they want.
func Start(ch chan<- []float64, cfg Config) (*Visualizer, error) {
	bars := cfg.Bars
	if bars == 0 {
		bars = DefaultBars
	}
	fps := cfg.FPS
	if fps == 0 {
		fps = DefaultFPS
	}
	fftSize := cfg.FFTSize
	if fftSize == 0 {
		fftSize = DefaultFFTSize
	}
	if bars < 1 || bars > 1024 {
		return nil, fmt.Errorf("invalid bar count %d (max 1024)", bars)
	}
	if fps < 1 || fps > 240 {
		return nil, fmt.Errorf("invalid fps %d (1..240)", fps)
	}

	src := cfg.Source
	if src == nil {
		var err error
		src, err = audiocap.Open(cfg.Audio)
		if err != nil {
			return nil, err
		}
	}
	format := src.Format()
	if format.SampleRate <= 0 || format.Channels <= 0 {
		src.Close() // ignore error, we already have one
		return nil, fmt.Errorf("bad capture format %d Hz / %d channels", format.SampleRate, format.Channels)
	}
	an, err := NewAnalyzer(format.SampleRate, bars, fftSize)
	if err != nil {
		src.Close() // ignore error, we're returning that one instead
		return nil, err
	}

	v := &Visualizer{
		src:        src,
		ch:         ch,
		ring:       newRing(max(fftSize*2, format.SampleRate)),
		an:         an,
		fps:        fps,
		onErr:      cfg.OnError,
		done:       make(chan struct{}),
		samples:    make([]float32, fftSize),
		bars:       make([]float64, bars),
		captureEnd: make(chan struct{}),
	}
	v.wg.Add(2)
	go v.capture()
	go v.run()
	return v, nil
}

// Close releases the audio source and stops the goroutines; nil-safe.
func (v *Visualizer) Close() {
	if v == nil {
		return
	}
	v.closed.Do(func() {
		close(v.done)
		v.src.Close() // ignore error, nothing useful to do with it here
	})
	v.wg.Wait()
}

func (v *Visualizer) capture() {
	defer v.wg.Done()
	defer close(v.captureEnd)

	channels := v.src.Format().Channels
	samples := make([]float32, 4096*channels)
	mono := make([]float32, 4096)

	// note: mono mixing is dumb but easy to change later

	for {
		n, err := v.src.Read(samples)
		if n > 0 {
			frames := n / channels
			if channels == 1 {
				copy(mono[:frames], samples[:frames])
			} else {
				for f := 0; f < frames; f++ {
					var sum float32
					for c := 0; c < channels; c++ {
						sum += samples[f*channels+c]
					}
					mono[f] = sum / float32(channels)
				}
			}
			v.ring.write(mono[:frames])
		}
		if err != nil {
			if !errors.Is(err, audiocap.ErrClosed) && !errors.Is(err, io.EOF) && v.onErr != nil {
				v.onErr(err)
			}
			return
		}
	}
}

func (v *Visualizer) run() {
	defer v.wg.Done()
	ticker := time.NewTicker(time.Second / time.Duration(v.fps))
	defer ticker.Stop()

	for {
		select {
		case <-v.done:
			return
		case <-ticker.C:
			// When capture ends we still want the bars to fade out smoothly
			// instead of freezing on the last frame. Just clear the ring.
			select {
			case <-v.captureEnd:
				v.ring.clear()
			default:
			}
			v.ring.latest(v.samples)
			v.an.AnalyzeInto(v.bars, v.samples)

			frame := make([]float64, len(v.bars))
			copy(frame, v.bars)
			select {
			case v.ch <- frame:
			default:
			}
		}
	}
}
