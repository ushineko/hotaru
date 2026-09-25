package gui

import (
	"fmt"
	"slices"
	"sort"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/dialogs"
	"github.com/ushineko/fynedesygn/widgets"
	"github.com/ushineko/hotaru/internal/api"
)

/*
What each device should be *doing* with a scene's colours.

A scene carries a mode per device -- the keyboard rippling under typing, the
cooler breathing -- and until now the only place that was ever asked was the
mapping wizard, which asks once about a machine rather than every time about a
scene. `hotaru scene write --effect` could set one and the window could not,
which is the parity rule backwards.

**Only the devices that have a choice to make.** A device advertising one mode
has nothing to offer, and a list of eighteen devices where four of them are
selectable is a list somebody has to read to find out.

The default is nothing at all: a scene that names no effect leaves every
device in whatever mode it is in, showing the colours it was given. That is
what almost every scene wants, which is why "leave it alone" is the first
option rather than a mode that happens to be common.
*/
func effectFields(window fyne.Window, devices []api.Device, get func(device string) api.Effect,
	set func(device string, effect api.Effect),
) fyne.CanvasObject {
	found := withModes(devices)
	if len(found) == 0 {
		return widgets.Note("No device here offers more than one mode.", fd.StatusInfo)
	}

	rows := make([]fyne.CanvasObject, 0, len(found)*2+2)
	rows = append(rows, effectHeading())
	for _, device := range found {
		rows = append(rows, effectField(window, device, get, set))
	}
	rows = append(rows, widgets.DimWrapped(
		"A mode is the device's own: what it does with the colours once it has them."))
	return container.NewVBox(rows...)
}

// effectHeading names the columns, so three ragged pairs read as a table.
func effectHeading() fyne.CanvasObject {
	return effectRow(
		column(effectNameWidth, widgets.Dim("DEVICE")),
		widgets.Dim("THIS SCENE"),
		column(effectNowWidth, widgets.Dim("DOING NOW")),
	)
}

/*
effectField is one device's mode, chosen from what it says it has, with what
it is doing right now beside it.

**"Leave it alone" is not an answer until you know what it leaves.** The
chooser said what the scene sets and nothing about the machine, so a keyboard
sitting in a reactive mode looked identical to one sitting in Direct, and the
difference is what the scene will actually look like when it is applied.
*/
func effectField(window fyne.Window, device api.Device, get func(device string) api.Effect,
	set func(device string, effect api.Effect),
) fyne.CanvasObject {
	current := get(device.Name)
	choices := append([]string{noEffect}, device.Modes...)

	/*
		The settings under the chooser are rebuilt when the mode changes,
		because which of them exist is a fact about the mode: Direct has
		neither, a reactive mode has both, and a rainbow has a speed and no
		colour. Drawing all of them always would offer a colour to a mode that
		shows the frame, which is the promise this spec exists to stop making.
	*/
	settings := container.NewVBox()
	var redraw func(api.Effect)
	redraw = func(effect api.Effect) {
		// The colour is picked and then shown, so the row redraws itself
		// rather than waiting for whatever built it to come round again.
		settings.Objects = effectSettings(window, device, effect, set, func(changed api.Effect) {
			set(device.Name, changed)
			redraw(changed)
		})
		settings.Refresh()
	}

	choose := widget.NewSelect(choices, func(picked string) {
		effect := get(device.Name)
		if picked == noEffect {
			set(device.Name, api.Effect{})
			redraw(api.Effect{})
			return
		}
		effect.Mode = picked
		set(device.Name, effect)
		redraw(effect)
	})
	choose.Selected = noEffect
	if current.Named() {
		choose.Selected = current.Mode
	}
	redraw(current)

	name := widget.NewLabel(device.Name)
	name.Truncation = fyne.TextTruncateEllipsis

	return container.NewVBox(
		effectRow(
			column(effectNameWidth, name),
			choose,
			column(effectNowWidth, widgets.Dim(doingNow(device))),
		),
		settings,
	)
}

/*
effectSettings are the mode's own: the colour it shows and the speed it runs
at, for the modes that have them.

Indented under the row rather than beside it. Six devices at four controls
across is a table nobody can read, and these belong to the mode the row above
just chose.
*/
func effectSettings(window fyne.Window, device api.Device, effect api.Effect,
	set func(device string, effect api.Effect), changed func(api.Effect),
) []fyne.CanvasObject {
	if !effect.Named() {
		return nil
	}
	var out []fyne.CanvasObject
	if slices.Contains(device.OneColour, effect.Mode) {
		out = append(out, effectIndent(effectColourField(window, effect, changed)))
	}
	if pace, ok := device.Paced[effect.Mode]; ok {
		out = append(out, effectIndent(effectSpeedField(device, effect, pace, set)))
	}
	return out
}

