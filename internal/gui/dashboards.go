package gui

import (
	"context"
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/shell"
	"github.com/ushineko/fynedesygn/widgets"
	"github.com/ushineko/hotaru/internal/api"
	"github.com/ushineko/hotaru/internal/readings"
)

/*
DashboardsSection is the cooler's screen, as something somebody can change.

The list and the editor, in one section and never at once: an editor beside
the thing it edits halves both on a window somebody has put beside something
else.

The preview is a frame the *service* rendered. A second renderer here would be
a second answer about what the panel shows, and the one nobody checks is the
one on the panel.
*/
type DashboardsSection struct {
	app *App

	// editing is the draft, and what makes this the editor rather than the
	// list. Nil is the list.
	editing *api.Dashboard
	// name is the draft's name as somebody is typing it, which is not the
	// draft's own until it is saved.
	name string

	// frame is the last preview the service drew, kept so a rebuild does not
	// blank the picture while the next one is on its way.
	frame fyne.Resource
	// cost is what that frame costs the panel, in the words under it.
	cost string

	/*
		picture and costLabel are what a finished preview writes into.

		Held rather than rebuilt: a frame arriving invalidated the whole
		section in the first version, which took the control out from under
		anybody typing in it. They are replaced on every rebuild and are nil
		until the editor has been drawn once.
	*/
	picture   *canvas.Image
	costLabel *widget.Label
	// drawn counts the frames, which is what gives each one a name of its
	// own for Fyne's image cache.
	drawn int
}

// OpenDashboards gives a section its app, which the shell normally does. For
// tests, like OpenEditor.
func OpenDashboards(d *DashboardsSection, app *App) { d.app = app }

// Title is the name in the navigation.
func (d *DashboardsSection) Title() string { return "Screen" }

// Icon is the navigation's icon for this section.
func (d *DashboardsSection) Icon() fyne.Resource { return theme.ComputerIcon() }

// Busy keeps the poll from rebuilding the editor under somebody's hands.
func (d *DashboardsSection) Busy() bool { return d.editing != nil }

// Changed says this section draws what is saved, which moves when somebody
// saves something rather than when a fan speeds up.
func (d *DashboardsSection) Changed(before, after Snapshot) bool {
	return (before.Err == nil) != (after.Err == nil)
}

// Arrive puts the section back to its list. Editing something, navigating
// away and coming back should not resume a draft nobody remembers.
func (d *DashboardsSection) Arrive() { d.editing, d.frame, d.cost = nil, nil, "" }

// Build draws the list, or the editor when there is a draft.
func (d *DashboardsSection) Build(sh *shell.Shell) fyne.CanvasObject {
	got := d.app.machine.Read()
	if got.Err != nil {
		return notRunning(got.Err)
	}
	stored, err := d.app.client.Dashboards(context.Background())
	if err != nil {
		return container.NewVBox(title("Screen"), widgets.Note(err.Error(), fd.StatusWarn))
	}
	if d.editing != nil {
		return d.editor(sh, stored)
	}
	return d.list(sh, stored)
}

// list is what the screen can be asked to draw.
func (d *DashboardsSection) list(sh *shell.Shell, got api.DashboardsResponse) fyne.CanvasObject {
	add := widget.NewButtonWithIcon("New screen", theme.ContentAddIcon(), func() {
		fresh := blank()
		d.editing, d.name = &fresh, ""
		sh.Invalidate()
	})
	add.Importance = widget.HighImportance

	rows := make([]fyne.CanvasObject, 0, len(got.Dashboards))
	for _, one := range got.Dashboards {
		rows = append(rows, d.row(sh, one, one.Name == got.Active))
	}

	return container.NewBorder(
		container.NewVBox(title("Screen"), add), nil, nil, nil,
		container.NewVScroll(container.NewVBox(rows...)),
	)
}

