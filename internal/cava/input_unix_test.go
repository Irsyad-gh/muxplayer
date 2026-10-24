//go:build !windows

package cava

import (
	"strings"
	"testing"
)

func TestCavaInputOverride(t *testing.T) {
	t.Setenv("MUXPLAYER_CAVA_INPUT", "pipewire")
	if cfg := conf(); !strings.Contains(cfg, "[input]\nmethod = pipewire\n") {
		t.Fatalf("input override missing from config:\n%s", cfg)
	}
}
