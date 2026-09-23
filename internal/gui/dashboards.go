package gui

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/gif"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/imagecache"
	"github.com/ushineko/fynedesygn/shell"
	"github.com/ushineko/fynedesygn/widgets"
	"github.com/ushineko/hotaru/internal/api"
	"github.com/ushineko/hotaru/internal/dashboard"
	"github.com/ushineko/hotaru/internal/readings"
	xdraw "golang.org/x/image/draw"
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

	// drawing counts the previews in flight. See Settle.
	drawing sync.WaitGroup
	// drawn counts the frames, which is what gives each one a name of its
	// own for Fyne's image cache.
	drawn int
}

// OpenDashboards gives a section its app, which the shell normally does. For
// tests, like OpenEditor.
func OpenDashboards(d *DashboardsSection, app *App) { d.app = app }

/*
Settle waits for the previews this section has in flight.

For tests. A preview is rendered by the service and drawn when it answers,
which is right in a running window and a loose end in a test: the goroutine
outlives the test that started it and touches the interface while the next
one is drawing. Fyne's test driver runs `fyne.Do` inline on the calling
goroutine, so that is two goroutines shaping text at once, and its shaper
panics.
*/
func (d *DashboardsSection) Settle() { d.drawing.Wait() }

// EditDashboard puts a section into its editing state, and DraftDashboard is
// what it is editing. For tests: the editor is reached by a button on a row,
// and a test that pressed it would be a test about rows.
func EditDashboard(d *DashboardsSection, one api.Dashboard) {
	d.editing, d.name = &one, one.Name
}

// DraftDashboard is the draft the editor holds, for tests to read back.
func DraftDashboard(d *DashboardsSection) api.Dashboard {
	if d.editing == nil {
		return api.Dashboard{}
	}
	return *d.editing
}

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
		return container.NewVBox(widgets.Note(err.Error(), fd.StatusWarn))
	}
	if d.editing != nil {
		return d.editor(sh, stored)
	}

	/*
		Said once, at the top, on a machine with nowhere to draw.

		Screens are worth making either way -- they are a file, they travel
		to a machine that has a panel, and editing one previews here -- so
		this is a note rather than a closed door. What it replaces is
		clicking "Show it" and watching nothing happen.
	*/
	list := d.list(sh, stored)
	if got.Cooling.Screen == "" || got.Cooling.ScreenDetail != "" {
		return container.NewBorder(
			widgets.Note(nowhere(got.Cooling), fd.StatusInfo), nil, nil, nil, list)
	}
	return list
}

// nowhere says why nothing can be drawn, in the words the System section uses
// for the same fact.
func nowhere(cooling api.Cooling) string {
	switch {
	case cooling.Absent:
		return "No cooler on this machine. Screens can still be made."
	case cooling.Screen == "":
		return "This cooler has no display. Screens can still be made."
	}
	return "The display cannot be reached: " + cooling.ScreenDetail
}

/*
list is what the screen can be asked to draw.

A table rather than a column of lines. Each dashboard is a name, three
choices and a picture of what it looks like, and the choices only mean
anything read down the column: "ring, midnight, starfield" against "grid,
ice, starfield" says something, and the same words run together in a sentence
per row say much less.
*/
func (d *DashboardsSection) list(sh *shell.Shell, got api.DashboardsResponse) fyne.CanvasObject {
	add := widget.NewButtonWithIcon("New screen", theme.ContentAddIcon(), func() {
		fresh := blank()
		d.editing, d.name = &fresh, ""
		sh.Invalidate()
	})
	add.Importance = widget.HighImportance

	rows := []fyne.CanvasObject{heading()}
	for _, one := range got.Dashboards {
		rows = append(rows, d.row(sh, one, one.Name == got.Active), widget.NewSeparator())
	}

	return container.NewBorder(
		container.NewVBox(add), nil, nil, nil,
		container.NewVScroll(container.NewVBox(rows...)),
	)
}

/*
The columns, and what each is wide enough for.

Fixed, because a column that sized itself to its contents would move every
time somebody renamed a dashboard -- and the point of a table is that the
same thing is in the same place on every line.
*/
const (
	shotSize   = 72  // the picture of the dashboard
	nameWidth  = 150 // "coolant  (shipped)"
	factWidth  = 110 // "starfield", "stacked", "midnight"
	behindWide = 190 // a picture's name, which is whatever somebody called it
)

// heading names the columns.
func heading() fyne.CanvasObject {
	return listRow([]fyne.CanvasObject{
		column(shotSize, widgets.Dim("")),
		column(nameWidth, widgets.Dim("NAME")),
		column(factWidth, widgets.Dim("LAYOUT")),
		column(factWidth, widgets.Dim("COLOURS")),
		column(behindWide, widgets.Dim("BEHIND")),
	})
}

