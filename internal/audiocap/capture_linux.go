//go:build linux

package audiocap

import (
	"fmt"
	"sync"

	"github.com/jfreymuth/pulse"
)

// pulseSource captures the monitor source of an output sink. PipeWire exposes
// the same protocol through pipewire-pulse, so this works on both PulseAudio
// and PipeWire desktops.
type pulseSource struct {
	client *pulse.Client
	stream *pulse.RecordStream
	buf    *buffer
	once   sync.Once
}

func open(opts Options) (Source, error) {
	client, err := pulse.NewClient(pulse.ClientApplicationName("muxplayer"))
	if err != nil {
		return nil, fmt.Errorf("connect to PulseAudio/PipeWire: %w", err)
	}

	sink, err := client.DefaultSink()
	if opts.Device != "" {
		sink, err = client.SinkByID(opts.Device)
	}
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("find output sink: %w", err)
	}

	rate := opts.SampleRate
	if rate <= 0 {
		rate = 48000
	}

	var buf *buffer
	writer := pulse.Float32Writer(func(samples []float32) (int, error) {
		if buf != nil {
			buf.push(samples)
		}
		return len(samples), nil
	})
	recordOpts := []pulse.RecordOption{
		pulse.RecordSampleRate(rate),
		pulse.RecordMediaName("muxplayer visualizer"),
		pulse.RecordMonitor(sink),
	}
	if opts.Channels == 1 {
		recordOpts = append(recordOpts, pulse.RecordMono)
	} else {
		recordOpts = append(recordOpts, pulse.RecordStereo)
	}
	// RecordLatency must come after the sample rate and channel options.
	recordOpts = append(recordOpts, pulse.RecordLatency(0.05))

	stream, err := client.NewRecord(writer, recordOpts...)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("create record stream: %w", err)
	}

	format := Format{SampleRate: stream.SampleRate(), Channels: stream.Channels()}
	if format.SampleRate <= 0 {
		format.SampleRate = rate
	}
	if format.Channels <= 0 {
		format.Channels = opts.Channels
	}
	buf = newBuffer(format, format.SampleRate*format.Channels)
	stream.Start()

	return &pulseSource{client: client, stream: stream, buf: buf}, nil
}

func (s *pulseSource) Format() Format { return s.buf.Format() }

func (s *pulseSource) Read(dst []float32) (int, error) { return s.buf.Read(dst) }

func (s *pulseSource) Close() error {
	s.once.Do(func() {
		s.buf.close(ErrClosed)
		// Closing the connection also removes our record stream on the server.
		s.client.Close()
	})
	return nil
}
