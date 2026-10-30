//go:build windows

package audiocap

// note: COM cleanup here is hand-rolled and fragile, handle with care

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows exposes the rendered audio stream of an output endpoint through
// WASAPI loopback. The endpoint's mix format (usually 32-bit float) is handed
// to us unchanged; the analyser works with any sample rate or channel count.

const (
	wfTagPCM        = 0x0001
	wfTagFloat      = 0x0003
	wfTagExtensible = 0xFFFE

	audclntShareModeShared            = 0x00000000
	audclntStreamFlagsLoopback        = 0x00020000
	audclntBufferFlagsSilent          = 0x00000002
	audclntSBufferEmpty        uint32 = 0x08890001

	clsctxAll = 0x17

	eRender  = 0
	eConsole = 0

	// IUnknown
	comRelease = 2

	// IMMDeviceEnumerator
	comEnumGetDefaultEndpoint = 4

	// IMMDevice
	comDeviceActivate = 3

	// IAudioClient
	comClientInitialize = 3
	comClientGetMixFmt  = 8
	comClientStart      = 10
	comClientStop       = 11
	comClientGetService = 14

	// IAudioCaptureClient
	comCaptureGetBuffer     = 3
	comCaptureReleaseBuffer = 4
)

var (
	clsidMMDeviceEnumerator = windows.GUID{
		Data1: 0xBCDE0395, Data2: 0xE52F, Data3: 0x467C,
		Data4: [8]byte{0x8E, 0x3D, 0xC4, 0x57, 0x92, 0x91, 0x69, 0x2E},
	}
	iidIMMDeviceEnumerator = windows.GUID{
		Data1: 0xA95664D2, Data2: 0x9614, Data3: 0x4F35,
		Data4: [8]byte{0xA7, 0x46, 0xDE, 0x8D, 0xB6, 0x36, 0x17, 0xE6},
	}
	iidIAudioClient = windows.GUID{
		Data1: 0x1CB9AD4C, Data2: 0xDBFA, Data3: 0x4C32,
		Data4: [8]byte{0xB1, 0x78, 0xC2, 0xF5, 0x68, 0xA7, 0x03, 0xB2},
	}
	iidIAudioCaptureClient = windows.GUID{
		Data1: 0xC8ADBD64, Data2: 0xE71E, Data3: 0x48A0,
		Data4: [8]byte{0xA4, 0xDE, 0x18, 0x5C, 0x39, 0x5C, 0xD3, 0x17},
	}
	ksSubtypePCM = windows.GUID{
		Data1: 0x00000001, Data2: 0x0000, Data3: 0x0010,
		Data4: [8]byte{0x80, 0x00, 0x00, 0xAA, 0x00, 0x38, 0x9B, 0x71},
	}
	ksSubtypeIEEE = windows.GUID{
		Data1: 0x00000003, Data2: 0x0000, Data3: 0x0010,
		Data4: [8]byte{0x80, 0x00, 0x00, 0xAA, 0x00, 0x38, 0x9B, 0x71},
	}
)

var (
	modOle32           = windows.NewLazySystemDLL("ole32.dll")
	procCoInitialize   = modOle32.NewProc("CoInitializeEx")
	procCoUninitialize = modOle32.NewProc("CoUninitialize")
	procCoCreate       = modOle32.NewProc("CoCreateInstance")
	procCoTaskMemFree  = modOle32.NewProc("CoTaskMemFree")
)

type hresult uint32

func (h hresult) failed() bool { return h&0x80000000 != 0 }

func (h hresult) err(op string) error {
	return fmt.Errorf("%s: 0x%08X", op, uint32(h))
}

// waveFormatEx mirrors the first 18 bytes of WAVEFORMATEX. Go may pad the
// struct itself, so code must never use unsafe.Sizeof as the format length.
type waveFormatEx struct {
	formatTag      uint16
	channels       uint16
	samplesPerSec  uint32
	avgBytesPerSec uint32
	blockAlign     uint16
	bitsPerSample  uint16
	cbSize         uint16
}

// waveFormatExtensible is the flat 40-byte WAVEFORMATEXTENSIBLE layout. The
// fields after cbSize deliberately do not embed waveFormatEx: the C layout
// puts wValidBitsPerSample at offset 18, dwChannelMask at 20 and SubFormat at
// 24, while an embedded Go struct would be padded to 20/22/26.
type waveFormatExtensible struct {
	formatTag      uint16
	channels       uint16
	samplesPerSec  uint32
	avgBytesPerSec uint32
	blockAlign     uint16
	bitsPerSample  uint16
	cbSize         uint16
	validBits      uint16
	channelMask    uint32
	subFormat      windows.GUID
}

const _ = uint(40 - unsafe.Sizeof(waveFormatExtensible{})) // compile-time size check, not an error

type sampleKind int

const (
	sampleFloat32 sampleKind = iota
	sampleInt16
	sampleInt24
	sampleInt32
)

