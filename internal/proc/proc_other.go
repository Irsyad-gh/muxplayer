//go:build !windows

// Package proc ties child processes to the lifetime of muxplayer.
package proc

import "os/exec"

// KillOnExit is a no-op outside Windows; the shutdown path kills children.
func KillOnExit(cmd *exec.Cmd) {}
