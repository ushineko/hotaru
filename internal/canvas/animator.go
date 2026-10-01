package canvas

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/render"
	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/hidraw"
	"github.com/ushineko/sanshoku/lighting"
)

/*
DefaultInterval is how often an animated effect sends a frame, unless a rule
says otherwise.

GG's median interval in sanshoku spec 010's capture, and deliberately not the
device's floor. The floor (16 ms on the first canvas device) is the fastest
the device shows every frame; 56 ms is the pace its vendor chose, at a third
of the cost.
*/
const DefaultInterval = 56 * time.Millisecond

// Surface is the part of a canvas the animator draws on.
type Surface interface {
	Keys() []lighting.Key
	Frame(ctx context.Context, px []lighting.Pixel) error
	Floor() time.Duration
}

// Show is what to draw: an effect, what the scene said about it, and how often
// to send a frame while it moves.
type Show struct {
	Effect render.Renderer
	Params render.Params
	// Interval is for an animated effect; zero is DefaultInterval. Never
	// shorter than the canvas's floor, whatever it says.
	Interval time.Duration
}

/*
Status is what an animator is doing, for a listing.

Drawing is an animated effect being streamed. A canvas that is not drawing is
holding: it was sent one frame and shows it, and nothing is being sent.
*/
type Status struct {
	Effect   string
	Drawing  bool
	Interval time.Duration
	// Rate is frames sent a second over the last few seconds, while drawing.
	Rate float64
	// Frames is every frame sent since the animator started.
	Frames int
}

/*
Clock is the animator's time, a seam so that a test drives the ticks rather
than sleeping through them. The production one is the system clock.
*/
type Clock interface {
	Now() time.Time
	NewTicker(d time.Duration) Ticker
}

// Ticker is a time.Ticker behind an interface, for Clock.
type Ticker interface {
	C() <-chan time.Time
	Stop()
}

/*
Animator is the only writer to one canvas.

Owned by the daemon and started and stopped with the attachment (spec 060
R3.1). The service hands it what to draw with Show; it sends one frame for an
effect that does not move and a frame per tick for one that does, and it
never sends a frame identical to the last one it sent.

Stopping it sends nothing. The device holds its last frame, and handing the
lighting back to the firmware reboots the device on the first canvas device,
so that is something a person asks for (Device.Release), never a side effect
of the service stopping.
*/
type Animator struct {
	surface Surface
	clock   Clock
	report  func(format string, args ...any)
	want    chan Show
	redraw  chan struct{}

	mu     sync.Mutex
	status Status
	sent   []time.Time
}

// rateWindow is how far back Status counts frames to give a rate.
const rateWindow = 3 * time.Second

// NewAnimator is an animator for one canvas. A nil clock is the system clock,
// and a nil report says nothing.
func NewAnimator(surface Surface, clock Clock, report func(string, ...any)) *Animator {
	if clock == nil {
		clock = systemClock{}
	}
	if report == nil {
		report = func(string, ...any) {}
	}
	return &Animator{surface: surface, clock: clock, report: report, want: make(chan Show, 1), redraw: make(chan struct{}, 1)}
}

/*
Show replaces what the animator draws.

It does not wait. The latest call wins, as the per-device write queue's latest
write does: a scene superseded before its first frame was never anything
anybody wanted to see.
*/
func (a *Animator) Show(s Show) {
	for {
		select {
		case a.want <- s:
			return
		default:
			select {
			case <-a.want:
			default:
			}
		}
	}
}

/*
Redraw sends what is showing again, even though it has not changed.

A device can lose its frame without the animator knowing: through a USB
receiver the first canvas device reboots behind a node that never changes,
so nothing says it went, and it comes back on its own effect. A restore
(OpenRGB reconnecting, whose exit is what reboots it, or somebody asking
for one) is when that is likely, and one frame is what it costs.
*/
func (a *Animator) Redraw() {
	select {
	case a.redraw <- struct{}{}:
	default:
	}
}

// Status is what the animator is doing now.
func (a *Animator) Status() Status {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := a.status
	if out.Drawing && len(a.sent) > 1 {
		span := a.sent[len(a.sent)-1].Sub(a.sent[0])
		if span > 0 {
			out.Rate = float64(len(a.sent)-1) / span.Seconds()
		}
	}
	return out
}

