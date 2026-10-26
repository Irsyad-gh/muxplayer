package visualizer

import "math"

// fft replaces re/im with their discrete Fourier transform. Both slices must
// have the same power-of-two length. The implementation is a compact iterative
// radix-2 Cooley-Tukey FFT, so the visualizer has no FFTW/ffmpeg dependency.
func fft(re, im []float64) {
	n := len(re)
	if n <= 1 {
		return
	}

	// Bit-reversal permutation.
	j := 0
	for i := 1; i < n; i++ {
		bit := n >> 1
		for j&bit != 0 {
			j ^= bit
			bit >>= 1
		}
		j ^= bit
		if i < j {
			re[i], re[j] = re[j], re[i]
			im[i], im[j] = im[j], im[i]
		}
	}

	for length := 2; length <= n; length <<= 1 {
		angle := -2 * math.Pi / float64(length)
		wlenR, wlenI := math.Cos(angle), math.Sin(angle)
		half := length / 2
		for start := 0; start < n; start += length {
			wr, wi := 1.0, 0.0
			for k := 0; k < half; k++ {
				a := start + k
				b := a + half
				vR := re[b]*wr - im[b]*wi
				vI := re[b]*wi + im[b]*wr
				re[b] = re[a] - vR
				im[b] = im[a] - vI
				re[a] += vR
				im[a] += vI
				wr, wi = wr*wlenR-wi*wlenI, wr*wlenI+wi*wlenR
			}
		}
	}
}
