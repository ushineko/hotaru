package gui

import (
	"context"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"github.com/ushineko/fynedesygn/shell"
	"github.com/ushineko/hotaru/internal/api"
	"github.com/ushineko/hotaru/internal/version"
)

/*
App is the window's state: a client, what it last heard, and the poll.

One object rather than a package of globals so a headless test can build two.
*/
type App struct {
	client  *api.Client
	machine *Machine
	shell   *shell.Shell

	// every is the poll interval. /v1/events is in the architecture and is
	// not built; until it is, the window asks. Stated rather than hidden,
	// because a GUI polling a service that polls hardware is two intervals
	// somebody will eventually have to reason about.
	every time.Duration
	stop  chan struct{}

	// preview is the window's hold on a draft the hardware is showing.
	preview preview

	// drawn is the snapshot the window is currently showing, so a poll that
	// changed nothing can rebuild nothing.
	drawn Snapshot

	// create is the group the picture library lives in, so a dropped file
	// can send the window to the part that takes it rather than to a
	// section title that stopped existing when it was grouped.
	create *Create

	// pictures takes files dropped on the window, wherever the navigation
	// happens to be.
	pictures *PicturesSection
}

/*
AppID is the window's application ID.

One string in three places: Fyne sets it as the Wayland app_id, the desktop
entry is named for it, and that entry's StartupWMClass repeats it. They have to
agree or the icon appears in one place and not the others.
*/
const AppID = "io.github.ushineko.hotaru"

// Poll is how often the window asks the service what it can see. Slow enough
// to be free, fast enough that a device appearing shows up while somebody is
// still looking at the window.
const Poll = 2 * time.Second

// Machine is the last snapshot the poll took, for tests that build a section
// the way the window does.
func (a *App) Machine() Snapshot { return a.machine.Read() }

// Client is the service this window talks to. For tests, which ask the same
// service what the window's actions did to it.
func (a *App) Client() *api.Client { return a.client }

// New builds the app around a client.
func New(client *api.Client) *App {
	return &App{client: client, machine: &Machine{}, every: Poll}
}

// Options describes the window to fynedesygn's shell.
func (a *App) Options(socket string) shell.Options {
	create := NewCreate(a)

	// By type rather than by position: a dropped file goes to the library
	// wherever the library happens to sit in the tab strip, and the strip's
	// order is about what somebody opens most rather than about this.
	var pictures *PicturesSection
	for _, part := range create.parts {
		if found, ok := part.(*PicturesSection); ok {
			pictures = found
		}
	}

	return shell.Options{
		AppID:   AppID,
		Name:    "hotaru",
		Version: version.Version,
		Icon:    Icon(),
		Sections: []shell.Section{
			&ServiceSection{app: a},
			&SystemSection{app: a},
			/*
				Pictures, Screen and Scenes under one entry, in the order
				the work is done in: a picture is what you bring from
				outside, the screen is what the panel does with one, and a
				scene names both. See create.go.
			*/
			create,
			/*
				The standard Appearance section, as every program on this
				module has.

				Fyne draws its own widgets, so the scheme, the font and the
				text size are the whole of what makes this window look like
				it belongs on the desktop it is running on -- which is why
				they are a section and not a line in a preferences dialog.

				The sample is a line of hotaru's own output, because what
				somebody is choosing a monospace font for here is reading
				that.
			*/
			/*
				Wrapped, because the poll rebuilds whatever is on screen
				unless the section says what it watches -- and a rebuild
				takes the page back to the top under somebody who is
				scrolling it. The appearance does not follow the machine.
			*/
			still{shell.AppearanceSection("hotaru: keys: 18 shortcuts registered")},
			about(socket),
		},
		SettingsPath: a.settingsPath(),

		/*
			Every shape fynedesygn offers, and the control that comes with
			listing more than one.

			Three sections is few enough that icons alone stay legible, and a
			window somebody keeps open beside something else is a window they
			may want the navigation out of entirely.
		*/
		NavModes:      []shell.NavMode{shell.NavLabels, shell.NavIcons, shell.NavHidden},
		NavPlacements: []shell.NavPlacement{shell.NavLeft, shell.NavTop},
		OnCreate: func(s *shell.Shell) {
			a.shell, a.pictures, a.create = s, pictures, create
		},
		OnStart: func(s *shell.Shell) {
			/*
				A picture dragged onto the window from a file manager.

				Registered here rather than by the section itself: the
				callback belongs to the window, and a section that set it
				would leave it pointing at itself long after the navigation
				moved on.
			*/
			if s.Window != nil {
				s.Window.SetOnDropped(func(_ fyne.Position, uris []fyne.URI) {
					a.dropped(s, uris)
				})
			}
			a.Start(context.Background())
		},
		OnStop:       func(*shell.Shell) { a.Stop() },
		StatusBar:    func(*shell.Shell) []fyne.CanvasObject { return a.status(socket) },
		OnInvalidate: func(*shell.Shell) { a.refresh(context.Background()) },
	}
}

