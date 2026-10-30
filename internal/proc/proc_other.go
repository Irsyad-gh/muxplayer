//go:build !windows

// Package proc makes sure child processes die when muxplayer does.
package proc

// note: windows has the real job-object code; everywhere else the normal shutdown path is enough

import "os/exec"

// KillOnExit is a no-op outside Windows; the shutdown path kills children.
func KillOnExit(cmd *exec.Cmd) {}
