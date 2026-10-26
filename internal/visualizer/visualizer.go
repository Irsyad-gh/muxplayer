// Package visualizer turns captured audio into normalised spectrum bars.
//
// Start opens the platform audio capture by default, but accepts any
// audiocap.Source through Config. The capture reader, FFT analyser and frame
// timer are independent of the terminal UI: callers receive a []float64 of bar
// heights in the range 0..1 on a channel and can render it however they like.
//
// Typical use:
//
//	frames := make(chan []float64, 4)
//	v, err := visualizer.Start(frames, visualizer.Config{})
//	if err != nil {
//		// visualizer is unavailable; the rest of the program can keep running
//	}
//	defer v.Close()
//	for bars := range frames {
//		draw(bars)
//	}
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

// Config controls a Visualizer. The zero value is valid.
type Config struct {
	// Bars is the number of level values per frame. Zero uses DefaultBars.
	Bars int

	// FPS is the number of frames per second. Zero uses DefaultFPS.
	FPS int

	// FFTSize is the analysis window in samples. Zero uses DefaultFFTSize.
	// It must be a power of two and at least 256.
	FFTSize int

	// Audio configures the built-in system-audio capture. It is ignored when
	// Source is set.
	Audio audiocap.Options

	// Source supplies samples from a custom capture backend. The Visualizer
	// takes ownership and closes it. When nil, audiocap.Open is used.
	Source audiocap.Source

	// OnError, when set, is called from the capture goroutine if capture fails
	// after Start returned. It should return quickly and must not call Close.
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

// Start opens audio capture and begins producing frames. Each frame is a fresh
// slice, so callers may retain it. Sends are non-blocking: if the channel is not
// being read, the newest frame is dropped rather than slowing audio capture.
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
		return nil, fmt.Errorf("visualizer: invalid bar count %d", bars)
	}
	if fps < 1 || fps > 240 {
		return nil, fmt.Errorf("visualizer: invalid frame rate %d", fps)
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
		_ = src.Close()
		return nil, fmt.Errorf("visualizer: invalid capture format %d Hz / %d channels", format.SampleRate, format.Channels)
	}
	an, err := NewAnalyzer(format.SampleRate, bars, fftSize)
	if err != nil {
		_ = src.Close()
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

// Close stops the visualizer and releases the audio source. It is safe to call
// more than once and is safe on a nil receiver.
func (v *Visualizer) Close() {
	if v == nil {
		return
	}
	v.closed.Do(func() {
		close(v.done)
		_ = v.src.Close()
	})
	v.wg.Wait()
}

func (v *Visualizer) capture() {
	defer v.wg.Done()
	defer close(v.captureEnd)

	channels := v.src.Format().Channels
	samples := make([]float32, 4096*channels)
	mono := make([]float32, 4096)

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
			// Once capture stops, discard stale samples and let the bars fall
			// to zero instead of freezing on the last frame.
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
