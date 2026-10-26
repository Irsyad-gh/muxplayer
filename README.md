# muxplayer

A fast, dependency-light TUI music player for Linux, macOS, and Windows, built
with Go and driven by [mpv](https://mpv.io/). You can browse your library, queue
tracks, and enjoy cover art, an accent colour sampled from the artwork, and a
live system-audio visualizer all inside the terminal. Capture and FFT analysis
are native Go — no cava or other visualizer process is needed.

![Version](https://img.shields.io/badge/version-0.6.3-blue)
![Go](https://img.shields.io/badge/Go-1.21%2B-00ADD8?logo=go&logoColor=white)
![Platform](https://img.shields.io/badge/platform-Linux%20%7C%20macOS%20%7C%20Windows-lightgrey)
![License](https://img.shields.io/badge/license-MIT-green)

**Current release:** `0.6.3`

# This project is still in development / ! \
**Please expect some bug and instability and report it to us**

## Features

- **Playlist + file explorer** in a two-tab TUI
- **mpv playback** through mpv's JSON IPC channel — no shell wrappers
- **Shuffle modes** — off, shuffle bag (no repeats), or shuffle by folder
- **Repeat modes** — off, all, one, or repeat N×
- **Cover art** from embedded tags or folder images, drawn as a true 1:1 square
- **Adaptive accent colour** sampled from the current cover
- **Native mirrored visualizer** — a 64-band FFT spectrum at 30 fps, captured straight from system audio (Linux and Windows)
- **Mouse support** — click, double-click, scroll, and drag to seek
- **Recursive folder import** and a playlist that persists between runs, saved
  atomically after every change
- **Start from the command line** with a folder to browse or an audio file to
  play right away
- **Automatic skip** of tracks that fail to play
- **MPRIS** — desktop widgets and media keys can see and control playback
- **Debug mode** — `--debug` mirrors live diagnostics in the original window
  while the UI runs in a separate terminal window
- **Small dependency set** — no cgo and no external visualizer; godbus is used for MPRIS and a pure-Go PulseAudio client for Linux audio capture

## Requirements

### Runtime

| Dependency | Required | Purpose |
| --- | --- | --- |
| [mpv](https://mpv.io/) | **Yes** | Audio playback and metadata (JSON IPC) |
| [ffmpeg](https://ffmpeg.org/) | **Very Recommended** | Embedded cover-art extraction (including shared artwork for MPRIS controllers) |
| **D-Bus session bus** (Linux/macOS) | Recommended | MPRIS desktop controller integration |
| cava | No | No longer required by update 0.6.0 |

The player runs fine without `ffmpeg`; you just lose cover art. The visualizer
is built in and only needs the operating system's audio output (see
[Visualizer](#visualizer)).

Other requirements:

- **Linux, macOS, or Windows** (the terminal and mpv-IPC layers are per-OS;
  everything else is shared, with build tags)
- A **truecolour-capable terminal** for the best results
- A terminal of at least **60×18** cells
- **Go 1.21+** only if you build from source

### Supported audio formats (provided by mpv)

`.mp3` · `.flac` · `.ogg` · `.opus` · `.wav` · `.m4a` · `.aac` · `.wma` · `.ape`

## Installation

### 1. Install the system dependencies

**Arch Linux**

```sh
sudo pacman -S mpv ffmpeg go
```

**Debian / Ubuntu**

```sh
sudo apt update
sudo apt install mpv ffmpeg golang-go
```

**Fedora**

```sh
sudo dnf install mpv ffmpeg golang
```

`ffmpeg` may require RPM Fusion on Fedora. Install only `mpv` if you want the
smallest setup, then add the optional tools later.

**macOS**

```sh
brew install mpv ffmpeg
# macOS system-audio capture is not implemented yet, so the built-in
# visualizer is unavailable there. Playback and cover art work normally.
```

**Windows**

```powershell
winget install shinchiro.mpv
winget install Gyan.FFmpeg
# or use scoop
```

On Windows, run muxplayer inside **Windows Terminal** (or any VT-enabled
console). The visualizer uses WASAPI loopback on the default output device, so
it follows whatever mpv is playing without any extra setup.

### 2. Build and install muxplayer

```sh
git clone https://github.com/Irsyad-gh/muxplayer.git
cd muxplayer
go build -o muxplayer .
```

Install it somewhere on your `PATH`:

```sh
# System-wide
sudo install -Dm755 muxplayer /usr/local/bin/muxplayer

# ...or per-user (make sure ~/.local/bin is on your PATH)
install -Dm755 muxplayer ~/.local/bin/muxplayer
```

Alternatively, let Go install it into its binary directory (make sure
`$(go env GOPATH)/bin` is on your `PATH`):

```sh
go install .
```

### 3. Verify

```sh
muxplayer --version # or muxplayer -v
```

`--version` prints the version and exits immediately, it does not start mpv or
open the TUI.

## Usage

```bash
muxplayer [directory|file]
muxplayer --debug [directory|file]  # or -d
muxplayer --help                    # or -h
muxplayer --version                 # or -v
```

| Command | Description |
| --- | --- |
| `muxplayer` | Start in `~/Music` if it exists, otherwise `$HOME` |
| `muxplayer ~/Music` | Start the explorer in a specific directory |
| `muxplayer ~/Music/song.mp3` | Open the track's folder, add it to the playlist, and start playing |
| `muxplayer --debug` | Open the UI in a new terminal window and stream its diagnostics in the original one |
| `muxplayer --help` | Print usage and exit (`-h` also works) |
| `muxplayer --version` | Print the version and exit (`-v` also works) |
| `muxplayer --debug` | The origin terminal show debug mode, while the interface is in new window (`-d` also works) |

The saved playlist is loaded on startup; if it is empty, muxplayer opens on the
**Explorer** tab.

### Debug mode

```sh
muxplayer --debug ~/Music
```

Debug mode launches the player UI in a **new terminal window** and keeps the
current window as a live console. Every diagnostic line written by the player
window, terminal size changes, key and mouse input, mpv commands and events,
cover-art extraction, audio capture, MPRIS, and shutdown, is mirrored there as it
happens. Close the player window or press `Ctrl-C` in the console to stop both.

muxplayer picks an installed terminal emulator automatically (`$TERMINAL`,
`x-terminal-emulator`, GNOME Terminal, Konsole, kitty, alacritty, foot, WezTerm,
xterm, and others). Inside tmux without a graphical display it opens a new tmux
window instead. To force a particular terminal, set `MUXPLAYER_TERMINAL` to its
executable name or path:

```sh
MUXPLAYER_TERMINAL=kitty muxplayer --debug
```

The hidden `--debug-child` flag is used internally by the launcher; run it only
if you want to start a player whose diagnostics go to the current terminal's
stderr.

## Key bindings

### Navigation

| Key | Action |
| --- | --- |
| `Tab` | Switch between **Playlist** and **Explorer** |
| `↑` / `↓`, `k` / `j` | Move selection |
| `PgUp` / `PgDn` | Move 10 rows |
| `Home` / `g` | Jump to the top |
| `End` / `G` | Jump to the bottom |
| `Enter` | Play selection; in Explorer, open a folder or add-and-play a file |
| `Backspace` | Go to the parent directory (Explorer) |
| `q`, `Ctrl-C` | Save the playlist and quit |

### Playback

| Key | Action |
| --- | --- |
| `Space` | Pause/resume; starts the selection when idle |
| `n` | Next track |
| `p` | Previous track (restarts the current track if it is past 3 s) |
| `←` / `→` | Seek −5 s / +5 s |
| `+` / `=` | Volume +5% |
| `-` / `_` | Volume −5% |

### Library

| Key | Action |
| --- | --- |
| `a` | Add the selected file or folder (recursively) to the playlist — Explorer only |
| `d` | Remove the selected track from the playlist — Playlist only |

### Modes

| Key | Action |
| --- | --- |
| `s` | Cycle shuffle: **off → bag → folder** |
| `r` | Cycle repeat: **off → all → one → N×** |
| `[` / `]` | Decrease/increase the N× repeat count (2–99) |

**Shuffle modes**

| Mode | Behaviour |
| --- | --- |
| `off` | Play in playlist order |
| `bag` | Shuffle into a bag and play every track once before reshuffling |
| `folder` | Shuffle folder order while keeping tracks inside each folder together |

**Repeat modes**

| Mode | Behaviour |
| --- | --- |
| `off` | Stop when the queue ends |
| `all` | Loop the queue |
| `one` | Repeat the current track |
| `N×` | Repeat the current track N times, then advance |

### Mouse

| Action | Result |
| --- | --- |
| Click a tab | Switch tabs |
| Click a row | Select it |
| Double-click a row | Play it |
| Wheel over the list | Scroll three rows |
| Wheel over the right panel | Adjust volume |
| Click/drag the progress bar | Seek |
| Click the buttons | Repeat, previous, pause/play, next, shuffle |

## Cover art

muxplayer first asks `ffmpeg` to extract artwork embedded in the audio file. If
that fails, it looks for a folder image next to the track:

```text
cover.jpg  folder.jpg  cover.png  folder.png  front.jpg
```

Artwork is centre-cropped to a square and drawn with truecolour half-blocks as
a true **1:1 square**. muxplayer reads your terminal's cell pixel size to do
this: a cell is usually twice as tall as it is wide, and the artwork is
pre-scaled by the inverse of that ratio so it never looks stretched. The
cover's average colour becomes the interface accent; tracks without art get a
deterministic colour derived from their path.

## Desktop integration (MPRIS)

On Linux and macOS, muxplayer registers itself on the D-Bus session bus as
`org.mpris.MediaPlayer2.muxplayer` (or an instance-suffixed variant when
another instance owns the name). Desktop media suites — bar widgets, media-key
daemons, phone-link bridges — then show muxplayer's title, artist, artwork, and
position, and can **play/pause, stop, skip, seek, set volume, and quit** the
player.

media suites — bar widgets, media-key daemons, phone-link bridges — which then
show the current title, artist, and artwork and can play/pause, stop, skip,
seek, set the volume, and quit the player. Without a session bus (headless
sessions, Windows), playback is untouched. Exported artwork for controllers is
extracted to `~/.cache/muxplayer/art`.


## Visualizer

muxplayer captures system audio itself and renders a mirrored 64-band spectrum
under the playlist. Low frequencies sit in the middle and high frequencies at
the edges, so the visualization stays balanced on both sides. There is no cava
process and no generated cava config: the capture readers and FFT analyser are
part of the Go binary.

- **Linux:** talks to PulseAudio over its native protocol. PipeWire works
  through its PulseAudio compatibility server (`pipewire-pulse`). The monitor
  source of the default output sink is captured.
- **Windows:** uses WASAPI loopback on the default output device, so no extra
  setup is required.
- **macOS:** native system-audio capture is not implemented yet. Playback works
  normally, but the visualizer shows a short notice. macOS does not expose the
  output mix to ordinary applications without a CoreAudio process tap or a
  loopback device.

The visualizer is optional: if no audio server or output device is available,
muxplayer still starts normally.

If Linux captures the wrong output, list your sinks and choose one by name:

```sh
pactl list short sinks
MUXPLAYER_AUDIO_SINK=alsa_output.pci-0000_03_00.6.HiFi__Speaker__sink muxplayer ~/Music
```

The PulseAudio client also honours `PULSE_SERVER` if your server is not at the
default location.

## Configuration and files

muxplayer has no config file — just a few files generated at runtime:

| Path | Contents |
| --- | --- |
| `$XDG_CONFIG_HOME/muxplayer/playlist.txt` | Saved playlist (defaults to `~/.config/muxplayer/playlist.txt`) |
| `$XDG_CACHE_HOME/muxplayer/art/` | Artwork extracted for MPRIS controllers |
| temp dir `muxplayer-<pid>.sock` (Unix) / `\\.\pipe\muxplayer-<pid>` (Windows) | mpv JSON IPC endpoint |

Only tracks that still exist are restored from the saved playlist. The playlist
is written shortly after every change, so a crash, `SIGKILL`, or a power cut
cannot lose it. Each save goes to a temporary file that is atomically renamed
over the old one, which means an interrupted save leaves the previous playlist
intact instead of a truncated file.

| Environment variable | Description |
| --- | --- |
| `MUXPLAYER_AUDIO_SINK` | PulseAudio/PipeWire sink name to visualize; empty uses the default output (Linux) |
| `MUXPLAYER_CELL_ASPECT` | Cell height ÷ cell width, overriding terminal detection for the square cover (default `2`; try `1` on square-cell terminals) |
| `MUXPLAYER_TERMINAL` | Terminal emulator used by `--debug` on Linux/macOS (executable name or path) |
| `MUXPLAYER_DEBUG` | Print debug diagnostics to stderr without opening a second window |

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

The Go code uses two small third-party modules: [godbus/dbus](https://github.com/godbus/dbus)
for MPRIS and [jfreymuth/pulse](https://github.com/jfreymuth/pulse), a pure-Go
PulseAudio client used by the Linux capture backend. Neither requires cgo, and
the visualizer does not launch an external program. Platform-specific files sit
next to the package they serve (see [Project layout](#project-layout)) and are
selected with build tags.

Override the reported version at build time:

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

The audio and visualizer packages are intentionally independent of the TUI and
of each other's internals. `audiocap` exposes a pull-based `Source` of
interleaved float32 samples; `visualizer` turns any `Source` into bar frames.
A new capture backend only needs to implement `Source` in a file with the right
build tag, and a new consumer can reuse `visualizer.Analyzer` without opening
system audio at all.

Platform code lives beside what it serves: `term/raw_unix.go`,
`term/raw_windows.go`, `term/signals_*`, `term/termios_*`, `mpv/ipc_*`,
`audiocap/capture_*`, and `mpris/server_*`.

## FAQ

1. **failed to start mpv (is it installed?)**
Install `mpv` and make sure it is on your `PATH`:

```sh
mpv --version
```

2. **Visulizer off ?**
The built-in visualizer could not open system audio. On Linux, make sure
PulseAudio or PipeWire's PulseAudio server is running (`pactl info`), and
optionally pick a sink with `MUXPLAYER_AUDIO_SINK`. On Windows, check that the
default output device is available. On macOS, native system-audio capture is
not implemented yet. Run with `MUXPLAYER_DEBUG=1` to see the exact capture
error.

3. **No cover art**
Install `ffmpeg` for embedded artwork, or place one of the folder images listed
above next to the track. Some files simply contain no artwork.

4. **Cover looks stretched (not square)**
muxplayer normally detects your terminal's cell pixel size automatically. On
terminals that do not report it, set the ratio by hand — for example
`MUXPLAYER_CELL_ASPECT=1` for square cells or the usual `2` for 1:2 cells.

5. **Visualizer is flat or reacts to the wrong output**
On Linux, check `pactl get-default-sink` and `pactl list short sinks`, then set
`MUXPLAYER_AUDIO_SINK` to the sink where mpv is playing. On Windows, the
visualizer follows the default output device, so make sure mpv uses that same
device.

6. **`terminal too small (min 60x18)`**
Enlarge the terminal window.

7. **`--debug` does not open a window**
muxplayer could not find a terminal emulator. Set one explicitly, for example
`MUXPLAYER_TERMINAL=kitty muxplayer --debug`, or run inside tmux (which opens a
new tmux window instead).

8. **Colours look wrong**
Use a terminal with truecolour support and a font that includes block and
box-drawing glyphs.

9. **No sound**
Check that mpv itself can play the file:

```sh
mpv /path/to/track.mp3
```

10. **Playlist did not persist**
Playlist changes are saved atomically within a fraction of a second, so even a
crash or `SIGKILL` keeps them. If the file still looks stale, check that
`$XDG_CONFIG_HOME/muxplayer` is writable and run with `MUXPLAYER_DEBUG=1` to see
the exact save error.

## For Future Development 

- Search or filtering in the playlist/explorer
- Native dependencies and without relying on mpv and ffmpeg
- macOS CoreAudio process-tap capture for the visualizer
- Support for other Windows terminals
- Visualizer only on less than 60x18 window
- More customization

## Contributing

Issues and pull requests are welcome at
[github.com/Irsyad-gh/muxplayer](https://github.com/Irsyad-gh/muxplayer).
Please run `gofmt`, `go vet -stdmethods=false`, and `go test ./...` before
submitting a change.

> Why `-stdmethods=false`? The MPRIS interface must export a method literally
> named `Seek` taking microseconds (`Seek(int64)`). That conflicts with Go's
> `io.Seeker` naming convention, which vet's `stdmethods` check flags as a
> false positive. Every other vet check stays enabled.

## License

[MIT](LICENSE) © 2026 Irsyad Dzaky Nurghany

yes, this project is built using AI helper tools
