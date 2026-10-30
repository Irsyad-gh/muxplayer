# muxplayer

A terminal music player for Linux, macOS, and Windows, written in Go and driven by [mpv](https://mpv.io/). You browse your library, build a queue, and get cover art, an accent colour pulled from the artwork, and a live system-audio visualizer, all inside the terminal. The audio capture and FFT are plain Go, so you don't need cava or any other visualizer process running alongside it.

![Version](https://img.shields.io/badge/version-0.7.2-blue)
![Go](https://img.shields.io/badge/Go-1.21%2B-00ADD8?logo=go&logoColor=white)
![Platform](https://img.shields.io/badge/platform-Linux%20%7C%20macOS%20%7C%20Windows-lightgrey)
![License](https://img.shields.io/badge/license-MIT-green)

Current release: `0.7.2`

> **Heads up:** muxplayer is still in development. Expect some bugs and the occasional crash. If you hit one, please [open an issue](https://github.com/Irsyad-gh/muxplayer/issues).

## What it does

- Two tabs: a playlist and a file explorer.
- Plays through mpv's JSON IPC channel, with no shell wrappers in between.
- Shuffle can be off, a shuffle bag (every track once before any repeats), or shuffle by folder.
- Repeat can be off, all, one, or N times.
- Shows cover art from embedded tags or from a folder image, drawn as a true 1:1 square, and samples an accent colour from it.
- Draws a mirrored 64-band FFT spectrum at 30 fps, captured straight from system audio (Linux and Windows).
- Takes mouse input: click, double-click, scroll, and drag to seek.
- Imports folders recursively. The playlist is saved atomically after every change and comes back on the next run.
- Starts from the command line with either a folder to browse or an audio file to play right away.
- Searches the explorer as you type, by name or by `.ext`, with relative-path results, file-type cycling, and A-Z/Z-A sorting.
- Skips tracks that fail to play.
- Exposes MPRIS, so desktop widgets and media keys can see and control playback.
- Has a `--debug` mode that opens the UI in a second terminal window and mirrors live diagnostics in the first.
- Keeps dependencies small. There is no cgo and no external visualizer. godbus handles MPRIS and a pure-Go PulseAudio client handles Linux capture.

## Requirements

### Runtime

| Dependency | Required | Used for |
| --- | --- | --- |
| [mpv](https://mpv.io/) | Yes | Audio playback and metadata (JSON IPC) |
| [ffmpeg](https://ffmpeg.org/) | Strongly recommended | Extracting embedded cover art, including the artwork shared with MPRIS controllers |
| D-Bus session bus (Linux/macOS) | Recommended | MPRIS desktop controller integration |
| cava | No | Not needed since 0.6.0 |

muxplayer runs fine without ffmpeg, you just won't get cover art. The visualizer is built in and only needs your operating system's audio output (see [Visualizer](#visualizer)).

You'll also need:

- Linux, macOS, or Windows. The terminal and mpv-IPC layers are per-OS, and everything else is shared through build tags.
- A truecolour terminal, for the best results.
- A terminal of at least 60×18 cells.
- Go 1.21 or newer, but only if you build from source.

### Audio formats

mpv does the decoding, so anything it plays will work. The formats muxplayer lists in the explorer are `.mp3`, `.flac`, `.ogg`, `.opus`, `.wav`, `.m4a`, `.aac`, `.wma`, and `.ape`.

## Installation

### 1. System dependencies

Arch Linux:

```sh
sudo pacman -S mpv ffmpeg go
```

Debian / Ubuntu:

```sh
sudo apt update
sudo apt install mpv ffmpeg golang-go
```

Fedora:

```sh
sudo dnf install mpv ffmpeg golang
```

On Fedora, ffmpeg may need RPM Fusion. If you want the smallest possible setup, install only mpv and add the rest later.

macOS:

```sh
brew install mpv ffmpeg
# macOS system-audio capture is not implemented yet, so the built-in
# visualizer is unavailable there. Playback and cover art work normally.
```

Windows:

```powershell
winget install shinchiro.mpv
winget install Gyan.FFmpeg
# or use scoop
```

On Windows, run muxplayer inside Windows Terminal (or any console with VT support). The visualizer uses WASAPI loopback on the default output device, so it follows whatever mpv is playing without extra setup.

### 2. Build muxplayer

```sh
git clone https://github.com/Irsyad-gh/muxplayer.git
cd muxplayer
go build -o muxplayer .
```

Then put the binary somewhere on your `PATH`:

```sh
# System-wide
sudo install -Dm755 muxplayer /usr/local/bin/muxplayer

# ...or per-user (make sure ~/.local/bin is on your PATH)
install -Dm755 muxplayer ~/.local/bin/muxplayer
```

Or let Go install it into its own binary directory (make sure `$(go env GOPATH)/bin` is on your `PATH`):

```sh
go install .
```

### 3. Check that it works

```sh
muxplayer --version # or muxplayer -v
```

This prints the version and exits. It doesn't start mpv or open the TUI.

## Usage

```bash
muxplayer [directory|file]
muxplayer --debug [directory|file]  # or -d
muxplayer --help                    # or -h
muxplayer --version                 # or -v
```

| Command | What it does |
| --- | --- |
| `muxplayer` | Starts in `~/Music` if it exists, otherwise `$HOME` |
| `muxplayer ~/Music` | Starts the explorer in that directory |
| `muxplayer ~/Music/song.mp3` | Opens the track's folder, adds the track to the playlist, and plays it |
| `muxplayer --debug` (or `-d`) | Opens the UI in a new terminal window and streams diagnostics in the original one |
| `muxplayer --help` (or `-h`) | Prints usage and exits |
| `muxplayer --version` (or `-v`) | Prints the version and exits |

The saved playlist loads on startup. If it's empty, muxplayer opens on the Explorer tab.

### Debug mode

```sh
muxplayer --debug ~/Music
```

Debug mode launches the player UI in a new terminal window and keeps your current window as a live console. Everything the player window logs shows up there as it happens: terminal size changes, key and mouse input, mpv commands and events, cover-art extraction, audio capture, MPRIS, and shutdown. Close the player window or press `Ctrl-C` in the console to stop both.

muxplayer picks an installed terminal emulator on its own. It checks `$TERMINAL`, `x-terminal-emulator`, GNOME Terminal, Konsole, kitty, alacritty, foot, WezTerm, xterm, and a few others. Inside tmux with no graphical display, it opens a new tmux window instead. To force a specific terminal, set `MUXPLAYER_TERMINAL` to its executable name or path:

```sh
MUXPLAYER_TERMINAL=kitty muxplayer --debug
```

There's also a hidden `--debug-child` flag that the launcher uses internally. You only need it if you want a player whose diagnostics go to the current terminal's stderr.

## Key bindings

### Navigation

| Key | Action |
| --- | --- |
| `Tab` | Switch between Playlist and Explorer |
| `↑` / `↓`, `k` / `j` | Move the selection |
| `PgUp` / `PgDn` | Move 10 rows |
| `Home` / `g` | Jump to the top |
| `End` / `G` | Jump to the bottom |
| `Enter` | Play the selection. In Explorer, opens a folder or adds a file and plays it |
| `Backspace` | Go to the parent directory (Explorer) |
| `q`, `Ctrl-C` | Save the playlist and quit |

### Playback

| Key | Action |
| --- | --- |
| `Space` | Pause or resume. Starts the selection if nothing is playing |
| `n` | Next track |
| `p` | Previous track (restarts the current one if you're past 3 s) |
| `←` / `→` | Seek −5 s / +5 s |
| `+` / `=` | Volume +5% |
| `-` / `_` | Volume −5% |

### Library

| Key | Action |
| --- | --- |
| `a` | Add the selected file or folder (recursively) to the playlist. Explorer only |
| `d` | Remove the selected track from the playlist. Playlist only |

### Explorer search and filter

The search bar sits above the file list. Press `/` to focus it and start typing; the listing updates as you go. Searches ignore case and cover the folder you're in plus everything beneath it. Matches from subfolders are shown with paths relative to the current folder, like `albums/live.mp3`. Your query, type filter, and sort order stay in place while you navigate.

| Key | Action |
| --- | --- |
| `/` | Focus the search bar |
| typing | Match names anywhere. A term starting with `.` matches that file type |
| `Backspace` | Delete the last character |
| `Ctrl-U` | Clear the query |
| `Enter` | Keep the results and go back to the list |
| `Esc` | Clear the filter and go back to the normal listing |
| `t` | Cycle the type filter: any, then each file type found under this folder |
| `o` | Toggle result order, A-Z or Z-A |

Some examples. `live` matches any file or folder with "live" in its name, including ones in subfolders. `.flac` shows FLAC files from the whole tree. `live .flac` combines the two. An extension-only search lists files only. Hidden files and folders are skipped. Folders that match by name come first so you can step into them, and `Esc` takes you back to normal browsing.

With the mouse, click the search bar to focus it, or click the `t:…` and `o:…` chips to change the type filter or sort order.

### Modes

| Key | Action |
| --- | --- |
| `s` | Cycle shuffle: off, bag, folder |
| `r` | Cycle repeat: off, all, one, N× |
| `[` / `]` | Lower or raise the N× repeat count (2 to 99) |

Shuffle modes:

| Mode | Behaviour |
| --- | --- |
| `off` | Plays in playlist order |
| `bag` | Shuffles everything into a bag and plays every track once before reshuffling |
| `folder` | Shuffles the order of folders but keeps the tracks inside each folder together |

Repeat modes:

| Mode | Behaviour |
| --- | --- |
| `off` | Stops when the queue ends |
| `all` | Loops the queue |
| `one` | Repeats the current track |
| `N×` | Repeats the current track N times, then moves on |

### Mouse

| Action | Result |
| --- | --- |
| Click a tab | Switch tabs |
| Click a row | Select it |
| Double-click a row | Play it |
| Click the search bar | Focus the search box (Explorer) |
| Click `t:…` / `o:…` | Cycle the file-type filter / name order (Explorer) |
| Wheel over the list | Scroll three rows |
| Wheel over the right panel | Change the volume |
| Click or drag the progress bar | Seek |
| Click the buttons | Repeat, previous, pause/play, next, shuffle |

## Cover art

muxplayer first asks ffmpeg to pull out any artwork embedded in the audio file. If that fails, it looks for an image next to the track:

```text
cover.jpg  folder.jpg  cover.png  folder.png  front.jpg
```

The artwork is cropped to a square from the centre and drawn with truecolour half-blocks. To keep it from looking stretched, muxplayer reads your terminal's cell pixel size. A cell is usually twice as tall as it is wide, so the image is pre-scaled by the inverse of that ratio. The cover's average colour becomes the interface accent. Tracks with no art get a colour derived deterministically from their path.

## Desktop integration (MPRIS)

On Linux and macOS, muxplayer registers on the D-Bus session bus as `org.mpris.MediaPlayer2.muxplayer`. If another instance already owns that name, it uses an instance-suffixed variant. Bar widgets, media-key daemons, phone-link bridges, and similar tools can then show muxplayer's title, artist, artwork, and position, and can play/pause, stop, skip, seek, set the volume, and quit the player.

Without a session bus (headless sessions, Windows), playback itself is unaffected. Artwork exported for controllers is extracted to `~/.cache/muxplayer/art`.

## Visualizer

muxplayer captures system audio itself and draws a mirrored 64-band spectrum under the playlist, with low frequencies in the middle and high frequencies at the edges so both sides stay balanced. The capture readers and the FFT analyser are part of the Go binary. There's no cava process and no generated cava config.

- **Linux:** talks to PulseAudio over its native protocol. PipeWire works through its PulseAudio compatibility server (`pipewire-pulse`). muxplayer captures the monitor source of the default output sink.
- **Windows:** uses WASAPI loopback on the default output device, so there's nothing to set up.
- **macOS:** native capture isn't implemented yet. Playback works, but the visualizer only shows a short notice. macOS doesn't give ordinary applications access to the output mix unless you use a CoreAudio process tap or a loopback device.

The visualizer is optional. If no audio server or output device is available, muxplayer still starts normally.

If Linux captures the wrong output, list your sinks and pick one by name:

```sh
pactl list short sinks
MUXPLAYER_AUDIO_SINK=alsa_output.pci-0000_03_00.6.HiFi__Speaker__sink muxplayer ~/Music
```

The PulseAudio client also respects `PULSE_SERVER` if your server isn't in the default location.

## Configuration and files

There's no config file. muxplayer only creates a few files at runtime:

| Path | Contents |
| --- | --- |
| `$XDG_CONFIG_HOME/muxplayer/playlist.txt` | Saved playlist (defaults to `~/.config/muxplayer/playlist.txt`) |
| `$XDG_CACHE_HOME/muxplayer/art/` | Artwork extracted for MPRIS controllers |
| `muxplayer-<pid>.sock` in the temp dir (Unix), `\\.\pipe\muxplayer-<pid>` (Windows) | mpv JSON IPC endpoint |

When the playlist is restored, only tracks that still exist are kept. It's written shortly after every change, so a crash, a `SIGKILL`, or a power cut won't lose it. Each save goes to a temporary file that is then atomically renamed over the old one, so an interrupted save leaves the previous playlist intact rather than a truncated file.

| Environment variable | Description |
| --- | --- |
| `MUXPLAYER_AUDIO_SINK` | PulseAudio/PipeWire sink name to visualize. Empty means the default output (Linux) |
| `MUXPLAYER_CELL_ASPECT` | Cell height ÷ cell width, overriding terminal detection for the square cover (default `2`; try `1` on square-cell terminals) |
| `MUXPLAYER_TERMINAL` | Terminal emulator used by `--debug` on Linux/macOS (executable name or path) |
| `MUXPLAYER_DEBUG` | Prints debug diagnostics to stderr without opening a second window |

## Building and testing

```sh
# Build
go build -o muxplayer .

# Tests
go test ./...

# Race detector
go test -race ./...

# Static checks
go vet -stdmethods=false ./...
gofmt -l .

# Cross-compile
GOOS=darwin  GOARCH=arm64 go build -o muxplayer-mac .
GOOS=windows GOARCH=amd64 go build -o muxplayer.exe .
```

The code uses two small third-party modules: [godbus/dbus](https://github.com/godbus/dbus) for MPRIS, and [jfreymuth/pulse](https://github.com/jfreymuth/pulse), a pure-Go PulseAudio client used by the Linux capture backend. Neither needs cgo, and the visualizer never launches an external program. Platform-specific files sit next to the package they belong to (see [Project layout](#project-layout)) and are selected with build tags.

You can override the reported version at build time:

```sh
go build -ldflags "-X main.version=0.1.0" -o muxplayer .
```

## Project layout

```text
main.go                 command entry point: flags, startup, mpv launch
internal/
├── app/                TUI state, playlist, drawing, keyboard/mouse, cover view
├── audiocap/           native system-audio capture (PulseAudio/PipeWire, WASAPI)
├── visualizer/         reusable FFT analyser, sample ring and frame loop
├── cover/              artwork loading (ffmpeg) and colour helpers
├── debugmode/          --debug terminal launcher and log mirroring
├── diag/               stderr logging
├── mpv/                mpv process and JSON IPC client
├── mpris/              MPRIS D-Bus interface for desktop media controllers
└── term/               raw mode, terminal size, signals
```

The audio and visualizer packages are deliberately independent of the TUI and of each other's internals. `audiocap` exposes a pull-based `Source` of interleaved float32 samples, and `visualizer` turns any `Source` into bar frames. To add a capture backend, implement `Source` in a file with the right build tag. A new consumer can reuse `visualizer.Analyzer` without opening system audio at all.

Platform code lives beside what it serves: `term/raw_unix.go`, `term/raw_windows.go`, `term/signals_*`, `term/termios_*`, `mpv/ipc_*`, `audiocap/capture_*`, and `mpris/server_*`.

## FAQ

**failed to start mpv (is it installed?)**

Install mpv and make sure it's on your `PATH`:

```sh
mpv --version
```

**The visualizer is off.**

The built-in visualizer couldn't open system audio. On Linux, check that PulseAudio, or PipeWire's PulseAudio server, is running (`pactl info`). You can also pick a sink with `MUXPLAYER_AUDIO_SINK`. On Windows, check that the default output device is available. On macOS, capture isn't implemented yet. Run with `MUXPLAYER_DEBUG=1` to see the exact capture error.

**There's no cover art.**

Install ffmpeg for embedded artwork, or put one of the folder images listed above next to the track. Some files just don't contain any artwork.

**The cover looks stretched.**

muxplayer normally detects your terminal's cell pixel size. Some terminals don't report it, and in that case you can set the ratio yourself: `MUXPLAYER_CELL_ASPECT=1` for square cells, or `2` for the usual 1:2 cells.

**The visualizer is flat, or reacts to the wrong output.**

On Linux, compare `pactl get-default-sink` with `pactl list short sinks`, then set `MUXPLAYER_AUDIO_SINK` to the sink mpv is playing to. On Windows, the visualizer follows the default output device, so make sure mpv is using that same device.

**"terminal too small (min 60x18)"**

Make the terminal window bigger.

**`--debug` doesn't open a window.**

muxplayer couldn't find a terminal emulator. Set one explicitly, for example `MUXPLAYER_TERMINAL=kitty muxplayer --debug`, or run inside tmux, which opens a new tmux window instead.

**The colours look wrong.**

Use a terminal with truecolour support and a font that has block and box-drawing glyphs.

**There's no sound.**

Check that mpv can play the file on its own:

```sh
mpv /path/to/track.mp3
```

**The playlist didn't persist.**

Changes are saved within a fraction of a second, so even a crash or `SIGKILL` should keep them. If the file still looks stale, check that `$XDG_CONFIG_HOME/muxplayer` is writable, and run with `MUXPLAYER_DEBUG=1` to see the exact save error.

## Planned

- Native dependencies, without relying on mpv and ffmpeg
- macOS CoreAudio process-tap capture for the visualizer
- Support for other Windows terminals
- A visualizer-only view for windows smaller than 60×18
- More customization

## Contributing

Issues and pull requests are welcome at [github.com/Irsyad-gh/muxplayer](https://github.com/Irsyad-gh/muxplayer). Before sending a change, please run `gofmt`, `go vet -stdmethods=false`, and `go test ./...`.

About `-stdmethods=false`: the MPRIS interface has to export a method literally named `Seek` that takes microseconds (`Seek(int64)`). Go's `io.Seeker` convention uses the same name, so vet's `stdmethods` check flags it as a false positive. Every other vet check stays on.

## License

[MIT](LICENSE) © 2026 Irsyad Dzaky Nurghany

Yes, this project was built with the help of AI tools.