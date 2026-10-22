package main

import (
	"context"
	"os"
	"os/exec"
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