/*
row is one dashboard: what it looks like, what it is, and what to do with it.

The picture first, because it is the thing somebody is choosing between. It
is the frame the service would draw, at a size that fits a list -- rendered
once per dashboard and kept, since re-rendering six of them on every build
would cost more than the list shows.
*/
func (d *DashboardsSection) row(sh *shell.Shell, one api.Dashboard, active bool) fyne.CanvasObject {
	name := one.Name
	if one.Shipped {
		name += "  (shipped)"
	}
	label := widget.NewLabel(name)
	label.Truncation = fyne.TextTruncateEllipsis
	if active {
		label.TextStyle = fyne.TextStyle{Bold: true}
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

	/*
		A scene from a dashboard, the same way a picture makes one.

		The frame the panel would draw is the picture: a screen full of amber
		reads across the case as amber, which is what somebody choosing a
		dashboard and then a set of colours was doing by hand.
	*/
	scene := widget.NewButtonWithIcon("", theme.ColorPaletteIcon(), func() {
		makeScene(sh, one.Name, one.Name, d.app.machine.Read().Devices,
			func(ctx context.Context, name string, distance float64, effects map[string]string) (api.Scene, error) {
				return d.app.client.SceneFromDashboard(ctx, one.Name, name, distance, effects)
			})
	})

	/*
		The delete button is always in the row, and disabled where there is
		nothing to delete.

		A shipped dashboard cannot be removed -- the store takes the request
		and it is still there afterwards -- and leaving the button out
		instead would put every other row's buttons at a different place on
		the line, which is the jumble this table is fixing.
	*/
	forget := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
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
	})
	if one.Shipped {
		forget.Disable()
	}

	behind := widget.NewLabel(backgroundName(one))
	behind.Truncation = fyne.TextTruncateEllipsis

	return listRow([]fyne.CanvasObject{
		d.shot(one),
		column(nameWidth, label),
		column(factWidth, widgets.Dim(arrangementName(one))),
		column(factWidth, widgets.Dim(themeName(one))),
		column(behindWide, behind),
	}, use, edit, scene, forget)
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

/*
defaultLabel is what a slot draws when it has no label of its own.

Asked of the dashboard package rather than worked out here, so the placeholder
in the editor and the words on the panel cannot drift apart. The window does
not render -- that is the service's job and deliberately only its job -- but
what a thing is *called* is the model's, and there is one answer to it.
*/
func defaultLabel(source, second string, units bool) string {
	return dashboard.Slot{
		Source: readings.Source(source), Second: readings.Source(second),
	}.Words(units)
}

/*
shot is a small picture of what a dashboard draws.

Rendered by the service, like the editor's preview, and for the same reason:
the panel takes a 640x640 GIF and the service is what makes them.

**Fetched once per dashboard and kept.** Six dashboards on every build is six
renders and six GIF encodes, and a list that cost that much to draw would be
slower than the thing it lists. The key is the dashboard's description, so a
dashboard somebody edits gets a new picture and one they only looked at does
not.

The readings move constantly and the picture does not follow them. That is
the point: this is a picture of the *dashboard*, not of the machine.
*/
func (d *DashboardsSection) shot(one api.Dashboard) fyne.CanvasObject {
	return dashboardShot(d.app, one)
}

// dashboardShot is a picture of what a dashboard draws, for any section that
// wants one: the list, and the scene editor's screen chooser.
func dashboardShot(app *App, one api.Dashboard) fyne.CanvasObject {
	picture := canvas.NewImageFromImage(nil)
	picture.FillMode = canvas.ImageFillContain
	picture.SetMinSize(fyne.NewSize(shotSize, shotSize))

	key := shotKey(one)
	if held, ok := imagecache.Shared.Get(key, func() (image.Image, error) {
		return nil, errNoShotYet
	}); ok == nil && held != nil {
		picture.Image = held
		return picture
	}

	go func() {
		frame, err := app.client.PreviewDashboard(context.Background(), one.Name, &one)
		if err != nil {
			return
		}
		body, err := base64.StdEncoding.DecodeString(frame.Image)
		if err != nil {
			return
		}
		small, err := imagecache.Shared.Get(key, func() (image.Image, error) {
			return shrinkFrame(body)
		})
		if err != nil {
			return
		}
		onScreen(func() {
			// In place. Invalidating here would rebuild the list under
			// whoever is reading it, once per dashboard.
			picture.Image = small
			picture.Refresh()
		})
	}()
	return picture
}

// errNoShotYet is how shot asks the cache whether it already has a picture
// without drawing one: a decode that fails caches nothing.
var errNoShotYet = errors.New("not rendered yet")

/*
shotKey names a dashboard's picture by what the dashboard says.

Not by its name: a dashboard somebody edits keeps its name and draws something
else, and a key that did not change would show them what it used to look like.
*/
func shotKey(one api.Dashboard) string {
	described, err := json.Marshal(one)
	if err != nil {
		return "hotaru/shot/" + one.Name
	}
	return fmt.Sprintf("hotaru/shot/%x", sha256.Sum256(described))
}

// shrinkFrame decodes a rendered frame and scales it to the list's size.
func shrinkFrame(body []byte) (image.Image, error) {
	first, err := gif.Decode(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("that frame does not decode: %w", err)
	}
	const side = shotSize * 2 // twice drawn, for a scaled desktop
	small := image.NewRGBA(image.Rect(0, 0, side, side))
	xdraw.CatmullRom.Scale(small, small.Bounds(), first, first.Bounds(), draw.Src, nil)
	return small, nil
}
