package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ushineko/hotaru/internal/canvas"
	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/config"
	"github.com/ushineko/hotaru/internal/devices"
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
	// Borrowed is a device that stays attached when released: the
	// cooler's, which another owner holds (spec 073).
	Borrowed() bool
	// Hold is how long a frame lasts on a device whose frames fade, or zero.
	Hold() time.Duration
	// Zones is the device's own zones, where it names them, or nil (spec
	// 075).
	Zones() []lighting.Zone
	// Resize gives a zone whose length the device cannot read its lights.
	Resize(zone string, lights int) error

	// Effects is the firmware's own stored effects, or none (spec 071).
	Effects() []string
	// Catalogue describes each effect's colours and speeds, where the
	// device does (spec 077).
	Catalogue() []lighting.EffectInfo
	// Stores is whether an effect write wears the device's storage.
	Stores() bool
	// Firmware is the effect the firmware was last read or written to run.
	Firmware() (lighting.Effect, bool)
	// ReadFirmware reads it from the device.
	ReadFirmware(ctx context.Context) (lighting.Effect, error)
	// SetFirmware writes an effect, its colours and speed to the device,
	// where they differ. Nil colours and speed keep what it has.
	SetFirmware(ctx context.Context, effect string, colours []lighting.Colour, speed *int, restart bool) (bool, error)
	// Say puts a line in the service's log, naming the device.
	Say(format string, args ...any)
	// Watch is the power-cycle watcher's notes (spec 071 R6).
	Watch() canvas.Watch
}

/*
AttachCanvas gives the service a canvas device to draw on.

From here until DetachCanvas the device is listed, written and restored, and a
name a rule says it was formerly called is listed as handed to it (spec 073).
*/
func (s *Service) AttachCanvas(c Canvas) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.canvases == nil {
		s.canvases = map[string]*drawn{}
	}
	// Nothing is drawn yet, so a device with an effect of its own shows it.
	s.canvases[c.Name()] = &drawn{Canvas: c, stored: len(c.Effects()) > 0}
}

// DetachCanvas takes a canvas device away, as the daemon does when it goes.
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
device again afterwards and sends it nothing until a scene asks. The cooler's
lights keep their frame and stay attached (spec 073), and their record under
a former name is forgotten too.
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
	if former := devices.RuleFor(s.config(), device.Name()).Formerly; former != nil {
		// Or the next full reconcile adopts it and undoes the release.
		s.forget(former.Name)
	}
	if device.Borrowed() {
		/*
			It stays attached, so its animator would go on drawing over
			the release. It is stopped first, and the frame the device
			commits is the last one it was sent.
		*/
		device.Show(canvas.Show{})
		settle(ctx, device)
	}
	if err := device.Release(ctx); err != nil {
		return device.Name(), fmt.Errorf("hand %s back to its firmware: %w", device.Name(), err)
	}
	return device.Name(), nil
}

/*
formerly is the OpenRGB listing a canvas's hardware had before OpenRGB stopped
listing it, standing in for it as the canvas's twin (spec 073): the former
name, the canvas's lights and their names, and its zones under their former
names. Handed to the canvas, so it is never written; scenes against it are
carried onto the canvas as spec 060 carries a listed twin's.
*/
func formerly(canvas devices.Device, former config.Former) devices.Device {
	back := make(map[string]string, len(former.Zones))
	for from, to := range former.Zones {
		back[to] = from
	}
	twin := canvas
	twin.Name, twin.Location, twin.HandedTo, twin.Canvas = former.Name, "", canvas.Name, nil
	twin.Zones = make([]devices.Zone, len(canvas.Zones))
	for i, zone := range canvas.Zones {
		if from, ok := back[zone.Name]; ok {
			zone.Name = from
		}
		twin.Zones[i] = zone
	}
	return twin
}

/*
zones is a canvas's zones: the device's own where it names them (spec 075),
as OpenRGB named them, so scenes and segments written against OpenRGB find
them; else spec 073's, from the key names.
*/
func (d *drawn) zones(keys []lighting.Key) []devices.Zone {
	own := d.Zones()
	if len(own) == 0 {
		return keyZones(keys)
	}
	out := make([]devices.Zone, 0, len(own))
	for _, z := range own {
		zone := devices.Zone{
			Name: z.Name, Shape: shapeOf(z), First: z.First, Count: z.Count,
			Parts: append([]int(nil), z.Parts...),
		}
		if z.Resizable() {
			zone.Min, zone.Max = z.Min, z.Max
		}
		out = append(out, zone)
	}
	return out
}

