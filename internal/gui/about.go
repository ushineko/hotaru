package gui

import (
	"time"

	"fyne.io/fyne/v2"
	"github.com/ushineko/fynedesygn/markdown"
	"github.com/ushineko/fynedesygn/mermaid"
	"github.com/ushineko/fynedesygn/shell"
	"github.com/ushineko/hotaru"
	"github.com/ushineko/hotaru/internal/version"
)

// Project is where hotaru lives, and the one thing in the window that outlives
// the machine it is running on.
const Project = "https://github.com/ushineko/hotaru"

/*
settleResize coalesces the re-measure a width change forces.

Fyne hands a widget a Resize for every step of a window drag, from inside the
event poll, and measuring a document means rendering every block to ask its
height. Without this the window moves in bursts while the README is on screen.
*/
const settleResize = 120 * time.Millisecond

/*
about is the README, shown.

Not a restatement of it. A program that describes itself twice has one
description somebody maintains and another they forget, and the one in the
window is the one that goes stale -- so the front page is embedded and
rendered, diagram and all, rather than summarised into a paragraph nobody
checks.
*/
func about(socket string) shell.Section {
	return still{readme(socket)}
}

/*
still is a section that nothing on the machine can change.

The window polls every two seconds and rebuilds whatever is on screen when
something it watches moves; a section that says nothing about what it watches
is rebuilt whenever anything does, which on a machine with a running pump is
every poll. For the README that is a document torn down and built again under
whoever is reading it, twice a minute, for numbers it does not draw.
*/
type still struct{ *shell.FuncSection }

// Changed implements Watcher: nothing here follows the machine.
func (still) Changed(_, _ Snapshot) bool { return false }

func readme(socket string) *shell.FuncSection {
	var pane *markdown.Pane

	section := shell.AboutSection(shell.About{
		Icon:    Icon(),
		Name:    "hotaru",
		Version: version.Version,
		Blurb:   "lights, cooler, action!",
		URL:     Project,
		URLText: "github.com/ushineko/hotaru",
		Facts: []shell.Fact{
			{Label: "Licence", Value: "MIT"},
			{Label: "Service", Value: socket},
		},
		Extra: func(s *shell.Shell) fyne.CanvasObject {
			pane = markdown.New(document())
			// One scrollbar for the section, rather than a document that
			// scrolls inside a page that also scrolls.
			pane.Follow(s.Scroller())
			return pane
		},
	})

	return section.OnDetach(func() {
		if pane != nil {
			pane.Detach()
			pane = nil
		}
	})
}

/*
document is the README and where its pictures come from.

Apart so a test can render a block and ask what it became: a mermaid fence
whose image is missing draws as its own source with an apology, which is a
failure nothing else reports.
*/
func document() (string, markdown.Options) {
	return hotaru.README, markdown.Options{
		FS:           hotaru.Diagrams,
		Diagrams:     mermaid.NewSet(hotaru.Diagrams, "diagrams"),
		SettleResize: settleResize,
	}
}
