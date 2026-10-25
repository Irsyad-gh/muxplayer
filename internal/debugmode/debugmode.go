// Package debugmode runs the player UI in a separate terminal window and
// mirrors its diagnostic log in the original window.
package debugmode

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"muxplayer/internal/diag"
)

const (
	childFlag = "--debug-child"
	pidEnv    = "MUXPLAYER_DEBUG_PID"
)

// EnterChild is called by the hidden child process that owns the new terminal
// window. It publishes its PID so the debug console can stop it cleanly.
func EnterChild() {
	diag.Debugf("debug UI process started (pid %d)", os.Getpid())
	diag.Debugf("terminal debug mode: diagnostics are mirrored in the original window")
	if path := os.Getenv(pidEnv); path != "" {
		tmp := path + ".tmp"
		if os.WriteFile(tmp, []byte(strconv.Itoa(os.Getpid())), 0o600) == nil {
			_ = os.Rename(tmp, path)
		}
	}
}

// Run opens a new terminal window for the player, tails the child's stderr in
// the current window, and returns the child's exit status.
func Run(self string, args []string, version string) (int, error) {
	exe, err := os.Executable()
	if err == nil {
		self = exe
	}
	if self == "" {
		return 1, errors.New("cannot locate the muxplayer executable")
	}

	dir, err := os.MkdirTemp("", "muxplayer-debug-")
	if err != nil {
		return 1, fmt.Errorf("cannot create debug directory: %w", err)
	}
	defer os.RemoveAll(dir)

	logPath := filepath.Join(dir, "debug.log")
	statusPath := filepath.Join(dir, "status")
	pidPath := filepath.Join(dir, "pid")
	if err := os.WriteFile(logPath, nil, 0o600); err != nil {
		return 1, fmt.Errorf("cannot create debug log: %w", err)
	}

	script, err := writeLauncher(dir, self, args, logPath, statusPath, pidPath)
	if err != nil {
		return 1, err
	}
	cmd, err := launchTerminal(script)
	if err != nil {
		return 1, err
	}
	// Surface launcher errors (for example, a terminal emulator that cannot
	// connect to the display) in the debug console.
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return 1, fmt.Errorf("cannot open the debug terminal: %w", err)
	}
	// Reap the launcher in the background. Detaching terminals return
	// immediately; ones that wait exit when their window closes.
	go func() { _ = cmd.Wait() }()

	fmt.Printf("muxplayer %s — debug mode\n", version)
	fmt.Println(strings.Repeat("─", 48))
	fmt.Println("The player UI is running in a separate terminal window.")
	fmt.Println("Debug output from that window is mirrored below.")
	fmt.Println("Close the player, or press Ctrl-C here, to stop it.")
	fmt.Println(strings.Repeat("─", 48))
	fmt.Println()

	return follow(logPath, statusPath, pidPath)
}

// follow copies new log bytes to stdout until the child exits or the user
// interrupts the console.
func follow(logPath, statusPath, pidPath string) (int, error) {
	f, err := os.Open(logPath)
	if err != nil {
		return 1, fmt.Errorf("cannot read debug log: %w", err)
	}
	defer f.Close()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, terminationSignals()...)
	defer signal.Stop(sigCh)

	buf := make([]byte, 32*1024)
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()

	started := time.Now()
	sawChild := false
	lastAliveCheck := time.Now()
	var deadSince time.Time

	for {
		select {
		case <-sigCh:
			fmt.Print("\r\nstopping the debug UI…\r\n")
			if pid, ok := readInt(pidPath); ok {
				terminate(pid)
			}
			waitForStatus(statusPath, 3*time.Second)
			drain(f, buf)
			return 0, nil
		case <-tick.C:
			drain(f, buf)

			pid, hasPid := readInt(pidPath)
			if hasPid {
				sawChild = true
			}
			if code, ok := readInt(statusPath); ok {
				drain(f, buf)
				fmt.Printf("\r\nmuxplayer exited with status %d\r\n", code)
				return code, nil
			}
			if !sawChild {
				if time.Since(started) > 10*time.Second {
					return 1, fmt.Errorf("the debug UI did not start; %s", terminalHint())
				}
				continue
			}
			// If the child disappeared without its wrapper writing a status
			// (for example, the window was closed), do not hang forever.
			if time.Since(lastAliveCheck) > 500*time.Millisecond {
				lastAliveCheck = time.Now()
				if processAlive(pid) {
					deadSince = time.Time{}
					continue
				}
				if deadSince.IsZero() {
					deadSince = time.Now()
				}
				// Give the launcher a moment to record the exit status.
				if time.Since(deadSince) < time.Second {
					continue
				}
				if code, ok := readInt(statusPath); ok {
					fmt.Printf("\r\nmuxplayer exited with status %d\r\n", code)
					return code, nil
				}
				return 1, errors.New("the debug UI stopped unexpectedly")
			}
		}
	}
}

func drain(f *os.File, buf []byte) {
	for {
		n, err := f.Read(buf)
		if n > 0 {
			_, _ = os.Stdout.Write(buf[:n])
		}
		if n == 0 || err != nil {
			return
		}
	}
}

func readInt(path string) (int, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	s := strings.TrimSpace(string(b))
	if s == "" {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return n, true
}

func waitForStatus(path string, d time.Duration) {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if _, ok := readInt(path); ok {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
}