type wasapiSource struct {
	buf  *buffer
	stop chan struct{}
	done chan struct{}
	once sync.Once
}

func open(opts Options) (Source, error) {
	s := &wasapiSource{stop: make(chan struct{}), done: make(chan struct{})}
	ready := make(chan error, 1)
	go s.run(opts, ready)
	if err := <-ready; err != nil {
		<-s.done
		return nil, err
	}
	return s, nil
}

func (s *wasapiSource) Format() Format { return s.buf.Format() }

func (s *wasapiSource) Read(dst []float32) (int, error) { return s.buf.Read(dst) }

func (s *wasapiSource) Close() error {
	s.once.Do(func() { close(s.stop) })
	<-s.done
	return nil
}

func (s *wasapiSource) run(opts Options, ready chan<- error) {
	defer close(s.done)
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := coInitialize(); err != nil {
		ready <- err
		return
	}
	defer procCoUninitialize.Call()

	dev, err := openLoopback()
	if err != nil {
		ready <- err
		return
	}
	defer dev.close()

	s.buf = newBuffer(dev.format, dev.format.SampleRate*dev.format.Channels)
	ready <- nil

	err = s.capture(dev)
	switch {
	case err == nil, errors.Is(err, ErrClosed):
		s.buf.close(ErrClosed)
	default:
		s.buf.close(err)
	}
}

func coInitialize() error {
	r, _, _ := procCoInitialize.Call(0, 0) // COINIT_MULTITHREADED
	if hr := hresult(uint32(r)); hr.failed() {
		return hr.err("CoInitializeEx")
	}
	return nil
}

// methodAddr returns the address of a COM vtable slot. The bound is larger than
// every interface used here and keeps the pointer arithmetic inside Go's rules.
func methodAddr(obj unsafe.Pointer, index int) uintptr {
	vtbl := *(**[64]uintptr)(obj)
	return vtbl[index]
}

func comCall(obj unsafe.Pointer, index int, args ...uintptr) hresult {
	all := make([]uintptr, 0, len(args)+1)
	all = append(all, uintptr(obj))
	all = append(all, args...)
	r, _, _ := syscall.SyscallN(methodAddr(obj, index), all...)
	return hresult(uint32(r))
}

func release(obj unsafe.Pointer) {
	if obj != nil {
		comCall(obj, comRelease)
	}
}

func coTaskMemFree(p unsafe.Pointer) {
	if p != nil {
		procCoTaskMemFree.Call(uintptr(p))
	}
}

func createEnumerator() (unsafe.Pointer, error) {
	var enum unsafe.Pointer
	r, _, _ := procCoCreate.Call(
		uintptr(unsafe.Pointer(&clsidMMDeviceEnumerator)),
		0,
		clsctxAll,
		uintptr(unsafe.Pointer(&iidIMMDeviceEnumerator)),
		uintptr(unsafe.Pointer(&enum)),
	)
	if hr := hresult(uint32(r)); hr.failed() {
		return nil, hr.err("CoCreateInstance(MMDeviceEnumerator)")
	}
	return enum, nil
}

type wasapiDevice struct {
	device  unsafe.Pointer
	client  unsafe.Pointer
	capture unsafe.Pointer
	format  Format
	kind    sampleKind
	stride  int // bytes per interleaved frame
}

func openLoopback() (*wasapiDevice, error) {
	enum, err := createEnumerator()
	if err != nil {
		return nil, err
	}
	defer release(enum)

	var device unsafe.Pointer
	if hr := comCall(enum, comEnumGetDefaultEndpoint, eRender, eConsole, uintptr(unsafe.Pointer(&device))); hr.failed() {
		return nil, hr.err("IMMDeviceEnumerator.GetDefaultAudioEndpoint")
	}

	var client unsafe.Pointer
	if hr := comCall(device, comDeviceActivate,
		uintptr(unsafe.Pointer(&iidIAudioClient)), clsctxAll, 0,
		uintptr(unsafe.Pointer(&client))); hr.failed() {
		release(device)
		return nil, hr.err("IMMDevice.Activate")
	}

	d := &wasapiDevice{device: device, client: client}

	var mix unsafe.Pointer
	if hr := comCall(client, comClientGetMixFmt, uintptr(unsafe.Pointer(&mix))); hr.failed() {
		d.close()
		return nil, hr.err("IAudioClient.GetMixFormat")
	}
	defer coTaskMemFree(mix)

	if d.format, d.kind, d.stride, err = parseFormat(mix); err != nil {
		d.close()
		return nil, err
	}
	if hr := comCall(client, comClientInitialize,
		audclntShareModeShared, audclntStreamFlagsLoopback,
		0, 0, uintptr(mix), 0); hr.failed() {
		d.close()
		return nil, hr.err("IAudioClient.Initialize")
	}
	if hr := comCall(client, comClientGetService,
		uintptr(unsafe.Pointer(&iidIAudioCaptureClient)),
		uintptr(unsafe.Pointer(&d.capture))); hr.failed() {
		d.close()
		return nil, hr.err("IAudioClient.GetService")
	}
	if hr := comCall(client, comClientStart); hr.failed() {
		d.close()
		return nil, hr.err("IAudioClient.Start")
	}
	return d, nil
}

