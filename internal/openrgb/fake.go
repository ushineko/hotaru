package openrgb

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

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

	// Delay is how long each write takes, for hardware that is not instant
	// and for tests that need a write to still be in flight.
	Delay time.Duration

	// Writes is every frame written, in order, for a test to assert against.
	Writes []Write
	// Modes is every mode set, in order.
	Modes []ModeWrite

	/*
		Sequence is every write in one list, frames and modes together.

		The two lists above say what was written and not what order the two
		kinds arrived in, and the order is the whole of what spec 056 is
		about: the same three packets in a different sequence are the
		difference between a keyboard showing an effect's colour and showing
		the last scene's. Each entry is "mode:<name>" or "frame".
	*/
	Sequence []string

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
	Colour     *colour.Colour
	// Colours are every colour the write named, where it named several.
	Colours []colour.Colour
	Speed   *int
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

// Device is one device by name.
func (f *Fake) Device(_ context.Context, name string) (devices.Device, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Unreachable != nil {
		return devices.Device{}, f.Unreachable
	}
	for _, d := range f.devices {
		if strings.EqualFold(d.Name, name) {
			out := d
			out.Colours = append([]colour.Colour(nil), d.Colours...)
			return out, nil
		}
	}
	return devices.Device{}, fmt.Errorf("no device called %q", name)
}

// SetMode records the write, and applies it unless this device lies about it.
func (f *Fake) SetMode(_ context.Context, device, mode string, style Style) error {
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
		f.Modes = append(f.Modes, ModeWrite{
			Device: device, Mode: mode,
			Brightness: style.Brightness, Colour: style.Colour, Speed: style.Speed,
			Colours: append([]colour.Colour(nil), style.Colours...),
		})
		f.Sequence = append(f.Sequence, "mode:"+mode)
		if lie, ok := f.Lies[f.devices[i].Name]; ok && strings.EqualFold(lie, mode) {
			return nil // accepted, not honoured: only a read-back can tell
		}
		f.devices[i].ActiveMode = mode
		f.applyModeColour(&f.devices[i], mode, style.Palette())
		return nil
	}
	return fmt.Errorf("no device called %q", device)
}

// SetFrame records the frame and shows it.
func (f *Fake) SetFrame(_ context.Context, device string, frame devices.Frame) error {
	f.mu.Lock()
	delay := f.Delay
	f.mu.Unlock()
	if delay > 0 {
		time.Sleep(delay)
	}

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
		f.Sequence = append(f.Sequence, "frame")
		if f.showsBuffer(f.devices[i]) {
			f.devices[i].Colours = append([]colour.Colour(nil), frame.Colours...)
		}
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

/*
Vanish makes the server stop answering, the way a process that has exited does.

Unreachable can be set directly while nothing is using the fake. This is for
the other case -- taking the server away from something already talking to it,
which is the only interesting moment to take it away -- and it takes the lock
that every read of the field already holds.
*/
func (f *Fake) Vanish(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Unreachable = err
}

/*
Gone is whether Unreachable is the socket rather than an answer.

Through the same test the hardware path uses, so a test saying the server went
away sets Unreachable to what a vanished server actually returns -- EPIPE, a
reset, an unexpected EOF -- rather than to a sentinel that only the fake
understands. An Unreachable that is some other error is a server that is
answering badly, which is not this.
*/
func (f *Fake) Gone() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return isGone(f.Unreachable)
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

/*
applyModeColour models a mode that carries its own colour.

Real hardware does this and it is the failure spec 009 exists for: a device put
into such a mode displays the colour stored in the mode, and a frame written
afterwards goes to a buffer the mode does not read. A fake that always showed
the last frame could never have caught it.

The mode's slots are filled the way the hardware path fills them, between the
mode's ColoursMin and ColoursMax (spec 061), and the lights show the first.
A device listed with neither has one slot, as the modes in tests written
before the slots were modelled do.
*/
func (f *Fake) applyModeColour(d *devices.Device, name string, want []colour.Colour) {
	for i := range d.Modes {
		m := &d.Modes[i]
		if !strings.EqualFold(m.Name, name) || !m.ModeColour {
			continue
		}
		if len(want) > 0 {
			most := m.ColoursMax
			if most == 0 {
				most = 1
			}
			m.Colours = Slots(want, len(m.Colours), m.ColoursMin, most)
			m.Colour = m.Colours[0]
		}
		if m.PerLED {
			return // the buffer is what it shows; the mode's colour is spare
		}
		for j := range d.Colours {
			d.Colours[j] = m.Colour
		}
		return
	}
}

// showsBuffer reports whether a device in its current mode displays the frame
// it is sent, rather than the colour its mode holds.
func (f *Fake) showsBuffer(d devices.Device) bool {
	mode, ok := d.Mode(d.ActiveMode)
	if !ok {
		return true
	}
	return mode.PerLED || !mode.ModeColour
}
