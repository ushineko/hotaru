// Package cooler is the liquid cooler, through sanshoku: a sanshoku.Device in,
// the service's Cooler out. The protocol, the node search, the usbfs claim, the
// placement and the push floor are sanshoku's nzxt driver, which is this
// package's old code ported with its measurements (hotaru spec 059).
package cooler

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/gif"
	"sync"
	"time"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/cooling"
	"github.com/ushineko/sanshoku/nzxt"
	"github.com/ushineko/sanshoku/screen"
)

// Status is what the cooler reports about itself.
type Status = cooling.Status

// Device is one cooler, for reporting. HID is the control node; USB, the
// panel's usbfs node, is empty because sanshoku does not report it. Screen is
// the panel in the words the window shows, empty for none.
type Device struct {
	Product        uint16
	Name, HID, USB string
	Screen         string
}

// sentinel is an error that is another underneath without saying so.
type sentinel struct {
	text  string
	under error
}

func (s sentinel) Error() string { return s.text }
func (s sentinel) Unwrap() error { return s.under }

// ErrNoCooler is a machine with no cooler this package drives, and ErrNoScreen
// a cooler whose panel cannot be driven. Both are ordinary states, not failures.
var (
	ErrNoCooler error = sentinel{"no supported liquid cooler", sanshoku.ErrAbsent}
	ErrNoScreen error = sentinel{"no screen on this cooler", screen.ErrNoPanel}
)

// Open finds a supported cooler and opens it. Every candidate is tried, since
// only one of a device's nodes answers. A node this user may not open is
// reported as that, naming the udev rule, rather than as absence (#136).
func Open(ctx context.Context) (*Cooler, error) {
	found, err := sanshoku.Scan(ctx, nzxt.Driver{})
	var denied error
	for _, candidate := range found {
		dev, failed := candidate.Open(ctx)
		switch {
		case failed == nil:
			return New(dev), nil
		case sanshoku.IsPermission(failed):
			denied = fmt.Errorf("%w: %s at %s may not be opened by this user; "+
				"packaging/60-hotaru.rules grants it to the seat: %w",
				sanshoku.ErrUnavailable, candidate.Name, candidate.Path, failed)
		default:
			err = failed
		}
	}
	switch {
	case denied != nil:
		return nil, denied
	case err == nil:
		return nil, ErrNoCooler
	case errors.Is(err, sanshoku.ErrAbsent), errors.Is(err, sanshoku.ErrUnsupported):
		return nil, fmt.Errorf("%w: %w", ErrNoCooler, err)
	}
	return nil, fmt.Errorf("look for a cooler: %w", err)
}

// Cooler is one open cooler, as the service sees it. The driver owns the
// handle, its mutex and its freshness; this adds the service's errors and the
// one fact the daemon needs, that the device has gone.
type Cooler struct {
	dev    sanshoku.Device
	source cooling.Source // nil for a device that reports no status
	panel  screen.Panel   // nil for a device without one
	once   sync.Once
	gone   chan struct{}
	mu     sync.Mutex
	wrong  error // why the panel could not be drawn on, once something tried
}

// New adapts an open device. Open calls it; a test hands it a fake.
func New(dev sanshoku.Device) *Cooler {
	c := &Cooler{dev: dev, gone: make(chan struct{})}
	c.source, _ = dev.(cooling.Source)
	c.panel, _ = dev.(screen.Panel)
	return c
}

// Status is the cooler's reading, taken now or within the driver's freshness.
func (c *Cooler) Status(ctx context.Context) (Status, error) {
	if c.source == nil {
		return Status{}, ErrNoCooler
	}
	s, err := c.source.Status(ctx)
	return s, c.check(err)
}

// Device is what is being read, for reporting.
func (c *Cooler) Device() Device {
	id := c.dev.Identity()
	d := Device{Product: id.Product, Name: id.Name, HID: id.Path}
	if c.panel != nil {
		w, h := c.panel.Size()
		d.Screen = fmt.Sprintf("%dx%d LCD", w, h)
	}
	return d
}

// Show puts a GIF on the screen; the firmware keeps no still picture (spec 012).
func (c *Cooler) Show(ctx context.Context, data []byte) error {
	return c.draw(func(p screen.Panel) error {
		g, err := gif.DecodeAll(bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("that is not a GIF: %w", err)
		}
		return p.Image(ctx, g) //nolint:wrapcheck // the driver names the device
	})
}

// Readout hands the screen back to the cooler's own display.
func (c *Cooler) Readout(ctx context.Context) error {
	return c.draw(func(p screen.Panel) error { return p.Readout(ctx) })
}

// Appearance sets brightness and orientation, which the device keeps.
func (c *Cooler) Appearance(ctx context.Context, brightness, degrees int) error {
	return c.draw(func(p screen.Panel) error { return p.Appearance(ctx, brightness, degrees) })
}

// Panel is the screen this cooler has, and why it cannot be drawn on: the
// error appears the first time a draw finds the panel cannot be claimed.
func (c *Cooler) Panel() (string, error) {
	if c.panel == nil {
		return "", ErrNoScreen
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Device().Screen, c.wrong
}

// Floor is how long the panel needs between frames this size; zero for none.
func (c *Cooler) Floor(size int) time.Duration {
	if c.panel == nil {
		return 0
	}
	return c.panel.Floor(size)
}

// Close closes the device, handing the panel back if anything drew on it.
func (c *Cooler) Close() error {
	if err := c.dev.Close(); err != nil {
		return fmt.Errorf("close the cooler: %w", err)
	}
	return nil
}

// Gone is closed when the device has gone away. The adapter is closed by then.
func (c *Cooler) Gone() <-chan struct{} { return c.gone }

// draw is one panel call. No panel, or one that will not be claimed, is
// ErrNoScreen, as a machine without one; the reason is kept for the window.
func (c *Cooler) draw(call func(screen.Panel) error) error {
	if c.panel == nil {
		return ErrNoScreen
	}
	err := call(c.panel)
	if errors.Is(err, screen.ErrNoPanel) {
		err = fmt.Errorf("%w: %w", ErrNoScreen, err)
		c.mu.Lock()
		c.wrong = err
		c.mu.Unlock()
	}
	return c.check(err)
}

// check closes the adapter, once, on a device that has gone away, so the
// daemon can look for it again rather than hold a dead handle.
func (c *Cooler) check(err error) error {
	if errors.Is(err, sanshoku.ErrGone) {
		c.once.Do(func() {
			_ = c.dev.Close()
			close(c.gone)
		})
	}
	return err
}
