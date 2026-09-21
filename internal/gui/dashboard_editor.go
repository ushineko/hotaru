package gui

import (
	"context"
	"encoding/base64"
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/shell"
	"github.com/ushineko/fynedesygn/widgets"
	"github.com/ushineko/hotaru/internal/api"
)

/*
The editor: a form on the left, the frame it describes on the right.

A form rather than a canvas. The panel is 640x640 behind a round bezel in a
case, and free placement would mostly be a way of putting a number where the
bezel cuts it off -- so the arrangements are drawn against the panel and the
choice is which reading fills each of their slots.

Every change redraws the preview, and the preview is rendered by the service
because that is what makes the frames the panel takes.
*/
func (d *DashboardsSection) editor(sh *shell.Shell, got api.DashboardsResponse) fyne.CanvasObject {
	form := container.NewVBox(
		d.nameField(),
		widget.NewSeparator(),
		d.arrangementField(sh, got),
		d.themeField(sh, got),
		widget.NewSeparator(),
		d.headlineField(sh),
		d.ringFields(sh, got),
		widget.NewSeparator(),
		d.slotFields(sh, got),
		widget.NewSeparator(),
		d.backgroundFields(sh),
		d.captionField(sh),
		widget.NewSeparator(),
		d.letteringFields(sh),
	)

	return container.NewBorder(
		container.NewVBox(d.actions(sh)), nil, nil, nil,
		container.NewHSplit(
			container.NewVScroll(form),
			container.NewVScroll(d.preview(sh)),
		),
	)
}

// actions are the controls that stay put: fynedesygn's rule that what acts on
// a page is affixed to it rather than scrolling away underneath somebody.
func (d *DashboardsSection) actions(sh *shell.Shell) fyne.CanvasObject {
	save := widget.NewButtonWithIcon("Save", theme.DocumentSaveIcon(), func() { d.save(sh) })
	save.Importance = widget.HighImportance

	show := widget.NewButton("Save and show it", func() { d.saveAndShow(sh) })
	back := widget.NewButton("Cancel", func() {
		d.editing, d.frame, d.cost = nil, nil, ""
		sh.Invalidate()
	})
	return container.NewHBox(save, show, back)
}

func (d *DashboardsSection) nameField() fyne.CanvasObject {
	entry := widget.NewEntry()
	entry.SetPlaceHolder("what to call it")
	entry.SetText(d.name)
	entry.OnChanged = func(s string) { d.name = s }
	return field("Name", entry)
}

func (d *DashboardsSection) arrangementField(sh *shell.Shell, got api.DashboardsResponse) fyne.CanvasObject {
	names := make([]string, 0, len(got.Arrangements))
	for _, one := range got.Arrangements {
		names = append(names, one.Name)
	}

	choose := widget.NewSelect(names, func(picked string) {
		if picked == d.editing.Arrangement {
			return
		}
		d.editing.Arrangement = picked
		/*
			The only two changes that rebuild the form, because they are the
			two that change its shape: an arrangement has its own number of
			rings and readings, and a picture brings a chooser and a slider
			with it. Everything else writes into the draft and redraws the
			picture, which leaves the controls where somebody left them.
		*/
		sh.Invalidate()
	})
	choose.SetSelected(arrangementName(*d.editing))

	return container.NewVBox(
		field("Layout", choose),
		widgets.Dim(roomFor(got, arrangementName(*d.editing))),
	)
}

/*
roomFor says what an arrangement has room for.

Worth saying rather than leaving somebody to discover it: a dashboard carries
four slots and two rings whatever it is drawn as, and switching to an
arrangement with fewer draws fewer -- the rest are kept, not thrown away, so
switching back brings them home.
*/
func roomFor(got api.DashboardsResponse, name string) string {
	for _, one := range got.Arrangements {
		if one.Name != name {
			continue
		}
		switch {
		case one.Rings == 0:
			return fmt.Sprintf("%d readings, no rings.", one.Slots)
		case one.Slots == 0:
			return fmt.Sprintf("The headline and %d rings.", one.Rings)
		}
		return fmt.Sprintf("%d readings and %d rings.", one.Slots, one.Rings)
	}
	return ""
}