/*
thingsOf is the device's fans, rings and strips, for the effects that draw
per thing or along a zone's chain of them (spec 077 R3.4). A zone is split as
its device counts its parts (the Kraken's fans), else as the rules' segments
tile it (the Aura's side fans, as light map wrote them), else it is one
thing. It is a ring where the device or a rule's shapes says so.
*/
func thingsOf(zones []devices.Zone, rule devices.Rule) []render.Thing {
	var out []render.Thing
	for z, zone := range zones {
		if zone.Count == 0 {
			continue
		}
		ring := zone.Shape == devices.ShapeRing || rule.Shapes[zone.Name] == "ring"
		parts := zone.Parts
		if len(parts) < 2 {
			parts = tiles(zone, rule)
		}
		if len(parts) == 0 {
			parts = []int{zone.Count}
		}
		first := zone.First
		for _, n := range parts {
			lights := make([]int, n)
			for i := range lights {
				lights[i] = first + i
			}
			out = append(out, render.Thing{Lights: lights, Zone: z, Ring: ring})
			first += n
		}
	}
	return out
}

// tiles is how the rules' segments divide a zone, each segment's size in
// order, where they cover it end to end with no gap; else none.
func tiles(zone devices.Zone, rule devices.Rule) []int {
	var spans [][2]int
	for _, seg := range rule.Segments {
		if !strings.EqualFold(seg.Zone, zone.Name) || seg.LEDs == nil {
			continue
		}
		spans = append(spans, [2]int{seg.LEDs.First, seg.LEDs.Last})
	}
	slices.SortFunc(spans, func(a, b [2]int) int { return a[0] - b[0] })
	var sizes []int
	next := 0
	for _, sp := range spans {
		if sp[0] != next || sp[1] < sp[0] {
			return nil
		}
		sizes = append(sizes, sp[1]-sp[0]+1)
		next = sp[1] + 1
	}
	if next != zone.Count {
		return nil
	}
	return sizes
}

/*
shapeOf is a zone's shape as the driver gives it (sanshoku spec 032). One the
driver does not know, an addressable header's, is a line until somebody says
otherwise: it may be a strip or fans in a chain, and only a line or a ring is
asked about chaining.
*/
func shapeOf(z lighting.Zone) devices.Shape {
	switch z.Shape {
	case lighting.ShapeSingle:
		return devices.ShapeSingle
	case lighting.ShapeRing:
		return devices.ShapeRing
	case lighting.ShapeGrid:
		return devices.ShapeGrid
	case lighting.ShapeSet:
		return devices.ShapeSet
	case lighting.ShapeLine:
		return devices.ShapeLine
	}
	if z.Count == 1 && !z.Resizable() {
		return devices.ShapeSingle
	}
	return devices.ShapeLine
}

/*
keyZones is a canvas's zones. Where every key is named a word and a number
("ring 3", "fan 12"), as the cooler's are, each word is a zone in key order
(Ring, Fan), so a scene can address the ring as OpenRGB's listing could
(spec 073). Any other canvas is one grid of keys.
*/
func keyZones(keys []lighting.Key) []devices.Zone {
	whole := []devices.Zone{{Name: "Keys", Shape: devices.ShapeGrid, First: 0, Count: len(keys)}}
	var zones []devices.Zone
	for i, key := range keys {
		word, number, ok := strings.Cut(key.Name, " ")
		if !ok || word == "" {
			return whole
		}
		if _, err := strconv.Atoi(number); err != nil {
			return whole
		}
		name := strings.ToUpper(word[:1]) + word[1:]
		if len(zones) > 0 && zones[len(zones)-1].Name == name {
			zones[len(zones)-1].Count++
			continue
		}
		zones = append(zones, devices.Zone{Name: name, Shape: devices.ShapeLine, First: i, Count: 1})
	}
	if len(zones) == 0 {
		return whole
	}
	return zones
}

// Stays is whether a released canvas stays attached: the cooler's lights,
// which do not reboot when handed back (spec 073).
func (s *Service) Stays(name string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, d := range s.drawnNow() {
		if d.Name() == name {
			return d.Borrowed()
		}
	}
	return false
}

