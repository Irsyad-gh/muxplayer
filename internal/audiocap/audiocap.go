// Package audiocap captures system output as interleaved float32 samples.
// Linux uses PulseAudio, Windows uses WASAPI; other platforms say no.
package audiocap

// note: platform backends live behind build tags; this file should stay boring

import (
	"errors"
	"fmt"
)

var (
	// ErrUnsupported is returned when the current platform has no capture
	// backend yet.
	ErrUnsupported = errors.New("system audio capture not supported here")

	// ErrClosed is returned by Source.Read after the source has been closed.
	ErrClosed = errors.New("capture stream closed")
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

// Options configures Open. A backend may ignore requests it cannot satisfy:
// the Format of the returned Source describes what is actually captured.
type Options struct {
	SampleRate int    // desired capture rate; 0 = backend default (normally 48 kHz)
	Channels   int    // 0 = stereo; some backends always use the native format
	Device     string // PulseAudio/PipeWire sink to capture, "" = default sink; WASAPI loopback always follows the default endpoint
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
		return nil, fmt.Errorf("capture: %w", err)
	}
	return src, nil
}
