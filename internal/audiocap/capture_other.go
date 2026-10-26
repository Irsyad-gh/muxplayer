//go:build !linux && !windows

package audiocap

// open is the fallback for platforms that do not have a native backend yet.
// A future backend only needs to implement Source and live in a file with the
// appropriate build tag.
func open(Options) (Source, error) {
	return nil, ErrUnsupported
}
