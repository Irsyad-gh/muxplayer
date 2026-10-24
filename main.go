// Command muxplayer is a terminal music player. It drives mpv over its JSON
// IPC socket, draws cava's spectrum, and uses only the Go standard library.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"muxplayer/internal/app"
	"muxplayer/internal/diag"
	"muxplayer/internal/mpv"
)

// version is reported by --version. Override it at build time with:
//
//	go build -ldflags "-X main.version=1.2.3"
var version = "0.3.0"

func main() {
	if hasVersionFlag(os.Args[1:]) {
		fmt.Printf("muxplayer %s\n", version)
		fmt.Printf("2026 (C) Irsyad Dzaky Nurghany \n")
		fmt.Printf("https://github.com/Irsyad-gh/muxplayer \n")
		return
	}
	player, err := mpv.Start()
	if err != nil {
		diag.Errf("failed to start mpv (is it installed?): %v", err)
		os.Exit(1)
	}
	app.Run(player, startDir(os.Args[1:]))
}

func hasVersionFlag(args []string) bool {
	for _, a := range args {
		if a == "--version" || a == "-version" {
			return true
		}
	}
	return false
}

// startDir picks the folder to open on launch: a CLI argument if given, else
// ~/Music when it exists, else the home directory.
func startDir(args []string) string {
	home, _ := os.UserHomeDir()
	dir := home
	if music := filepath.Join(home, "Music"); isDir(music) {
		dir = music
	}
	if len(args) > 0 {
		if abs, err := filepath.Abs(args[0]); err == nil {
			dir = abs
		}
	}
	return dir
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}
