package dashboard

import (
	"context"
	"crypto/sha256"
	"errors"
	"image"
	"math"
	"reflect"
	"time"

	"github.com/ushineko/hotaru/internal/cooler"
	"github.com/ushineko/hotaru/internal/motion"
	"github.com/ushineko/hotaru/internal/readings"
)

/*
Mover is a panel that also takes motion frames: 16-bit colour at the device's
own pace (spec 081). A panel that is a Mover is a Framer as well, for its
hold, and a dashboard on it moves: numbers and rings as its Motion says,
trails always, and an animated background plays.
*/
type Mover interface {
	Moves() bool
	Move(ctx context.Context, img image.Image) error
}

/*
Backdrop is an animated picture's frames and how long each shows, for a
dashboard whose background moves. Fewer than two frames is a still picture.
*/
type Backdrop struct {
	Frames []image.Image
	Delays []time.Duration
}

/*
motionPace is the shortest time between motion frames: about 20 a second,
NZXT CAM's pace and the panel's (sanshoku spec 038).
*/
const motionPace = 50 * time.Millisecond

// readOffset is where in each second the reading is taken: early enough that
// a late loop still lands in the same one-second bucket.
const readOffset = 100 * time.Millisecond

/*
animation is what a dashboard on a Mover is in the middle of: the reading the
panel is heading to, a transition towards it, the trail and where it has
drifted to, and the background's frame.
*/
type animation struct {
	ready bool
	d     Dashboard

	frames  []image.Image
	delays  []time.Duration
	frame   int
	next    time.Time
	still   *image.RGBA
	picture bool

	// The reading heading to, its text and the gate's key for it.
	reading Reading
	text    *image.RGBA
	key     [32]byte

	// The transition: the text and the rings from what was on the panel to
	// the reading, begun at began, lasting length.
	change   *motion.Change
	ringFrom Reading
	began    time.Time
	length   time.Duration

	// The trail, the one before it and when it changed, for the drift.
	trails, prev Trails
	sampled      time.Time
	shiftPx      int

	nextRead time.Time
	// dirty is a frame owed now: a fresh start or a change that jumps.
	// settle is a full-colour still owed when a transition ends.
	dirty, settle bool
}

// clock is the pusher's time.
func (p *Pusher) clock() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

/*
animate is one cycle on a panel that takes motion frames: take a reading
once a second, start a transition when what the dashboard says changes, and
send what the panel should show now. It says how long to wait.

Every frame is a motion frame (Q565): while something moves, when the trail
has drifted a pixel, when a change jumps or ends, and when the firmware's hold
is nearly up (spec 080); nothing otherwise. 16-bit colour on a 640-pixel panel
is not told apart from 24-bit, and a frame is tens of kilobytes where a raw
one is 1.2 MB (the maintainer's choice, after the live run's cost).
*/
func (p *Pusher) animate(ctx context.Context, m Mover, f Framer) time.Duration {
	now := p.clock()
	a := &p.anim
	if !a.ready || !p.sent || !now.Before(a.nextRead) {
		p.sample(ctx, now)
	}

	t, moving := a.progress(now)
	looping := len(a.frames) > 1
	if looping && !now.Before(a.next) {
		a.frame = (a.frame + 1) % len(a.frames)
		a.next = now.Add(max(a.delays[a.frame], motionPace))
	}
	shift, px := a.drift(now)
	drifted := px != a.shiftPx
	resend := p.sent && now.Sub(p.at) >= f.FrameHold()
	settling := a.settle && !moving
	if !moving && !looping && !drifted && !a.dirty && !settling && !resend {
		return a.wait(now, false, 0)
	}

	img := p.compose(a, t, shift)
	if err := m.Move(ctx, img); err != nil {
		if p.Report != nil && ctx.Err() == nil {
			p.Report("hotaru: dashboard: %v", err)
		}
		if errors.Is(err, cooler.ErrNoScreen) {
			p.stopped = true
		}
		return a.wait(now, moving || looping, p.clock().Sub(now))
	}
	a.shiftPx, a.dirty = px, false
	if !moving {
		a.settle = false
	}
	p.sent, p.shown, p.at = true, img, now
	return a.wait(now, moving || looping, p.clock().Sub(now))
}

