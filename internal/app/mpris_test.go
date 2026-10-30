package app

// note: the fake player records calls instead of pretending to be mpv

import (
	"testing"

	"muxplayer/internal/mpris"
)

func TestSnapshotIdle(t *testing.T) {
	m := newTestModel()
	st := m.snapshot()
	if st.Track != "" || st.Status() != "Stopped" {
		t.Fatal("idle model must snapshot as Stopped")
	}
	if st.Loop != mpris.LoopNone || st.Shuffle {
		t.Error("defaults must be loop-none, no shuffle")
	}
	if st.Vol != 80 {
		t.Errorf("volume: got %v, want the model's 80", st.Vol)
	}
	// One track: skipping controls stay, two tracks: both are available.
	m.play(3)
	st = m.snapshot()
	if st.Track != "/b/4.mp3" {
		t.Fatalf("playing track: got %q", st.Track)
	}
	if st.Title != "4" {
		t.Errorf("missing tags fall back to the file name: got %q", st.Title)
	}
	if st.Loop != mpris.LoopNone {
		t.Errorf("defaults map to no loop: got %q", st.Loop)
	}
	// Loop and shuffle mirror the current modes.
	m.repeat, m.shuffle = repeatOne, shuffleBag
	if st := m.snapshot(); st.Loop != mpris.LoopTrack || !st.Shuffle {
		t.Errorf("repeat-one + bag shuffle: got loop %q shuffle %v", st.Loop, st.Shuffle)
	}
}

func TestControllerActions(t *testing.T) {
	m := newTestModel()
	// Play/toggle when idle starts the selection.
	m.action(mpris.Action{Kind: mpris.ActPlay})
	if m.cur != 0 {
		t.Fatal("play when idle should start the selection")
	}
	if m.action(mpris.Action{Kind: mpris.ActQuit}) != true {
		t.Error("quit must ask the app to shut down")
	}
	// Loop mapping.
	m.action(mpris.Action{Kind: mpris.ActLoop, Arg: mpris.LoopTrack})
	if m.repeat != repeatOne {
		t.Error("Track loop must select repeat-one")
	}
	m.action(mpris.Action{Kind: mpris.ActLoop, Arg: mpris.LoopPlaylist})
	if m.repeat != repeatAll {
		t.Error("Playlist loop must select repeat-all")
	}
	m.action(mpris.Action{Kind: mpris.ActLoop, Arg: mpris.LoopNone})
	if m.repeat != repeatOff {
		t.Error("None loop must select repeat-off")
	}
	// Shutdown via controller is identical to pressing stop.
	m.action(mpris.Action{Kind: mpris.ActStop})
	if m.cur != -1 || m.tpos != 0 {
		t.Error("stop must return to the idle state")
	}
}
