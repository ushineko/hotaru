package openrgb

import (
	"context"
	"strings"
	"sync"
	"time"
)

/*
SettleDelay is how long a device is given to finish changing mode before
anything else is written to it.

OpenRGB 1.0 does not finish a mode change before it answers. The server
hands the change to the controller's own thread, which in a per-LED mode
writes every zone again, while a frame that arrives next is written from the
network thread. Nothing keeps the two apart, and on an NZXT cooler they
interleave: frames are lost, and the ring's channel stops showing them until
the next mode change (spec 068, #24). The protocol says nothing when that
thread is done, so the wait is a fixed one.

120 ms is what spec 057 measured a keyboard needing to finish entering a
mode. A Hue 2 device in Direct sends about six reports.
*/
const SettleDelay = 120 * time.Millisecond

/*
holds keeps a device's next frame back until its last mode change has
settled.

A mode change starts a timer that closes a channel when it fires; a frame to
the same device waits on that channel. Other devices are not held, and a
frame sent after the timer has fired does not wait.
*/
type holds struct {
	mu sync.Mutex
	// after runs f once d has passed: time.AfterFunc, or a test's clock.
	after func(d time.Duration, f func())
	busy  map[string]chan struct{}
}

func newHolds() *holds {
	return &holds{
		after: func(d time.Duration, f func()) { time.AfterFunc(d, f) },
		busy:  map[string]chan struct{}{},
	}
}

// start holds a device from now until SettleDelay has passed. A later start
// replaces an earlier one, so the hold always runs from the last mode change.
func (h *holds) start(device string) {
	key := strings.ToLower(device)
	settled := make(chan struct{})

	h.mu.Lock()
	h.busy[key] = settled
	h.mu.Unlock()

	h.after(SettleDelay, func() {
		close(settled)
		h.mu.Lock()
		if h.busy[key] == settled {
			delete(h.busy, key)
		}
		h.mu.Unlock()
	})
}

// wait returns once the device is not held, or with the context's error if
// the caller gives up first.
func (h *holds) wait(ctx context.Context, device string) error {
	h.mu.Lock()
	settled := h.busy[strings.ToLower(device)]
	h.mu.Unlock()

	if settled == nil {
		return nil
	}
	select {
	case <-settled:
		return nil
	case <-ctx.Done():
		return ctx.Err() //nolint:wrapcheck // the caller's own cancellation
	}
}
