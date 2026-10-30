package cover

// note: these tests only run when ffmpeg is on PATH

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSize(t *testing.T) {
	// A 2:1 cell aspect keeps the classic square buffer, scaled down slightly.
	if pw, ph := Size(36, 30, 2); pw != 30 || ph != 30 {
		t.Errorf("2:1 aspect: got %dx%d, want 30x30", pw, ph)
	}
	// Square cells: the buffer is pre-stretched tall so it still looks square.
	if pw, ph := Size(36, 30, 1); pw != 16 || ph != 32 {
		t.Errorf("1:1 aspect: got %dx%d, want 16x32", pw, ph)
	}
	// The region must never exceed the vertical space (h-11 cells).
	for _, asp := range []float64{0.5, 0.8, 1, 1.3, 1.6, 2, 2.4, 3, 4} {
		pw, ph := Size(200, 30, asp)
		if ph/2 > 19 {
			t.Errorf("aspect %v: cover is %d cells tall, max is 19", asp, ph/2)
		}
		if pw%2 != 0 || ph%2 != 0 {
			t.Errorf("aspect %v: buffer %dx%d must be even", asp, pw, ph)
		}
	}
	// Too narrow to be worth drawing.
	if pw, ph := Size(6, 30, 2); pw != 0 || ph != 0 {
		t.Errorf("tiny panel: got %dx%d, want 0x0", pw, ph)
	}
	// Out-of-range aspects fall back to the default.
	if pw, ph := Size(36, 30, 99); pw != 30 || ph != 30 {
		t.Errorf("invalid aspect: got %dx%d, want 30x30", pw, ph)
	}
}

func TestVF(t *testing.T) {
	filter := vf(18, 36)
	if !strings.Contains(filter, "crop=min(iw\\,ih):min(iw\\,ih)") {
		t.Errorf("filter should centre-crop to a square: %q", filter)
	}
	if !strings.HasSuffix(filter, "scale=18:36") {
		t.Errorf("filter should stretch to the buffer size: %q", filter)
	}
}

func TestLoadDimensions(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	// A deliberately non-square source to exercise the square crop.
	src := filepath.Join(dir, "art.png")
	if err := exec.Command("ffmpeg", "-v", "quiet", "-f", "lavfi",
		"-i", "testsrc=size=200x120:duration=1", "-frames:v", "1", src).Run(); err != nil {
		t.Skipf("could not generate test image: %v", err)
	}

	for _, size := range [][2]int{{18, 18}, {18, 36}, {36, 18}} {
		pix := Load(src, size[0], size[1])
		if len(pix) != size[0]*size[1]*3 {
			t.Errorf("Load(%d, %d) returned %d bytes, want %d",
				size[0], size[1], len(pix), size[0]*size[1]*3)
		}
	}
}
