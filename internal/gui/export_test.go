package gui

import "sync"

/*
The tests draw on one goroutine, theirs (spec 079). A worker's hand-back to
the screen is queued, and Settle draws the queue on the caller's goroutine,
as the window's driver draws on its one thread. Fyne's test driver runs
fyne.Do in place instead, so workers drew beside each other and the test, and
the text shaper panicked (#219).
*/
var screen struct { //nolint:gochecknoglobals // the tests' stand-in for the UI thread
	mu    sync.Mutex
	queue []func()
}

func init() { //nolint:gochecknoinits // installs the stand-in before any test
	onScreen = func(do func()) {
		screen.mu.Lock()
		screen.queue = append(screen.queue, do)
		screen.mu.Unlock()
	}
}

// Settle draws what workers have handed to the screen, on the caller's
// goroutine, until nothing is left; a drawing may hand more.
func Settle() {
	for {
		screen.mu.Lock()
		if len(screen.queue) == 0 {
			screen.mu.Unlock()
			return
		}
		next := screen.queue[0]
		screen.queue = screen.queue[1:]
		screen.mu.Unlock()
		next()
	}
}

// Queued is how many hand-backs wait for Settle.
func Queued() int {
	screen.mu.Lock()
	defer screen.mu.Unlock()
	return len(screen.queue)
}

// OnScreen is onScreen, for a test of the queue itself.
func OnScreen(do func()) { onScreen(do) }

// StartCommand is startCommand, for the tests outside the package.
var StartCommand = startCommand
