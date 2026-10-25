//go:build windows

package debugmode

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

// writeLauncher creates the batch file a new console window runs. The batch
// file runs the UI in the foreground and records its exit status afterwards.
func writeLauncher(dir, self string, args []string, logPath, statusPath, pidPath string) (string, error) {
	path := filepath.Join(dir, "launch.cmd")
	cwd, err := os.Getwd()
	if err != nil {
		cwd = dir
	}

	var b strings.Builder
	b.WriteString("@echo off\r\n")
	b.WriteString("setlocal\r\n")
	b.WriteString("set \"MUXPLAYER_DEBUG=1\"\r\n")
	fmt.Fprintf(&b, "set \"%s=%s\"\r\n", pidEnv, batValue(pidPath))
	fmt.Fprintf(&b, "cd /d %s\r\n", batArg(cwd))
	fmt.Fprintf(&b, "%s %s", batArg(self), batArg(childFlag))
	for _, a := range args {
		fmt.Fprintf(&b, " %s", batArg(a))
	}
	fmt.Fprintf(&b, " 2>>%s\r\n", batArg(logPath))
	fmt.Fprintf(&b, "(echo %%ERRORLEVEL%%)> %s\r\n", batArg(statusPath+".tmp"))
	fmt.Fprintf(&b, "move /Y %s %s >nul\r\n", batArg(statusPath+".tmp"), batArg(statusPath))

	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		return "", fmt.Errorf("cannot write debug launcher: %w", err)
	}
	return path, nil
}

// launchTerminal starts the launcher in a fresh console window. Windows
// Terminal becomes the default when the user has configured it as such.
func launchTerminal(script string) (*exec.Cmd, error) {
	return exec.Command("cmd.exe", "/c", "start", "", script), nil
}

// batArg quotes a batch-file argument. Percent signs are doubled so they are
// not mistaken for environment variable references.
func batArg(s string) string {
	return `"` + batValue(s) + `"`
}

func batValue(s string) string {
	s = strings.ReplaceAll(s, "%", "%%")
	return strings.ReplaceAll(s, `"`, `""`)
}

var (
	kernel32               = syscall.NewLazyDLL("kernel32.dll")
	procOpenProcess        = kernel32.NewProc("OpenProcess")
	procGetExitCodeProcess = kernel32.NewProc("GetExitCodeProcess")
	procCloseHandle        = kernel32.NewProc("CloseHandle")
)

const (
	processQueryLimitedInformation = 0x1000
	stillActive                    = 259
)

func terminationSignals() []os.Signal {
	return []os.Signal{os.Interrupt}
}

func terminate(pid int) {
	p, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	_ = p.Kill()
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	h, _, _ := procOpenProcess.Call(processQueryLimitedInformation, 0, uintptr(uint32(pid)))
	if h == 0 {
		return false
	}
	defer procCloseHandle.Call(h)
	var code uint32
	ok, _, _ := procGetExitCodeProcess.Call(h, uintptr(unsafe.Pointer(&code)))
	return ok != 0 && code == stillActive
}

func terminalHint() string {
	return "check that cmd.exe can open a new console window"
}
