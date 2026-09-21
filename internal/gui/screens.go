package gui

import (
	"context"
	"image"
	"sync"

	"fyne.io/fyne/v2"
	"github.com/ushineko/fynedesygn/imagecache"
	"github.com/ushineko/hotaru/internal/api"
)

/*
screens is everything the panel can be asked to show: the saved dashboards and
the stored pictures, fetched together.

They arrive together because the chooser asks one question -- what goes on the
screen when this scene is on -- and from the scene's side a dashboard and a
picture are the same kind of answer.
*/
type screens struct {
	boards   []api.Dashboard
	pictures []api.Image
}

/*
held is the chooser's list, kept so opening it twice costs one fetch.

**The chooser used to ask the service once per option.** It fetched the
dashboards and the pictures to build the list, and then, for every line in it,
called the same two routes again to find the one picture that line needed --
twenty-odd round trips on the UI thread before anything appeared, which is the
pause this fixes.

It is dropped and re-warmed in Arrive, so a picture added in the Pictures tab
is in the list by the time the Scenes tab is on screen.
*/
func (s *ScenesSection) screens() screens {
	s.knownMu.Lock()
	known := s.known
	s.knownMu.Unlock()
	if known != nil {
		return *known
	}
	return s.fetchScreens()
}

// fetchScreens asks the service and keeps the answer.
func (s *ScenesSection) fetchScreens() screens {
	var got screens
	if boards, err := s.app.client.Dashboards(context.Background()); err == nil {
		got.boards = boards.Dashboards
	}
	if stored, err := s.app.client.Images(context.Background()); err == nil {
		got.pictures = stored
	}

	s.knownMu.Lock()
	s.known = &got
	s.knownMu.Unlock()
	return got
}

/*
warmScreens fetches the list, and the thumbnails for it, off the UI thread.

Arriving at the section is the moment somebody might open the chooser, and a
picture decoded now is a picture the chooser does not stop to decode later.
Failures are ignored: this is a head start, not the fetch.
*/
func (s *ScenesSection) warmScreens() {
	if s.app == nil {
		return
	}
	got := s.fetchScreens()
	for _, stored := range got.pictures {
		_, _ = imagecache.Shared.Get(thumbKey(stored), func() (image.Image, error) {
			return shrink(stored.Path)
		})
	}
}

// forgetScreens drops the kept list, so the next look is a fresh one.
func (s *ScenesSection) forgetScreens() {
	s.knownMu.Lock()
	s.known = nil
	s.knownMu.Unlock()
}

// kept is the section's side of screens: the list and the lock over it.
type kept struct {
	known   *screens
	knownMu sync.Mutex
}

/*
Beside stacks the chooser's thumbnails at the radio group's own pitch.

A VBox puts padding between its children and Fyne's radio group puts none
between its options, so a column of pictures built as a VBox drifts a few
pixels further out of line with every row -- by the tenth option the picture
is beside the wrong name. This lays them at exactly the height one option
takes.
*/
type Beside struct{ Pitch float32 }

// MinSize is one cell's width by a cell per object.
func (b Beside) MinSize(objects []fyne.CanvasObject) fyne.Size {
	width := float32(0)
	for _, o := range objects {
		width = fyne.Max(width, o.MinSize().Width)
	}
	return fyne.NewSize(width, b.Pitch*float32(len(objects)))
}

// Layout puts each object in its own cell, in order, with nothing between
// them.
func (b Beside) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	for i, o := range objects {
		o.Resize(fyne.NewSize(size.Width, b.Pitch))
		o.Move(fyne.NewPos(0, b.Pitch*float32(i)))
	}
}
