package gui

import (
	"context"
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

// New builds the app around a client.
func New(client *api.Client) *App {
	return &App{client: client, machine: &Machine{}, every: Poll}
}

// Options describes the window to fynedesygn's shell.
func (a *App) Options(socket string) shell.Options {
	create := NewCreate(a)
	pictures, _ := create.parts[0].(*PicturesSection)

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
			a.shell, a.pictures = s, pictures
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
					s.Select("Pictures")
					a.pictures.Dropped(s, uris)
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
