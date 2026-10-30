package visualizer

import (
	"fmt"
	"math"
)

const (
	minDB = -70.0 // quietest level shown
	maxDB = -5.0  // headroom above a full-scale sine

	// note: attack/release tuned by ear, not by science

	attack  = 0.55 // how quickly bars rise
	release = 0.18 // how quickly bars fall
)

// Analyzer turns mono PCM samples into spectrum bars. It's the reusable DSP
// bit and doesn't care where the audio came from.
type Analyzer struct {
	rate      int
	bars      int
	fftSize   int
	window    []float64
	windowSum float64
	re, im    []float64
	edges     []int
	levels    []float64
}

// NewAnalyzer sets up the FFT and band edges for mono samples at sampleRate.
// fftSize must be a power of two >= 256; bands span 40 Hz..16 kHz (or Nyquist).
func NewAnalyzer(sampleRate, bars, fftSize int) (*Analyzer, error) {
	if sampleRate <= 0 {
		return nil, fmt.Errorf("bad sample rate %d", sampleRate)
	}
	if bars <= 0 {
		return nil, fmt.Errorf("bad bar count %d", bars)
	}
	if fftSize < 256 || fftSize&(fftSize-1) != 0 {
		return nil, fmt.Errorf("fft size %d not a power of two >= 256", fftSize)
	}

	a := &Analyzer{
		rate:    sampleRate,
		bars:    bars,
		fftSize: fftSize,
		window:  make([]float64, fftSize),
		re:      make([]float64, fftSize),
		im:      make([]float64, fftSize),
		edges:   make([]int, bars+1),
		levels:  make([]float64, bars),
	}

	for i := range a.window {
		// Hann window keeps neighbouring bars from bleeding into each other.
		a.window[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(fftSize-1))
		a.windowSum += a.window[i]
	}

	nyquist := float64(sampleRate) / 2
	low := 40.0
	high := min(16000, nyquist*0.95)
	if high <= low*1.5 {
		low = max(20, high/8)
	}

	binHz := float64(sampleRate) / float64(fftSize)
	maxBin := fftSize / 2
	prev := 0
	for i := 0; i <= bars; i++ {
		freq := low * math.Pow(high/low, float64(i)/float64(bars))
		bin := int(math.Round(freq / binHz))
		if bin < 1 {
			bin = 1
		}
		if bin > maxBin {
			bin = maxBin
		}
		// With a big FFT several low bands can land on the same bin. Give each
		// one a bin of its own while we still have room.
		if bin <= prev && prev < maxBin {
			bin = prev + 1
		}
		a.edges[i] = bin
		prev = bin
	}
	return a, nil
}

func (a *Analyzer) Bands() int { return a.bars }

// AnalyzeInto writes bar levels (0..1) into dst. It uses the newest fftSize
// samples, zero-pads the front, and smooths the result over time.
func (a *Analyzer) AnalyzeInto(dst []float64, samples []float32) []float64 {
	for i := range a.re {
		a.re[i] = 0
		a.im[i] = 0
	}

	n := len(samples)
	if n > a.fftSize {
		samples = samples[n-a.fftSize:]
		n = a.fftSize
	}
	offset := a.fftSize - n
	for i := 0; i < n; i++ {
		a.re[offset+i] = float64(samples[i]) * a.window[offset+i]
	}

	fft(a.re, a.im)

	bars := min(len(dst), a.bars)
	for i := 0; i < bars; i++ {
		peak := 0.0
		for k := a.edges[i]; k < a.edges[i+1]; k++ {
			power := a.re[k]*a.re[k] + a.im[k]*a.im[k]
			if power > peak {
				peak = power
			}
		}

		// A windowed sine of amplitude A has a peak magnitude of about
		// A*windowSum/2, so this normalisation recovers A in 0..1.
		amp := math.Sqrt(peak) * 2 / a.windowSum
		db := 20 * math.Log10(amp+1e-12)
		raw := (db - minDB) / (maxDB - minDB)
		raw = min(1, max(0, raw))

		prev := a.levels[i]
		if raw >= prev {
			prev += (raw - prev) * attack
		} else {
			prev += (raw - prev) * release
		}
		a.levels[i] = prev
		dst[i] = prev
	}
	return dst
}
