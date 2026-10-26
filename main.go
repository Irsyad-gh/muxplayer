// Command muxplayer is a terminal music player. It drives mpv over its JSON
// IPC socket and draws a spectrum from natively captured system audio.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"muxplayer/internal/app"
	"muxplayer/internal/cover"
	"muxplayer/internal/debugmode"
	"muxplayer/internal/diag"
	"muxplayer/internal/mpv"
)

// version is reported by --version. Override it at build time with:
//
//	go build -ldflags "-X main.version=1.2.3"
var version = "0.6.3"

func main() {
	os.Exit(run())
}

func run() (code int) {
	opts := parseArgs(os.Args[1:])
	if opts.help {
		printUsage()
		return 0
	}
	if opts.version {
		fmt.Printf("muxplayer %s\n", version)
		fmt.Printf("2026 (C) Irsyad Dzaky Nurghany \n")
		fmt.Printf("https://github.com/Irsyad-gh/muxplayer \n")
		return 0
	}
	if opts.debugChild {
		// The debug launcher starts this process inside a new terminal window.
		// It reports its PID so the original window can stop it on Ctrl-C.
		debugmode.EnterChild()
	} else if opts.debug {
		code, err := debugmode.Run(os.Args[0], opts.args, version)
		if err != nil {
			diag.Errf("%v", err)
			return 1
		}
		return code
	}
	player, err := mpv.Start()
	if err != nil {
		diag.Errf("failed to start mpv (is it installed?): %v", err)
		return 1
	}
	dir, track := startTarget(opts.path)
	diag.Debugf("opening %s", dir)
	app.Run(player, dir, track)
	return 0
}

type options struct {
	help       bool
	debug      bool
	debugChild bool
	version    bool
	path       string   // optional start directory or audio file
	args       []string // original arguments, minus --debug/--debug-child
}

// printUsage writes the command's help text. It never starts mpv or the TUI.
func printUsage() {
	fmt.Printf(`muxplayer %s a terminal music player based on mpv

Usage:
  muxplayer [path]               Start in a directory, ~/Music, or $HOME;
                                 an audio file starts playing immediately
  muxplayer --debug [path]       Run the UI in a new terminal window and
                                 mirror its debug output in this window
  muxplayer --version            Print the version and exit
  muxplayer --help               Show this help and exit

Options:
  -h,  --help       Show this help and exit
  -d,  --debug      Enable debug mode (UI in a new terminal window)
  -v,  --version    Print the version and exit

Environment:
  MUXPLAYER_TERMINAL       Terminal emulator used by --debug
  MUXPLAYER_AUDIO_SINK     PulseAudio/PipeWire sink to visualize (Linux)
  MUXPLAYER_CELL_ASPECT    Cell height / width override for cover art
  MUXPLAYER_DEBUG          Print debug diagnostics to stderr

Playlist and configuration are saved under $XDG_CONFIG_HOME/muxplayer.
`, version)
}

// parseArgs separates the CLI flags from the optional start path. It keeps the
// remaining arguments so debug mode can pass them to the child.
func parseArgs(args []string) options {
	var o options
	for _, a := range args {
		switch a {
		case "-h", "-help", "--help":
			o.help = true
		case "-v", "-version", "--version":
			o.version = true
		case "-d", "-debug", "--debug":
			o.debug = true
		case "-debug-child", "--debug-child":
			o.debugChild = true
		default:
			o.args = append(o.args, a)
			if o.path == "" && !isFlag(a) {
				o.path = a
			}
		}
	}
	return o
}

func isFlag(s string) bool { return len(s) > 0 && s[0] == '-' }

func hasVersionFlag(args []string) bool {
	for _, a := range args {
		if a == "--version" || a == "-version" {
			return true
		}
	}
	return false
}

// startTarget resolves the optional CLI argument. A directory becomes the
// explorer root; a supported audio file becomes that directory plus a track to
// play. Anything else is ignored in favour of the default start directory.
func startTarget(arg string) (dir, track string) {
	dir = defaultDir()
	if arg == "" {
		return dir, ""
	}
	abs, err := filepath.Abs(arg)
	if err != nil {
		diag.Warnf("ignoring %q: %v", arg, err)
		return dir, ""
	}
	switch {
	case isDir(abs):
		return abs, ""
	case isFile(abs):
		if !cover.IsAudio(abs) {
			diag.Warnf("not a supported audio file: %s", abs)
			return dir, ""
		}
		return filepath.Dir(abs), abs
	default:
		diag.Warnf("no such file or directory: %s", abs)
		return dir, ""
	}
}

// defaultDir prefers ~/Music when it exists, else the home directory.
func defaultDir() string {
	home, _ := os.UserHomeDir()
	if music := filepath.Join(home, "Music"); isDir(music) {
		return music
	}
	return home
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular()
}
