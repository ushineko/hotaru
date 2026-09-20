package openrgb

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/devices"
)

/*
Fake is an OpenRGB server that exists only in memory.

Not a test helper tucked behind a build tag: the service is developed against
this, and the whole suite runs with no hardware, no OpenRGB and no root — which
is what lets a PKGBUILD's check() run it and what lets someone fix a bug on a
laptop with no RGB in it at all.

It can also misbehave on purpose, because the interesting failures are not
errors. Lies makes a device accept a mode and not take it, which is the ASUS
board reporting success and leaving its headers dark; Unreachable makes every
call fail the way a stopped server does.
*/
type Fake struct {
	mu sync.Mutex

	devices []devices.Device
	version uint32

	// Lies maps a device name to a mode it accepts without honouring. The
	// write succeeds, the active mode does not change, and only a read-back
	// notices — which is the point.
	Lies map[string]string

	// Unreachable makes every call fail, as a stopped server does.
	Unreachable error

	// Writes is every frame written, in order, for a test to assert against.
	Writes []Write
	// Modes is every mode set, in order.
	Modes []ModeWrite

	closed bool
}

// Write is one frame that was written to a device.
type Write struct {
	Device string
	Frame  devices.Frame
}

// ModeWrite is one mode that was set.
type ModeWrite struct {
	Device     string
	Mode       string
	Brightness *int
}

// NewFake is a server holding these devices.
func NewFake(list ...devices.Device) *Fake {
	f := &Fake{version: 3, Lies: map[string]string{}}
	for _, d := range list {
		if d.Colours == nil {
			d.Colours = make([]colour.Colour, d.LEDCount)
		}
		f.devices = append(f.devices, d)
	}
	return f
}

// Devices is the current list, including what each device is showing.
func (f *Fake) Devices(context.Context) ([]devices.Device, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Unreachable != nil {
		return nil, f.Unreachable
	}
	out := make([]devices.Device, len(f.devices))
	for i, d := range f.devices {
		out[i] = d
		out[i].Colours = append([]colour.Colour(nil), d.Colours...)
	}
	return out, nil
}

// SetMode records the write, and applies it unless this device lies about it.
func (f *Fake) SetMode(_ context.Context, device, mode string, brightness *int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Unreachable != nil {
		return f.Unreachable
	}

	for i := range f.devices {
		if !strings.EqualFold(f.devices[i].Name, device) {
			continue
		}
		if _, ok := f.devices[i].Mode(mode); !ok {
			return fmt.Errorf("%s has no mode called %q", device, mode)
		}
		f.Modes = append(f.Modes, ModeWrite{Device: device, Mode: mode, Brightness: brightness})
		if lie, ok := f.Lies[f.devices[i].Name]; ok && strings.EqualFold(lie, mode) {
			return nil // accepted, not honoured: only a read-back can tell
		}
		f.devices[i].ActiveMode = mode
		return nil
	}
	return fmt.Errorf("no device called %q", device)
}

// SetFrame records the frame and shows it.
func (f *Fake) SetFrame(_ context.Context, device string, frame devices.Frame) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Unreachable != nil {
		return f.Unreachable
	}

	for i := range f.devices {
		if !strings.EqualFold(f.devices[i].Name, device) {
			continue
		}
		if want, got := f.devices[i].LEDCount, len(frame.Colours); want != got {
			return fmt.Errorf("%s has %d LEDs and the frame has %d", device, want, got)
		}
		f.Writes = append(f.Writes, Write{Device: device, Frame: frame})
		f.devices[i].Colours = append([]colour.Colour(nil), frame.Colours...)
		return nil
	}
	return fmt.Errorf("no device called %q", device)
}

// ProtocolVersion is whatever the fake claims to have agreed.
func (f *Fake) ProtocolVersion() uint32 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.version
}

// Close marks the fake closed; using it afterwards is a test's own bug.
func (f *Fake) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

// Closed reports whether Close was called.
func (f *Fake) Closed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

// Showing is what a device currently displays, for asserting the end state
// rather than the sequence that produced it.
func (f *Fake) Showing(device string) (devices.Frame, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, d := range f.devices {
		if strings.EqualFold(d.Name, device) {
			return devices.Frame{Device: d.Name, Colours: append([]colour.Colour(nil), d.Colours...)}, true
		}
	}
	return devices.Frame{}, false
}

// Add puts another device on the server, as a late-enumerating one appears.
func (f *Fake) Add(d devices.Device) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if d.Colours == nil {
		d.Colours = make([]colour.Colour, d.LEDCount)
	}
	f.devices = append(f.devices, d)
}

var _ Client = (*Fake)(nil)
var _ Client = (*Conn)(nil)
