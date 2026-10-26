package visualizer

import (
	"math"
	"testing"
)

func sine(rate int, freq float64, n int) []float32 {
	out := make([]float32, n)
	for i := range out {
		out[i] = float32(math.Sin(2 * math.Pi * freq * float64(i) / float64(rate)))
	}
	return out
}

func analyze(a *Analyzer, samples []float32, times int) []float64 {
	bars := make([]float64, a.Bands())
	for i := 0; i < times; i++ {
		a.AnalyzeInto(bars, samples)
	}
	return bars
}

func peak(bars []float64) int {
	best := 0
	for i := range bars {
		if bars[i] > bars[best] {
			best = i
		}
	}
	return best
}

func TestAnalyzerTone(t *testing.T) {
	a, err := NewAnalyzer(48000, 64, 8192)
	if err != nil {
		t.Fatal(err)
	}
	bars := analyze(a, sine(48000, 1000, 8192), 12)
	if got := bars[peak(bars)]; got < .5 {
		t.Fatalf("1 kHz tone peaked at %v, want >= .5 (%v)", got, bars)
	}
}

func TestAnalyzerBandOrder(t *testing.T) {
	low, err := NewAnalyzer(48000, 64, 8192)
	if err != nil {
		t.Fatal(err)
	}
	high, err := NewAnalyzer(48000, 64, 8192)
	if err != nil {
		t.Fatal(err)
	}
	lowBars := analyze(low, sine(48000, 150, 8192), 8)
	highBars := analyze(high, sine(48000, 8000, 8192), 8)
	if peak(lowBars) >= peak(highBars) {
		t.Fatalf("150 Hz peaked at bar %d, 8 kHz at bar %d; want low < high", peak(lowBars), peak(highBars))
	}
}

func TestAnalyzerSilence(t *testing.T) {
	a, err := NewAnalyzer(48000, 64, 8192)
	if err != nil {
		t.Fatal(err)
	}
	bars := analyze(a, make([]float32, 8192), 2)
	for i, v := range bars {
		if v != 0 {
			t.Fatalf("silent bar %d = %v, want 0", i, v)
		}
	}
}

func TestAnalyzerDecays(t *testing.T) {
	a, err := NewAnalyzer(48000, 64, 8192)
	if err != nil {
		t.Fatal(err)
	}
	tone := sine(48000, 440, 8192)
	bars := analyze(a, tone, 12)
	before := bars[peak(bars)]

	for frame := 0; frame < 25; frame++ {
		a.AnalyzeInto(bars, make([]float32, 8192))
		for i, v := range bars {
			if v < 0 || v > 1 {
				t.Fatalf("bar %d out of range: %v", i, v)
			}
		}
	}
	after := bars[peak(bars)]
	if after >= before*.05 {
		t.Fatalf("bars did not decay enough: %v -> %v", before, after)
	}
}

func TestAnalyzerValidation(t *testing.T) {
	for _, tc := range []struct {
		rate, bars, fft int
	}{
		{0, 64, 8192},
		{48000, 0, 8192},
		{48000, 64, 1000},
		{48000, 64, 128},
	} {
		if _, err := NewAnalyzer(tc.rate, tc.bars, tc.fft); err == nil {
			t.Errorf("NewAnalyzer(%d, %d, %d) succeeded, want error", tc.rate, tc.bars, tc.fft)
		}
	}
}
