//go:build !windows

// The D-Bus side of the MPRIS interface: bus setup, introspection, exported
// methods, and property updates. Uses godbus's prop server so every property
// change magically becomes a PropertiesChanged signal.
package mpris

import (
	"fmt"
	"hash/fnv"
	"net/url"
	"os"
	"strconv"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/godbus/dbus/v5/prop"
)

// Path is the object path shared by both interfaces; each track advertises
// itself under /muxplayer/track/<hash>.
const (
	Path       = dbus.ObjectPath("/org/mpris/MediaPlayer2")
	trackPfx   = "/muxplayer/track/"
	noTrack    = dbus.ObjectPath("/org/mpris/MediaPlayer2/TrackList/NoTrack")
	rootName   = "org.mpris.MediaPlayer2"
	playerName = "org.mpris.MediaPlayer2.Player"
)

// Server is the D-Bus endpoint. The app loop pushes snapshots in and reads
// controller actions out.
type Server struct {
	conn    *dbus.Conn
	actions chan Action
	props   *prop.Properties
	last    Snapshot // previous snapshot, for change detection
}

// Start connects to the session bus and exports the player interfaces. It
// returns nil when no bus is available, leaving playback untouched.
func Start() *Server {
	conn, err := dbus.SessionBus()
	if err != nil {
		return nil
	}
	s := &Server{actions: make(chan Action, 16)}
	root := &rootIface{s: s}
	player := &playerIface{s: s}
	if err = conn.Export(root, Path, rootName); err != nil {
		conn.Close()
		return nil
	}
	if err = conn.Export(player, Path, playerName); err != nil {
		conn.Close()
		return nil
	}
	if s.props, err = prop.Export(conn, Path, propsSpec(s)); err != nil {
		conn.Close()
		return nil
	}
	exportIntrospection(conn, root, player, s.props)
	if !s.claimName(conn) {
		conn.Close()
		return nil
	}
	return s
}

// claimName takes a unique name for this instance. Setup succeeded, so claim
// failure only downgrades the controller experience.
func (s *Server) claimName(conn *dbus.Conn) bool {
	r, err := conn.RequestName(Service, dbus.NameFlagDoNotQueue)
	if err == nil && r == dbus.RequestNameReplyPrimaryOwner {
		return true
	}
	// Another instance owns the name; take an instance-suffixed one instead.
	alt := fmt.Sprintf("%s.instance%d", Service, os.Getpid())
	_, err = conn.RequestName(alt, dbus.NameFlagDoNotQueue)
	return err == nil
}

// exportIntrospection serves the combined XML for both interfaces, which lets
// controllers discover the full method and property surface in one call.
func exportIntrospection(conn *dbus.Conn, root *rootIface, player *playerIface, props *prop.Properties) error {
	n := &introspect.Node{
		Name: string(Path),
		Interfaces: []introspect.Interface{
			introspect.IntrospectData,
			prop.IntrospectData,
			{Name: rootName, Methods: introspect.Methods(root), Properties: props.Introspection(rootName)},
			{Name: playerName, Methods: introspect.Methods(player), Properties: props.Introspection(playerName)},
		},
	}
	return conn.Export(introspect.NewIntrospectable(n), Path, "org.freedesktop.DBus.Introspectable")
}

// propsSpec declares the MPRIS interfaces. Controller-writable properties
// feedback through callbacks, which send actions to the app loop.
func propsSpec(s *Server) prop.Map {
	send := func(kind string) func(*prop.Change) *dbus.Error {
		return func(c *prop.Change) *dbus.Error {
			s.send(kind, c.Value)
			return nil
		}
	}
	return prop.Map{
		rootName: {
			"Identity":            {Value: identity, Emit: prop.EmitConst},
			"CanQuit":             {Value: true, Emit: prop.EmitConst},
			"CanRaise":            {Value: false, Emit: prop.EmitConst},
			"CanSetFullscreen":    {Value: false, Emit: prop.EmitConst},
			"Fullscreen":          {Value: false, Emit: prop.EmitFalse},
			"SupportedUriSchemes": {Value: []string{"file"}, Emit: prop.EmitConst},
			"SupportedMimeTypes":  {Value: mimeTypes, Emit: prop.EmitConst},
		},
		playerName: {
			"PlaybackStatus": {Value: "Stopped", Emit: prop.EmitTrue},
			"LoopStatus":     {Value: LoopNone, Writable: true, Emit: prop.EmitTrue, Callback: send(ActLoop)},
			"Rate":           {Value: 1.0, Emit: prop.EmitConst},
			"Shuffle":        {Value: false, Writable: true, Emit: prop.EmitTrue, Callback: send(ActShuffle)},
			"Metadata":       {Value: metadataMap(*new(Snapshot)), Emit: prop.EmitTrue},
			"Volume":         {Value: 100.0, Writable: true, Emit: prop.EmitTrue, Callback: send(ActVolume)},
			"Position":       {Value: int64(0), Emit: prop.EmitFalse},
			"MinimumRate":    {Value: 1.0, Emit: prop.EmitConst},
			"MaximumRate":    {Value: 1.0, Emit: prop.EmitConst},
			"CanGoNext":      {Value: false, Emit: prop.EmitTrue},
			"CanGoPrevious":  {Value: false, Emit: prop.EmitTrue},
			"CanPlay":        {Value: true, Emit: prop.EmitConst},
			"CanPause":       {Value: true, Emit: prop.EmitConst},
			"CanSeek":        {Value: true, Emit: prop.EmitConst},
			"CanControl":     {Value: true, Emit: prop.EmitConst},
		},
	}
}

