//go:build !linux && !windows

package audiocap

// note: this is what you get on a platform nobody has ported yet

// open is the fallback for platforms without a backend. A real one just needs
// to implement Source in a file with the right build tag.
func open(Options) (Source, error) {
	return nil, ErrUnsupported
}
