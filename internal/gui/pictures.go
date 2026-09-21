package gui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/shell"
	"github.com/ushineko/fynedesygn/widgets"
	"github.com/ushineko/hotaru/internal/api"
)

/*
PicturesSection is what the cooler's screen can show.

The panel takes 640x640 GIFs and nothing else, and refuses everything else by
displaying nothing at all -- so a wallpaper has to be converted before it can
be shown. That happens in the service, because the CLI can do it too; this is
where somebody picks a file and looks at the result.
*/
type PicturesSection struct {
	app *App

	// thumbs are the converted pictures, drawn from the stored files. Kept
	// between rebuilds: re-reading and re-decoding a megabyte of GIFs on
	// every poll would make the section cost more than it shows.
	thumbs map[string]fyne.Resource
}

// OpenPictures gives a section its app, which the shell normally does. For
// tests, like OpenEditor.
func OpenPictures(p *PicturesSection, app *App) { p.app = app }

// Title is the name in the navigation.
func (p *PicturesSection) Title() string { return "Pictures" }

// Icon is the navigation's icon for this section.
func (p *PicturesSection) Icon() fyne.Resource { return theme.FileImageIcon() }

// Changed says this section draws the library, which moves when somebody adds
// or removes something rather than when a fan speeds up.
func (p *PicturesSection) Changed(before, after Snapshot) bool {
	return (before.Err == nil) != (after.Err == nil)
}

// Build draws the library and the way into it.
func (p *PicturesSection) Build(sh *shell.Shell) fyne.CanvasObject {
	got := p.app.machine.Read()
	if got.Err != nil {
		return notRunning(got.Err)
	}

	add := widget.NewButtonWithIcon("Add a picture", theme.ContentAddIcon(), func() {
		p.add(sh)
	})
	add.Importance = widget.HighImportance

	stored, err := p.app.client.Images(context.Background())
	if err != nil {
		return container.NewVBox(title("Pictures"), add, widgets.Note(err.Error(), fd.StatusWarn))
	}

	var body []fyne.CanvasObject
	if len(stored) == 0 {
		body = append(body, widgets.Note(
			"Any JPEG, PNG or GIF. It is scaled to the panel and cropped to the middle.",
			fd.StatusInfo))
	}
	for _, image := range stored {
		body = append(body, p.card(sh, image))
	}

	return container.NewBorder(
		container.NewVBox(title("Pictures"), add), nil, nil, nil,
		container.NewVScroll(container.NewVBox(body...)),
	)
}

// card is one stored picture: what it looks like, what it costs, and the two
// things worth doing with it.
func (p *PicturesSection) card(sh *shell.Shell, image api.Image) fyne.CanvasObject {
	facts := []string{size(image.Bytes)}
	if image.Frames > 1 {
		facts = append(facts, fmt.Sprintf("%d frames", image.Frames))
	}

	show := widget.NewButtonWithIcon("Show it", theme.VisibilityIcon(), func() {
		p.show(sh, image)
	})
	forget := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
		sh.Perform("removing "+image.Name, func(ctx context.Context) error {
			if err := p.app.client.RemoveImage(ctx, image.Name); err != nil {
				return err
			}
			delete(p.thumbs, image.Name)
			sh.Invalidate()
			return nil
		})
	})

	return widgets.Card(image.Name,
		container.NewBorder(nil, nil, p.thumbnail(image), nil,
			container.NewVBox(
				widgets.Dim(strings.Join(facts, " · ")),
				widgets.Dim(image.Path),
				container.NewHBox(show, forget),
			),
		),
	)
}

// thumbSize is how big a stored picture is drawn in the listing. Small enough
// that a dozen fit on a screen, big enough to tell two wallpapers apart.
const thumbSize = 96

/*
thumbnail draws the converted picture itself.

The converted one, not the original: what somebody needs to see is what the
panel will show, which is the whole reason a conversion is worth previewing.
*/
func (p *PicturesSection) thumbnail(image api.Image) fyne.CanvasObject {
	if p.thumbs == nil {
		p.thumbs = map[string]fyne.Resource{}
	}
	resource, drawn := p.thumbs[image.Name]
	if !drawn {
		body, err := os.ReadFile(image.Path) //nolint:gosec // a path the service reported
		if err != nil {
			return widgets.Dim("(cannot read it)")
		}
		resource = fyne.NewStaticResource(image.Name+".gif", body)
		p.thumbs[image.Name] = resource
	}

	picture := canvas.NewImageFromResource(resource)
	picture.FillMode = canvas.ImageFillContain
	picture.SetMinSize(fyne.NewSize(thumbSize, thumbSize))
	return picture
}