func (d *DashboardsSection) themeField(sh *shell.Shell, got api.DashboardsResponse) fyne.CanvasObject {
	choose := widget.NewSelect(got.Themes, func(picked string) {
		if picked == d.editing.Theme {
			return
		}
		d.editing.Theme = picked
		d.redraw(sh)
	})
	choose.SetSelected(themeName(*d.editing))
	return field("Colours", choose)
}

func (d *DashboardsSection) headlineField(sh *shell.Shell) fyne.CanvasObject {
	return field("Big number", d.slotRow(sh, &d.editing.Headline))
}

/*
slotRow is one reading and what to call it.

The label is the dashboard's own word where it has one, and the reading's own
where it does not -- so somebody who wants "CPU" gets it without typing, and
somebody who wants "PROC" can have that instead.
*/
func (d *DashboardsSection) slotRow(sh *shell.Shell, slot *api.DashboardSlot) fyne.CanvasObject {
	choose := widget.NewSelect(sourceNames(), func(picked string) {
		source := sourceOf(picked)
		if source == slot.Source {
			return
		}
		slot.Source = source
		d.redraw(sh)
	})
	choose.SetSelected(sourceLabel(slot.Source))

	label := widget.NewEntry()
	label.SetPlaceHolder(defaultLabel(slot.Source))
	label.SetText(slot.Label)
	label.OnChanged = func(s string) { slot.Label = s }
	label.OnSubmitted = func(string) { d.redraw(sh) }

	return container.NewBorder(nil, nil, choose, nil, label)
}

func (d *DashboardsSection) ringFields(sh *shell.Shell, got api.DashboardsResponse) fyne.CanvasObject {
	room := ringsFor(got, arrangementName(*d.editing))
	if room == 0 {
		return widgets.Dim("This layout draws no rings.")
	}

	rows := make([]fyne.CanvasObject, 0, room)
	for i := range room {
		rows = append(rows, labelled(fmt.Sprintf("Ring %d", i+1), d.ringRow(sh, i)))
	}
	return container.NewVBox(rows...)
}

/*
ringRow is one arc, or none.

"Nothing" is an option rather than an absence, because a dashboard with fewer
rings than the layout allows has to be expressible -- and the rings are
drawn outermost first, so leaving a gap in the middle would be a question
nobody asked.
*/
func (d *DashboardsSection) ringRow(sh *shell.Shell, at int) fyne.CanvasObject {
	const none = "nothing"

	options := append([]string{none}, sourceNames()...)
	choose := widget.NewSelect(options, func(picked string) {
		rings := d.editing.Rings
		for len(rings) <= at {
			rings = append(rings, "")
		}
		if picked == none {
			rings = rings[:at]
		} else {
			rings[at] = sourceOf(picked)
		}
		d.editing.Rings = trimEmpty(rings)
		d.redraw(sh)
	})

	picked := none
	if at < len(d.editing.Rings) {
		picked = sourceLabel(d.editing.Rings[at])
	}
	choose.SetSelected(picked)
	return choose
}

// trimEmpty drops the trailing blanks a partly-filled ring list leaves.
func trimEmpty(rings []string) []string {
	for len(rings) > 0 && rings[len(rings)-1] == "" {
		rings = rings[:len(rings)-1]
	}
	return rings
}

func ringsFor(got api.DashboardsResponse, name string) int {
	for _, one := range got.Arrangements {
		if one.Name == name {
			return one.Rings
		}
	}
	return 0
}

func slotsFor(got api.DashboardsResponse, name string) int {
	for _, one := range got.Arrangements {
		if one.Name == name {
			return one.Slots
		}
	}
	return 0
}

