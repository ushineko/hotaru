/*
Package coolertest is a cooler that exists only in memory: a sanshoku.Device
for cooler.New, so a test reaches the adapter without a device node.

Not a transport fake. The protocol and its misbehaviours are sanshoku's and
are tested there against its own fake; what is tested with this one is what
hotaru does with a reading, a refusal and a device that has gone.
*/
package coolertest

import (
	"context"
	"image/gif"
	"sync"
	"time"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/cooling"
)

/*
Device is a cooler with no panel.

Status returns Reading, or Err while it is set. GoneOnce makes the next Status
return sanshoku.ErrGone, once, as an unplugged cooler does.
*/
type Device struct {
	mu sync.Mutex

	Name    string
	Reading cooling.Status
	Err     error
	// GoneOnce is a cooler unplugged before the next reading.
	GoneOnce bool

	statuses int
	closed   bool
}

// New is a cooler reporting plausible numbers.
func New() *Device {
	return &Device{
		Name: "fake cooler",
		Reading: cooling.Status{
			Coolant: 37.5, PumpRPM: 2608, PumpDuty: 81, FanRPM: 1190, FanDuty: 51,
			HasPump: true, HasFan: true,
		},
	}
}

// Identity is a Kraken Elite's IDs under the fake's name.
func (d *Device) Identity() sanshoku.Identity {
	return sanshoku.Identity{
		Vendor: 0x1E71, Product: 0x3012, Bus: sanshoku.BusUSB,
		Name: d.Name, Path: "/dev/hidraw-fake",
	}
}

// Status is the reading, the error, or the device gone.
func (d *Device) Status(context.Context) (cooling.Status, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.statuses++
	switch {
	case d.GoneOnce:
		d.GoneOnce = false
		return cooling.Status{}, sanshoku.ErrGone
	case d.Err != nil:
		return cooling.Status{}, d.Err
	}
	s := d.Reading
	s.Taken = time.Now()
	return s, nil
}

// Close marks the device closed. Closing twice is not an error, as it is not
// on the real one.
func (d *Device) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.closed = true
	return nil
}

// Closed reports whether the device was closed.
func (d *Device) Closed() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.closed
}

// Statuses is how many readings were asked for.
func (d *Device) Statuses() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.statuses
}

/*
Panel is a cooler with a 640x640 panel: a Device that also satisfies
screen.Panel. Refuse is what every panel call returns while it is set, as a
panel another program has claimed does.
*/
type Panel struct {
	*Device

	Refuse error

	mu       sync.Mutex
	images   []*gif.GIF
	readouts int
}

// NewPanel is a cooler with a panel, reporting plausible numbers.
func NewPanel() *Panel { return &Panel{Device: New()} }

// Size is the Kraken Elite's.
func (p *Panel) Size() (w, h int) { return 640, 640 }

// Image records what was drawn.
func (p *Panel) Image(_ context.Context, g *gif.GIF) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.Refuse != nil {
		return p.Refuse
	}
	p.images = append(p.images, g)
	return nil
}

// Readout counts a hand-back to the firmware's display, and returns Refuse.
func (p *Panel) Readout(context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.Refuse == nil {
		p.readouts++
	}
	return p.Refuse
}

// Readouts is how many times the panel was handed back to the firmware.
func (p *Panel) Readouts() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.readouts
}

// Appearance returns Refuse.
func (p *Panel) Appearance(context.Context, int, int) error { return p.Refuse }

// Floor is two seconds, the measured floor for a dashboard-sized frame.
func (p *Panel) Floor(int) time.Duration { return 2 * time.Second }

// Images is how many pictures were drawn.
func (p *Panel) Images() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.images)
}
