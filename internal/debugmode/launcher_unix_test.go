//go:build linux || darwin

package debugmode

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShQuote(t *testing.T) {
	cases := map[string]string{
		"":           "''",
		"/music":     "'/music'",
		"it's":       `'it'\''s'`,
		"a b":        "'a b'",
		"$HOME `id`": "'$HOME `id`'",
		"two\nlines": "'two\nlines'",
	}
	for in, want := range cases {
		if got := shQuote(in); got != want {
			t.Errorf("shQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWriteLauncher(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "debug.log")
	statusPath := filepath.Join(dir, "status")
	pidPath := filepath.Join(dir, "pid")
	script, err := writeLauncher(dir, "/opt/mux player/muxplayer", []string{"--debug", "/music"}, logPath, statusPath, pidPath)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(script) != dir {
		t.Errorf("script = %q, want it inside %q", script, dir)
	}
	st, err := os.Stat(script)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode()&0o111 == 0 {
		t.Errorf("script is not executable: %v", st.Mode())
	}
	b, err := os.ReadFile(script)
	if err != nil {
		t.Fatal(err)
	}
	body := string(b)
	for _, want := range []string{
		"export MUXPLAYER_DEBUG=1",
		"'/opt/mux player/muxplayer' '--debug-child' '--debug' '/music'",
		"2>>'" + logPath + "'",
		statusPath,
		pidPath,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("launcher does not contain %q:\n%s", want, body)
		}
	}
}

func TestSpecArgs(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"gnome-terminal", "--wait --"},
		{"xfce4-terminal", "--disable-server -x"},
		{"kitty", ""},
		{"wezterm", "start --"},
		{"xterm", "-e"},
		{"kitty.exe", ""},
	}
	for _, c := range cases {
		got := strings.Join(specArgs(c.name), " ")
		if got != c.want {
			t.Errorf("specArgs(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}