/*
sample takes a reading and the trail, and decides what the panel heads to.

A different dashboard, or the panel back from somebody else's picture, is a
fresh start drawn at once. Otherwise a reading whose text or rings differ
from the last is a change: the tick advances once, and a transition begins
from what is on the panel now, which is the middle of the last one if it is
still running.
*/
func (p *Pusher) sample(ctx context.Context, now time.Time) {
	a := &p.anim
	d, behind := Shipped()[0], image.Image(nil)
	if p.Look != nil {
		d, behind = p.Look(ctx)
	}
	// A copy: a Reading holds a map, and the one a caller hands over may
	// change after this has kept it as what the panel shows.
	r := between(Reading{}, p.Read(ctx), 1)
	var trails Trails
	if p.Trails != nil {
		trails = p.Trails(ctx, d)
	}
	/*
		The next reading at a tenth of a second past the next whole second,
		not a second from now: the history keeps one-second buckets, and a
		loop that runs a little late each time would skip one now and then,
		which a one-second trail draws as a gap (found on the live run).
	*/
	a.nextRead = now.Truncate(framePace).Add(framePace + readOffset)

	if !a.ready || !p.sent || !reflect.DeepEqual(d, a.d) {
		*a = animation{ready: true, d: d, nextRead: a.nextRead, dirty: true}
		if p.Backdrop != nil {
			if b := p.Backdrop(ctx, d); len(b.Frames) > 1 {
				a.frames, a.delays, a.next = b.Frames, b.Delays, now.Add(b.Delays[0])
			}
		}
		a.still, a.picture = BackgroundLayer(d, behind)
		if len(a.frames) > 1 {
			_, a.picture = BackgroundLayer(d, a.frames[0])
		}
		a.reading, a.ringFrom = r, r
		a.text = TextLayer(d, r, a.picture)
		a.key = keyOf(a.text, RingsLayer(d, r))
		a.trails, a.prev = trails, trails
		a.sampled = now.Add(-d.Trail.Interval())
		a.shiftPx = -1
		p.tick++
		return
	}

	text := TextLayer(d, r, a.picture)
	if key := keyOf(text, RingsLayer(d, r)); key != a.key {
		t, moving := a.progress(now)
		shownText, shownRing := a.text, a.reading
		if moving {
			shownText, shownRing = a.textAt(t), a.ringAt(t)
		}
		a.change, a.length = nil, 0
		moves := d.Motion.Numbers != "" || d.Motion.Ring != ""
		if moves {
			a.change = motion.Prepare(d.Motion.Numbers, shownText, text)
			a.length = motion.Duration(d.Motion.Numbers)
			a.began = now
		}
		a.ringFrom, a.reading, a.text, a.key = shownRing, r, text, key
		a.dirty, a.settle = !moves, moves
		p.tick++
	} else {
		a.reading = r
	}

	if !sameTrails(trails, a.trails) {
		a.prev, a.trails, a.sampled = a.trails, trails, now
	}
}

