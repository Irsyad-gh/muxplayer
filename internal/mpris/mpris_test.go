package mpris

// note: these tests don't need a real session bus, which is the whole point

import "testing"

func TestStatus(t *testing.T) {
	for _, tc := range []struct {
		st   Snapshot
		want string
	}{
		{Snapshot{}, "Stopped"},
		{Snapshot{Track: "/a/1.mp3", Playing: true}, "Playing"},
		{Snapshot{Track: "/a/1.mp3"}, "Paused"},
	} {
		if got := tc.st.Status(); got != tc.want {
			t.Errorf("Status: got %q, want %q", got, tc.want)
		}
	}
}
