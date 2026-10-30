package mpris

// note: these tests don't need a real session bus, which is the whole point

import (
	"reflect"
	"testing"

	"github.com/godbus/dbus/v5"
)

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

func TestTrackID(t *testing.T) {
	// Stable for equal paths, distinct for different ones, and valid D-Bus.
	if trackID("/a/1.mp3") != trackID("/a/1.mp3") {
		t.Fatal("IDs for the same path must match")
	}
	if trackID("/a/1.mp3") == trackID("/b/2.mp3") {
		t.Fatal("IDs for distinct paths must differ")
	}
	if !trackID("/spaces persist.mp3").IsValid() {
		t.Error("trackID must stay a valid D-Bus object path")
	}
}

func TestMetadataMap(t *testing.T) {
	// Idle: only the reserved NoTrack id appears.
	idle := metadataMap(Snapshot{})
	if len(idle) != 1 {
		t.Fatalf("idle metadata has %d entries, want exactly the NoTrack id", len(idle))
	}
	if !reflect.DeepEqual(idle["mpris:trackid"], dbus.MakeVariant(noTrack)) {
		t.Error("idle metadata must advertise NoTrack")
	}
	// Playing: spec fields appear with correct types and units.
	full := metadataMap(Snapshot{
		Track: "/a/1.mp3", Title: "One", Artist: "A", Album: "X",
		ArtURL: "file:///a/cover.jpg", Dur: 2.5,
	})
	if got, ok := full["mpris:length"].Value().(int64); !ok || got != 2_500_000 {
		t.Errorf("length: got %v, want 2,500,000 µs", full["mpris:length"])
	}
	if got, ok := full["xesam:artist"].Value().([]string); !ok || !reflect.DeepEqual(got, []string{"A"}) {
		t.Errorf("artist: got %v, want a one-element string array", full["xesam:artist"])
	}
	if id, ok := full["mpris:trackid"].Value().(dbus.ObjectPath); !ok || !id.IsValid() {
		t.Errorf("trackid: got %v, want a valid object path", full["mpris:trackid"])
	}
	// A missing artist or cover must stay out of the map so controllers skip it.
	lean := metadataMap(Snapshot{Track: "/a/1.mp3"})
	for _, k := range []string{"xesam:artist", "xesam:album", "mpris:arturl", "mpris:length"} {
		if _, ok := lean[k]; ok {
			t.Errorf("empty %s must be omitted", k)
		}
	}
}
