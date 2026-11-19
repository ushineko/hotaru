package dashboard

import (
	"context"
	"errors"
	"image"
	"time"

	"github.com/ushineko/hotaru/internal/cooler"
)

/*
Framer is a panel that also takes streamed frames, in full colour (spec 080).

A frame shows at once and has no floor, but the firmware keeps it only until
FrameHold has passed with nothing newer, then takes the panel back for its
own readout. So a dashboard on frames sends the picture again before then,
which a GIF in a bucket never needed. Frames says whether this panel takes
them at all; a cooler adapter has the methods whatever its panel is.
*/
type Framer interface {
	Frames() bool
	Frame(ctx context.Context, img image.Image) error
	FrameHold() time.Duration
}

/*
framePace is how often a dashboard on frames is rendered and compared.

The maintainer's choice for spec 080: a reading that moves shows within about
a second. A render is about 3 ms, and a frame is sent only when what it says
has changed, or when the firmware's hold is nearly up.
*/
const framePace = time.Second

// Panel is the screen, as much of it as pushing needs.
type Panel interface {
	Show(ctx context.Context, gif []byte) error
	/*
		Floor is the shortest interval at which a frame of this many bytes
		appears. It is the device's, because pushing faster is invisible:
		every write reports success and the panel stops updating. The
		pusher asks it for every frame, so a heavier design raises it
		without touching this file. See spec 013 and sanshoku's nzxt driver,
		which carries the measurements.
	*/
	Floor(size int) time.Duration
}

/*
Pusher draws the dashboard for as long as the service runs.

Two rules decide when it writes. It never writes more often than the frame's
own floor, and it never writes a frame that says what the last one said: an
idle machine holds coolant, pump and fan steady for hours, and a push costs the
device a settling period for a picture nobody could tell apart.

On a panel that takes frames (spec 080) there is no floor: it looks every
second, and the second rule holds except that the firmware drops a frame after
its hold, so the same picture goes again before then.

On a panel that takes motion frames (spec 081) a change moves as the
dashboard's Motion says, the trail drifts, and an animated background plays:
see animate.
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
	/*
		Backdrop is an animated background's frames, for a panel that takes
		motion frames (spec 081). Optional: without it, or for a still
		picture, the background is Look's picture.
	*/
	Backdrop func(ctx context.Context, d Dashboard) Backdrop
	// Report says what went wrong, and is optional.
	Report func(format string, args ...any)
	// Now is the clock, for the resend on frames. Nil is time.Now.
	Now func() time.Time

	// held is set while somebody else has the screen -- `hotaru screen show`,
	// or the readout. The dashboard is what the panel shows by default, not
	// what it shows regardless.
	held chan bool

	tick int
	last [32]byte
	sent bool
	size int // of the last GIF sent, for its floor

	// after is the loop's wait, time.After unless a test drives the loop.
	after func(time.Duration) <-chan time.Time

	// shown is the last frame sent on frames, and at when, for the resend.
	shown image.Image
	at    time.Time

	// stopped is a panel that cannot be drawn on. Set once, and the loop
	// stops trying.
	stopped bool

	// anim is the dashboard in motion, on a panel that takes motion frames.
	anim animation
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
	after := time.After
	if p.after != nil {
		after = p.after
	}
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
		case <-after(wait):
		}
	}
}

// cycle renders one frame, pushes it if it would look different, and says how
// long to wait before the next one.
func (p *Pusher) cycle(ctx context.Context) time.Duration {
	// A panel that takes motion frames is drawn by animate, which reads once
	// a second and draws as often as something moves (spec 081).
	if m, ok := p.Panel.(Mover); ok && m.Moves() {
		if f, ok := p.Panel.(Framer); ok && f.Frames() {
			return p.animate(ctx, m, f)
		}
	}
	look, behind := Shipped()[0], image.Image(nil)
	if p.Look != nil {
		look, behind = p.Look(ctx)
	}
	var trails Trails
	if p.Trails != nil {
		trails = p.Trails(ctx, look)
	}
	frame := Render(look, p.Read(ctx), p.tick, behind, trails)
	if f, ok := p.Panel.(Framer); ok && f.Frames() {
		return p.frame(ctx, f, frame)
	}
	if p.sent && frame.Content == p.last {
		return p.Panel.Floor(p.size)
	}
	data := frame.GIF()
	floor := p.Panel.Floor(len(data))
	if err := p.Panel.Show(ctx, data); err != nil {
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
	p.last, p.sent, p.size = frame.Content, true, len(data)
	return floor
}

/*
frame is one cycle on a panel that takes frames: send the frame if it says
something new, send the last one again if the firmware's hold is nearly up,
and otherwise send nothing.

A resend is the picture already on the panel, tick and all. The frame just
rendered carries the next tick, and sending it would mark an update that
did not happen.
*/
func (p *Pusher) frame(ctx context.Context, f Framer, frame Frame) time.Duration {
	now := time.Now
	if p.Now != nil {
		now = p.Now
	}
	changed := !p.sent || frame.Content != p.last
	if !changed && now().Sub(p.at) < f.FrameHold() {
		return framePace
	}
	img := image.Image(frame.Image)
	if !changed {
		img = p.shown
	}
	if err := f.Frame(ctx, img); err != nil {
		if p.Report != nil && ctx.Err() == nil {
			p.Report("hotaru: dashboard: %v", err)
		}
		// As on a GIF: a panel that cannot be drawn on is not retried.
		if errors.Is(err, cooler.ErrNoScreen) {
			p.stopped = true
		}
		return framePace
	}
	if changed {
		p.tick++
		p.last, p.sent = frame.Content, true
	}
	p.shown, p.at = img, now()
	return framePace
}

/*
Redraw says that what is being drawn has changed, rather than what it says.

The gate compares the frame's content, so editing the dashboard on screen --
a different arrangement of the same numbers, a new theme, another background
-- would show nothing until a reading moved. Forgetting the last frame is how
the loop is told to look again.
*/
func (p *Pusher) Redraw() { p.sent = false }
