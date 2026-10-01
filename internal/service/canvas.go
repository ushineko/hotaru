package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/ushineko/hotaru/internal/canvas"
	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/config"
	"github.com/ushineko/hotaru/internal/devices"
	"github.com/ushineko/hotaru/internal/openrgb"
	"github.com/ushineko/hotaru/internal/render"
	"github.com/ushineko/sanshoku/lighting"
)

/*
Canvas is a device whose lights hotaru draws, as the service sees it: one
attachment and the animator that is its only writer (spec 060).

An interface for the same reason Cooler is one: the service does not open
devices, and a machine with none is the empty case rather than a special one.
*/
type Canvas interface {
	Name() string
	Path() string
	Keys() []lighting.Key
	// Show hands the animator what to draw. It does not wait.
	Show(canvas.Show)
	// Redraw sends what is showing again. It does not wait.
	Redraw()
	Status() canvas.Status
	// Release hands the lighting back to the device's firmware.
	Release(ctx context.Context) error
}

/*
AttachCanvas gives the service a canvas device to draw on.

From here until DetachCanvas the device is listed beside OpenRGB's devices,
and the OpenRGB device that is the same hardware is listed as handed to it
and is not written to.
*/
func (s *Service) AttachCanvas(c Canvas) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.canvases == nil {
		s.canvases = map[string]*drawn{}
	}
	s.canvases[c.Name()] = &drawn{Canvas: c}
}

// DetachCanvas takes a canvas device away, as the daemon does when it goes.
// The twin is OpenRGB's again from here.
func (s *Service) DetachCanvas(c Canvas) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if d, ok := s.canvases[c.Name()]; ok && d.Canvas == c {
		delete(s.canvases, c.Name())
	}
}