// Lengths are the zone lengths the rules give a device (spec 075).
func (s *Service) Lengths(name string) map[string]int {
	return devices.RuleFor(s.config(), name).Lengths
}

// settleWait bounds the wait for an animator to stop drawing.
const settleWait = time.Second

// settle waits, briefly, for a canvas's animator to stop drawing.
func settle(ctx context.Context, device Canvas) {
	until := time.Now().Add(settleWait)
	for device.Status().Drawing && time.Now().Before(until) && ctx.Err() == nil {
		time.Sleep(10 * time.Millisecond)
	}
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
		state := CanvasState{Device: d.Name(), Effect: d.Status().Effect, Canvas: d.canvasOf()}
		if state.Own {
			if firmware, known := d.Firmware(); known {
				state.Effect = firmware.Name
			}
		}
		out = append(out, state)
	}
	return out
}

func (d *drawn) canvasOf() devices.Canvas {
	status := d.Status()
	d.mu.Lock()
	own := d.stored
	d.mu.Unlock()
	out := devices.Canvas{
		Drawing: status.Drawing, Interval: status.Interval, Rate: status.Rate, Frames: status.Frames, Own: own,
		Fades: d.Hold() > 0,
	}
	if watch := d.Watch(); watch.Watched {
		out.Watch = &devices.Watch{Answering: watch.Answering, Asked: watch.Asked, PutBack: watch.PutBack}
	}
	return out
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

	// stored is the firmware's own effect being what the device shows, with
	// nothing drawn over it (spec 071 R2.3).
	stored bool
	// flashed is when the device's storage was last written. See flashGap.
	flashed time.Time
}

// choice is a renderer and the style the write path set it with.
type choice struct {
	effect render.Renderer
	style  devices.Style
}

