package gui

import (
	"fmt"
	"os"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/widgets"
	"github.com/ushineko/hotaru/internal/config"
)

/*
status is the bar along the bottom: whether the service is there, what it
sees, and how warm the coolant is.

Laid out in sequence and never in a Border centre -- a centre region is sized
from what is left over, and a long socket path pushes its neighbours into each
other. fynedesygn learned that one already.
*/
func (a *App) status(socket string) []fyne.CanvasObject {
	got := a.machine.Read()
	if got.Err != nil {
		return []fyne.CanvasObject{
			widgets.StatusText("service not running", fd.StatusWarn),
			widgets.Sep(),
			widgets.Dim(socket),
		}
	}

	out := []fyne.CanvasObject{
		widgets.StatusText(got.Health.State, healthStatus(got.Health.State)),
		widgets.Sep(),
		widgets.Dim(fmt.Sprintf("%d of %d devices", scoped(got.Devices), len(got.Devices))),
	}
	if reading, has := cooler(got); has {
		out = append(out, widgets.Sep(),
			widgets.StatusText(fmt.Sprintf("%.1f °C", reading.Coolant),
				coolantStatus(reading.Coolant)))
	}
	return out
}

/*
settingsPath is where the window keeps its view state.

Beside hotaru's own configuration rather than in a directory of its own, and
holding nothing but geometry, scheme and the last section. **The service never
reads this file and this program never writes the service's** -- one writer per
file is the rule the whole architecture rests on, and the editor is exactly
where somebody would be tempted to break it.
*/
func (a *App) settingsPath() string {
	dir, err := config.Dir()
	if err != nil {
		// Let the shell fall back to its own default rather than failing to
		// open a window over a missing home directory.
		return ""
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return ""
	}
	return filepath.Join(dir, "gui.yml")
}

/*
title is a section's name and a rule under it, and nothing else.

fynedesygn's Heading takes a blurb and every section in every program built on
it has one. This program's sections do not: a line of prose under "The service"
explaining what a service is tells somebody looking at their own machine
something they knew before they opened the window. The facts are the content.
*/
func title(text string) fyne.CanvasObject {
	return container.NewVBox(
		widget.NewLabelWithStyle(text, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewSeparator(),
	)
}
