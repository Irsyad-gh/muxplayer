//go:build !windows

package cava

import (
	"os"
	"os/exec"
)

// find locates cava on PATH.
func find() (string, error) { return exec.LookPath("cava") }

// inputConf returns the optional [input] section, letting the user force a
// backend with MUXPLAYER_CAVA_INPUT. On Unix any backend cava was built with
// can be selected this way.
func inputConf() string {
	src := os.Getenv("MUXPLAYER_CAVA_INPUT")
	if src == "" {
		return ""
	}
	return "[input]\nmethod = " + src + "\n"
}
