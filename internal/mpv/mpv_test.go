package mpv

// note: the interesting mpv failures happen in the child process, so these
// tests just cover the bits we can exercise without one

import (
	"strings"
	"testing"
)

func TestTailBufferKeepsEnd(t *testing.T) {
	b := &tailBuffer{max: 8}
	b.Write([]byte("0123456789"))
	if got := b.String(); got != "23456789" {
		t.Fatalf("tail = %q, want the newest 8 bytes", got)
	}
}

func TestCommandLine(t *testing.T) {
	args := commandLine("/tmp/mux.sock")
	joined := strings.Join(args, " ")
	for _, want := range []string{"--idle=yes", "--no-terminal", "--input-ipc-server=/tmp/mux.sock"} {
		if !strings.Contains(joined, want) {
			t.Errorf("command line is missing %q: %q", want, joined)
		}
	}
}

func TestExitErrIncludesStderr(t *testing.T) {
	p := &Player{tail: &tailBuffer{max: 64}}
	p.tail.Write([]byte("failed to open audio device"))
	if err := p.ExitErr(); !strings.Contains(err.Error(), "failed to open audio device") {
		t.Fatalf("exit error = %v, want mpv's stderr tail", err)
	}
}
