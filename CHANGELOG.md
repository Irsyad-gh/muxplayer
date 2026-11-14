# Changelog

## 1.0.0 — 2026-11-14

First stable release.

### Fixed

- Windows test builds no longer pull in Unix-only MPRIS helpers.
- mpv now fails fast when it dies before IPC is ready, reports its stderr tail,
  and stops the player with an error if mpv exits while running.
- Save failures are shown in the UI footer instead of being written over the
  alternate screen.
- A stale mpv socket is removed before launch.
- `--version` can report the commit and build date from release builds.

### Changed

- The Go module path is now `github.com/Irsyad-gh/muxplayer`, so
  `go install github.com/Irsyad-gh/muxplayer@latest` works.
- The project requires Go 1.26 or newer.
- Linux, macOS, and Windows builds are tested in CI on every push.
- Tagged releases build Linux, macOS, and Windows archives for amd64 and arm64,
  with checksums.

### Known limitations

- System-audio capture is not implemented on macOS, so the visualizer is off
  there.
- Windows support targets Windows Terminal (or another VT-capable console).
- MPRIS is Unix-only. Windows has no media-key integration yet.

## Earlier releases

See the [commit history](https://github.com/Irsyad-gh/muxplayer/commits/main)
for changes before 1.0.0.
