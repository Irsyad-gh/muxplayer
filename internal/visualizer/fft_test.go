package visualizer

import (
	"math"
	"testing"
)

func dft(re, im []float64) ([]float64, []float64) {
	n := len(re)
	outR := make([]float64, n)
	outI := make([]float64, n)
	for k := 0; k < n; k++ {
		for j := 0; j < n; j++ {
			angle := -2 * math.Pi * float64(k*j) / float64(n)
			outR[k] += re[j]*math.Cos(angle) - im[j]*math.Sin(angle)
			outI[k] += re[j]*math.Sin(angle) + im[j]*math.Cos(angle)
		}
	}
	return outR, outI
}

func TestFFTMatchesDFT(t *testing.T) {
	re := []float64{1, -2, 3.5, 0, 4, -1, 2, .25}
	im := []float64{0, .5, -1, 2, 0, 1.5, -2, 0}
	wantR, wantI := dft(re, im)

	fft(re, im)
	for i := range re {
		if math.Abs(re[i]-wantR[i]) > 1e-9 || math.Abs(im[i]-wantI[i]) > 1e-9 {
			t.Fatalf("bin %d = (%v, %v), want (%v, %v)", i, re[i], im[i], wantR[i], wantI[i])
		}
	}
}