// drawnNow is every attached canvas, by name.
func (s *Service) drawnNow() []*drawn {
	out := make([]*drawn, 0, len(s.canvases))
	for _, d := range s.canvases {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}

// ErrNoCanvas is a release asked of a device hotaru does not draw.
var ErrNoCanvas = errors.New("no device hotaru draws matches that")

/*
ReleaseCanvas hands a canvas device's lighting back to its firmware, and
returns the device's name.

Never done on hotaru's own initiative (spec 060 R4.1): on the first canvas
device it reboots the board, which re-enumerates on a new node. What the
device was asked to show is forgotten first, so that the daemon attaches the
device again afterwards and sends it nothing until a scene asks.
*/
func (s *Service) ReleaseCanvas(ctx context.Context, name string) (string, error) {
	s.mu.RLock()
	all := s.drawnNow()
	s.mu.RUnlock()

	var matched []*drawn
	for _, d := range all {
		if named([]string{name}, d.Name()) {
			matched = append(matched, d)
		}
	}
	switch len(matched) {
	case 0:
		names := make([]string, 0, len(all))
		for _, d := range all {
			names = append(names, d.Name())
		}
		if len(names) == 0 {
			return "", fmt.Errorf("%w: hotaru is drawing on no device", ErrNoCanvas)
		}
		return "", fmt.Errorf("%w: it draws on %s", ErrNoCanvas, strings.Join(names, ", "))
	case 1:
	default:
		return "", fmt.Errorf("%q matches more than one device hotaru draws; give more of the name", name)
	}

	device := matched[0]
	s.forget(device.Name())
	if err := device.Release(ctx); err != nil {
		return device.Name(), fmt.Errorf("hand %s back to its firmware: %w", device.Name(), err)
	}
	return device.Name(), nil
}

/*
CanvasState is one canvas device's drawing, for health.

Effect is the renderer it was last asked for; with Drawing false the device
is holding the one frame it was sent.
*/
type CanvasState struct {
	Device string
	Effect string
	devices.Canvas
}

// Canvases is what every attached canvas device is drawing.
func (s *Service) Canvases() []CanvasState {
	s.mu.RLock()
	all := s.drawnNow()
	s.mu.RUnlock()

	out := make([]CanvasState, 0, len(all))
	for _, d := range all {
		status := d.Status()
		out = append(out, CanvasState{Device: d.Name(), Effect: status.Effect, Canvas: canvasOf(status)})
	}
	return out
}

func canvasOf(status canvas.Status) devices.Canvas {
	return devices.Canvas{
		Drawing: status.Drawing, Interval: status.Interval, Rate: status.Rate, Frames: status.Frames,
	}
}

/*
drawn is an attached canvas and what the service last asked of it.

The write path speaks to every device in two steps, a mode and then a frame
(writeFrame), and a canvas device takes them the same way. The mode is
chosen and nothing is drawn; the frame then draws the chosen effect with it.
That is what makes a static scene one frame on the device rather than two,
the old frame under the new mode and then the new one. An effect that takes
no frame (Off) is drawn when it is chosen, because no frame follows it.
*/
type drawn struct {
	Canvas

	mu sync.Mutex
	// chosen is a mode set and not yet drawn.
	chosen *choice
	// shown is what the animator was last handed, and colours the frame it
	// was drawn with.
	shown   choice
	colours []colour.Colour
}

// choice is a renderer and the style the write path set it with.
type choice struct {
	effect render.Renderer
	style  openrgb.Style
}

func (d *drawn) choose(mode string, style openrgb.Style, rule devices.Rule) error {
	effect, ok := render.Lookup(mode)
	if !ok {
		return fmt.Errorf("%s has no mode called %q", d.Name(), mode)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.chosen = &choice{effect: effect, style: style}
	if effect.Blank {
		d.showLocked(nil, rule)
	}
	return nil
}

func (d *drawn) draw(frame devices.Frame, rule devices.Rule) error {
	if want, got := len(d.Keys()), len(frame.Colours); want != got {
		return fmt.Errorf("%s has %d lights and the frame has %d", d.Name(), want, got)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.showLocked(frame.Colours, rule)
	return nil
}

// showLocked hands the animator the chosen effect, or the one already showing
// for a frame that arrives with no mode before it. Static where there is
// neither, because a frame on its own means "show these colours".
func (d *drawn) showLocked(colours []colour.Colour, rule devices.Rule) {
	pick := d.shown
	if d.chosen != nil {
		pick = *d.chosen
	}
	if pick.effect.Draw == nil {
		pick.effect, _ = render.Lookup(render.Static)
	}
	d.chosen, d.shown = nil, pick
	d.colours = append([]colour.Colour(nil), colours...)
	d.Show(canvas.Show{
		Effect: pick.effect,
		Params: render.Params{
			Colours: d.colours, Palette: pick.style.Palette(),
			Speed: pick.style.Speed, Brightness: pick.style.Brightness,
		},
		Interval: rule.FrameInterval,
	})
}

/*
device is the canvas as a devices.Device, so that scenes, targets, segments,
the wizard and every listing treat it as any other.

One zone, "Keys", with a light per key in the canvas's order. Its modes are
hotaru's renderers, carrying the flags a firmware mode would: Static takes a
colour per light, Breathing a colour of its own, and every one a brightness,
because a brightness is a multiply in the renderer. Each says how many
colours of its own it takes, as a firmware mode does through OpenRGB (spec
061): none needed, and as many as the renderer draws.
*/
func (d *drawn) device() devices.Device {
	keys := d.Keys()
	status := d.Status()

	d.mu.Lock()
	active := d.shown
	if d.chosen != nil {
		active = *d.chosen
	}
	colours := append([]colour.Colour(nil), d.colours...)
	d.mu.Unlock()

	out := devices.Device{
		Name:       d.Name(),
		Location:   d.Path(),
		ActiveMode: active.effect.Name,
		Zones:      []devices.Zone{{Name: "Keys", Shape: devices.ShapeGrid, First: 0, Count: len(keys)}},
		LEDCount:   len(keys),
		Colours:    colours,
	}
	for _, key := range keys {
		out.LEDNames = append(out.LEDNames, key.Name)
	}
	if len(out.Colours) != len(keys) {
		out.Colours = nil // nothing drawn yet: not saying, rather than black
	}
	canvased := canvasOf(status)
	out.Canvas = &canvased

	for _, r := range render.Renderers() {
		mode := devices.Mode{
			Name: r.Name, PerLED: r.PerLight, Brightness: true, ModeColour: r.OneColour, ColoursMax: r.Colours,
		}
		current := r.Name == active.effect.Name
		if palette := active.style.Palette(); r.Colours > 0 && current && len(palette) > 0 {
			mode.Colours = palette[:min(len(palette), r.Colours)]
			mode.Colour = mode.Colours[0]
		}
		if r.Paced {
			now := render.DefaultSpeed
			if current && active.style.Speed != nil {
				now = *active.style.Speed
			}
			mode.Speed = &devices.Speed{Slowest: render.SlowestSpeed, Fastest: render.FastestSpeed, Now: now}
		}
		out.Modes = append(out.Modes, mode)
	}
	return out
}

/*
lights is the OpenRGB server with the attached canvas devices beside it: the
openrgb.Client every operation in this package already speaks.

A canvas device is listed, read back and written through the same calls as an
OpenRGB device, so applying, reconciling, previewing and probing reach it with
no second code path. The OpenRGB device that is the same hardware is marked
HandedTo and refuses every write here, whatever asked for it.
*/
type lights struct {
	openrgb.Client
	cfg   *config.Config
	drawn []*drawn

	mu sync.Mutex
	// handed is OpenRGB's device names mapped to the canvas each is handed
	// to, from the last listing.
	handed map[string]string
}

// Devices is OpenRGB's devices, twins marked, and then the canvas devices.
func (l *lights) Devices(ctx context.Context) ([]devices.Device, error) {
	found, err := l.Client.Devices(ctx)
	if err != nil {
		return nil, err //nolint:wrapcheck // the client says which server
	}
	handed := map[string]string{}
	for i := range found {
		if to := twinOf(found[i], l.drawn, l.cfg); to != "" {
			found[i].HandedTo = to
			handed[found[i].Name] = to
		}
	}
	l.mu.Lock()
	l.handed = handed
	l.mu.Unlock()

	for _, d := range l.drawn {
		found = append(found, d.device())
	}
	return found, nil
}

// Device is one device by name, a canvas device's own snapshot for one.
func (l *lights) Device(ctx context.Context, name string) (devices.Device, error) {
	if d := l.canvas(name); d != nil {
		return d.device(), nil
	}
	found, err := l.Client.Device(ctx, name)
	if err != nil {
		return found, err //nolint:wrapcheck // the client says which server
	}
	found.HandedTo = twinOf(found, l.drawn, l.cfg)
	return found, nil
}

// SetMode chooses a canvas device's renderer, refuses a twin, and passes
// anything else to OpenRGB.
func (l *lights) SetMode(ctx context.Context, device, mode string, style openrgb.Style) error {
	if d := l.canvas(device); d != nil {
		return d.choose(mode, style, devices.RuleFor(l.cfg, d.Name()))
	}
	if err := l.refuse(ctx, device); err != nil {
		return err
	}
	return l.Client.SetMode(ctx, device, mode, style) //nolint:wrapcheck // the client says which server
}

// SetFrame draws a frame on a canvas device, refuses a twin, and passes
// anything else to OpenRGB.
func (l *lights) SetFrame(ctx context.Context, device string, frame devices.Frame) error {
	if d := l.canvas(device); d != nil {
		return d.draw(frame, devices.RuleFor(l.cfg, d.Name()))
	}
	if err := l.refuse(ctx, device); err != nil {
		return err
	}
	return l.Client.SetFrame(ctx, device, frame) //nolint:wrapcheck // the client says which server
}

func (l *lights) canvas(name string) *drawn {
	for _, d := range l.drawn {
		if strings.EqualFold(d.Name(), name) {
			return d
		}
	}
	return nil
}

/*
refuse is the twin rule at the last place it can be kept.

Apply, reconcile and probe skip a twin before writing, and say why. This is
for anything that does not, so that no path reaches OpenRGB with a write for
hardware hotaru is drawing on: two streams to one keyboard alternate frames.
*/
func (l *lights) refuse(ctx context.Context, device string) error {
	l.mu.Lock()
	handed := l.handed
	l.mu.Unlock()
	if handed == nil {
		if _, err := l.Devices(ctx); err != nil {
			return err
		}
		l.mu.Lock()
		handed = l.handed
		l.mu.Unlock()
	}
	if to, ok := handed[device]; ok {
		return &Handed{Device: device, To: to}
	}
	return nil
}

// Handed is a write refused because the device's lights are drawn by hotaru
// as another device.
type Handed struct{ Device, To string }

func (e *Handed) Error() string { return handedReason(e.Device, e.To) }

func handedReason(device, to string) string {
	return fmt.Sprintf("%s is drawn by hotaru as %s, so OpenRGB is not written to", device, to)
}

/*
twinOf is the canvas device an OpenRGB device is the same hardware as, or "".

A rule on the canvas device that names its twin decides alone. Otherwise the
twin is the OpenRGB device whose location is the canvas's own hidraw node:
OpenRGB writes a HID device's location as "HID: /dev/hidrawN". The paths are
compared whole, so /dev/hidraw1 does not match /dev/hidraw10.

The location is what OpenRGB found when it started, and it is not updated
when a device re-enumerates on another node (measured on 2026-10-01: OpenRGB
listed the keyboard at /dev/hidraw5 after it had moved to /dev/hidraw4). A
twin that will not match is named in a rule; spec 060 records the reading.
*/
func twinOf(device devices.Device, all []*drawn, cfg *config.Config) string {
	for _, d := range all {
		if rule := devices.RuleFor(cfg, d.Name()); rule.Twin != "" {
			if strings.Contains(strings.ToLower(device.Name), strings.ToLower(rule.Twin)) {
				return d.Name()
			}
			continue
		}
		where := strings.TrimSpace(strings.TrimPrefix(device.Location, "HID:"))
		if where != "" && where == d.Path() {
			return d.Name()
		}
	}
	return ""
}

/*
handedByRule reports whether a name is the OpenRGB twin of an attached canvas
device by a rule's twin key. It is how reconcile recognises a twin that
OpenRGB is not listing, when there is no location to match.
*/
func (s *Service) handedByRule(name string) bool {
	s.mu.RLock()
	all := s.drawnNow()
	s.mu.RUnlock()
	return twinOf(devices.Device{Name: name}, all, s.config()) != ""
}