/*
show puts a picture on the panel for as long as a modal is open.

The same shape as previewing a scene, and for the same reason: a screen
somebody has to remember to put back is a screen that stays wrong. Dismissing
the modal hands the panel back to the dashboard.
*/
func (p *PicturesSection) show(sh *shell.Shell, image api.Image) {
	sh.Perform("showing "+image.Name, func(ctx context.Context) error {
		return p.app.client.ShowImage(ctx, image.Name)
	})

	body := container.NewVBox(
		widget.NewLabel(image.Name),
		widgets.Dim("Close this to put the dashboard back."),
	)
	modal := dialog.NewCustom("On the screen", "Done", body, sh.Window)
	modal.SetOnClosed(func() {
		sh.Perform("putting the dashboard back", func(ctx context.Context) error {
			return p.app.client.Screen(ctx, api.ScreenRequest{Dashboard: true})
		})
	})
	modal.Show()
}

/*
Dropped is a file dragged onto the window from a file manager.

Window-wide rather than a target somewhere in the Pictures section: a person
dragging a wallpaper at a program is not aiming at a rectangle, and a drop that
lands two pixels outside one and does nothing is the program being pedantic
about something it could simply accept.

Anything that is not an image is refused by name, because a directory of
holiday photographs contains one thing that is not a photograph.
*/
func (p *PicturesSection) Dropped(sh *shell.Shell, uris []fyne.URI) {
	for _, uri := range uris {
		path := uri.Path()
		say("dropped: %s", path)

		source, err := os.ReadFile(path) //nolint:gosec // a file the user dragged in
		if err != nil {
			sh.Flash("Cannot read "+filepath.Base(path)+": "+err.Error(), fd.StatusBad)
			continue
		}
		// One at a time: each one is a decision -- a name, and whether the
		// crop kept the picture -- and a stack of dialogs is not a queue
		// anybody can work through.
		p.preview(sh, suggested(uri.Name()), source)
		return
	}
}

/*
add converts a file and keeps it.

The name is offered from the filename, because somebody choosing
"Jovian_single_3840x2160.jpg" means "jovian" and should not have to say so.
*/
func (p *PicturesSection) add(sh *shell.Shell) {
	say("add: opening the file chooser")
	open := dialog.NewFileOpen(func(file fyne.URIReadCloser, err error) {
		say("add: chooser returned err=%v file=%v", err, file)
		if err != nil || file == nil {
			return
		}
		defer func() { _ = file.Close() }()

		source, err := os.ReadFile(file.URI().Path()) //nolint:gosec // a file the user chose
		if err != nil {
			say("add: cannot read it: %v", err)
			sh.Flash(err.Error(), fd.StatusBad)
			return
		}
		say("add: read %d bytes from %s", len(source), file.URI().Path())
		p.preview(sh, suggested(file.URI().Name()), source)
	}, sh.Window)

	// Grid view because the alternative is a column of filenames. Fyne's
	// picker draws a generic badge per file rather than a thumbnail, so the
	// picture somebody is choosing is not visible until it is chosen -- which
	// is what the conversion preview below is for.
	open.SetView(dialog.GridView)
	open.SetFilter(storage.NewExtensionFileFilter([]string{".jpg", ".jpeg", ".png", ".gif"}))
	roomy(open, sh)
}

/*
preview converts the picture and shows the result before anything is kept.

The conversion is the thing worth looking at. A wallpaper is wide and the panel
is square, so what arrives on the cooler is the middle of the picture in 256
colours -- and whether that is still the picture somebody wanted is a question
only they can answer, in front of the answer.

Nothing is stored until they say so: the service converts and hands it back.
*/
func (p *PicturesSection) preview(sh *shell.Shell, suggestion string, source []byte) {
	/*
		A goroutine rather than Perform, and that is not a shortcut.

		Perform puts a modal popup up for the duration, and Fyne's overlay
		stack drops everything above an overlay when that overlay is removed
		-- so a dialog opened from inside the operation is taken down by the
		busy popup finishing. The conversion worked, the dialog was built,
		and the window did nothing: every step logged success.

		The work still stays off the UI thread, which is what the rule is
		about, and a conversion takes a third of a second.
	*/
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), convertWithin)
		defer cancel()

		say("preview: converting %d bytes", len(source))
		converted, frames, err := p.app.client.ConvertImage(ctx, source)
		if err != nil {
			say("preview: conversion failed: %v", err)
			fyne.Do(func() { sh.Flash("Converting failed: "+err.Error(), fd.StatusBad) })
			return
		}
		say("preview: converted to %d bytes, %d frame(s)", len(converted), frames)
		fyne.Do(func() { p.keep(sh, suggestion, source, converted, frames) })
	}()
}

