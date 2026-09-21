package cooler

import (
	"context"
	"sync"
	"time"
)

/*
Owner is the one thing allowed to talk to a cooler.

Two callers writing to one HID endpoint interleave control transfers on a
single interrupt endpoint, which is a corruption risk rather than a contention
problem, so access is serialised. The predecessor project needed a priority
queue for this because every call was a separate `liquidctl` process fighting
for one node; hotaru holds the handle, so a mutex is the whole mechanism.

**Reads coalesce by freshness, not by superseding.** Lighting's queue drops a
waiting write when a newer one arrives, because nobody wants the
second-to-last scene applied after the last. A read is not like that: a caller
whose request was superseded still wants a number. So concurrent readers share
one recent answer instead of each paying for a reading and queueing behind the
others -- which is what a dashboard, a status endpoint and a telemetry consumer
asking at once actually want.
*/
type Owner struct {
	mu sync.Mutex
	c  *Cooler

	// fresh is how long a reading is served to later callers.
	fresh time.Duration

	last Status
	at   time.Time

	/*
		screen is opened on first use and kept.

		Every screen command travels the same control channel as a status
		reading -- only the pixels go over the bulk endpoint -- so the two
		cannot interleave and the owner holds both. Opening it lazily means a
		machine that never draws anything never claims the interface, leaving
		it to liquidctl or anything else that wants it.
	*/
	screen *Screen
}

/*
Freshness is how old a reading may be before another is taken.

Short enough that a dashboard is honest and long enough that three consumers
asking at once cost one exchange. A reading takes about two milliseconds, so
this is not about the cost of the read: it is about not writing to a device
more often than anybody needs.
*/
const Freshness = 250 * time.Millisecond

// Own takes charge of an open cooler.
func Own(c *Cooler) *Owner { return &Owner{c: c, fresh: Freshness} }

/*
Status is the cooler's reading, taken now or recently.

An error is never cached: a device that failed once is asked again rather than
being written off for a quarter of a second, because the next caller may be a
person who has just plugged it back in.
*/
func (o *Owner) Status(ctx context.Context) (Status, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	if !o.at.IsZero() && time.Since(o.at) < o.fresh {
		return o.last, nil
	}
	status, err := o.c.Status(ctx)
	if err != nil {
		return Status{}, err
	}
	o.last, o.at = status, time.Now()
	return status, nil
}

// Device is what is being read, for reporting.
func (o *Owner) Device() Device { return o.c.Device() }

/*
Close releases the cooler, handing the screen back first.

A machine that is no longer running hotaru should not keep showing whatever
hotaru last drew: the panel is somebody's cooler, and leaving a stale dashboard
on it is the same discourtesy as leaving their lights on a colour they did not
choose.
*/
func (o *Owner) Close() error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.screen != nil {
		ctx, cancel := context.WithTimeout(context.Background(), returnScreen)
		_ = o.screen.Liquid(ctx)
		cancel()
		_ = o.screen.Close()
	}
	return o.c.Close()
}

// returnScreen bounds handing the panel back while the service is stopping.
// A shutdown that hangs on a screen is worse than one that leaves a picture.
const returnScreen = 2 * time.Second

/*
Show puts a GIF on the screen.

A GIF, not a still image: the firmware does not retain a static picture, and
one frame is enough to get the retention that a GIF has. See spec 012.
*/
func (o *Owner) Show(ctx context.Context, gif []byte) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	screen, err := o.panel()
	if err != nil {
		return err
	}
	return screen.Image(ctx, gif)
}

// Readout hands the screen back to the cooler's own display.
func (o *Owner) Readout(ctx context.Context) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	screen, err := o.panel()
	if err != nil {
		return err
	}
	return screen.Liquid(ctx)
}

// Appearance sets the screen's brightness and orientation, which the device
// keeps across restarts.
func (o *Owner) Appearance(ctx context.Context, brightness, degrees int) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	screen, err := o.panel()
	if err != nil {
		return err
	}
	return screen.Appearance(ctx, brightness, degrees)
}

// panel opens the screen once, on first use. The caller holds the lock.
func (o *Owner) panel() (*Screen, error) {
	if o.screen != nil {
		return o.screen, nil
	}
	screen, err := o.c.Screen()
	if err != nil {
		return nil, err
	}
	o.screen = screen
	return screen, nil
}