func (d *DashboardsSection) slotFields(sh *shell.Shell, got api.DashboardsResponse) fyne.CanvasObject {
	room := slotsFor(got, arrangementName(*d.editing))
	if room == 0 {
		return widgets.Dim("This layout draws the big number and nothing else.")
	}

	for len(d.editing.Slots) < room {
		d.editing.Slots = append(d.editing.Slots, api.DashboardSlot{Source: "cpu_c"})
	}
	rows := make([]fyne.CanvasObject, 0, room)
	for i := range room {
		rows = append(rows, labelled(fmt.Sprintf("Reading %d", i+1), d.slotRow(sh, &d.editing.Slots[i])))
	}
	return container.NewVBox(rows...)
}

func (d *DashboardsSection) backgroundFields(sh *shell.Shell) fyne.CanvasObject {
	kinds := []string{"starfield", "plain", "picture"}
	kind := d.editing.Background.Kind
	if kind == "" {
		kind = "starfield"
	}

	choose := widget.NewSelect(kinds, func(picked string) {
		if picked == d.editing.Background.Kind {
			return
		}
		d.editing.Background.Kind = picked
		sh.Invalidate() // the shape changes: see arrangementField
	})
	choose.SetSelected(kind)

	rows := []fyne.CanvasObject{field("Behind", choose)}
	if kind == "picture" {
		rows = append(rows, d.pictureField(sh), d.dimField(sh),
			widgets.Note("A photograph is a bigger frame, and the panel needs longer "+
				"between bigger frames.", fd.StatusInfo))
	}
	return container.NewVBox(rows...)
}

func (d *DashboardsSection) pictureField(sh *shell.Shell) fyne.CanvasObject {
	stored, err := d.app.client.Images(context.Background())
	if err != nil || len(stored) == 0 {
		return widgets.Note("No pictures yet. Add one under Pictures.", fd.StatusInfo)
	}
	names := make([]string, 0, len(stored))
	for _, image := range stored {
		names = append(names, image.Name)
	}

	choose := widget.NewSelect(names, func(picked string) {
		if picked == d.editing.Background.Picture {
			return
		}
		d.editing.Background.Picture = picked
		d.redraw(sh)
	})
	if d.editing.Background.Picture != "" {
		choose.SetSelected(d.editing.Background.Picture)
	}
	return field("Picture", choose)
}

func (d *DashboardsSection) dimField(sh *shell.Shell) fyne.CanvasObject {
	dim := d.editing.Background.Dim
	if dim <= 0 {
		dim = 55
	}
	slider := widget.NewSlider(0, 90)
	slider.Step = 5
	slider.Value = float64(dim)
	slider.OnChangeEnded = func(v float64) {
		d.editing.Background.Dim = int(v)
		d.redraw(sh)
	}
	return field("Darkened", slider)
}

func (d *DashboardsSection) captionField(sh *shell.Shell) fyne.CanvasObject {
	entry := widget.NewEntry()
	entry.SetPlaceHolder("a line of your own")
	entry.SetText(d.editing.Caption)
	entry.OnChanged = func(s string) { d.editing.Caption = s }
	entry.OnSubmitted = func(string) { d.redraw(sh) }
	return field("Caption", entry)
}

/*
letteringFields are the face, and a size, a colour and an edge for each of the
two kinds of text on the panel.

Two kinds because they are read differently: a label is a word somebody has
learned the shape of and glances past, and a reading is what they are looking
at from across the room. Making one bigger is usually a reason to leave the
other alone.
*/
func (d *DashboardsSection) letteringFields(sh *shell.Shell) fyne.CanvasObject {
	fonts := widget.NewSelect(dashboardFonts, func(picked string) {
		if picked == d.editing.Lettering.Font {
			return
		}
		d.editing.Lettering.Font = picked
		d.redraw(sh)
	})
	fonts.SetSelected(fontName(d.editing.Lettering.Font))

	return container.NewVBox(
		field("Font", fonts),
		widgets.Dim("Labels"),
		d.sizeField(sh, "Size", &d.editing.Lettering.Labels),
		d.colourField(sh, "Colour", &d.editing.Lettering.Labels),
		d.edgeField(sh, "Outline", &d.editing.Lettering.Labels),
		widgets.Dim("Readings"),
		d.sizeField(sh, "Size", &d.editing.Lettering.Values),
		d.colourField(sh, "Colour", &d.editing.Lettering.Values),
		d.edgeField(sh, "Outline", &d.editing.Lettering.Values),
		widgets.Note("A colour is #rrggbb. The coolant keeps its own green, amber "+
			"and red: that one means something.", fd.StatusInfo),
	)
}

