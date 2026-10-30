//go:build windows

// Windows has no D-Bus session bus, so nothing is published; the stub keeps
// the shared types compiled and makes every call a no-op.
package mpris

// note: windows gets a no-op server so the rest of the app doesn't care

// Server is an inert placeholder on Windows.
type Server struct{}

// Start is a no-op on Windows and always returns nil.
func Start() *Server { return nil }

// Update is a no-op on Windows.
func (s *Server) Update(Snapshot) {}

// Actions always returns nil; receiving from a nil channel blocks forever.
func (s *Server) Actions() <-chan Action { return nil }

// Close is a no-op on Windows.
func (s *Server) Close() {}