/*
Start begins polling, and stops when the window is not on screen.

A background window that keeps a service busy is a background window somebody
closes for the wrong reason. The check is the shell's own OnScreen, so a
minimised or hidden window costs nothing.
*/
func (a *App) Start(ctx context.Context) {
	a.refresh(ctx)
	a.stop = make(chan struct{})

	go func() {
		ticker := time.NewTicker(a.every)
		defer ticker.Stop()
		for {
			select {
			case <-a.stop:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				if a.shell != nil && !a.shell.OnScreen() {
					continue
				}
				a.refresh(ctx)
				a.redraw()
			}
		}
	}()
}

// Stop ends the poll. Called when the window closes, and safe twice.
func (a *App) Stop() {
	if a.stop != nil {
		close(a.stop)
		a.stop = nil
	}
}

/*
Refresh asks the service what it can see, once.

Exported because a test drives it: the poll is a goroutine and a ticker, and a
test that waited on one would be a test about timing rather than about what the
window says.
*/
func (a *App) Refresh(ctx context.Context) { a.refresh(ctx) }

func (a *App) refresh(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	a.machine.Refresh(ctx, a.client)
}

/*
redraw rebuilds the current section, and only when the machine moved.

Rebuilding is not free to look at: the section is built fresh, so a list
rebuilt under a pointer jumps and the scene somebody was about to click moves
out from under them. A two-second poll did that to a list of eighteen. An idle
machine now rebuilds nothing at all.

A section in the middle of something -- an open editor, a colour being dragged
-- says so and is left alone even when the machine did move. Its own work is
worth more than a fresher reading.

fyne.Do, never DoAndWait: the poll goroutine must not wait on the thread it is
feeding.
*/
func (a *App) redraw() {
	if a.shell == nil || !a.shell.OnScreen() {
		return
	}
	got := a.machine.Read()
	current := a.shell.Current()

	if busy, ok := current.(Busy); ok && busy.Busy() {
		return
	}
	/*
		Whether this is a change is the *section's* question.

		A snapshot differs from the one before it on almost every poll,
		because the pump and the fans never sit still -- so "has anything
		moved?" is always yes, and the Scenes list was rebuilt under the
		pointer every two seconds by numbers it does not draw. A section that
		says what it watches is rebuilt when that moves and left alone
		otherwise.
	*/
	/*
		A section that can take a new snapshot without being rebuilt gets
		one, whether or not anything it watches moved.

		This is the half that makes a narrow Changed affordable: the System
		section is rebuilt when a device appears, and its coolant reading
		arrives here every poll instead.
	*/
	if ticker, ok := current.(Ticker); ok {
		fyne.Do(func() { ticker.Tick(got) })
	}

	/*
		The status bar every poll, whatever the section says.

		It is the window's one line about the machine as a whole -- health,
		how many devices are in scope, the coolant -- and it was only
		repainted when a section was rebuilt. On a section that watches
		little, nothing rebuilt it: the bar kept whatever it said when the
		window opened, which on a fresh window is "0 of 0 devices" under a
		list of six. Found in a screenshot, where the window is new every
		time and the bar was wrong in every image.

		Cheap enough to do unconditionally: a handful of labels, against a
		poll that already asked the service four questions.
	*/
	fyne.Do(a.shell.RedrawStatus)

	changed := !got.Same(a.drawn)
	if watcher, ok := current.(Watcher); ok {
		changed = watcher.Changed(a.drawn, got)
	}
	a.drawn = got

	if changed {
		fyne.Do(func() { a.shell.Invalidate() })
	}
}

