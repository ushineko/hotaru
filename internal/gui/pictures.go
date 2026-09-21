package gui

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/draw"
	"image/gif"
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
	"github.com/ushineko/fynedesygn/dialogs"
	"github.com/ushineko/fynedesygn/imagecache"
	"github.com/ushineko/fynedesygn/shell"
	"github.com/ushineko/fynedesygn/widgets"
	"github.com/ushineko/hotaru/internal/api"
	xdraw "golang.org/x/image/draw"
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

	/*
		waiting is the rest of a dropped stack.

		Somebody dragging four wallpapers in at once means four, and the first
		version took the first and dropped the others on the floor. They are
		asked about one at a time -- each one is a name and a decision about a
		crop -- so the rest wait here until the one in front is answered.
	*/
	waiting []dropped
}

// dropped is a file waiting to be looked at.
type dropped struct {
	name   string
	source []byte
}

// OpenPictures gives a section its app, which the shell normally does. For
// tests, like OpenEditor.
func OpenPictures(p *PicturesSection, app *App) { p.app = app }

// Waiting is how many dropped files have not been dealt with yet. For tests:
// a drop handler that quietly discarded the rest of a stack would pass any
// test that only looked at the first one.
func (p *PicturesSection) Waiting() int { return len(p.waiting) }

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
		return container.NewVBox(add, widgets.Note(err.Error(), fd.StatusWarn))
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
		container.NewVBox(add), nil, nil, nil,
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
	lights := widget.NewButtonWithIcon("Make a scene", theme.ColorPaletteIcon(), func() {
		p.scene(sh, image)
	})
	forget := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
		sh.Perform("removing "+image.Name, func(ctx context.Context) error {
			if err := p.app.client.RemoveImage(ctx, image.Name); err != nil {
				return err
			}
			onScreen(func() {
				imagecache.Shared.Forget(thumbKey(image))
				sh.Invalidate()
			})
			return nil
		})
	})

	return widgets.Card(image.Name,
		container.NewBorder(nil, nil, p.thumbnail(image), nil,
			container.NewVBox(
				widgets.Dim(strings.Join(facts, " · ")),
				widgets.Dim(image.Path),
				container.NewHBox(show, lights, forget),
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
func (p *PicturesSection) thumbnail(stored api.Image) fyne.CanvasObject {
	return pictureShot(stored)
}

// pictureShot is a stored picture at thumbnail size, for any section that
// wants one: the library, and the scene editor's screen chooser.
func pictureShot(stored api.Image) fyne.CanvasObject {
	/*
		Through the shared cache, so the thumbnails survive this section
		being rebuilt and are bounded when they do not.

		The key carries the size and the time it was added, because a
		picture replaced under the same name is a different picture and a
		key that did not change would serve the old one.
	*/
	small, err := imagecache.Shared.Get(thumbKey(stored),
		func() (image.Image, error) { return shrink(stored.Path) })
	if err != nil {
		return widgets.Dim("(cannot read it)")
	}

	picture := canvas.NewImageFromImage(small)
	picture.FillMode = canvas.ImageFillContain
	picture.SetMinSize(fyne.NewSize(thumbSize, thumbSize))
	return picture
}

/*
thumbKey names a thumbnail in the shared cache.

The size and the time it was added travel with the name, because a picture
replaced under the same name is a different picture -- and a key that did not
change would serve the old one, which is the one way to use that cache badly.
*/
func thumbKey(stored api.Image) string {
	return fmt.Sprintf("hotaru/thumb/%s/%d/%s", stored.Name, stored.Bytes, stored.Added)
}

/*
shrink reads a stored picture and returns it at thumbnail size.

The first frame, scaled down, decoded once and kept -- rather than the file's
bytes handed to Fyne, which is what this used to do.

Handing `canvas.Image` a GIF makes it decode the whole animation on every
build, at the panel's own 640x640, to draw a square 96 pixels across:
`berserk-slide` is sixty frames, so one visit to this section decoded about
25 MB of paletted images. A heap profile put 73 MB in `image.NewPaletted`
after a minute of moving between sections. See spec 027.

A thumbnail at this size is 37 KB, and it does not move -- which is the other
half of the trade. An animation is shown by the panel, not by this list, and
`Show it` is two inches to the right.
*/
func shrink(path string) (image.Image, error) {
	body, err := os.ReadFile(path) //nolint:gosec // a path the service reported
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	// gif.Decode rather than DecodeAll: the first frame is the thumbnail, and
	// decoding the rest is the cost this function exists to avoid.
	first, err := gif.Decode(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	/*
		Twice the drawn size, so it still reads on a scaled desktop, and
		nothing like the 640 the panel wants.
	*/
	const side = thumbSize * 2
	small := image.NewRGBA(image.Rect(0, 0, side, side))
	xdraw.CatmullRom.Scale(small, small.Bounds(), first, first.Bounds(), draw.Src, nil)
	return small, nil
}

/*
scene builds a scene whose lights match the picture.

The quick way to a theme, and the reason the library is worth having beyond
the panel: somebody chose a wallpaper because they liked how its colours sit
beside each other, and that judgement is the tedious half of making a scene by
hand.
*/
func (p *PicturesSection) scene(sh *shell.Shell, image api.Image) {
	makeScene(sh, image.Name, image.Name,
		func(ctx context.Context, name string, distance float64) (api.Scene, error) {
			return p.app.client.SceneFromImage(ctx, image.Name, name, distance)
		})
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
		p.waiting = append(p.waiting, dropped{name: suggested(uri.Name()), source: source})
	}

	/*
		A stack is a question, so it is asked.

		Eight wallpapers dropped at once might be eight pictures or one reel,
		and the program cannot tell which from the files. Guessing either way
		is wrong half the time and silently: eight entries somebody has to
		delete, or one animation they did not want.
	*/
	if len(p.waiting) > 1 {
		p.stack(sh)
		return
	}
	p.next(sh)
}

// stack asks what a dropped pile of pictures is.
func (p *PicturesSection) stack(sh *shell.Shell) {
	count := len(p.waiting)
	body := container.NewVBox(
		widget.NewLabel(fmt.Sprintf("%d pictures.", count)),
		widgets.DimWrapped("A slideshow holds each one in turn and fades between them, "+
			"as one picture the panel plays. Separately keeps them as "+
			"themselves, one question each."),
	)

	dialog.ShowCustomConfirm("What are these?", "One slideshow", "Keep them separately",
		body,
		func(reel bool) {
			if reel {
				p.slideshow(sh)
				return
			}
			p.next(sh)
		}, sh.Window)
}

/*
slideshow builds one animation out of everything waiting.

The pictures are already read, so the work is the encoding -- and a reel of
eight photographs with a crossfade between each is most of the panel's memory,
which is why it happens in the service where the budget is known.
*/
func (p *PicturesSection) slideshow(sh *shell.Shell) {
	sources := make([][]byte, 0, len(p.waiting))
	for _, file := range p.waiting {
		sources = append(sources, file.source)
	}
	suggestion := p.waiting[0].name
	p.waiting = nil

	entry := widget.NewEntry()
	entry.SetText(suggestion)

	dialog.ShowForm(fmt.Sprintf("A slideshow of %d pictures", len(sources)), "Make it", "Cancel",
		[]*widget.FormItem{widget.NewFormItem("Name", entry)},
		func(ok bool) {
			if !ok || entry.Text == "" {
				return
			}
			say("slideshow: %d pictures as %s", len(sources), entry.Text)
			sh.Perform("building the slideshow", func(ctx context.Context) error {
				stored, err := p.app.client.AddSlideshow(ctx, entry.Text, sources)
				if err != nil {
					return err
				}
				onScreen(func() {
					sh.Flash(fmt.Sprintf("%s is %s, %d frames.",
						stored.Name, size(stored.Bytes), stored.Frames), fd.StatusGood)
					sh.Invalidate()
				})
				return nil
			})
		}, sh.Window)
}

/*
next takes the first file still waiting and shows it.

A queue rather than a stack of dialogs. Each picture is two decisions -- what
to call it, and whether the crop kept it -- so they are asked one at a time,
and answering one brings up the next.
*/
func (p *PicturesSection) next(sh *shell.Shell) {
	if len(p.waiting) == 0 {
		return
	}
	first := p.waiting[0]
	p.waiting = p.waiting[1:]
	p.preview(sh, first.name, first.source)
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
			onScreen(func() { sh.Flash("Converting failed: "+err.Error(), fd.StatusBad) })
			return
		}
		say("preview: converted to %d bytes, %d frame(s)", len(converted), frames)
		onScreen(func() { p.keep(sh, suggestion, source, converted, frames) })
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

	title := "This is what the panel will show"
	if left := len(p.waiting); left > 0 {
		// Said, because somebody who dropped four wants to know how many
		// questions they have agreed to answer.
		title = fmt.Sprintf("%s (%d more waiting)", title, left)
	}

	confirm := dialog.NewCustomConfirm(title, "Keep it", "Cancel",
		body,
		func(ok bool) {
			if ok && entry.Text != "" {
				p.store(sh, entry.Text, source)
			}
			// Either answer moves the queue on. A cancelled picture is
			// answered as much as a kept one.
			p.next(sh)
		}, sh.Window)
	roomy(confirm, sh)
}

// store keeps a converted picture under a name.
func (p *PicturesSection) store(sh *shell.Shell, name string, source []byte) {
	sh.Perform("keeping "+name, func(ctx context.Context) error {
		stored, err := p.app.client.AddImage(ctx, name, source)
		if err != nil {
			return err
		}
		onScreen(func() {
			sh.Flash(fmt.Sprintf("%s is %s.", stored.Name, size(stored.Bytes)), fd.StatusGood)
			sh.Invalidate()
		})
		return nil
	})
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
roomy shows a dialog at most of the window.

fynedesygn's, since its spec 026: the same fault was found there in a file
browser and here in a chooser of shortcuts, and two programs had two copies
of the workaround. Kept as a name of this package's own because every call
site already reads `roomy(d, sh)`, and because a shell is what this window
has to hand where the library takes a window.
*/
func roomy(d dialog.Dialog, sh *shell.Shell) { dialogs.Roomy(d, sh.Window) }

// Roomy is roomy, exported so a test can call it on a real dialog: the bug it
// exists to prevent is a nil dereference inside Fyne, which nothing short of
// the real type reproduces.
func Roomy(d dialog.Dialog, sh *shell.Shell) { roomy(d, sh) }

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
