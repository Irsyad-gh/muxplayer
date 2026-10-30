//go:build linux || darwin

package debugmode

// note: terminal detection is guesswork; there is no standard way to ask

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
)

// writeLauncher creates the shell script a terminal emulator runs. The script
// runs the UI as a foreground child, so the terminal keeps stdin connected,
// then records the exit status for the debug console.
func writeLauncher(dir, self string, args []string, logPath, statusPath, pidPath string) (string, error) {
	name := "launch.sh"
	if runtime.GOOS == "darwin" {
		name = "launch.command"
	}
	path := filepath.Join(dir, name)
	cwd, err := os.Getwd()
	if err != nil {
		cwd = dir
	}

	var b strings.Builder
	b.WriteString("#!/bin/sh\n")
	b.WriteString("export MUXPLAYER_DEBUG=1\n")
	fmt.Fprintf(&b, "export %s=%s\n", pidEnv, shQuote(pidPath))
	fmt.Fprintf(&b, "cd %s || { printf '1' > %s; exit 1; }\n", shQuote(cwd), shQuote(statusPath))
	fmt.Fprintf(&b, "%s %s", shQuote(self), shQuote(childFlag))
	for _, a := range args {
		fmt.Fprintf(&b, " %s", shQuote(a))
	}
	fmt.Fprintf(&b, " 2>>%s\n", shQuote(logPath))
	b.WriteString("code=$?\n")
	fmt.Fprintf(&b, "printf '%%d' \"$code\" > %s\n", shQuote(statusPath+".tmp"))
	fmt.Fprintf(&b, "mv %s %s\n", shQuote(statusPath+".tmp"), shQuote(statusPath))

	if err := os.WriteFile(path, []byte(b.String()), 0o700); err != nil {
		return "", fmt.Errorf("write debug launcher: %w", err)
	}
	return path, nil
}

// launchTerminal picks an installed terminal emulator. MUXPLAYER_TERMINAL and
// TERMINAL override autodetection; tmux is used when no display is available.
func launchTerminal(script string) (*exec.Cmd, error) {
	if name := os.Getenv("MUXPLAYER_TERMINAL"); name != "" {
		path, err := exec.LookPath(name)
		if err != nil {
			return nil, fmt.Errorf("terminal %q not found: %w", name, err)
		}
		return terminalCommand(path, specArgs(filepath.Base(path)), script), nil
	}
	if name := os.Getenv("TERMINAL"); name != "" {
		if path, err := exec.LookPath(name); err == nil {
			return terminalCommand(path, specArgs(filepath.Base(path)), script), nil
		}
	}
	if runtime.GOOS == "darwin" {
		if _, err := exec.LookPath("open"); err == nil {
			return exec.Command("open", "-a", "Terminal", script), nil
		}
	}
	if !hasDisplay() {
		if cmd := tmuxCommand(script); cmd != nil {
			return cmd, nil
		}
	}
	for _, name := range knownTerminals {
		path, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		return terminalCommand(path, specArgs(name), script), nil
	}
	if cmd := tmuxCommand(script); cmd != nil {
		return cmd, nil
	}
	return nil, errors.New("no terminal found; set MUXPLAYER_TERMINAL (e.g. kitty)")
}

func hasDisplay() bool {
	return os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != ""
}

func tmuxCommand(script string) *exec.Cmd {
	if os.Getenv("TMUX") == "" {
		return nil
	}
	path, err := exec.LookPath("tmux")
	if err != nil {
		return nil
	}
	return exec.Command(path, "new-window", "-n", "muxplayer", script)
}

func terminalCommand(path string, extra []string, script string) *exec.Cmd {
	args := append([]string{}, extra...)
	args = append(args, script)
	return exec.Command(path, args...)
}

// knownTerminals is ordered from the most common desktop defaults to the
// small X11 fallbacks. Each one is probed with exec.LookPath.
var knownTerminals = []string{
	"x-terminal-emulator",
	"gnome-terminal",
	"kgx",
	"ptyxis",
	"konsole",
	"xfce4-terminal",
	"mate-terminal",
	"tilix",
	"terminator",
	"kitty",
	"alacritty",
	"foot",
	"wezterm",
	"ghostty",
	"rio",
	"urxvt",
	"rxvt",
	"st",
	"xterm",
	"lxterminal",
	"qterminal",
	"sakura",
	"terminology",
	"cool-retro-term",
}

// specArgs returns the arguments a terminal needs before the command to run.
func specArgs(name string) []string {
	name = strings.TrimSuffix(filepath.Base(name), ".exe")
	switch name {
	case "gnome-terminal":
		return []string{"--wait", "--"}
	case "kgx", "ptyxis":
		return []string{"--"}
	case "xfce4-terminal":
		return []string{"--disable-server", "-x"}
	case "mate-terminal":
		return []string{"-x"}
	case "terminator":
		return []string{"-x"}
	case "wezterm":
		return []string{"start", "--"}
	case "kitty", "foot":
		return nil
	default:
		return []string{"-e"}
	}
}

// shQuote makes a POSIX shell single-quoted argument.
func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func terminationSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP}
}

// terminate asks the UI to shut down; the app restores the terminal before
// exiting, so the debug window is left in a usable state.
func terminate(pid int) {
	p, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	p.Signal(syscall.SIGTERM) // ignore error, process may be gone
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func terminalHint() string {
	return "set MUXPLAYER_TERMINAL to a terminal emulator"
}
