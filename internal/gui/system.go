package gui

import (
	"fmt"
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/shell"
	"github.com/ushineko/fynedesygn/widgets"
	"github.com/ushineko/hotaru/internal/api"
	"github.com/ushineko/hotaru/internal/colour"
)

/*
SystemSection draws the machine's lighting.

The distance this travels is the whole reason the window exists. The program
hotaru replaces could express a machine's lighting as a context menu of device
names; a menu says "NZXT Kraken 2024 ELITE Series RGB", and this says: two
zones, the ring is twenty-four lights and the fans twenty-four more, both in
scope, in Direct, showing this purple right now.

Every fact in that sentence is already on the wire. The value here is entirely
in being a picture rather than a table.
*/
type SystemSection struct {
	app *App
	// loaded and cooling are the cards that follow the machine, held so
	// Tick can refill them without the section being rebuilt.
	loaded, cooling *fyne.Container
}

// OpenSystem gives a section its app, which the shell normally does. For
// tests, like OpenEditor.
func OpenSystem(s *SystemSection, app *App) { s.app = app }

// Title is the name in the navigation.
func (s *SystemSection) Title() string { return "System" }

// Icon is the navigation's icon for this section.
func (s *SystemSection) Icon() fyne.Resource { return theme.StorageIcon() }

// Changed says this section draws the devices: their colours, their modes and
// who is holding a draft on them. The cooler's numbers are not its business.
func (s *SystemSection) Changed(before, after Snapshot) bool {
	if len(before.Devices) != len(after.Devices) {
		return true
	}
	for i := range after.Devices {
		if !sameDevice(before.Devices[i], after.Devices[i]) {
			return true
		}
	}
	return (before.Err == nil) != (after.Err == nil)
}

// Build draws the section from the last snapshot. Stateless, as the shell
// wants: every change rebuilds it, so only this has to know every reason
// something is or is not shown.
/*
loaded is what the machine is currently showing: the scene somebody applied
and what is on the cooler's panel.

Said as "last applied" rather than "this is the scene", because that is what
it is. The service remembers the last thing it was asked for; something that
changed the lights by another route -- a `hotaru light set`, another program,
a device that woke up wrong -- leaves the label saying what it said. Claiming
more than that would be the kind of plausible-looking screen this program has
been caught behind before.
*/
func loaded(got Snapshot) fyne.CanvasObject {
	scene := got.Status.Scene
	if scene == "" {
		scene = "nothing yet"
	}
	showing := got.Status.Showing
	if showing == "" {
		showing = "whatever it was showing"
	}

	return widgets.Card("Loaded",
		widgets.PlainRow("Scene", scene),
		widgets.PlainRow("Screen", showing),
	)
}

/*
Tick refills the cards that follow the machine, leaving the rest alone.

The section is rebuilt when a device appears or goes away. Everything else it
draws that moves -- the coolant, the pump, the scene somebody just applied --
arrives here.
*/
func (s *SystemSection) Tick(got Snapshot) {
	if s.loaded != nil {
		s.loaded.Objects = []fyne.CanvasObject{loaded(got)}
		s.loaded.Refresh()
	}
	if s.cooling != nil {
		s.cooling.Objects = []fyne.CanvasObject{coolingCard(got)}
		s.cooling.Refresh()
	}
}

// Build draws what is loaded, the cooler, and every device this machine has.
func (s *SystemSection) Build(_ *shell.Shell) fyne.CanvasObject {
	got := s.app.machine.Read()
	if got.Err != nil {
		return notRunning(got.Err)
	}
	if len(got.Devices) == 0 {
		return container.NewVBox(
			title("No devices"),
			widgets.Note("The Service section says why.", fd.StatusWarn),
		)
	}

	/*
		The two cards that move are held in slots and refilled, not rebuilt
		with the section.

		Cooling changes every poll and the device list does not, so a
		Changed that watched the coolant would rebuild every device row twice
		a minute -- which is the churn this window spent a day removing. The
		slots are refilled by Tick instead: six widgets rather than sixty.
	*/
	s.loaded = container.NewStack(loaded(got))
	s.cooling = container.NewStack(coolingCard(got))

	body := []fyne.CanvasObject{title("This machine"), s.loaded, s.cooling}
	for _, device := range got.Devices {
		body = append(body, drawDevice(device))
	}
	return container.NewVBox(body...)
}

