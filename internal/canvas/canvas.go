/*
Package canvas is the devices whose lights hotaru draws, through sanshoku: a
sanshoku.Device that satisfies lighting.Canvas in, an attached device and its
animator out (spec 060).

A canvas device has no effects of its own. Its vendor's software draws every
effect on the host and streams the result, and the device holds the last
frame it was sent until it is rebooted. So hotaru draws the effect (package
render), and this package sends it: one frame for a scene that does not move,
a stream at the scene's interval for one that does.

The only product knowledge here is which sanshoku drivers can produce a
canvas, and it is one line (drivers, below). The adapter, the animator and
their tests are written against lighting.Canvas and nothing else.
*/
package canvas

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/lighting"
	"github.com/ushineko/sanshoku/steelseries"
)

/*
drivers are the sanshoku drivers that can find a canvas device.

The one place hotaru names a product family for drawing. A driver here finds
more than canvases (the steelseries driver finds every SteelSeries battery
too), so Open keeps only what satisfies lighting.Canvas and closes the rest
unused.
*/
func drivers() []sanshoku.Driver {
	return []sanshoku.Driver{steelseries.Driver{}}
}

// ErrNoCanvas is a machine with no device hotaru draws on. An ordinary state,
// reported once and then waited out, as a machine with no cooler is.
var ErrNoCanvas = fmt.Errorf("no device to draw on: %w", sanshoku.ErrAbsent)

/*
Open finds every canvas device and opens the ones skip does not name.

skip is the paths already attached, so a second handle is never opened on a
device the daemon already draws on. Every candidate is tried, since only one
of a device's nodes answers; a candidate that opens and is not a canvas is
closed again at once. A node this user may not open is reported as that,
naming the udev rule, rather than as absence.
*/
func Open(ctx context.Context, skip func(path string) bool) ([]*Device, error) {
	found, scanned := sanshoku.Scan(ctx, drivers()...)
	var (
		out    []*Device
		denied error
	)
	for _, candidate := range found {
		if skip != nil && skip(candidate.Path) {
			continue
		}
		dev, err := candidate.Open(ctx)
		switch {
		case err == nil:
			if c, ok := New(dev); ok {
				out = append(out, c)
				continue
			}
			_ = dev.Close()
		case sanshoku.IsPermission(err):
			denied = fmt.Errorf("%w: %s at %s may not be opened by this user; "+
				"packaging/60-hotaru.rules grants it to the seat: %w",
				sanshoku.ErrUnavailable, candidate.Name, candidate.Path, err)
		}
	}
	switch {
	case len(out) > 0:
		return out, nil
	case denied != nil:
		return nil, denied
	case scanned != nil && !errors.Is(scanned, sanshoku.ErrAbsent):
		return nil, fmt.Errorf("look for a device to draw on: %w", scanned)
	}
	return nil, ErrNoCanvas
}

/*
Device is one open canvas device, as hotaru sees it.

The driver owns the handle and serialises every exchange on it; this adds
the one fact the daemon needs, that the device has gone, and the one way it
can find that out while nothing is being drawn.
*/
type Device struct {
	dev  sanshoku.Device
	draw lighting.Canvas
	keys []lighting.Key
	once sync.Once
	gone chan struct{}

	// node is the kernel's name for the device behind the hidraw node, read
	// when it was opened. See Present.
	node string
}

// New adapts an open device, and says whether it is a canvas at all. Open
// calls it; a test hands it a fake.
func New(dev sanshoku.Device) (*Device, bool) {
	draw, ok := dev.(lighting.Canvas)
	if !ok {
		return nil, false
	}
	return &Device{
		dev: dev, draw: draw, keys: draw.Keys(), gone: make(chan struct{}),
		node: kernelNode(dev.Identity().Path),
	}, true
}

// Name is the kernel's name for the device, through sanshoku.
func (d *Device) Name() string { return d.dev.Identity().Name }

// Path is the node the device was opened on.
func (d *Device) Path() string { return d.dev.Identity().Path }

// Keys are the lights a frame may address, in the device's order.
func (d *Device) Keys() []lighting.Key { return append([]lighting.Key(nil), d.keys...) }

// Floor is the shortest interval at which the device shows every frame.
func (d *Device) Floor() time.Duration { return d.draw.Floor() }

// Frame sends one frame and waits for the device to acknowledge it.
func (d *Device) Frame(ctx context.Context, px []lighting.Pixel) error {
	return d.check(d.draw.Frame(ctx, px))
}

/*
Release hands the lighting back to the device's firmware.

On the device this was written for that reboots it, so the handle is dead
afterwards whether or not anything else is sent: the adapter is closed and
Gone fires here rather than waiting for a frame nobody may ever send.
*/
func (d *Device) Release(ctx context.Context) error {
	err := d.draw.Release(ctx)
	if err == nil {
		d.lose()
	}
	return d.check(err) //nolint:wrapcheck // the driver names the device
}

// Close closes the device. Closing twice is not an error.
func (d *Device) Close() error {
	if err := d.dev.Close(); err != nil {
		return fmt.Errorf("close %s: %w", d.Name(), err)
	}
	return nil
}

// Gone is closed when the device has gone away. The adapter is closed by then.
func (d *Device) Gone() <-chan struct{} { return d.gone }

/*
Present reports whether the device the handle was opened on is still there.

The question a frame answers with sanshoku.ErrGone, asked without sending one.
A device that holds its last frame is sent nothing for hours, and when it
reboots (OpenRGB's exit sends it the reboot command) nothing would notice
until the next scene. So the daemon asks this every few seconds.

The kernel names the device behind a hidraw node with a sequence number that
moves on every enumeration (0003:VVVV:PPPP.NNNN), so the same node path
coming back on a rebooted device still reads as a different device. A path
this cannot read when the device is opened, as a test's is, is always
present: the frame's ErrGone is then the only signal.
*/
func (d *Device) Present() bool {
	if d.node == "" {
		return true
	}
	return kernelNode(d.dev.Identity().Path) == d.node
}

// Node is the kernel's name for the device as it was when opened, or "" for
// a path Present cannot read.
func (d *Device) Node() string { return d.node }

/*
KernelNode is the kernel's name for the device behind a hidraw node now, or
"" for a path that is not one. A node whose name is a lost device's Node is
that device still being removed, not the device back.
*/
func KernelNode(path string) string { return kernelNode(path) }

// Lose closes the adapter as if the device had gone. The daemon calls it when
// Present says the device has.
func (d *Device) Lose() { d.lose() }

func (d *Device) lose() {
	d.once.Do(func() {
		_ = d.dev.Close()
		close(d.gone)
	})
}

// check closes the adapter, once, on a device that has gone away, so the
// daemon can look for it again rather than hold a dead handle.
func (d *Device) check(err error) error {
	if errors.Is(err, sanshoku.ErrGone) {
		d.lose()
	}
	return err
}

// kernelNode is the name of the HID device behind a hidraw node, or "" for a
// path that is not one this can read.
func kernelNode(path string) string {
	if !strings.HasPrefix(path, "/dev/hidraw") {
		return ""
	}
	target, err := os.Readlink(filepath.Join("/sys/class/hidraw", filepath.Base(path), "device"))
	if err != nil {
		return ""
	}
	return filepath.Base(target)
}

/*
Attached is one attachment: the open device and the animator that is its only
writer. What the daemon hands the service.
*/
type Attached struct {
	*Device
	*Animator
}