func parseFormat(p unsafe.Pointer) (Format, sampleKind, int, error) {
	wf := (*waveFormatEx)(p)
	if wf.channels == 0 || wf.samplesPerSec == 0 || wf.blockAlign == 0 {
		return Format{}, 0, 0, errors.New("empty WASAPI mix format")
	}
	tag := wf.formatTag
	if tag == wfTagExtensible {
		if wf.cbSize < 22 {
			return Format{}, 0, 0, errors.New("truncated WAVEFORMATEXTENSIBLE")
		}
		ext := (*waveFormatExtensible)(p)
		switch ext.subFormat {
		case ksSubtypePCM:
			tag = wfTagPCM
		case ksSubtypeIEEE:
			tag = wfTagFloat
		default:
			return Format{}, 0, 0, fmt.Errorf("unknown WAVEFORMATEXTENSIBLE subtype %v", ext.subFormat)
		}
	}

	bytesPerSample := int(wf.blockAlign) / int(wf.channels)
	if bytesPerSample*int(wf.channels) != int(wf.blockAlign) {
		return Format{}, 0, 0, fmt.Errorf("bad block alignment %d for %d channels", wf.blockAlign, wf.channels)
	}
	bits := int(wf.bitsPerSample)

	var kind sampleKind
	switch {
	case tag == wfTagFloat && bits == 32 && bytesPerSample == 4:
		kind = sampleFloat32
	case tag == wfTagPCM && bits == 16 && bytesPerSample == 2:
		kind = sampleInt16
	case tag == wfTagPCM && bits == 24 && bytesPerSample == 3:
		kind = sampleInt24
	case tag == wfTagPCM && bits == 32 && bytesPerSample == 4:
		kind = sampleInt32
	default:
		return Format{}, 0, 0, fmt.Errorf("bad mix format: tag %d, %d bits, %d bytes/sample", tag, bits, bytesPerSample)
	}
	return Format{SampleRate: int(wf.samplesPerSec), Channels: int(wf.channels)}, kind, int(wf.blockAlign), nil
}

func (d *wasapiDevice) close() {
	if d == nil {
		return
	}
	if d.client != nil {
		comCall(d.client, comClientStop)
	}
	release(d.capture)
	release(d.client)
	release(d.device)
	d.capture, d.client, d.device = nil, nil, nil
}

func (s *wasapiSource) capture(d *wasapiDevice) error {
	channels := d.format.Channels
	var scratch []float32
	for {
		select {
		case <-s.stop:
			return ErrClosed
		default:
		}

		var data unsafe.Pointer
		var frames, flags uint32
		var devicePos, qpcPos uint64
		hr := comCall(d.capture, comCaptureGetBuffer,
			uintptr(unsafe.Pointer(&data)),
			uintptr(unsafe.Pointer(&frames)),
			uintptr(unsafe.Pointer(&flags)),
			uintptr(unsafe.Pointer(&devicePos)),
			uintptr(unsafe.Pointer(&qpcPos)))
		if uint32(hr) == audclntSBufferEmpty {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		if hr.failed() {
			return hr.err("IAudioCaptureClient.GetBuffer")
		}
		if frames == 0 {
			comCall(d.capture, comCaptureReleaseBuffer, 0)
			time.Sleep(time.Millisecond)
			continue
		}

		need := int(frames) * channels
		if cap(scratch) < need {
			scratch = make([]float32, need)
		}
		samples := scratch[:need]
		if flags&audclntBufferFlagsSilent != 0 || data == nil {
			clear(samples)
		} else {
			raw := unsafe.Slice((*byte)(data), int(frames)*d.stride)
			decode(samples, raw, d.kind)
		}
		s.buf.push(samples)
		comCall(d.capture, comCaptureReleaseBuffer, uintptr(frames))
	}
}

func decode(dst []float32, src []byte, kind sampleKind) {
	switch kind {
	case sampleFloat32:
		for i := range dst {
			dst[i] = math.Float32frombits(binary.LittleEndian.Uint32(src[i*4:]))
		}
	case sampleInt16:
		for i := range dst {
			dst[i] = float32(int16(binary.LittleEndian.Uint16(src[i*2:]))) / 32768
		}
	case sampleInt24:
		for i := range dst {
			b := src[i*3:]
			v := int32(b[0]) | int32(b[1])<<8 | int32(b[2])<<16
			if v&0x800000 != 0 {
				v |= ^0xFFFFFF
			}
			dst[i] = float32(v) / 8388608
		}
	case sampleInt32:
		for i := range dst {
			dst[i] = float32(int32(binary.LittleEndian.Uint32(src[i*4:]))) / 2147483648
		}
	}
}
