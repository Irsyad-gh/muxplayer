package app

import "testing"

func TestParseCtrlArrowAndVolumeKeys(t *testing.T) {
	cases := []struct {
		seq  string
		want string
	}{
		{"\x1b[D", "left"},
		{"\x1b[C", "right"},
		{"\x1b[1;5D", "ctrl-left"},
		{"\x1b[1;5C", "ctrl-right"},
		{"\x1b[1;6D", "ctrl-left"}, // Ctrl+Shift+Left
		{"\x1b[5~", "pgup"},
		{"\x1b[6~", "pgdn"},
		{"+", "+"},
		{"-", "-"},
		{"\x1b[27;5;43~", "ctrl-+"}, // xterm Ctrl++
		{"\x1b[27;5;61~", "ctrl-+"}, // xterm Ctrl+=
		{"\x1b[27;5;45~", "ctrl--"}, // xterm Ctrl+-
		{"\x1b[43;5u", "ctrl-+"},    // kitty Ctrl++
		{"\x1b[61;5u", "ctrl-+"},    // kitty Ctrl+=
		{"\x1b[45;5u", "ctrl--"},    // kitty Ctrl+-
		{"\x1d", "ctrl-+"},          // legacy Ctrl+=
		{"\x1f", "ctrl--"},          // legacy Ctrl+-
	}
	for _, tc := range cases {
		in, used := parseInput(tc.seq)
		if in.key != tc.want || used != len(tc.seq) {
			t.Errorf("parseInput(%q) = %q used %d, want %q used %d",
				tc.seq, in.key, used, tc.want, len(tc.seq))
		}
	}
}

func TestParseUnknownCSI(t *testing.T) {
	// An unrecognised sequence must be consumed as a unit, not leak its
	// parameter bytes as ordinary key presses.
	in, used := parseInput("\x1b[1;5Z")
	if in.key != "" {
		t.Fatalf("unknown CSI decoded as %q", in.key)
	}
	if used != len("\x1b[1;5Z") {
		t.Fatalf("unknown CSI used %d bytes, want %d", used, len("\x1b[1;5Z"))
	}
}