// keyOf is what the gate compares: the text and the rings, and not the trail,
// whose movement is a drift and not a change.
func keyOf(text, rings *image.RGBA) [32]byte {
	h := sha256.New()
	h.Write(text.Pix)
	h.Write(rings.Pix)
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

// progress is how far through its transition the dashboard is, and whether
// it is still moving.
func (a *animation) progress(now time.Time) (float64, bool) {
	if a.length <= 0 {
		return 1, false
	}
	t := float64(now.Sub(a.began)) / float64(a.length)
	return min(1, max(0, t)), t < 1
}

// textAt is the text layer t of the way through the transition.
func (a *animation) textAt(t float64) *image.RGBA {
	if a.change == nil {
		return a.text
	}
	return a.change.At(t)
}

// ringAt is the reading the rings show t of the way through.
func (a *animation) ringAt(t float64) Reading {
	switch a.d.Motion.Ring {
	case Sweep:
		return between(a.ringFrom, a.reading, motion.Ease(t))
	case Comet:
		return between(a.ringFrom, a.reading, motion.Spring(t))
	}
	return a.reading
}

/*
drift is how far right of its place the trail is drawn, as a fraction of a
step, and that as whole pixels: a frame is worth sending only when the
pixels change (spec 081 R4.3).
*/
func (a *animation) drift(now time.Time) (float64, int) {
	below, above := a.d.Trail.Sources(a.d.Headline.Source)
	if below == "" && above == "" {
		return 0, 0
	}
	p := float64(now.Sub(a.sampled)) / float64(a.d.Trail.Interval())
	shift := 1 - min(1, max(0, p))
	step := float64(Size-2*rowInset-1) / float64(max(1, a.trails.span()-1))
	return shift, int(shift * step)
}

/*
wait is how long until the next cycle: the rest of the motion pace while
something moves, otherwise until the trail's next pixel or the next reading.

The rest, not the whole: spent is what drawing and sending this frame took,
and a frame over a photograph takes longer than the pace to send, so waiting
the whole pace after it halved the rate (found on the live run).
*/
func (a *animation) wait(now time.Time, moving bool, spent time.Duration) time.Duration {
	if moving {
		return max(motionPace-spent, time.Millisecond)
	}
	until := a.nextRead.Sub(now)
	if a.shiftPx > 0 {
		step := float64(Size-2*rowInset-1) / float64(max(1, a.trails.span()-1))
		perPixel := time.Duration(float64(a.d.Trail.Interval()) / step)
		until = min(until, max(perPixel, motionPace))
	}
	return max(until, motionPace)
}

/*
compose draws the frame: the background (this frame of an animated one), the
rings where the transition has them, the trail drifted, and the text, with
the tick over it.
*/
func (p *Pusher) compose(a *animation, t, shift float64) *image.RGBA {
	bg := a.still
	if len(a.frames) > 1 {
		bg, _ = BackgroundLayer(a.d, a.frames[a.frame])
	}
	rings := RingsLayer(a.d, a.ringAt(t))
	if a.d.Motion.Ring == Comet && t < 1 {
		motion.Glow(rings, RingsLayer(a.d, a.ringAt(max(0, t-0.12))), 1-t)
	}
	trails := TrailsLayer(a.d, a.reading, a.drifted(shift), a.picture)
	img := Compose(bg, rings, trails, a.textAt(t))
	drawTick(img, p.tick, ThemeOf(a.d.Theme))
	return img
}

/*
drifted is the trail drawn shift of a step right of its place, its range
blended from the last trail's, and, when the window is full, the sample that
left drawn at the left edge until it has moved out, so the trace keeps its
length (spec 081 R4.1, R4.2).
*/
func (a *animation) drifted(shift float64) Trails {
	tr := a.trails
	if shift <= 0 || len(a.prev.Below)+len(a.prev.Above) == 0 {
		return tr
	}
	below, above := a.d.Trail.Sources(a.d.Headline.Source)
	tr.Shift = shift
	tr.Ranges = map[readings.Source][2]float64{}
	blend := func(src readings.Source, now, before Series) Series {
		if src == "" || len(now) < 2 || len(before) < 2 {
			return now
		}
		lo0, hi0 := domain(before, src)
		lo1, hi1 := domain(now, src)
		p := 1 - shift
		tr.Ranges[src] = [2]float64{lo0 + (lo1-lo0)*p, hi0 + (hi1-hi0)*p}
		if len(before) == len(now) {
			return append(Series{before[0]}, now...)
		}
		return now
	}
	tr.Below = blend(below, a.trails.Below, a.prev.Below)
	tr.Above = blend(above, a.trails.Above, a.prev.Above)
	return tr
}

// sameTrails says whether two trails would draw the same, gaps included.
func sameTrails(a, b Trails) bool {
	same := func(x, y Series) bool {
		if len(x) != len(y) {
			return false
		}
		for i := range x {
			if x[i] != y[i] && !(math.IsNaN(x[i]) && math.IsNaN(y[i])) {
				return false
			}
		}
		return true
	}
	return same(a.Below, b.Below) && same(a.Above, b.Above) && a.Points == b.Points
}

// between is a reading t of the way from a to b, every source b has; t may
// pass 1, for the comet's overshoot.
func between(a, b Reading, t float64) Reading {
	var out Reading
	for _, s := range readings.All {
		bv, ok := b.Value(s)
		if !ok {
			continue
		}
		if av, ok := a.Value(s); ok {
			bv = av + (bv-av)*t
		}
		out.Set(s, bv)
	}
	return out
}
