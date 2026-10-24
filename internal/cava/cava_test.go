package cava

import (
	"strings"
	"testing"
)

func TestCavaConf(t *testing.T) {
	t.Setenv("MUXPLAYER_CAVA_INPUT", "")
	cfg := conf()
	for _, want := range []string{
		"bars = 64",
		"framerate = 30",
		"method = raw",
		"raw_target = /dev/stdout",
		"data_format = ascii",
		"bar_delimiter = 59",
		"frame_delimiter = 10",
	} {
		if !strings.Contains(cfg, want) {
			t.Errorf("cava config missing %q:\n%s", want, cfg)
		}
	}
}

func TestParseBars(t *testing.T) {
	// scan passes lines without the trailing newline.
	got := parseBars("0;250;1000;")
	if len(got) != 3 {
		t.Fatalf("got %d bars, want 3", len(got))
	}
	for i, want := range []float64{0, .25, 1} {
		if got[i] != want {
			t.Errorf("bar %d = %v, want %v", i, got[i], want)
		}
	}
}
