package dashboard

import (
	"context"
	"errors"
	"image"
	"time"

	"github.com/ushineko/hotaru/internal/cooler"
)

/*
Floor is the shortest interval at which a frame of a given size appears.

Measured on the development machine by pushing a frame repeatedly and watching
an indicator that advances on every update:

	frame                         1 s      1.5 s   2 s     3 s
	5 KB, seven-segment digits    lands
	8 KB, flat background         never    skips   lands
	21 KB, full starfield                          lands   lands

It is a settling time that scales with frame size, not a size limit. The device
accepts every transfer either way -- the HID exchange succeeds, the bulk write
completes, the bucket switch returns success -- and the screen simply does not
change. So the floor is checked against the encoded length rather than assumed,
which means somebody who adds a gradient later raises it without touching a
line of this file. See spec 013.

Nothing in the protocol announces any of this. Another cooler will have its own
floor, and pushing too fast is invisible, which is the worst failure mode
available: every write reports success and the panel quietly stops updating.
*/
func Floor(size int) time.Duration {
	switch {
	case size <= 6*1024:
		return time.Second
	case size <= 24*1024:
		return 2 * time.Second
	}
	// Beyond anything measured. Extrapolating downwards would be a guess that
	// looks like a fact on a screen nobody is watching closely.
	return 3 * time.Second
}

// Panel is the screen, as much of it as pushing needs.
type Panel interface {
	Show(ctx context.Context, gif []byte) error
}

/*
Pusher draws the dashboard for as long as the service runs.

Two rules decide when it writes. It never writes more often than the frame's
own floor, and it never writes a frame that says what the last one said: an
idle machine holds coolant, pump and fan steady for hours, and a push costs the
device a settling period for a picture nobody could tell apart.
*/
type Pusher struct {
	// Panel is where frames go.
	Panel Panel
	// Read takes a reading. It is called on every cycle whether or not the
	// result is pushed, because it is the comparison.
	Read func(ctx context.Context) Reading
	/*
		Look is which dashboard to draw, and the picture behind it where it
		has one.

		Asked every cycle rather than held, because somebody can change the
		active dashboard or edit the one on screen while this loop is
		running, and a pusher holding a copy would keep drawing the old one
		until the service restarted.
	*/
	Look func(ctx context.Context) (Dashboard, image.Image)
	/*
		Trails is where the dashboard's readings have been, for the band
		under the numbers (spec 046).

		Asked per cycle like Look, and given the dashboard rather than a
		source, because which readings the band draws is the dashboard's own
		decision. Optional: a pusher without one draws the panel exactly as
		it drew it before there were any.
	*/
	Trails func(ctx context.Context, d Dashboard) Trails
	// Report says what went wrong, and is optional.
	Report func(format string, args ...any)

	// held is set while somebody else has the screen -- `hotaru screen show`,
	// or the readout. The dashboard is what the panel shows by default, not
	// what it shows regardless.
	held chan bool

	tick int
	last [32]byte
	sent bool

	// stopped is a panel that cannot be drawn on. Set once, and the loop
	// stops trying.
	stopped bool
}

// NewPusher prepares a dashboard for a panel.
func NewPusher(panel Panel, read func(context.Context) Reading) *Pusher {
	return &Pusher{Panel: panel, Read: read, held: make(chan bool, 1)}
}

/*
Hold stops the dashboard drawing, because somebody asked for something else on
the screen. It stays stopped until Release: a picture that is replaced two
seconds later was not shown.
*/
func (p *Pusher) Hold() { p.set(true) }

// Release gives the screen back to the dashboard, which redraws immediately.
func (p *Pusher) Release() { p.set(false) }

func (p *Pusher) set(held bool) {
	if p.held == nil {
		return
	}
	select {
	case p.held <- held:
	default: // the loop has not read the last one yet; it will read this state
		select {
		case <-p.held:
		default:
		}
		select {
		case p.held <- held:
		default:
		}
	}
}

// Run draws until the context is cancelled.
func (p *Pusher) Run(ctx context.Context) {
	held := false
	for {
		wait := 2 * time.Second
		if !held && !p.stopped {
			wait = p.cycle(ctx)
		}
		select {
		case <-ctx.Done():
			return
		case held = <-p.held:
			if !held {
				p.sent = false // redraw at once: the panel shows somebody else's picture
			}
		case <-time.After(wait):
		}
	}
}

// cycle renders one frame, pushes it if it would look different, and says how
// long to wait before the next one.
func (p *Pusher) cycle(ctx context.Context) time.Duration {
	look, behind := Shipped()[0], image.Image(nil)
	if p.Look != nil {
		look, behind = p.Look(ctx)
	}
	var trails Trails
	if p.Trails != nil {
		trails = p.Trails(ctx, look)
	}
	frame := Render(look, p.Read(ctx), p.tick, behind, trails)
	floor := Floor(len(frame.GIF))

	if p.sent && frame.Content == p.last {
		return floor
	}
	if err := p.Panel.Show(ctx, frame.GIF); err != nil {
		if p.Report != nil && ctx.Err() == nil {
			p.Report("hotaru: dashboard: %v", err)
		}
		/*
			A panel that cannot be drawn on stops the loop rather than
			failing every two seconds for the life of the service.

			It is not a transient: the interface is held by something else,
			or this user may not open the usbfs node, and neither changes
			while hotaru runs. Retrying writes the same line to the journal
			forty thousand times a day and draws nothing either way -- and
			the reason is on the screen in System, once.
		*/
		if errors.Is(err, cooler.ErrNoScreen) {
			p.stopped = true
		}
		return floor
	}
	// The tick advances only on an accepted push, so it marks real updates
	// rather than ticking forever -- and so it cannot defeat the gate above.
	p.tick++
	p.last, p.sent = frame.Content, true
	return floor
}

/*
Redraw says that what is being drawn has changed, rather than what it says.

The gate compares the frame's content, so editing the dashboard on screen --
a different arrangement of the same numbers, a new theme, another background
-- would show nothing until a reading moved. Forgetting the last frame is how
the loop is told to look again.
*/
func (p *Pusher) Redraw() { p.sent = false }
