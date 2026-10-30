package main

// note: these tests re-exec the test binary, which is ugly but catches real startup bugs

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHasVersionFlag(t *testing.T) {
	cases := []struct {
		args []string
		want bool
	}{
		{[]string{"--version"}, true},
		{[]string{"-version"}, true},
		{[]string{"/music", "--version"}, true},
		{[]string{}, false},
		{[]string{"/music"}, false},
		{[]string{"--verbose"}, false},
	}
	for _, c := range cases {
		if got := hasVersionFlag(c.args); got != c.want {
			t.Errorf("hasVersionFlag(%q) = %v, want %v", c.args, got, c.want)
		}
	}
}

func TestParseArgs(t *testing.T) {
	cases := []struct {
		args  []string
		help  bool
		debug bool
		child bool
		path  string
		rest  []string
	}{
		{[]string{}, false, false, false, "", nil},
		{[]string{"--help"}, true, false, false, "", nil},
		{[]string{"-h", "/music"}, true, false, false, "/music", []string{"/music"}},
		{[]string{"--debug"}, false, true, false, "", nil},
		{[]string{"-debug", "/music"}, false, true, false, "/music", []string{"/music"}},
		{[]string{"/music", "--debug"}, false, true, false, "/music", []string{"/music"}},
		{[]string{"--debug-child", "/music"}, false, false, true, "/music", []string{"/music"}},
		{[]string{"--verbose", "/music"}, false, false, false, "/music", []string{"--verbose", "/music"}},
	}
	for _, c := range cases {
		got := parseArgs(c.args)
		if got.help != c.help || got.debug != c.debug || got.debugChild != c.child || got.path != c.path {
			t.Errorf("parseArgs(%q) = help %v debug %v child %v path %q, want %v %v %v %q",
				c.args, got.help, got.debug, got.debugChild, got.path, c.help, c.debug, c.child, c.path)
		}
		if strings.Join(got.args, "\x00") != strings.Join(c.rest, "\x00") {
			t.Errorf("parseArgs(%q) args = %q, want %q", c.args, got.args, c.rest)
		}
	}
}

// TestStartTarget checks how the CLI argument is turned into an explorer
// directory and an optional track to play on launch.
func TestStartTarget(t *testing.T) {
	dir := t.TempDir()
	audio := filepath.Join(dir, "song.mp3")
	if err := os.WriteFile(audio, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(other, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name  string
		arg   string
		wantD string
		wantT string
	}{
		{"no argument", "", defaultDir(), ""},
		{"directory", dir, dir, ""},
		{"audio file", audio, dir, audio},
		{"other file", other, defaultDir(), ""},
		{"missing path", filepath.Join(dir, "gone.flac"), defaultDir(), ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotDir, gotTrack := startTarget(c.arg)
			if gotDir != c.wantD || gotTrack != c.wantT {
				t.Errorf("startTarget(%q) = %q, %q; want %q, %q", c.arg, gotDir, gotTrack, c.wantD, c.wantT)
			}
		})
	}
}

// TestVersionFlagExitsWithoutStartingApp re-executes this test binary and runs
// main with --version. It must print the version and return without touching
// mpv or the terminal.
func TestVersionFlagExitsWithoutStartingApp(t *testing.T) {
	if os.Getenv("MUXPLAYER_TEST_VERSION") == "1" {
		os.Args = []string{"muxplayer", "--version"}
		main()
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestVersionFlagExitsWithoutStartingApp$")
	cmd.Env = append(os.Environ(), "MUXPLAYER_TEST_VERSION=1")
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("--version did not exit promptly:\n%s", out)
	}
	if err != nil {
		t.Fatalf("--version failed: %v\n%s", err, out)
	}
	output := string(out)
	if !strings.HasPrefix(output, "muxplayer "+version+"\n") {
		t.Errorf("output = %q, want version line for %q", output, version)
	}
	if strings.Contains(output, "\x1b[") {
		t.Errorf("version output contains terminal escape sequences: %q", output)
	}
}

// TestHelpFlagExitsWithoutStartingApp re-executes this test binary and runs
// main with --help. It must print usage and return without touching mpv or
// the terminal.
func TestHelpFlagExitsWithoutStartingApp(t *testing.T) {
	if os.Getenv("MUXPLAYER_TEST_HELP") == "1" {
		os.Args = []string{"muxplayer", "--help"}
		main()
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestHelpFlagExitsWithoutStartingApp$")
	cmd.Env = append(os.Environ(), "MUXPLAYER_TEST_HELP=1")
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("--help did not exit promptly:\n%s", out)
	}
	if err != nil {
		t.Fatalf("--help failed: %v\n%s", err, out)
	}
	output := string(out)
	for _, want := range []string{"muxplayer " + version, "Usage:", "--debug", "--version", "--help"} {
		if !strings.Contains(output, want) {
			t.Errorf("help output does not contain %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "\x1b[") {
		t.Errorf("help output contains terminal escape sequences: %q", output)
	}
}