// convertWithin bounds a conversion. A wallpaper takes a third of a second and
// a long animation rather more; this is the point at which something is wrong
// rather than slow.
const convertWithin = 60 * time.Second

// keep shows the converted picture, asks what to call it, and stores it.
func (p *PicturesSection) keep(sh *shell.Shell, suggestion string, source, converted []byte, frames int) {
	say("keep: showing the conversion")
	picture := canvas.NewImageFromResource(fyne.NewStaticResource("preview.gif", converted))
	picture.FillMode = canvas.ImageFillContain
	picture.SetMinSize(fyne.NewSize(previewSize, previewSize))

	entry := widget.NewEntry()
	entry.SetText(suggestion)

	facts := size(int64(len(converted)))
	if frames > 1 {
		facts += fmt.Sprintf(" · %d frames", frames)
	}

	body := container.NewVBox(
		picture,
		widgets.Dim("640x640, cropped to the middle · "+facts),
		container.NewBorder(nil, nil, widget.NewLabel("Name"), nil, entry),
	)

	confirm := dialog.NewCustomConfirm("This is what the panel will show", "Keep it", "Cancel",
		body,
		func(ok bool) {
			if !ok || entry.Text == "" {
				return
			}
			sh.Perform("keeping "+entry.Text, func(ctx context.Context) error {
				stored, err := p.app.client.AddImage(ctx, entry.Text, source)
				if err != nil {
					return err
				}
				delete(p.thumbs, stored.Name)
				sh.Flash(fmt.Sprintf("%s is %s.", stored.Name, size(stored.Bytes)), fd.StatusGood)
				sh.Invalidate()
				return nil
			})
		}, sh.Window)
	roomy(confirm, sh)
}

// previewSize is how big the conversion is shown. Large enough to judge a
// crop by, which is the decision being asked for.
const previewSize = 360

// suggested is a name for a file, which is its own minus the extension.
func suggested(filename string) string {
	return strings.TrimSuffix(filename, filepath.Ext(filename))
}

// size is a byte count as somebody reads it. The number matters: the panel's
// refresh floor scales with it.
func size(bytes int64) string {
	switch {
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(bytes)/(1<<10))
	}
	return fmt.Sprintf("%d bytes", bytes)
}

/*
roomy shows a dialog at most of the window's size.

Fyne opens one at a size that suits a confirmation, and a file browser is not
one: the default shows about four names, so a directory of wallpapers is read
through a slot.

**Shown first, then resized.** A FileDialog builds its window in Show and its
Resize dereferences it, so sizing one before showing it is a nil pointer --
which is a crash rather than a small dialog, and I put it in front of somebody.
*/
func roomy(d dialog.Dialog, sh *shell.Shell) { Roomy(d, sh) }

// Roomy is roomy, exported so a test can call it on a real dialog: the bug it
// exists to prevent is a nil dereference inside Fyne, which nothing short of
// the real type reproduces.
func Roomy(d dialog.Dialog, sh *shell.Shell) {
	d.Show()

	size := sh.Window.Canvas().Size()
	d.Resize(fyne.NewSize(
		max32(size.Width*0.85, minDialogWidth),
		max32(size.Height*0.85, minDialogHeight),
	))
}

// The floor, for a dialog opened over a window somebody has made small.
const (
	minDialogWidth  = 720
	minDialogHeight = 520
)

func max32(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

/*
say puts a line on stderr.

Diagnostics, kept rather than removed. A window reports its failures as banners
and says nothing at all about the steps in between, so "it doesn't do
anything" is a sentence with no way in -- which is exactly the state the LCD
was in until one journal line per push made it obvious. The lines are cheap and
the alternative is guessing.
*/
func say(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "hotaru-gui: "+format+"\n", args...)
}
