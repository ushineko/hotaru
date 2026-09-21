package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/ushineko/fynedesygn/shell"
)

/*
Create is the three sections somebody makes things with, under one entry.

Pictures, Screen and Scenes were three of the window's seven entries and are
one activity: a picture becomes something the panel shows, a dashboard is
another thing it shows, and a scene is the two of them plus the lights. Read
down the navigation they looked like three unrelated places; read in order
they are the order somebody does the work in.

**The order is the order.** Pictures first because a picture is the thing you
bring from outside, Screen next because it is what the panel does with one,
Scenes last because a scene names both.

A tab strip rather than a nested navigation. The shell's navigation is one
list and adding a second level to it would change every program on the
module; this is one section that draws one of three, which is a thing a
section may do.
*/
type Create struct {
	app *App
	// parts are the three, in order.
	parts []Section
	// at is the one showing. It outlives a rebuild, because a rebuild is not
	// somebody changing tabs.
	at int
}

/*
Section is what Create holds: a shell section that also reports what it
watches, which every section of this window does.

Declared here rather than taken from the shell because Create has to pass the
machine's snapshot through to whichever part is showing, and a plain
shell.Section cannot say whether it wants one.
*/
type Section interface {
	shell.Section
	Changed(before, after Snapshot) bool
}

// NewCreate builds the group in the order the work is done in.
func NewCreate(app *App) *Create {
	return &Create{app: app, parts: []Section{
		&PicturesSection{app: app},
		&DashboardsSection{app: app},
		&ScenesSection{app: app},
	}}
}

/*
Parts are the three, for the shell's own navigation to reach and for tests.

A test that asked the window for "Scenes" used to get a section; now it gets
this group, and a test that had to press a tab to find one would be a test
about tabs.
*/
func (c *Create) Parts() []Section { return c.parts }

// Show puts one of the parts in front, by title. For tests, and for anything
// that wants to send somebody to a particular one.
func (c *Create) Show(title string) bool {
	for i, part := range c.parts {
		if part.Title() == title {
			c.at = i
			return true
		}
	}
	return false
}

// Title is the name in the navigation.
func (c *Create) Title() string { return "Create" }

// Icon is the navigation's icon for this section.
func (c *Create) Icon() fyne.Resource { return theme.ContentAddIcon() }

// Changed asks whichever part is showing, because that is what is drawn.
func (c *Create) Changed(before, after Snapshot) bool {
	return c.current().Changed(before, after)
}

// Busy asks the part showing, so an editor open in one of them stops the
// poll rebuilding it.
func (c *Create) Busy() bool {
	if busy, ok := c.current().(Busy); ok {
		return busy.Busy()
	}
	return false
}

/*
Detach tells every part, not only the one showing.

The shell detaches every section on every swap for the same reason: a part
that was showing a moment ago may still hold a scroll callback or a lease,
and "it is not on screen" is exactly when that matters.
*/
func (c *Create) Detach() {
	for _, part := range c.parts {
		if d, ok := part.(shell.Detacher); ok {
			d.Detach()
		}
	}
}

// Arrive tells the part showing that the navigation reached it.
func (c *Create) Arrive() {
	if a, ok := c.current().(shell.Arriver); ok {
		a.Arrive()
	}
}

// Tick passes the machine through to the part showing.
func (c *Create) Tick(got Snapshot) {
	if ticker, ok := c.current().(Ticker); ok {
		ticker.Tick(got)
	}
}

// Build draws the tab strip and whichever part is chosen.
func (c *Create) Build(sh *shell.Shell) fyne.CanvasObject {
	tabs := make([]fyne.CanvasObject, 0, len(c.parts))
	for i, part := range c.parts {
		button := widget.NewButton(part.Title(), func() {
			if c.at == i {
				return
			}
			/*
				Changing tabs is arriving somewhere, so the part that was
				showing is detached and the new one told it arrived --
				the same two calls the shell makes when the navigation
				moves, because this is the navigation moving.
			*/
			c.Detach()
			c.at = i
			c.Arrive()
			sh.Invalidate()
		})
		if i == c.at {
			button.Importance = widget.HighImportance
		}
		tabs = append(tabs, button)
	}

	return container.NewBorder(
		container.NewHBox(tabs...), nil, nil, nil,
		c.current().Build(sh),
	)
}

// current is the part showing, clamped in case the list ever shortens.
func (c *Create) current() Section {
	if c.at < 0 || c.at >= len(c.parts) {
		c.at = 0
	}
	return c.parts[c.at]
}