/* ---- exported: org.mpris.MediaPlayer2 ---- */

type rootIface struct{ s *Server }

// Raise brings the player to the foreground; a terminal app cannot be raised.
func (r rootIface) Raise() *dbus.Error { return nil }

// Quit asks the player to save and shut down.
func (r rootIface) Quit() *dbus.Error { r.s.send(ActQuit, nil); return nil }

/* ---- exported: org.mpris.MediaPlayer2.Player ---- */

type playerIface struct{ s *Server }

// Play resumes playback or starts the first track.
func (p playerIface) Play() *dbus.Error { p.s.send(ActPlay, nil); return nil }

// Pause suspends playback.
func (p playerIface) Pause() *dbus.Error { p.s.send(ActPause, nil); return nil }

// PlayPause toggles between the two.
func (p playerIface) PlayPause() *dbus.Error { p.s.send(ActToggle, nil); return nil }

// Stop halts playback.
func (p playerIface) Stop() *dbus.Error { p.s.send(ActStop, nil); return nil }

// Next skips forward.
func (p playerIface) Next() *dbus.Error { p.s.send(ActNext, nil); return nil }

// Previous skips backward.
func (p playerIface) Previous() *dbus.Error { p.s.send(ActPrevious, nil); return nil }

// Seek offsets the position by microseconds.
func (p playerIface) Seek(micros int64) *dbus.Error {
	p.s.send(ActSeekRel, float64(micros)/1e6)
	return nil
}

// SetPosition jumps to pos microseconds if the track still is the current one.
func (p playerIface) SetPosition(track dbus.ObjectPath, pos int64) *dbus.Error {
	if track == noTrack {
		return nil
	}
	p.s.send(ActSeekAbs, max(0, float64(pos)/1e6))
	return nil
}

// OpenUri plays a local file.
func (p playerIface) OpenUri(uri string) *dbus.Error { p.s.send(ActOpen, uri); return nil }

/* ---- state plumbing ---- */

// send queues an action for the app loop, dropping it rather than block D-Bus.
func (s *Server) send(kind string, arg any) {
	if s == nil {
		return
	}
	select {
	case s.actions <- Action{Kind: kind, Arg: arg}:
	default:
	}
}

// Actions yields controller commands until Close.
func (s *Server) Actions() <-chan Action { return s.actions }

// Close releases the bus connection and name.
func (s *Server) Close() {
	if s == nil || s.conn == nil {
		return
	}
	_ = s.conn.Close()
}

// Update pushes a snapshot; only properties whose value changed reach the bus
// or trigger PropertiesChanged signals. Safe to call at high frequency since
// it persists position silently.
func (s *Server) Update(st Snapshot) {
	if s == nil {
		return
	}
	metaChanged := st.Track != s.last.Track || st.Title != s.last.Title ||
		st.Artist != s.last.Artist || st.Album != s.last.Album ||
		st.ArtURL != s.last.ArtURL || st.Dur != s.last.Dur
	p := playerName
	switch {
	case metaChanged:
		s.props.SetMust(p, "Metadata", metadataMap(st))
	case st.Track == "":
		return
	}
	if st.Status() != s.last.Status() {
		s.props.SetMust(p, "PlaybackStatus", st.Status())
	}
	s.props.SetMust(p, "Position", int64(st.Pos*1e6))
	if st.Vol != s.last.Vol {
		s.props.SetMust(p, "Volume", st.Vol)
	}
	if st.Loop != s.last.Loop {
		s.props.SetMust(p, "LoopStatus", st.Loop)
	}
	if st.Shuffle != s.last.Shuffle {
		s.props.SetMust(p, "Shuffle", st.Shuffle)
	}
	if st.HasNext != s.last.HasNext {
		s.props.SetMust(p, "CanGoNext", st.HasNext)
	}
	if st.HasPrev != s.last.HasPrev {
		s.props.SetMust(p, "CanGoPrevious", st.HasPrev)
	}
	s.last = st
}

// metadataMap builds the a{sv} Metadata property: the reserved NoTrack id when
// idle, otherwise the full spec fields that are known for the track.
func metadataMap(st Snapshot) map[string]dbus.Variant {
	m := map[string]dbus.Variant{"mpris:trackid": dbus.MakeVariant(noTrack)}
	if st.Track == "" {
		return m
	}
	if st.Title != "" {
		m["xesam:title"] = dbus.MakeVariant(st.Title)
	}
	if st.Artist != "" {
		m["xesam:artist"] = dbus.MakeVariant([]string{st.Artist})
	}
	if st.Album != "" {
		m["xesam:album"] = dbus.MakeVariant(st.Album)
	}
	if st.ArtURL != "" {
		m["mpris:arturl"] = dbus.MakeVariant(st.ArtURL)
	}
	if st.Dur > 0 {
		m["mpris:length"] = dbus.MakeVariant(int64(st.Dur * 1e6))
	}
	m["xesam:url"] = dbus.MakeVariant((&url.URL{Scheme: "file", Path: st.Track}).String())
	m["mpris:trackid"] = dbus.MakeVariant(trackID(st.Track))
	return m
}

// trackID derives a stable, path-safe mpris:trackid from the track path.
func trackID(path string) dbus.ObjectPath {
	h := fnv.New32a()
	_, _ = h.Write([]byte(path))
	return dbus.ObjectPath(trackPfx + strconv.FormatUint(uint64(h.Sum32()), 10))
}