// dashboardFonts are the faces the panel can draw in, and fontName is what
// the chooser shows for what a dashboard has: both are the service's list in
// the order it gives them.
var dashboardFonts = []string{"sans", "mono", "smallcaps"}

func fontName(font string) string {
	for _, one := range dashboardFonts {
		if one == font {
			return one
		}
	}
	return dashboardFonts[0]
}

// sizeField scales what the arrangement draws, as a percentage: the
// relationships between a panel's sizes were set by looking at one in a case,
// and this keeps them while making everything bigger or smaller.
func (d *DashboardsSection) sizeField(sh *shell.Shell, name string, text *api.DashboardText) fyne.CanvasObject {
	slider := widget.NewSlider(50, 150)
	slider.Step = 5
	slider.Value = 100
	if text.Size != 0 {
		slider.Value = float64(text.Size)
	}
	slider.OnChangeEnded = func(v float64) {
		text.Size = int(v)
		d.redraw(sh)
	}
	return field(name, slider)
}

// colourField takes a hex colour, and anything that is not one leaves the
// theme's: a dashboard typed into a colour that does not parse draws the way
// it did rather than in black on black.
func (d *DashboardsSection) colourField(sh *shell.Shell, name string, text *api.DashboardText) fyne.CanvasObject {
	entry := widget.NewEntry()
	entry.SetPlaceHolder("the theme's")
	entry.SetText(text.Colour)
	entry.OnChanged = func(s string) { text.Colour = s }
	entry.OnSubmitted = func(string) { d.redraw(sh) }
	return field(name, entry)
}

/*
edgeField is the dark edge the text carries.

A list rather than a slider, because one of its values is not a thickness:
"automatic" is two pixels over a picture and none over the theme's own
colours, which is what the panel has always drawn and what most dashboards
should keep.
*/
func (d *DashboardsSection) edgeField(sh *shell.Shell, name string, text *api.DashboardText) fyne.CanvasObject {
	choose := widget.NewSelect(edges, func(picked string) {
		text.Outline = edgeOf(picked)
		d.redraw(sh)
	})
	choose.SetSelected(edgeName(text.Outline))
	return field(name, choose)
}

var edges = []string{"automatic", "none", "1 px", "2 px", "3 px", "4 px", "6 px"}

func edgeName(outline *int) string {
	if outline == nil {
		return edges[0]
	}
	if *outline <= 0 {
		return edges[1]
	}
	return fmt.Sprintf("%d px", *outline)
}

func edgeOf(picked string) *int {
	switch picked {
	case edges[0]:
		return nil
	case edges[1]:
		none := 0
		return &none
	}
	var px int
	if _, err := fmt.Sscanf(picked, "%d px", &px); err != nil {
		return nil
	}
	return &px
}

/*
field is a control with its name beside it, at one width.

The picker's `labelled` puts a label to the left and lets it be as wide as its
own text, which is right for three rows and wrong for eleven: here the names
share a width so the controls line up down the form.
*/
func field(name string, control fyne.CanvasObject) fyne.CanvasObject {
	label := widget.NewLabel(name)
	label.Resize(fyne.NewSize(fieldLabelWidth, label.MinSize().Height))
	holder := container.NewGridWrap(
		fyne.NewSize(fieldLabelWidth, label.MinSize().Height), label)
	return container.NewBorder(nil, nil, holder, nil, control)
}

// fieldLabelWidth is wide enough for the longest field name in this form.
const fieldLabelWidth = 96