func (d *drawn) choose(ctx context.Context, mode string, style devices.Style, rule devices.Rule) error {
	if name := d.storedName(mode); name != "" {
		return d.chooseStored(ctx, name, style)
	}
	effect, ok := render.Lookup(mode)
	if !ok {
		return fmt.Errorf("%s has no mode called %q", d.Name(), mode)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.stored = false
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
	if d.stored && d.chosen == nil {
		/*
			The firmware's effect is showing, and the frame behind its mode
			is the colours it shows, kept for a listing. Drawing it would
			cover the effect somebody chose (spec 071 R2.4).
		*/
		d.colours = append([]colour.Colour(nil), frame.Colours...)
		return nil
	}
	d.showLocked(frame.Colours, rule)
	return nil
}

// storedName is the firmware's own spelling of a mode, where the mode is one
// of the firmware's stored effects, and "" otherwise.
func (d *drawn) storedName(mode string) string {
	for _, name := range d.Effects() {
		if strings.EqualFold(name, mode) {
			return name
		}
	}
	return ""
}

/*
flashGap is the least time between two writes to a device's storage (spec
071 R5.3). A write inside it waits for its end. The queue keeps one write
behind it per device, so a burst of scenes writes the storage once per gap
and then the last of them.
*/
const flashGap = 2 * time.Second

// flashNow and flashWait are the time a firmware write is measured against,
// and how it waits. Seams for a test.
var (
	flashNow  = time.Now
	flashWait = pause
)

/*
chooseStored makes the firmware's own effect what the device shows.

Drawing stops first, so that the keyboard takes its lights back within its
hold (sanshoku spec 019 E4). Then the effect and its colour are written to
the device's storage, where they differ from what was last read or written,
and only on an apply a person made (spec 071 R5.1). A reconcile that finds
the keyboard on something else leaves it there, because a person may have
chosen it with the keyboard's own keys (R6.3). A preview draws the colour
instead, and skips a different effect (R5.2).
*/
func (d *drawn) chooseStored(ctx context.Context, name string, style devices.Style) error {
	var colours []lighting.Colour
	for _, c := range style.Palette() {
		colours = append(colours, lighting.Colour{R: c.R, G: c.G, B: c.B})
	}
	for _, info := range d.Catalogue() {
		if strings.EqualFold(info.Name, name) && len(colours) > info.MaxColours {
			// As many as the effect takes, none for a rainbow: more is
			// refused by the driver, and the effect lost for colours it
			// would not have shown (spec 077 E4, Rainbow Flow).
			colours = colours[:info.MaxColours]
		}
	}
	var want *lighting.Colour
	if len(colours) > 0 {
		want = &colours[0]
	}
	now, known := d.Firmware()
	sameEffect := known && strings.EqualFold(now.Name, name)
	if !style.Restart && sameEffect && showsColours(now, colours) && (style.Speed == nil || (now.Speed != nil && *now.Speed == *style.Speed)) {
		d.showStored()
		return nil
	}

	switch writing(ctx) {
	case forPreview, forReconcile:
		if !d.Stores() {
			// Held in memory: putting it back, or showing it for a lease
			// that puts the record back after, costs nothing (spec 077
			// R2.3).
			break
		}
		if writing(ctx) == forReconcile {
			return &KeepsStored{Device: d.Name(), Effect: name, Why: "hotaru writes the device's storage only when a person asks"}
		}
		if !sameEffect {
			return &KeepsStored{Device: d.Name(), Effect: name, Why: "a preview does not write the device's storage"}
		}
		// The colour is drawn over the effect for the length of the lease.
		static, _ := render.Lookup(render.Static)
		d.mu.Lock()
		defer d.mu.Unlock()
		d.stored = false
		d.chosen = &choice{effect: static, style: style}
		return nil
	case forApply:
	}

	d.showStored()
	if d.Stores() {
		d.mu.Lock()
		wait := d.flashed.Add(flashGap).Sub(flashNow())
		d.mu.Unlock()
		if wait > 0 && !flashWait(ctx, wait) {
			return ctx.Err() //nolint:wrapcheck // the caller's own cancellation
		}
	}
	wrote, err := d.SetFirmware(ctx, name, colours, style.Speed, style.Restart)
	if wrote && d.Stores() {
		d.mu.Lock()
		d.flashed = flashNow()
		d.mu.Unlock()
		d.Say("wrote the effect %q%s to its storage", name, colourWords(want))
	}
	if err != nil {
		return fmt.Errorf("%s's own effect %q: %w", d.Name(), name, err)
	}
	return nil
}

// showStored stops drawing, so the firmware's effect shows.
func (d *drawn) showStored() {
	d.mu.Lock()
	d.stored, d.chosen, d.shown = true, nil, choice{}
	d.mu.Unlock()
	d.Show(canvas.Show{})
}

func colourWords(c *lighting.Colour) string {
	if c == nil {
		return ""
	}
	return fmt.Sprintf(" in %s", colour.Colour{R: c.R, G: c.G, B: c.B})
}

/*
KeepsStored is a write to a device's own stored effect that was not made,
and why: a reconcile or a preview, which never write the device's storage.
Reported as the device skipped, never as a fall-through to drawing, which
would cover the effect the keyboard is showing.
*/
type KeepsStored struct{ Device, Effect, Why string }

func (e *KeepsStored) Error() string {
	return fmt.Sprintf("%s keeps the effect it shows rather than %q: %s", e.Device, e.Effect, e.Why)
}

// purpose is why a write is being made, which decides whether it may write
// a device's storage.
type purpose int

const (
	forReconcile purpose = iota // the default: putting back, which never writes storage
	forApply                    // a person asked: light set, off, a scene, a key, a rule
	forPreview                  // a draft held for a while, which never writes storage
)

type purposeKey struct{}

// withPurpose marks a context with why its writes are made.
func withPurpose(ctx context.Context, p purpose) context.Context {
	return context.WithValue(ctx, purposeKey{}, p)
}

func writing(ctx context.Context) purpose {
	p, _ := ctx.Value(purposeKey{}).(purpose)
	return p
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
			Things: thingsOf(d.zones(d.Keys()), rule),
			Pace:   paceOr(pick.style.Pace, rule.Pace[strings.ToLower(pick.effect.Name)]),
		},
		Interval: rule.FrameInterval,
		Restart:  pick.style.Restart,
	})
}