/*
effectColourField is the one colour a mode like this shows: a swatch, and the
same wheel everything else in this window picks a colour with.

**Not a field somebody types a hex value into.** The argument for a window at
all is that a colour is looked at rather than spelled, and a control that is a
text box in one place and a wheel in another is two ways to do one thing where
the worse one is in the dialog people reach first. The wheel opens over this
dialog, which is what overlays are for.

Empty says what it falls back to, because "nothing" is otherwise
indistinguishable from black in a control that shows a colour.
*/
func effectColourField(window fyne.Window, effect api.Effect, changed func(api.Effect)) fyne.CanvasObject {
	swatch := canvas.NewRectangle(parse(effect.Colour))
	swatch.SetMinSize(fyne.NewSize(swatchWidth, swatchHeight))

	said := effect.Colour
	if said == "" {
		said = "the colour most of the scene is"
	}

	choose := widget.NewButton("Choose"+ellipsis, func() {
		pickOver(window, "A colour for "+effect.Mode, effect.Colour, func(picked string) {
			current := effect
			current.Colour = picked
			changed(current)
		})
	})
	clearing := widget.NewButton("Clear", func() {
		current := effect
		current.Colour = ""
		changed(current)
	})

	return container.NewBorder(nil, nil,
		column(effectSettingWidth, widgets.Dim("shows one colour")),
		container.NewHBox(choose, clearing),
		container.NewHBox(swatch, widgets.Dim(said)),
	)
}

/*
pickOver opens the wheel over whatever is already open, and reports the colour
if it is kept.

No preview on the hardware, which is this dialog's own rule rather than a
shortcoming: nothing chosen here lights anything as it is chosen. A mode picked
from the row above does not, and "Show it on the hardware" in the editor is how
somebody looks at any of it -- one button, for the scene as a whole, at the
moment they ask. Saving applies it.
*/
func pickOver(window fyne.Window, title, current string, keep func(string)) {
	picker := NewPicker(parse(Opening(current)))
	chooser := dialog.NewCustomConfirm(title, "Use it", "Cancel", picker.Object(),
		func(ok bool) {
			if ok {
				keep(picker.Colour())
			}
		}, window)
	dialogs.Roomy(chooser, window)
}

/*
effectSpeedField is how fast the mode runs, in the device's own units.

The ends of the slider are the mode's own numbers, which is why they are drawn:
a keyboard counts to 255 and something else counts to 4, and a percentage over
both would be a number that means nothing on either.
*/
func effectSpeedField(device api.Device, effect api.Effect, pace api.Speed,
	set func(device string, effect api.Effect),
) fyne.CanvasObject {
	low, high := min(pace.Slowest, pace.Fastest), max(pace.Slowest, pace.Fastest)
	at := pace.Now
	if effect.Speed != nil {
		at = *effect.Speed
	}

	slider := widget.NewSlider(float64(low), float64(high))
	slider.Step = 1
	slider.Value = float64(max(low, min(high, at)))
	shown := widgets.Dim(fmt.Sprintf("%d", int(slider.Value)))
	slider.OnChanged = func(v float64) {
		speed := int(v)
		shown.(*widget.Label).SetText(fmt.Sprintf("%d", speed))
		current := effect
		current.Speed = &speed
		set(device.Name, current)
	}
	return container.NewBorder(nil, nil, column(effectSettingWidth, widgets.Dim("speed")),
		shown, slider)
}

// effectIndent sets a setting in from the row that chose the mode, so the two
// read as one thing rather than as another column.
func effectIndent(what fyne.CanvasObject) fyne.CanvasObject {
	return container.NewBorder(nil, nil, column(effectIndentWidth, widgets.Dim("")), nil, what)
}

/*
effectRow is one line of the table: a name, the chooser, and what the device
is doing now.

The name and the last column hold a width so the choosers line up down the
list. Ragged pairs of label-and-control are readable at two rows and are a
wall at six, which is what this desk has.
*/
func effectRow(name, choose, now fyne.CanvasObject) fyne.CanvasObject {
	return container.NewBorder(nil, nil, name, now, choose)
}

// The table's fixed columns: wide enough for this desk's longest device name
// and for a mode name beside it.
const (
	effectNameWidth = 260
	effectNowWidth  = 210
	// The label column under a row, and how far in that row's settings sit.
	effectSettingWidth = 130
	effectIndentWidth  = 24
	// The swatch beside a colour, tall enough to read as a colour rather than
	// as a line.
	swatchWidth  = 48
	swatchHeight = 20
)

// ellipsis is what a button that opens something else ends with.
const ellipsis = "\u2026"

// doingNow is what the device is in at this moment, which is what "leave it
// alone" means for it.
func doingNow(device api.Device) string {
	if device.ActiveMode == "" {
		return "not saying"
	}
	return "now: " + device.ActiveMode
}

// noEffect is the first option and the default: the device keeps whatever mode
// it is in, showing the colours the scene gives it.
const noEffect = "leave it alone"

/*
withModes are the devices worth asking about, in name order.

More than one mode, because one mode is not a choice. In scope, because a
scene does not touch anything else -- and offering a mode for a device hotaru
will not write to is offering something that cannot happen.
*/
func withModes(devices []api.Device) []api.Device {
	var out []api.Device
	for _, device := range devices {
		if device.InScope && len(device.Modes) > 1 {
			out = append(out, device)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