/*
preview is the frame the service drew for the draft as it stands.

**It updates in place.** The first version invalidated the section when a
frame arrived, which rebuilds every widget in it -- so a preview landing while
somebody was typing a caption or holding a dropdown open took the control out
from under them. A picture whose Resource is swapped and refreshed changes
nothing else on the page.

The last frame stays up while the next is on its way, because drawing one is a
render and a GIF encode, and a preview that blanked between edits would
flicker at exactly the rate somebody is working.
*/
func (d *DashboardsSection) preview(sh *shell.Shell) fyne.CanvasObject {
	d.picture = canvas.NewImageFromResource(d.frame)
	d.picture.FillMode = canvas.ImageFillContain
	d.picture.SetMinSize(fyne.NewSize(framePreview, framePreview))

	if d.frame == nil {
		d.cost = "Drawing it…"
	}
	// A label of this section's own rather than widgets.Dim, which returns a
	// CanvasObject: the text under the picture is changed in place, and that
	// needs the label itself.
	d.costLabel = widget.NewLabel(d.cost)
	d.costLabel.Importance = widget.LowImportance
	if d.frame == nil {
		d.redraw(sh)
	}

	return container.NewVBox(
		container.NewCenter(d.picture),
		container.NewCenter(d.costLabel),
	)
}

// framePreview is how big the frame is drawn. The panel is 640 and a window
// beside something else is not, so it is shown at about half.
const framePreview = 320

/*
redraw asks the service for a frame of the draft.

Named for what it does to the picture rather than to the panel: nothing here
touches the hardware, which is what makes an editor safe to fiddle with.
*/
func (d *DashboardsSection) redraw(sh *shell.Shell) {
	if d.editing == nil {
		return
	}
	draft := *d.editing
	name := d.name
	if name == "" {
		name = "coolant" // a name the service can resolve; the body is what is drawn
	}

	go func() {
		frame, err := d.app.client.PreviewDashboard(context.Background(), name, &draft)
		if err != nil {
			onScreen(func() { sh.Flash(err.Error(), fd.StatusWarn) })
			return
		}
		body, err := base64.StdEncoding.DecodeString(frame.Image)
		if err != nil {
			return
		}
		onScreen(func() {
			/*
				A name of its own per frame.

				Fyne caches a decoded image against its resource's name, so
				handing it a second "preview.gif" can draw the first one
				back. The counter is what makes each frame a different
				picture as far as that cache is concerned.
			*/
			d.drawn++
			d.frame = fyne.NewStaticResource(fmt.Sprintf("preview-%d.gif", d.drawn), body)
			d.cost = fmt.Sprintf("%s · the panel needs %.0fs between frames this size",
				size(int64(frame.Bytes)), frame.Floor)

			// In place. Invalidating here would rebuild the form under
			// whoever is filling it in.
			if d.picture != nil {
				d.picture.Resource = d.frame
				d.picture.Refresh()
			}
			if d.costLabel != nil {
				d.costLabel.SetText(d.cost)
			}
		})
	}()
}

func (d *DashboardsSection) save(sh *shell.Shell) { d.write(sh, false) }

func (d *DashboardsSection) saveAndShow(sh *shell.Shell) { d.write(sh, true) }

func (d *DashboardsSection) write(sh *shell.Shell, show bool) {
	if d.name == "" {
		sh.Flash("It needs a name.", fd.StatusWarn)
		return
	}
	draft := *d.editing
	draft.Name = d.name

	sh.Perform("saving "+draft.Name, func(ctx context.Context) error {
		if err := d.app.client.SaveDashboard(ctx, draft); err != nil {
			return err
		}
		if show {
			if _, err := d.app.client.UseDashboard(ctx, draft.Name); err != nil {
				return err
			}
		}
		onScreen(func() {
			d.editing, d.frame, d.cost = nil, nil, ""
			sh.Flash(draft.Name+" is saved.", fd.StatusGood)
			sh.Invalidate()
		})
		return nil
	})
}
