package gui

import (
	"sort"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	fd "github.com/ushineko/fynedesygn"
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
func effectFields(devices []api.Device, get func(device string) string, set func(device, mode string)) fyne.CanvasObject {
	found := withModes(devices)
	if len(found) == 0 {
		return widgets.Note("No device here offers more than one mode.", fd.StatusInfo)
	}

	rows := make([]fyne.CanvasObject, 0, len(found)+2)
	rows = append(rows, effectHeading())
	for _, device := range found {
		rows = append(rows, effectField(device, get(device.Name), set))
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
func effectField(device api.Device, current string, set func(device, mode string)) fyne.CanvasObject {
	choices := append([]string{noEffect}, device.Modes...)

	choose := widget.NewSelect(choices, func(picked string) {
		if picked == noEffect {
			set(device.Name, "")
			return
		}
		set(device.Name, picked)
	})
	choose.Selected = noEffect
	if current != "" {
		choose.Selected = current
	}

	name := widget.NewLabel(device.Name)
	name.Truncation = fyne.TextTruncateEllipsis

	return effectRow(
		column(effectNameWidth, name),
		choose,
		column(effectNowWidth, widgets.Dim(doingNow(device))),
	)
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
)

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
