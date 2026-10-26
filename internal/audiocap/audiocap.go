// Package audiocap captures the audio that is playing on the system's output
// device as interleaved float32 samples.
//
// Capture is implemented natively, without launching an external program:
//
//   - Linux talks to PulseAudio, or to PipeWire's PulseAudio compatibility
//     server, over the native protocol.
//   - Windows uses WASAPI loopback on the default render endpoint.
//   - Other platforms currently return ErrUnsupported.
//
// The package deliberately knows nothing about FFTs or the TUI. A backend only
// has to implement open(Options) (Source, error) in a file with the appropriate
// build tag. Push-style APIs can use the shared buffer helper to expose Source's
// blocking Read. See internal/visualizer for the reusable spectrum analyser
// that sits on top of a Source.
package audiocap

import (
	"errors"
	"fmt"
)

var (
	// ErrUnsupported is returned when the current platform has no capture
	// backend yet.
	ErrUnsupported = errors.New("audiocap: system audio capture is unsupported on this platform")

	// ErrClosed is returned by Source.Read after the source has been closed.
	ErrClosed = errors.New("audiocap: capture stream closed")
)

// Format describes the sample layout of a Source.
type Format struct {
	SampleRate int
	Channels   int
}

// Source is an open system-audio capture stream.
//
// Read blocks until audio is available. Samples are interleaved float32 values
// in the range [-1, 1]. Close may be called from another goroutine and must
// unblock a pending Read. After Close, Read returns ErrClosed.
type Source interface {
	Format() Format
	Read([]float32) (int, error)
	Close() error
}

// Options configures Open.
//
// A backend may ignore requests it cannot satisfy: the Format returned by
// Source describes what is actually being captured.
type Options struct {
	// SampleRate is the desired capture rate. Zero lets the backend choose
	// (normally 48000 Hz).
	SampleRate int

	// Channels is the desired channel count. Zero means stereo. Some backends
	// always use the output device's native format.
	Channels int

	// Device selects the output to capture. On Linux it is a PulseAudio/PipeWire
	// sink name; an empty value captures the default sink. WASAPI loopback on
	// Windows always follows the default render endpoint.
	Device string
}

// Open starts capturing the system's audio output.
func Open(opts Options) (Source, error) {
	if opts.Channels <= 0 {
		opts.Channels = 2
	}
	if opts.SampleRate < 0 {
		opts.SampleRate = 0
	}
	src, err := open(opts)
	if err != nil {
		return nil, fmt.Errorf("audiocap: %w", err)
	}
	return src, nil
}