// paceOr is a write's own trim, or the rule's where it names none.
func paceOr(own, rule float64) float64 {
	if own > 0 {
		return own
	}
	return rule
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

	d.mu.Lock()
	active := d.shown
	if d.chosen != nil {
		active = *d.chosen
	}
	colours := append([]colour.Colour(nil), d.colours...)
	stored := d.stored
	d.mu.Unlock()
	firmware, known := d.Firmware()

	out := devices.Device{
		Name:       d.Name(),
		Location:   d.Path(),
		ActiveMode: active.effect.Name,
		Zones:      d.zones(keys),
		LEDCount:   len(keys),
		Colours:    colours,
	}
	for _, key := range keys {
		out.LEDNames = append(out.LEDNames, key.Name)
	}
	if len(out.Colours) != len(keys) {
		out.Colours = nil // nothing drawn yet: not saying, rather than black
	}
	canvased := d.canvasOf()
	out.Canvas = &canvased

	out.Modes = d.storedModes(firmware, known)
	if stored && known {
		out.ActiveMode = firmware.Name
		out.Colours = firmwareShows(firmware, len(keys))
	}
	for _, r := range render.Renderers() {
		if d.storedName(r.Name) != "" {
			continue // the firmware's own takes the name (spec 071 R2.1)
		}
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
showsColours is whether a running effect shows the colours asked for: all of
them where it says what they are, its first where it says only that. No
colours asked for, or an effect without colours, is always so.
*/
func showsColours(now lighting.Effect, want []lighting.Colour) bool {
	switch {
	case len(want) == 0 || !now.Coloured:
		return true
	case len(now.Colours) > 0:
		return slices.Equal(now.Colours, want)
	default:
		return now.Colour != nil && *now.Colour == want[0]
	}
}

/*
storedModes is the firmware's effects as modes, marked Stored, in the
firmware's order. Where the device has a catalogue, each says how many
colours it takes and the speeds it runs at (spec 077 R2.1); otherwise a
coloured effect takes one. The running one says its colours and speed, where
they were read.
*/
func (d *drawn) storedModes(firmware lighting.Effect, known bool) []devices.Mode {
	catalogue := map[string]lighting.EffectInfo{}
	for _, info := range d.Catalogue() {
		catalogue[strings.ToLower(info.Name)] = info
	}
	var out []devices.Mode
	for _, name := range d.Effects() {
		mode := devices.Mode{Name: name, Stored: true, WritesStorage: d.Stores()}
		running := known && strings.EqualFold(firmware.Name, name)
		info, described := catalogue[strings.ToLower(name)]
		switch {
		case described:
			mode.ModeColour, mode.ColoursMin, mode.ColoursMax = info.MaxColours > 0, info.MinColours, info.MaxColours
			if len(info.Speeds) > 1 {
				now := info.Speed
				if running && firmware.Speed != nil {
					now = *firmware.Speed
				}
				mode.Speed = &devices.Speed{Slowest: info.Speeds[0], Fastest: info.Speeds[len(info.Speeds)-1], Now: now}
			}
		case !colourlessEffect(name):
			mode.ModeColour, mode.ColoursMin, mode.ColoursMax = true, 1, 1
		}
		if running {
			for _, c := range firmware.Colours {
				mode.Colours = append(mode.Colours, colour.Colour{R: c.R, G: c.G, B: c.B})
			}
			if len(mode.Colours) == 0 && firmware.Colour != nil {
				mode.Colours = []colour.Colour{{R: firmware.Colour.R, G: firmware.Colour.G, B: firmware.Colour.B}}
			}
			if len(mode.Colours) > 0 {
				mode.Colour = mode.Colours[0]
			}
		}
		out = append(out, mode)
	}
	return out
}

/*
colourlessEffect is a firmware effect that takes no colour: off, and the
keyboard's own per-key colours. The firmware says so only for the running
effect (lighting.Effect.Coloured), and a listing needs it for every one.
*/
func colourlessEffect(name string) bool {
	return strings.EqualFold(name, "off") || strings.EqualFold(name, "self-define")
}

// firmwareShows is what the keys show under a firmware effect, where that is
// one colour: its colour, or black for off. Nil where it shows several.
func firmwareShows(firmware lighting.Effect, n int) []colour.Colour {
	var c colour.Colour
	switch {
	case !firmware.Coloured && strings.EqualFold(firmware.Name, "off"):
	case firmware.Colour != nil:
		c = colour.Colour{R: firmware.Colour.R, G: firmware.Colour.G, B: firmware.Colour.B}
	default:
		return nil
	}
	out := make([]colour.Colour, n)
	for i := range out {
		out[i] = c
	}
	return out
}

/*
lights is the devices hotaru draws, as every operation in this package speaks
to them (spec 076).

A device a rule says was formerly called something else is listed under that
name too, handed to the canvas, so a record or scene made under the old name
says where it went (spec 073); a write to it is refused with that reason.
*/
type lights struct {
	cfg   *config.Config
	drawn []*drawn
}

var _ machine = (*lights)(nil)

// Devices is the canvas devices, then the names they were formerly called.
func (l *lights) Devices(context.Context) ([]devices.Device, error) {
	found := make([]devices.Device, 0, len(l.drawn))
	listed := make(map[string]bool, len(l.drawn))
	for _, d := range l.drawn {
		found = append(found, d.device())
		listed[strings.ToLower(d.Name())] = true
	}
	for _, d := range l.drawn {
		former := devices.RuleFor(l.cfg, d.Name()).Formerly
		if former == nil || listed[strings.ToLower(former.Name)] {
			continue
		}
		listed[strings.ToLower(former.Name)] = true
		found = append(found, formerly(d.device(), *former))
	}
	return found, nil
}

// Device is one device by name.
func (l *lights) Device(ctx context.Context, name string) (devices.Device, error) {
	if d := l.canvas(name); d != nil {
		return d.device(), nil
	}
	found, _ := l.Devices(ctx)
	for _, device := range found {
		if strings.EqualFold(device.Name, name) {
			return device, nil
		}
	}
	return devices.Device{}, &NoDevice{Name: name}
}

// SetMode chooses a canvas device's renderer.
func (l *lights) SetMode(ctx context.Context, device, mode string, style devices.Style) error {
	if d := l.canvas(device); d != nil {
		return d.choose(ctx, mode, style, devices.RuleFor(l.cfg, d.Name()))
	}
	return l.refuse(device)
}

// SetFrame draws a frame on a canvas device.
func (l *lights) SetFrame(_ context.Context, device string, frame devices.Frame) error {
	if d := l.canvas(device); d != nil {
		return d.draw(frame, devices.RuleFor(l.cfg, d.Name()))
	}
	return l.refuse(device)
}

func (l *lights) canvas(name string) *drawn {
	for _, d := range l.drawn {
		if strings.EqualFold(d.Name(), name) {
			return d
		}
	}
	return nil
}

// refuse is the error for a write to a device that is not drawn: handed, for
// a name a canvas was formerly called, and otherwise not there.
func (l *lights) refuse(device string) error {
	for _, d := range l.drawn {
		if former := devices.RuleFor(l.cfg, d.Name()).Formerly; former != nil && strings.EqualFold(former.Name, device) {
			return &Handed{Device: device, To: d.Name()}
		}
	}
	return &NoDevice{Name: device}
}

/*
SetLength gives a zone of a device hotaru draws a length, now and until the
service stops: how light map finds a header's length by trying lengths
(spec 077 R4.3). Not recorded; the rules' lengths are what a restart uses.
*/
func (s *Service) SetLength(_ context.Context, device, zone string, lights int) error {
	s.mu.RLock()
	all := s.drawnNow()
	s.mu.RUnlock()
	for _, d := range all {
		if !strings.EqualFold(d.Name(), device) {
			continue
		}
		if err := d.Resize(zone, lights); err != nil {
			return &BadLength{Device: device, Zone: zone, Lights: lights, Why: err}
		}
		return nil
	}
	return &NoDevice{Name: device}
}

// BadLength is a length a device refused for a zone.
type BadLength struct {
	Device, Zone string
	Lights       int
	Why          error
}

func (e *BadLength) Error() string {
	return fmt.Sprintf("%s's %s cannot have %d lights: %v", e.Device, e.Zone, e.Lights, e.Why)
}

func (e *BadLength) Unwrap() error { return e.Why }

// Handed is a write refused because the device's lights are drawn by hotaru
// under another name.
type Handed struct{ Device, To string }

func (e *Handed) Error() string { return handedReason(e.Device, e.To) }

func handedReason(device, to string) string {
	return fmt.Sprintf("%s is drawn by hotaru as %s", device, to)
}

// NoDevice is a write or read for a device hotaru does not draw.
type NoDevice struct{ Name string }

func (e *NoDevice) Error() string { return fmt.Sprintf("no device called %q", e.Name) }

/*
PutBack writes a device's recorded stored effect and colour back to its
storage, where they differ from what it shows: the re-assert after a power
cycle (spec 071 R6.2), and the one reconcile that may write the storage.
*/
func (s *Service) PutBack(ctx context.Context, name string) (Restore, error) {
	return s.Reconcile(withPurpose(ctx, forApply), []string{name})
}
