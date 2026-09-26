package gui

import (
	"context"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/imagecache"
	"github.com/ushineko/fynedesygn/shell"
	"github.com/ushineko/fynedesygn/widgets"
	"github.com/ushineko/hotaru/internal/api"
)

/*
Renaming and cloning, in the window.

Three sections ask the same question -- what should this be called instead --
and the answer travels the same way: one call, which carries the references
with it, and a line saying how many moved. See spec 053.
*/

// The name dialog's size. One field and a note, and no wider than the words
// under it need.
const (
	nameDialogWidth  = 420
	nameDialogHeight = 200
)

/*
naming is what the name dialog needs to ask its question.

Filled and Unchanged are two different things, and collapsing them into one
name is a bug this had: a copy offers "planet1-copy" in the field, so taking
the offer *is* the answer -- and a dialog that treated what it had filled in
as "nothing was asked for" closed and did nothing, silently, on the one path
somebody was most likely to take.

A rename is the case where they are the same name, and that is a fact about a
rename rather than about the dialog.
*/
type naming struct {
	// Title and Confirm are what the dialog says and what its button says.
	Title, Confirm string

	// Filled is what is in the field when it opens. The current name for a
	// rename, because most renames are an edit of what is there -- a typo, a
	// word added -- and retyping it to change one letter is the sort of small
	// insult a program should not offer. A suggestion for a copy.
	Filled string

	// Unchanged is the name that means nothing was asked for, and is empty
	// where there is no such name.
	Unchanged string

	// Note is the line under the field: what else the answer will move.
	Note string
}

/*
wanted is the name to act on, or empty when nothing was asked for.

Split out of the dialog so the rule can be tested without driving a window,
which is what let the bug above sit behind a button.
*/
func (n naming) wanted(ok bool, typed string) string {
	name := strings.TrimSpace(typed)
	if !ok || name == "" || name == n.Unchanged {
		return ""
	}
	return name
}

// askName asks what something should be called, and calls back with the
// answer when there is one.
func askName(sh *shell.Shell, asking naming, then func(name string)) {
	entry := widget.NewEntry()
	entry.SetText(asking.Filled)

	body := container.NewVBox(labelled("Name", entry), widgets.DimWrapped(asking.Note))
	ask := dialog.NewCustomConfirm(asking.Title, asking.Confirm, "Cancel", body,
		func(ok bool) {
			if name := asking.wanted(ok, entry.Text); name != "" {
				then(name)
			}
		}, sh.Window)
	ask.Resize(fyne.NewSize(nameDialogWidth, nameDialogHeight))
	ask.Show()
}

/*
renamedLine is a rename, in one line.

The counts only when there are any. A rename of something nothing points at
is the ordinary case, and a sentence saying nothing else was updated is a
sentence somebody reads every time to learn nothing.
*/
func renamedLine(done api.Renamed) string {
	line := done.From + " is now " + done.To + "."
	if done.Changed != "" {
		line += " " + done.Changed + "."
	}
	return line
}

/*
forgetThumbnail drops what the window drew for a picture under a name it no
longer has.

The key carries the name, so the thumbnail cached under the old one is a
picture nothing can ask for again -- and the cache would hold it until
something else pushed it out. Deleting a picture wants the same thing for the
same reason, which is why this is one function and not two calls.
*/
func forgetThumbnail(stored api.Image) { imagecache.Shared.Forget(thumbKey(stored)) }

// rename moves a scene, and the keys bound to it with it.
func (s *ScenesSection) rename(sh *shell.Shell, scene api.Scene) {
	askName(sh, naming{
		Title: "Rename " + scene.Name, Confirm: "Rename",
		Filled: scene.Name, Unchanged: scene.Name,
		Note: "Any key bound to this scene follows it.",
	}, func(to string) {
		sh.Perform("renaming "+scene.Name, func(ctx context.Context) error {
			done, err := s.app.client.RenameScene(ctx, scene.Name, to)
			if err != nil {
				return err
			}
			onScreen(func() {
				sh.Flash(renamedLine(done), fd.StatusGood)
				sh.Invalidate()
			})
			return nil
		})
	})
}

// rename moves a screen, and the scenes naming it with it.
func (d *DashboardsSection) rename(sh *shell.Shell, one api.Dashboard) {
	askName(sh, naming{
		Title: "Rename " + one.Name, Confirm: "Rename",
		Filled: one.Name, Unchanged: one.Name,
		Note: "Any scene that puts this screen up follows it.",
	}, func(to string) {
		sh.Perform("renaming "+one.Name, func(ctx context.Context) error {
			done, err := d.app.client.RenameDashboard(ctx, one.Name, to)
			if err != nil {
				return err
			}
			onScreen(func() {
				sh.Flash(renamedLine(done), fd.StatusGood)
				sh.Invalidate()
			})
			return nil
		})
	})
}

/*
clone copies a screen under another name.

The shipped ones included, which is how somebody starts from one. The copy is
not put on the panel: copying a screen is not asking to see it.
*/
func (d *DashboardsSection) clone(sh *shell.Shell, one api.Dashboard) {
	/*
		Unchanged is the screen's own name, not the name in the field.

		The field holds a suggestion, and taking a suggestion is an answer.
		Only asking for the name it already has is asking for nothing, and
		the service refuses that with a sentence rather than a closed dialog.
	*/
	askName(sh, naming{
		Title: "Copy " + one.Name, Confirm: "Copy it",
		Filled: one.Name + "-copy", Unchanged: one.Name,
		Note: "The same screen under another name, yours to change. " +
			"It is not put on the panel.",
	}, func(to string) {
		sh.Perform("copying "+one.Name, func(ctx context.Context) error {
			made, err := d.app.client.CloneDashboard(ctx, one.Name, to)
			if err != nil {
				return err
			}
			onScreen(func() {
				sh.Flash(made.Name+" is a copy of "+one.Name+".", fd.StatusGood)
				sh.Invalidate()
			})
			return nil
		})
	})
}

/*
rename moves a picture, and everything showing it.

The thumbnail is dropped with it. It is cached under a key built from the
name, so the one under the old name is a picture nothing can reach again --
and the window would otherwise hold it until something else pushed it out.
*/
func (p *PicturesSection) rename(sh *shell.Shell, image api.Image) {
	askName(sh, naming{
		Title: "Rename " + image.Name, Confirm: "Rename",
		Filled: image.Name, Unchanged: image.Name,
		Note: "Any scene showing it, and any screen drawing it behind the " +
			"numbers, follows it.",
	}, func(to string) {
		sh.Perform("renaming "+image.Name, func(ctx context.Context) error {
			done, err := p.app.client.RenameImage(ctx, image.Name, to)
			if err != nil {
				return err
			}
			onScreen(func() {
				forgetThumbnail(image)
				sh.Flash(renamedLine(done), fd.StatusGood)
				sh.Invalidate()
			})
			return nil
		})
	})
}
