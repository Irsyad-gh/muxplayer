//go:build windows

package cava

import (
	"os"
	"os/exec"
	"path/filepath"
)

// find locates cava.exe. The official MSI installs it under
// %LOCALAPPDATA%\cava and appends that directory to the user PATH, but a
// terminal opened before the install keeps its old environment, so fall back
// to the known install locations.
func find() (string, error) {
	if p, err := exec.LookPath("cava"); err == nil {
		return p, nil
	}
	dirs := []string{
		os.Getenv("LOCALAPPDATA"),
		os.Getenv("ProgramFiles"),
		os.Getenv("ProgramFiles(x86)"),
	}
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		p := filepath.Join(dir, "cava", "cava.exe")
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	return "", exec.ErrNotFound
}

// inputConf is empty on Windows: cava's winscap backend always captures
// the default output device and the Windows build rejects any explicit input
// method, so MUXPLAYER_CAVA_INPUT cannot be honoured there.
func inputConf() string { return "" }
