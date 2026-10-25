# muxplayer

A fast, dependency-light TUI music player for Linux, macOS, and Windows, built
with Go and driven by [mpv](https://mpv.io/). Browse your library, queue
tracks, and enjoy cover art, an accent colour sampled from the artwork, and a
live [cava](https://github.com/karlstav/cava) visualizer — all inside the
terminal.

![Version](https://img.shields.io/badge/version-0.5.0-blue)
![Go](https://img.shields.io/badge/Go-1.21%2B-00ADD8?logo=go&logoColor=white)
![Platform](https://img.shields.io/badge/platform-Linux%20%7C%20macOS%20%7C%20Windows-lightgrey)
![License](https://img.shields.io/badge/license-MIT-green)

**Current release:** `0.5.0`

## Features

- **Playlist + file explorer** in a two-tab TUI
- **mpv playback** through mpv's JSON IPC channel — no shell wrappers
- **Shuffle modes** — off, shuffle bag (no repeats), or shuffle by folder
- **Repeat modes** — off, all, one, or repeat N×
- **Cover art** from embedded tags or folder images, drawn as a true 1:1 square
- **Adaptive accent colour** sampled from the current cover
- **cava visualizer** (64 bars at 30 fps, optional)
- **Mouse support** — click, double-click, scroll, and drag to seek
- **Recursive folder import** and a playlist that persists between runs
- **Automatic skip** of tracks that fail to play
- **MPRIS** — desktop widgets and media keys can see and control playback
- **Debug mode** — `--debug` mirrors live diagnostics in the original window
  while the UI runs in a separate terminal window
- **Stdlib-only Go code** with one dependency (godbus, for the MPRIS interface)

## Requirements

### Runtime

| Dependency | Required | Purpose |
| --- | --- | --- |
| [mpv](https://mpv.io/) | **Yes** | Audio playback and metadata (JSON IPC) |
| [cava](https://github.com/karlstav/cava) | No | Audio visualizer (Linux/macOS/Windows) |
| [ffmpeg](https://ffmpeg.org/) | No | Embedded cover-art extraction (including shared artwork for MPRIS controllers) |
| A **D-Bus session bus** (Linux/macOS) | No | MPRIS desktop controller integration |

The player runs fine without `cava` and `ffmpeg`; you just lose the visualizer
and cover art respectively.

Other requirements:

- **Linux, macOS, or Windows** (the terminal and mpv-IPC layers are per-OS;
  everything else is shared, with build tags)
- A **truecolour-capable terminal** for the best results
- A terminal of at least **60×18** cells
- **Go 1.21+** only if you build from source

### Supported audio formats

`.mp3` · `.flac` · `.ogg` · `.opus` · `.wav` · `.m4a` · `.aac` · `.wma` · `.ape`

## Installation

### 1. Install the system dependencies

**Arch Linux**

```sh
sudo pacman -S mpv cava ffmpeg go
```

**Debian / Ubuntu**

```sh
sudo apt update
sudo apt install mpv cava ffmpeg golang-go
```

**Fedora**

```sh
sudo dnf install mpv cava ffmpeg golang
```

> `ffmpeg` may require RPM Fusion on Fedora. Install only `mpv` if you want the
> smallest setup, then add the optional tools later.

**macOS**

```sh
brew install mpv ffmpeg
# cava is optional — install it if your setup provides it, for the visualizer
```

**Windows**

```powershell
scoop install mpv ffmpeg
# or: winget install mpv ; winget install Gyan.FFmpeg

# optional visualizer
winget install karlstav.cava
# or install cava_win_x64_install.msi from the cava releases page
```

> On Windows, run muxplayer inside **Windows Terminal** (or any VT-enabled
> console). cava builds natively for Windows; its `winscap` backend captures
> the default output device, so the visualizer follows whatever mpv is playing.
> If you installed cava after opening the terminal, restart it so the updated
> `PATH` is picked up — muxplayer also looks in `%LOCALAPPDATA%\cava\cava.exe`
> directly.

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
# muxplayer 0.5.0
```

`--version` prints the version and exits immediately — it does not start mpv or
open the TUI.

## Usage

```bash
muxplayer [directory]
muxplayer --debug [directory]  # or -d
muxplayer --help               # or -h
muxplayer --version            # or -v
```

| Command | Description |
| --- | --- |
| `muxplayer` | Start in `~/Music` if it exists, otherwise `$HOME` |
| `muxplayer ~/Music` | Start the explorer in a specific directory |
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
cover-art extraction, cava, MPRIS, and shutdown, is mirrored there as it
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

## Visualizer

When `cava` is installed, muxplayer launches it with a generated config using
64 bars and ASCII output, then renders the bars under the playlist. Without
`cava`, the player still works and shows a short notice instead.

cava runs on Linux, macOS, and Windows. On Windows it captures the default
output device through WASAPI loopback, so no extra setup is required. On
Linux/macOS, if cava picks the wrong audio backend, select one explicitly:

```sh
MUXPLAYER_CAVA_INPUT=pipewire muxplayer ~/Music
MUXPLAYER_CAVA_INPUT=pulse muxplayer ~/Music
MUXPLAYER_CAVA_INPUT=alsa muxplayer ~/Music
```

Any input method understood by cava can be used. `MUXPLAYER_CAVA_INPUT` is
ignored on Windows, where cava's Windows build only supports its built-in
loopback capture. If you install cava with the MSI or winget, use version
1.0.0 or newer.

## Configuration and files

muxplayer has no config file — just a few files generated at runtime:

| Path | Contents |
| --- | --- |
| `$XDG_CONFIG_HOME/muxplayer/playlist.txt` | Saved playlist (defaults to `~/.config/muxplayer/playlist.txt`) |
| `$XDG_CACHE_HOME/muxplayer/art/` | Artwork extracted for MPRIS controllers |
| temp dir `muxplayer-<pid>.sock` (Unix) / `\\.\pipe\muxplayer-<pid>` (Windows) | mpv JSON IPC endpoint |
| temp dir `muxplayer-cava-<pid>.conf` | Generated cava config |

Only tracks that still exist are restored from the saved playlist. The playlist
is written when you quit with `q`/`Ctrl-C`, or when the process receives
`SIGTERM`/`SIGHUP` (Unix); killing it with `SIGKILL` will not save changes.

| Environment variable | Description |
| --- | --- |
| `MUXPLAYER_CAVA_INPUT` | Force a cava input method (`pulse`, `pipewire`, `alsa`, …); ignored on Windows |
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

The Go code uses **one third-party module** — [godbus/dbus](https://github.com/godbus/dbus),
for the MPRIS D-Bus interface — on every platform. Platform-specific files sit
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
├── cava/               cava process, config and raw bar stream
├── cover/              artwork loading (ffmpeg) and colour helpers
├── debugmode/          --debug terminal launcher and log mirroring
├── diag/               stderr logging
├── mpv/                mpv process and JSON IPC client
├── mpris/              MPRIS D-Bus interface for desktop media controllers
└── term/               raw mode, terminal size, signals
```

Platform code lives beside what it serves: `term/raw_unix.go` and
`term/raw_windows.go`, `term/signals_*`, `term/termios_*`, `mpv/ipc_*`, and
`cava/input_*`.

## Troubleshooting

**`failed to start mpv (is it installed?)`**
Install `mpv` and make sure it is on your `PATH`:

```sh
mpv --version
```

**`cava not found — visualizer disabled`**
Install `cava`, or ignore it if you don't need the visualizer. On Windows:
`winget install karlstav.cava`, then reopen the terminal.

**No cover art**
Install `ffmpeg` for embedded artwork, or place one of the folder images listed
above next to the track. Some files simply contain no artwork.

**Cover looks stretched (not square)**
muxplayer normally detects your terminal's cell pixel size automatically. On
terminals that do not report it, set the ratio by hand — for example
`MUXPLAYER_CELL_ASPECT=1` for square cells or the usual `2` for 1:2 cells.

**Visualizer is flat or reacts to the wrong output**
On Linux/macOS, set `MUXPLAYER_CAVA_INPUT` to your audio backend. You can test
cava separately with `cava` to confirm it sees the audio. On Windows, cava
always visualizes the default output device — check that mpv and cava use the
same one.

**`terminal too small (min 60x18)`**
Enlarge the terminal window.

**`--debug` does not open a window**
muxplayer could not find a terminal emulator. Set one explicitly, for example
`MUXPLAYER_TERMINAL=kitty muxplayer --debug`, or run inside tmux (which opens a
new tmux window instead).

**Colours look wrong**
Use a terminal with truecolour support and a font that includes block and
box-drawing glyphs.

**No sound**
Check that mpv itself can play the file:

```sh
mpv /path/to/track.flac
```

**Playlist did not persist**
Quit with `q`/`Ctrl-C` (or send `SIGTERM`/`SIGHUP`) rather than killing the
process.

## Known limitations (0.1.0)

- No search or filtering in the playlist/explorer
- Directories only on the command line — passing a file does not start playback
- Playlist state is a flat list of absolute paths
- No gapless-playback or crossfade controls beyond mpv defaults

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
