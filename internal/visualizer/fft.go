package visualizer

import "math"

// fft replaces re/im with their DFT in place. Both slices need the same
// power-of-two length. It's a plain radix-2 Cooley-Tukey, no FFTW needed.
func fft(re, im []float64) {
	n := len(re)
	if n <= 1 {
		return
	}

	// Bit-reversal permutation — always takes a minute to read.
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

	// note: this is the 3rd version and we're still not happy

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
