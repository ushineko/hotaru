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
	return shell.Options{
		AppID:   AppID,
		Name:    "hotaru",
		Version: version.Version,
		Icon:    Icon(),
		Sections: []shell.Section{
			&ServiceSection{app: a},
			&SystemSection{app: a},
			&CoolingSection{app: a},
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
		OnCreate:      func(s *shell.Shell) { a.shell = s },
		OnStart:       func(*shell.Shell) { a.Start(context.Background()) },
		OnStop:        func(*shell.Shell) { a.Stop() },
		StatusBar:     func(*shell.Shell) []fyne.CanvasObject { return a.status(socket) },
		OnInvalidate:  func(*shell.Shell) { a.refresh(context.Background()) },
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

// redraw rebuilds the current section on the UI thread. fyne.Do, never
// DoAndWait: the poll goroutine must not wait on the thread it is feeding.
func (a *App) redraw() {
	if a.shell == nil || !a.shell.OnScreen() {
		return
	}
	fyne.Do(func() { a.shell.Invalidate() })
}
