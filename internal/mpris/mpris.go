// Package mpris publishes muxplayer as an MPRIS player on the session bus so
// desktop media controllers (bars, applets, media keys) can see and drive it.
//
// The platform-independent types and action names live here; the D-Bus
// implementation is server_unix.go, with a no-op stub for Windows.
package mpris

// Action kinds that media controllers invoke on the player.
const (
	ActPlay     = "play"
	ActPause    = "pause"
	ActToggle   = "toggle"
	ActNext     = "next"
	ActPrevious = "previous"
	ActStop     = "stop"
	ActQuit     = "quit"
	ActSeekRel  = "seek-rel" // float64 seconds
	ActSeekAbs  = "seek-abs" // float64 seconds
	ActVolume   = "volume"   // float64 percent
	ActLoop     = "loop"     // LoopNone/LoopTrack/LoopPlaylist
	ActShuffle  = "shuffle"  // bool
	ActOpen     = "open"     // string path or file URI
)

// Loop status values required by the MPRIS spec.
const (
	LoopNone     = "None"
	LoopTrack    = "Track"
	LoopPlaylist = "Playlist"
)

// Service prefixes the D-Bus bus name: org.mpris.MediaPlayer2.muxplayer.
const Service = "org.mpris.MediaPlayer2.muxplayer"

// identity is the player name controllers display.
const identity = "muxplayer"

// mimeTypes advertises the playable audio formats to controllers.
var mimeTypes = []string{
	"audio/mpeg", "audio/flac", "audio/ogg", "audio/opus", "audio/x-wav",
	"audio/mp4", "audio/aac", "audio/x-ms-wma", "audio/x-ape",
}

// Action is a controller-invoked command; Arg is kind-specific and may be nil.
// The muxplayer TUI loop receives actions through Server.Actions.
type Action struct {
	Kind string
	Arg  any
}

// Snapshot is the player state that the app pushes to the bus. Position and
// duration are seconds, volume is a percentage.
type Snapshot struct {
	Track   string // absolute path of the playing file, "" when idle
	Title   string
	Artist  string
	Album   string
	ArtURL  string
	Playing bool
	Shuffle bool
	Loop    string // LoopNone, LoopTrack or LoopPlaylist
	Pos     float64
	Dur     float64
	Vol     float64
	HasPrev bool
	HasNext bool
}

// Status derives the MPRIS PlaybackStatus value from a snapshot.
func (s Snapshot) Status() string {
	switch {
	case s.Track == "":
		return "Stopped"
	case s.Playing:
		return "Playing"
	default:
		return "Paused"
	}
}