/*
Watcher is implemented by a section that draws part of the machine rather than
all of it.

Without it every section is rebuilt whenever anything moves, which on a machine
with a running pump is every poll.
*/
type Watcher interface {
	Changed(before, after Snapshot) bool
}

/*
Ticker is implemented by a section that updates what it draws in place.

For the things that move faster than a section should be rebuilt. A section
that is rebuilt to show a new number loses the scroll position, the open
dropdown and the half-typed entry of whoever is using it, which is why the
rule is to build the tree once and update it -- and this is where the updating
arrives.

Called on the UI thread, and only for the section on screen.
*/
type Ticker interface {
	Tick(got Snapshot)
}

/*
Busy is implemented by a section that is in the middle of something.

The editor is: it holds a draft, a selection and an open colour picker, none of
which survive being rebuilt and none of which a change in the machine should
interrupt.
*/
type Busy interface {
	Busy() bool
}

/*
onScreen runs something on the UI thread.

Fyne is single-threaded for anything that draws, and an operation started by
Perform runs in a goroutine -- so a Flash, an Invalidate or a field the
builders read from must be handed back rather than called where the work
finished.

It is not a warning in a log. Fyne prints one, and then the text shaper
panicked mid-layout with an index out of range: a window that had been used for
a minute, on somebody's desk, because a status line was written from the wrong
goroutine.

fyne.Do, never DoAndWait: the worker must not wait on the thread it is feeding.
*/
func onScreen(do func()) { fyne.Do(do) }

/*
dropped takes files dragged onto the window: the library's part is shown, and
then it is handed them.

**The group, and then the part inside it.** "Pictures" stopped being a section
when it became one of three under Create, and the handler still asked for it
by that name -- so every dropped picture was converted, kept, and followed by
the window jumping to the front page, because a title nothing answered to
selected the first section. fynedesygn ignores an unknown title now (its spec
028); this asks for the two things that do exist.
*/
func (a *App) dropped(sh *shell.Shell, uris []fyne.URI) {
	if a.create != nil {
		a.create.Show("Pictures")
	}
	sh.Select("Create")
	if a.pictures != nil {
		a.pictures.Dropped(sh, uris)
	}
}

// Library is the picture section this window hands dropped files to, so a
// test can wait for what a drop started. See Settle.
func Library(a *App) *PicturesSection { return a.pictures }

// Drop is dropped, for tests: the handler it belongs to is a window callback,
// and a test that set one would be a test about Fyne.
func Drop(a *App, sh *shell.Shell, uris []fyne.URI) { a.dropped(sh, uris) }

/*
SectionNames are what the window can be asked to open on: its sections, and
the parts of Create by their own names.

The parts are named because that is how somebody thinks of them -- "open on
Pictures", not "open on Create and then press the second tab" -- and because
the screenshot script starts a window per image and cannot press a tab.

Built from the same constructor the window uses rather than from a second copy
of the list, and read before there is an app to ask, which is why it builds a
throwaway one.
*/
func SectionNames() []string {
	opts := New(nil).Options("")
	var out []string
	for _, section := range opts.Sections {
		out = append(out, section.Title())
		if group, ok := section.(*Create); ok {
			for _, part := range group.Parts() {
				out = append(out, part.Title())
			}
		}
	}
	return out
}

/*
OpenOn points the window at a section, or at one of Create's parts.

A part is not a section as far as the shell is concerned, so asking for
"Pictures" is asking for Create with Pictures in front. An unknown name is
left alone: the shell opens on the first section, which is what it does with a
name it does not have.
*/
func OpenOn(opts *shell.Options, name string) {
	if name == "" {
		return
	}
	for _, section := range opts.Sections {
		if strings.EqualFold(section.Title(), name) {
			opts.Section = section.Title()
			return
		}
		group, ok := section.(*Create)
		if !ok {
			continue
		}
		for _, part := range group.Parts() {
			if strings.EqualFold(part.Title(), name) {
				group.Show(part.Title())
				opts.Section = group.Title()
				return
			}
		}
	}
}
