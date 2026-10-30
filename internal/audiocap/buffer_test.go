package audiocap

// note: these tests race close against a blocked Read, so they can be flaky on slow machines

import (
	"errors"
	"testing"
	"time"
)

func TestBufferDropsOldest(t *testing.T) {
	b := newBuffer(Format{SampleRate: 48000, Channels: 1}, 3)
	b.push([]float32{1, 2})
	b.push([]float32{3, 4})

	out := make([]float32, 3)
	n, err := b.Read(out)
	if err != nil || n != 3 {
		t.Fatalf("Read = %d, %v; want 3, nil", n, err)
	}
	want := []float32{2, 3, 4}
	for i := range want {
		if out[i] != want[i] {
			t.Fatalf("out = %v, want %v", out, want)
		}
	}
}

func TestBufferWrapsReads(t *testing.T) {
	b := newBuffer(Format{SampleRate: 48000, Channels: 1}, 5)
	b.push([]float32{1, 2, 3, 4, 5})
	first := make([]float32, 2)
	n, err := b.Read(first)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 || first[0] != 1 || first[1] != 2 {
		t.Fatalf("first read = %v (%d)", first, n)
	}
	b.push([]float32{6, 7})

	rest := make([]float32, 5)
	n, err = b.Read(rest)
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Fatalf("rest read = %d", n)
	}
	want := []float32{3, 4, 5, 6, 7}
	for i := range want {
		if rest[i] != want[i] {
			t.Fatalf("rest = %v, want %v", rest, want)
		}
	}
}

func TestBufferCloseUnblocksRead(t *testing.T) {
	b := newBuffer(Format{SampleRate: 48000, Channels: 2}, 8)
	done := make(chan error, 1)
	go func() {
		_, err := b.Read(make([]float32, 4))
		done <- err
	}()

	time.Sleep(10 * time.Millisecond)
	b.close(ErrClosed)

	select {
	case err := <-done:
		if !errors.Is(err, ErrClosed) {
			t.Fatalf("Read error = %v, want ErrClosed", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Read did not unblock after Close")
	}

	if n, err := b.Read(make([]float32, 4)); n != 0 || !errors.Is(err, ErrClosed) {
		t.Fatalf("Read after close = %d, %v", n, err)
	}
}

func TestBufferDeliversDataBeforeError(t *testing.T) {
	b := newBuffer(Format{SampleRate: 48000, Channels: 1}, 8)
	b.push([]float32{1, 2})
	b.close(ErrClosed)

	out := make([]float32, 4)
	if n, err := b.Read(out); n != 2 || err != nil {
		t.Fatalf("Read = %d, %v; want 2, nil", n, err)
	}
	if n, err := b.Read(out); n != 0 || !errors.Is(err, ErrClosed) {
		t.Fatalf("second Read = %d, %v; want 0, ErrClosed", n, err)
	}
}
