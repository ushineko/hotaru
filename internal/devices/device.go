/*
Package devices turns what a controller says about itself into what hotaru
should send it.

Nothing here performs I/O. A Device is a snapshot someone else fetched, and
every function is a pure decision about it, which is what makes the awkward
hardware in specs/001 testable without owning any of it.

The governing rule: **a device's own report is the only trustworthy source.**
The configuration file corrects; it never asserts capability. A machine with no
rules at all still gets a mode each of its devices actually advertises.
*/
package devices

import (
	"fmt"
	"strings"

	"github.com/ushineko/hotaru/internal/colour"
)

// Device is one controller as it describes itself, plus what it is showing.
type Device struct {
	Name string
	// Location is where the controller is attached: an I2C address, a hidraw
	// node. With Serial it is what makes two controllers of the same model
	// distinguishable, which four identical sticks of RAM require.
	Location   string
	Serial     string
	Type       string
	Modes      []Mode
	ActiveMode string
	Zones      []Zone
	LEDCount   int

	// Colours is what the device reports it is currently showing, one per LED.
	// Observed state rather than configuration: it is the base a partial scene
	// composes onto, and the thing reconciliation compares desired state
	// against.
	Colours []colour.Colour
}

// Showing is the device's current lighting as a frame, for comparing against
// what it should be showing.
func (d *Device) Showing() Frame {
	return Frame{Device: d.Name, Colours: d.Colours}
}

/*
Mode is one of a device's lighting modes.

PerLED comes from the mode's own flags rather than from its name. OpenRGB
reports whether a mode accepts a colour per LED, so "can this device show three
fans in three colours?" is answered by the hardware instead of guessed from the
word "direct" — which the Python had to do, and which is wrong on any device
whose vendor named things differently.
*/
type Mode struct {
	Name       string
	PerLED     bool
	Brightness bool
}

// Zone is a contiguous run of LEDs within a device, as the device groups them.
type Zone struct {
	Name  string
	First int // index of the zone's first LED within the device
	Count int
}

// Last is the index of the zone's final LED.
func (z Zone) Last() int { return z.First + z.Count - 1 }

// Mode returns the device's own spelling of a mode, matched case-insensitively,
// and whether it has one. The device's spelling is what gets sent back, so a
// later read of the active mode can be compared against it.
func (d *Device) Mode(name string) (Mode, bool) {
	for _, m := range d.Modes {
		if strings.EqualFold(m.Name, name) {
			return m, true
		}
	}
	return Mode{}, false
}

// Zone returns a zone by name, matched case-insensitively.
func (d *Device) Zone(name string) (Zone, bool) {
	for _, z := range d.Zones {
		if strings.EqualFold(z.Name, name) {
			return z, true
		}
	}
	return Zone{}, false
}

// ModeNames is what the device advertises, in its own order and spelling — for
// a listing, and for telling someone why the mode they asked for is not there.
func (d *Device) ModeNames() []string {
	out := make([]string, 0, len(d.Modes))
	for _, m := range d.Modes {
		out = append(out, m.Name)
	}
	return out
}

// ZoneNames is the device's zones, in its own order and spelling.
func (d *Device) ZoneNames() []string {
	out := make([]string, 0, len(d.Zones))
	for _, z := range d.Zones {
		out = append(out, z.Name)
	}
	return out
}

/*
Unsupported is a device declining to do something, with the reason.

Returned rather than raised because one device that cannot express a scene is
not a failed scene: the caller reports it against that device and carries on
with the rest. The reason is written for a person reading CLI output.
*/
type Unsupported struct {
	Device string
	Why    string
}

func (e *Unsupported) Error() string { return e.Device + ": " + e.Why }

func unsupported(device, format string, args ...any) *Unsupported {
	return &Unsupported{Device: device, Why: fmt.Sprintf(format, args...)}
}
