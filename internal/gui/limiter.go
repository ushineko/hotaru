package gui

import (
	"sync"
	"time"

	"github.com/ushineko/hotaru/internal/api"
)

/*
live is the floor between two sends to the hardware.

Measured from the end of one send to the start of the next, which is the only
place a fixed number means anything here. Spacing sends by the moment they
*start* spends the gap inside the send: hardware slow enough to need the
spacing never gets any, because by the time a write comes back the window has
already passed. That was the bug -- the throttle only throttled when there was
nothing to throttle.

So this is a floor on top of however long the machine actually takes. Devices
that write in five milliseconds see a colour every 125; devices that take
three hundred see one every 420, and the pointer stays smooth either way.
*/
const live = 120 * time.Millisecond

/*
limiter carries the editor's draft to the hardware without ever having two
applies out at once.

**Latest wins, and the last one always goes.** Colours offered while a send is
in flight replace a single pending slot rather than queueing behind it: a
backlog of colours that have all been countermanded is a backlog nobody wants
applied. But dropping the *final* one is the bug this shape of code usually
has -- a drag that ends between two sends would leave the lights on the
second-to-last colour, which looks exactly like the picker being broken.

The send runs on the limiter's own goroutine, so the UI thread never waits on
a USB write. Everything it needs is in the value it is given: the scene is
built on the UI thread and handed over, so there is no draft to race on.
*/
type limiter struct {
	send  func(api.Scene)
	every time.Duration

	mu      sync.Mutex
	pending api.Scene
	waiting bool
	// idle is closed when the sending goroutine stops, and is nil when none
	// is running. stop waits on it.
	idle chan struct{}
	// halt is closed once, by stop, to cut short the gap between sends.
	halt    chan struct{}
	stopped bool
}

// newLimiter builds one around the slow call it is spacing out.
func newLimiter(send func(api.Scene)) *limiter {
	return &limiter{send: send, every: live, halt: make(chan struct{})}
}

/*
offer hands over the colour the hardware should be showing.

Called from the UI thread on every drag step, every slider notch and every
keystroke in the number boxes. It never blocks: it writes one field and, if
nothing is already sending, starts the goroutine that will.
*/
func (l *limiter) offer(scene api.Scene) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.stopped || l.send == nil {
		return
	}

	l.pending, l.waiting = scene, true
	if l.idle != nil {
		return
	}
	idle := make(chan struct{})
	l.idle = idle
	go l.run(idle)
}

/*
stop ends the sending and waits for it.

Waiting is the point. The editor releases the preview lease the moment the
modal is dismissed, and an apply still in flight would land *after* that
release -- leaving the lights on a preview colour with nothing holding it and
nothing due to put it back. So this blocks the UI thread for at most one
apply, once, at dismissal, in exchange for the ordering being certain.

Safe to call twice, and on a limiter that never sent anything.
*/
func (l *limiter) stop() {
	l.mu.Lock()
	if l.stopped {
		l.mu.Unlock()
		return
	}
	l.stopped, l.waiting = true, false
	idle := l.idle
	close(l.halt)
	l.mu.Unlock()

	if idle != nil {
		<-idle
	}
}

// run sends until there is nothing left to send, then lets itself go. The next
// offer starts a fresh one.
func (l *limiter) run(idle chan struct{}) {
	defer close(idle)

	for {
		l.mu.Lock()
		scene, has := l.pending, l.waiting && !l.stopped
		l.waiting = false
		if !has {
			l.idle = nil
			l.mu.Unlock()
			return
		}
		l.mu.Unlock()

		l.send(scene)

		// The gap belongs here, after the hardware has answered.
		select {
		case <-time.After(l.every):
		case <-l.halt:
		}
	}
}