/*
Run draws until the context is cancelled or the canvas goes away.

It returns the canvas's sanshoku.ErrGone, which ends the attachment (the
adapter has closed itself and the daemon looks for the device again), and nil
on a cancelled context. A frame the device does not acknowledge
(hidraw.ErrSilent) is said once per run and the stream carries on: the next
frame replaces it, and a journal line per dropped frame at 18 a second would
bury anything worth reading.
*/
func (a *Animator) Run(ctx context.Context) error {
	keys := a.surface.Keys()
	var (
		current Show
		showing bool
		start   time.Time
		last    []lighting.Pixel
		ticker  Ticker
		tick    <-chan time.Time
		said    bool
	)
	stop := func() {
		if ticker != nil {
			ticker.Stop()
			ticker, tick = nil, nil
		}
	}
	defer stop()

	for {
		// at is the instant this frame is drawn for: the tick's own time, so
		// a frame is the same whether the loop takes the tick late or not.
		var at time.Time
		select {
		case <-ctx.Done():
			return nil
		case next := <-a.want:
			/*
				The same effect again is not a new one.

				A reconcile sends what is already showing, and a moving effect
				restarted from t=0 every time it did would jump back to its
				first frame: a breathing keyboard going dark mid-breath for a
				write that changed nothing.
			*/
			if showing && same(next, current) {
				continue
			}
			current, showing, start = next, true, a.clock.Now()
			at = start
			stop()
			interval := a.interval(next)
			if next.Effect.Animated {
				ticker = a.clock.NewTicker(interval)
				tick = ticker.C()
			}
			a.mu.Lock()
			a.status.Effect, a.status.Drawing, a.status.Interval = next.Effect.Name, next.Effect.Animated, 0
			if next.Effect.Animated {
				a.status.Interval = interval
			}
			a.sent = a.sent[:0]
			a.mu.Unlock()
		case at = <-tick:
		case <-a.redraw:
			at = a.clock.Now()
			last = nil
		}
		if !showing {
			continue
		}

		px := current.Effect.Draw(at.Sub(start), keys, current.Params)
		if slices.Equal(px, last) {
			continue
		}
		err := a.surface.Frame(ctx, px)
		switch {
		case err == nil:
			last = px
			a.count()
			if !current.Effect.Animated {
				stop() // the retry below has landed; the device holds it now
			}
		case errors.Is(err, sanshoku.ErrGone):
			return err
		case ctx.Err() != nil:
			return nil
		default:
			// Not known to be showing, so the next frame is sent even if it
			// is the same one.
			last = nil
			/*
				An effect that does not move has no next tick to try again
				on, and the device would hold the scene before it for good.
				So it gets one, at the show's interval, until a frame lands.
			*/
			if !current.Effect.Animated && ticker == nil {
				ticker = a.clock.NewTicker(a.interval(current))
				tick = ticker.C()
			}
			if !said {
				if errors.Is(err, hidraw.ErrSilent) {
					a.report("a frame was not acknowledged, carrying on: %v", err)
				} else {
					a.report("a frame failed, carrying on: %v", err)
				}
				said = true
			}
		}
	}
}

// interval is how often to tick for this show: what it asks, or the default,
// and never under the canvas's floor.
func (a *Animator) interval(s Show) time.Duration {
	d := s.Interval
	if d <= 0 {
		d = DefaultInterval
	}
	return max(d, a.surface.Floor())
}

// count records a frame sent, for Status.
func (a *Animator) count() {
	now := a.clock.Now()
	a.mu.Lock()
	defer a.mu.Unlock()
	a.status.Frames++
	a.sent = append(a.sent, now)
	for len(a.sent) > 1 && now.Sub(a.sent[0]) > rateWindow {
		a.sent = a.sent[1:]
	}
}

// same reports whether two shows would draw the same frames.
func same(x, y Show) bool {
	return x.Effect.Name == y.Effect.Name && x.Interval == y.Interval &&
		slices.Equal(x.Params.Colours, y.Params.Colours) &&
		slices.Equal(x.Params.Palette, y.Params.Palette) &&
		samePointer(x.Params.Speed, y.Params.Speed) &&
		samePointer(x.Params.Brightness, y.Params.Brightness)
}

func samePointer[T int | colour.Colour](x, y *T) bool {
	if x == nil || y == nil {
		return x == y
	}
	return *x == *y
}

// systemClock is the clock a running service uses.
type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

func (systemClock) NewTicker(d time.Duration) Ticker { return systemTicker{time.NewTicker(d)} }

type systemTicker struct{ *time.Ticker }

func (t systemTicker) C() <-chan time.Time { return t.Ticker.C }