// drawDevice is one device: what it is, what it is doing, and its lights.
func drawDevice(device api.Device) fyne.CanvasObject {
	rows := []fyne.CanvasObject{lights(device)}

	mode := device.ActiveMode
	if mode == "" {
		mode = "not saying"
	}
	facts := []string{fmt.Sprintf("%d lights", device.LEDs), mode}
	if len(device.Zones) > 0 {
		facts = append(facts, fmt.Sprintf("%d zones", len(device.Zones)))
	}
	if device.Reassert != "" {
		// A device that does not hold what it is told. Worth saying in the
		// picture: it is the difference between "hotaru keeps changing this"
		// and "this keeps forgetting".
		facts = append(facts, "re-sent every "+device.Reassert)
	}
	if len(device.Segments) > 0 {
		facts = append(facts, "named: "+strings.Join(device.Segments, ", "))
	}
	rows = append(rows, widgets.Dim(strings.Join(facts, " · ")))

	switch {
	case device.Preview != nil:
		rows = append(rows, widgets.Note(
			fmt.Sprintf("Showing a draft held by %s.", holder(*device.Preview)), fd.StatusWarn))
	case !device.InScope:
		// Drawn, not omitted: a device hotaru is not driving is a fact
		// somebody is looking for, and leaving it out answers the question
		// with silence.
		rows = append(rows, widgets.Note("Out of scope.", fd.StatusInfo))
	}

	return widgets.Card(device.Name, rows...)
}

/*
lights draws a device's zones as blocks, proportional to their LED counts.

Proportional because a zone's size is a fact the device reports and a picture
can show for free: a keyboard's hundred keys next to a mousepad's three is the
shape of the machine.

What the case physically looks like is not on the wire, and guessing at it is
the over-fitting this project set out to avoid. So: a row, in device order.
*/
func lights(device api.Device) fyne.CanvasObject {
	colours := shownColours(device)
	if len(device.Zones) == 0 {
		return swatch(colours, 0, device.LEDs, device.InScope)
	}

	blocks := make([]fyne.CanvasObject, 0, len(device.Zones)*2)
	for i, zone := range device.Zones {
		if i > 0 {
			blocks = append(blocks, layout.NewSpacer())
		}
		blocks = append(blocks, container.NewVBox(
			swatch(colours, zone.First, zone.Count, device.InScope),
			widgets.Dim(fmt.Sprintf("%s · %d", zone.Name, zone.Count)),
		))
	}
	return container.NewHBox(blocks...)
}

// zoneHeight is how tall a light block is drawn.
const zoneHeight = 18

/*
swatch is a run of a device's LEDs, drawn as one bar of up to a few colours.

Not one rectangle per LED: a keyboard has a hundred and the row would be a
smear. A handful of samples across the run shows a gradient as a gradient and a
solid colour as a solid colour, which is what somebody is looking for.
*/
func swatch(colours []color.Color, first, count int, inScope bool) fyne.CanvasObject {
	const samples = 8
	if count <= 0 {
		count = 1
	}
	take := min(samples, count)

	bars := make([]fyne.CanvasObject, 0, take)
	for i := range take {
		at := first + i*count/take
		bar := canvas.NewRectangle(sample(colours, at, inScope))
		bar.SetMinSize(fyne.NewSize(zoneHeight, zoneHeight))
		bars = append(bars, bar)
	}
	return container.NewHBox(bars...)
}

// sample is the colour of one LED, or the placeholder for a device that does
// not report its buffer -- which is not a fault: some hardware simply will not
// say what it is showing.
func sample(colours []color.Color, at int, inScope bool) color.Color {
	if at < 0 || at >= len(colours) {
		return theme.Color(theme.ColorNameDisabled)
	}
	c := colours[at]
	if !inScope {
		return dim(c)
	}
	return c
}

// dim mutes a colour for a device hotaru is not driving, so out-of-scope
// hardware reads as out of scope at a glance rather than in the caption.
func dim(c color.Color) color.Color {
	r, g, b, a := c.RGBA()
	const toward = 3
	return color.NRGBA{
		R: eighth(r) / toward, G: eighth(g) / toward,
		B: eighth(b) / toward, A: eighth(a),
	}
}

// eighth narrows one of RGBA's sixteen-bit channels to the eight bits NRGBA
// holds. The high byte, which is the channel's own value at full range.
func eighth(v uint32) uint8 { return uint8(v >> 8) } //nolint:gosec // a byte by construction

// shownColours parses the colours a device reports, which arrive as "#rrggbb".
func shownColours(device api.Device) []color.Color {
	out := make([]color.Color, 0, len(device.Colours))
	for _, text := range device.Colours {
		out = append(out, parse(text))
	}
	return out
}

// ParseColour reads a colour the way the service does, for a caller outside
// this package that needs the same answer -- a test, above all.
func ParseColour(text string) color.Color { return parse(text) }

/*
parse reads a colour the way the service does.

Through hotaru's own parser rather than a local scan of "#rrggbb", because a
scene says "red" as often as it says "#ff0000" and a swatch that renders the
name as grey is a swatch that lies about what the scene will do. The package is
pure -- names and arithmetic, no devices -- so the window can share it.
*/
func parse(text string) color.Color {
	c, err := colour.Parse(text)
	if err != nil {
		return theme.Color(theme.ColorNameDisabled)
	}
	return color.NRGBA{R: c.R, G: c.G, B: c.B, A: 255}
}

func holder(preview api.Preview) string {
	if preview.Holder == "" {
		return "something"
	}
	return preview.Holder
}