// row is one dashboard: what it is, and the three things worth doing with it.
func (d *DashboardsSection) row(sh *shell.Shell, one api.Dashboard, active bool) fyne.CanvasObject {
	facts := []string{arrangementName(one), themeName(one), backgroundName(one)}
	if active {
		facts = append([]string{"on the screen"}, facts...)
	}

	use := widget.NewButton("Show it", func() {
		sh.Perform("showing "+one.Name, func(ctx context.Context) error {
			if _, err := d.app.client.UseDashboard(ctx, one.Name); err != nil {
				return err
			}
			onScreen(func() {
				sh.Flash("The screen is drawing "+one.Name+".", fd.StatusGood)
				sh.Invalidate()
			})
			return nil
		})
	})
	if active {
		use.Disable()
	}
	edit := widget.NewButton("Edit", func() {
		draft := one
		d.editing, d.name = &draft, one.Name
		sh.Invalidate()
	})

	buttons := []fyne.CanvasObject{use, edit}
	if !one.Shipped {
		buttons = append(buttons, widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
			dialog.ShowConfirm("Delete "+one.Name+"?", "", func(yes bool) {
				if !yes {
					return
				}
				sh.Perform("deleting "+one.Name, func(ctx context.Context) error {
					if err := d.app.client.DeleteDashboard(ctx, one.Name); err != nil {
						return err
					}
					onScreen(func() {
						sh.Flash(one.Name+" is gone.", fd.StatusGood)
						sh.Invalidate()
					})
					return nil
				})
			}, sh.Window)
		}))
	}

	name := one.Name
	if one.Shipped {
		name += "  (shipped)"
	}
	return container.NewBorder(nil, nil,
		widget.NewLabel(name), container.NewHBox(buttons...),
		widgets.Dim(strings.Join(facts, " · ")),
	)
}

// blank is a new screen: the shape of the one hotaru ships, because a form
// with every field empty is a form nobody knows how to start.
func blank() api.Dashboard {
	return api.Dashboard{
		Arrangement: "ring",
		Headline:    api.DashboardSlot{Source: "coolant", Label: "COOLANT"},
		Rings:       []string{"coolant"},
		Slots: []api.DashboardSlot{
			{Source: "cpu_c", Label: "CPU"},
			{Source: "gpu_c", Label: "GPU"},
			{Source: "pump_rpm", Label: "PUMP"},
		},
	}
}

func arrangementName(one api.Dashboard) string {
	if one.Arrangement == "" {
		return "ring"
	}
	return one.Arrangement
}

func themeName(one api.Dashboard) string {
	if one.Theme == "" {
		return "midnight"
	}
	return one.Theme
}

func backgroundName(one api.Dashboard) string {
	switch one.Background.Kind {
	case "plain":
		return "plain"
	case "picture":
		if one.Background.Picture == "" {
			return "a picture"
		}
		return one.Background.Picture
	}
	return "starfield"
}

/*
sourceNames are the readings a slot can show, in the words the service uses.

Asked of the readings package rather than listed here: a window with its own
copy of the list is a window that shows eight of nine after somebody adds a
sensor.
*/
func sourceNames() []string {
	all := readings.All
	out := make([]string, 0, len(all))
	for _, source := range all {
		out = append(out, sourceLabel(source))
	}
	return out
}

// sourceLabel is how one reading is offered: its name and its unit, because
// "CPU" alone appears twice and means two different numbers.
func sourceLabel(source any) string {
	s, ok := source.(readings.Source)
	if !ok {
		s = readings.Source(fmt.Sprint(source))
	}
	label, unit := readings.Describe(s)
	if unit == "" {
		return label
	}
	return label + " " + unit
}

// sourceOf is the reading behind one of those words.
func sourceOf(offered string) string {
	for _, source := range readings.All {
		if sourceLabel(source) == offered {
			return string(source)
		}
	}
	return offered
}

// defaultLabel is what a slot draws when it has no label of its own.
func defaultLabel(source string) string {
	label, _ := readings.Describe(readings.Source(source))
	return label
}
