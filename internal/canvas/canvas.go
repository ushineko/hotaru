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
	"github.com/ushineko/sanshoku/aula"
	"github.com/ushineko/sanshoku/battery"
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
	return []sanshoku.Driver{steelseries.Driver{}, aula.Driver{}}
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

	// onboard is the device's own firmware effect, where it has one that
	// keeps its choice and colour in storage (spec 071), and nil otherwise.
	onboard lighting.Onboard
	// hold is how long a frame shows before the device takes its lights
	// back, for a lighting.Fading canvas, and zero for one that keeps it.
	hold time.Duration

	mu sync.Mutex
	// firmware is the effect the device was last read or written to run.
	// See Firmware.
	firmware *lighting.Effect
	report   func(format string, args ...any)
	watch    Watch
}

/*
Watch is what the power-cycle watcher last learnt of a device with a stored
effect (spec 071 R6): whether it runs, whether the device answered its last
question and when that was, and when the watcher last put the effect back.
*/
type Watch struct {
	Watched   bool
	Answering bool
	Asked     time.Time
	PutBack   time.Time
}

// Watch is the watcher's notes.
func (d *Device) Watch() Watch {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.watch
}

// NoteAnswer records one question the watcher asked, and whether it was
// answered.
func (d *Device) NoteAnswer(answered bool, at time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.watch.Watched, d.watch.Answering, d.watch.Asked = true, answered, at
}

// NotePutBack records the watcher putting the device's effect back.
func (d *Device) NotePutBack(at time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.watch.PutBack = at
}

// New adapts an open device, and says whether it is a canvas at all. Open
// calls it; a test hands it a fake.
func New(dev sanshoku.Device) (*Device, bool) {
	draw, ok := dev.(lighting.Canvas)
	if !ok {
		return nil, false
	}
	out := &Device{
		dev: dev, draw: draw, keys: draw.Keys(), gone: make(chan struct{}),
		node: kernelNode(dev.Identity().Path),
	}
	if onboard, ok := dev.(lighting.Onboard); ok {
		out.onboard = onboard
	}
	if fading, ok := dev.(lighting.Fading); ok {
		out.hold = fading.Hold()
	}
	return out, true
}

// Hold is how long a frame shows before the device takes its lights back,
// or zero for a device that keeps its last frame.
func (d *Device) Hold() time.Duration { return d.hold }

// Effects is the firmware's own effects, in its order, or none for a device
// without them.
func (d *Device) Effects() []string {
	if d.onboard == nil {
		return nil
	}
	return d.onboard.Effects()
}

/*
Firmware is the effect the device's firmware runs, as last read from it or
written to it, and whether that is known.

A cache, so that a listing costs no radio traffic: reading the F75's two
blocks is about 200 ms on the air. ReadFirmware refreshes it, and the writes
below keep it as they leave the device.
*/
func (d *Device) Firmware() (lighting.Effect, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.firmware == nil {
		return lighting.Effect{}, false
	}
	return *d.firmware, true
}

// ReadFirmware reads the effect the firmware runs now, and keeps it.
func (d *Device) ReadFirmware(ctx context.Context) (lighting.Effect, error) {
	if d.onboard == nil {
		return lighting.Effect{}, fmt.Errorf("%s: %w", d.Name(), errNoFirmware)
	}
	effect, err := d.onboard.Effect(ctx)
	if err != nil {
		return lighting.Effect{}, d.check(err) //nolint:wrapcheck // the driver names the device
	}
	d.keep(effect)
	return effect, nil
}

/*
SetFirmware makes the firmware run an effect and, where it takes one, show a
colour, and reports what it wrote. The effect is set first: a power cycle
puts the F75 back on its factory effect, so the colour alone would land on
the wrong one (sanshoku spec 023 R2.8a).

sanshoku writes nothing where the stored value already matches, so a call
that changes nothing costs the two reads and no flash.
*/
func (d *Device) SetFirmware(ctx context.Context, effect string, c *lighting.Colour) (wrote bool, err error) {
	if d.onboard == nil {
		return false, fmt.Errorf("%s: %w", d.Name(), errNoFirmware)
	}
	wroteEffect, err := d.onboard.SetEffect(ctx, effect)
	if err != nil {
		return false, d.check(err) //nolint:wrapcheck // the driver names the device
	}
	wroteColour := false
	if c != nil {
		wroteColour, err = d.onboard.SetColour(ctx, *c)
		if err != nil && !errors.Is(err, lighting.ErrNoColour) {
			return wroteEffect, d.check(err) //nolint:wrapcheck // the driver names the device
		}
	}
	if _, err := d.ReadFirmware(ctx); err != nil {
		return wroteEffect || wroteColour, err
	}
	return wroteEffect || wroteColour, nil
}

func (d *Device) keep(effect lighting.Effect) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.firmware = &effect
}

// errNoFirmware is a firmware call to a device whose firmware keeps no
// effect of its own.
var errNoFirmware = errors.New("the device has no stored effect")

/*
Answers reports whether the device answered one short question, and whether
it can be asked one at all.

The battery question on the F75: one 20-byte report each way. A keyboard
that is off, asleep or out of range does not answer, which sanshoku reports
as no reading and no error. Any reply, even one it cannot decode, is an
answer. The watcher in the daemon asks this every ten seconds (spec 071 R6).
*/
func (d *Device) Answers(ctx context.Context) (answered, askable bool) {
	source, ok := d.dev.(battery.Source)
	if !ok {
		return false, false
	}
	found, err := source.Batteries(ctx)
	if ctx.Err() != nil {
		return false, true // stopped asking, which is not an answer
	}
	if errors.Is(err, sanshoku.ErrGone) {
		_ = d.check(err)
		return false, true
	}
	return len(found) > 0 || err != nil, true
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

// SetReport gives the device the service's log, for Say. Nil says nothing.
func (d *Device) SetReport(report func(format string, args ...any)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.report = report
}

// Say puts a line in the service's log, naming the device.
func (d *Device) Say(format string, args ...any) {
	d.mu.Lock()
	report := d.report
	d.mu.Unlock()
	if report != nil {
		report(d.Name()+": "+format, args...)
	}
}
