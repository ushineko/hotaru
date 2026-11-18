package dashboard

import (
	"context"
	"image"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/readings"
)

// framing is a panel that takes frames (spec 080), recording each one. Like
// panel, it is not a stand-in for the cooler; it records the pusher's
// decisions about when to send.
type framing struct {
	panel
	mu   sync.Mutex
	sent []image.Image
	err  error
}

func (*framing) Frames() bool             { return true }
func (*framing) FrameHold() time.Duration { return 25 * time.Second }

func (f *framing) Frame(_ context.Context, img image.Image) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, img)
	return nil
}

func (f *framing) frames() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sent)
}

// clock is a time a test moves by hand.
type clock struct{ now time.Time }

func (c *clock) Now() time.Time       { return c.now }
func (c *clock) pass(d time.Duration) { c.now = c.now.Add(d) }
func framed(f *framing, r *Reading) (*Pusher, *clock) {
	c := &clock{now: time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)}
	p := NewPusher(f, func(context.Context) Reading { return *r })
	p.Now = c.Now
	return p, c
}

// On frames the dashboard looks every second, sends a frame that says
// something new, and sends nothing else: no GIF, no floor.
func TestOnFramesAChangeIsSentAndNothingElse(t *testing.T) {
	f := &framing{}
	r := reading()
	p, c := framed(f, &r)

	require.Equal(t, framePace, p.cycle(context.Background()))
	require.Equal(t, 1, f.frames())
	for range 5 {
		c.pass(time.Second)
		p.cycle(context.Background())
	}
	require.Equal(t, 1, f.frames(), "an unchanged frame was sent again before the hold")

	r.Set(readings.Coolant, 41)
	c.pass(time.Second)
	p.cycle(context.Background())
	require.Equal(t, 2, f.frames())
	require.Zero(t, f.count(), "a GIF went to a panel that takes frames")
	require.Empty(t, f.asked, "a floor was asked for on frames")
}

/*
The firmware takes the panel back 30 s after the last frame, so an unchanged
frame goes again once FrameHold has passed: the same picture, tick and all,
and the tick does not advance for it.
*/
func TestOnFramesTheSamePictureGoesAgainBeforeTheHold(t *testing.T) {
	f := &framing{}
	r := reading()
	p, c := framed(f, &r)

	p.cycle(context.Background())
	tick := p.tick
	c.pass(24 * time.Second)
	p.cycle(context.Background())
	require.Equal(t, 1, f.frames())

	c.pass(time.Second)
	p.cycle(context.Background())
	require.Equal(t, 2, f.frames(), "the frame was not sent again at the hold")
	require.Same(t, f.sent[0], f.sent[1], "the resend was not the picture on the panel")
	require.Equal(t, tick, p.tick, "a resend advanced the tick")

	c.pass(25 * time.Second)
	p.cycle(context.Background())
	require.Equal(t, 3, f.frames(), "the hold restarts from the resend")
}

// While somebody else has the screen nothing is sent, resends included: a
// resend over a picture would replace it. Released, the dashboard is sent at
// once.
func TestOnFramesAHeldScreenIsLeftAloneAndReleasedAtOnce(t *testing.T) {
	f := &framing{}
	r := reading()
	p, _ := framed(f, &r)
	// The loop's waits end at once, and each is counted, so the test knows
	// the loop went round without sleeping through a pace.
	var rounds atomic.Int64
	p.after = func(time.Duration) <-chan time.Time {
		rounds.Add(1)
		done := make(chan time.Time, 1)
		done <- time.Time{}
		return done
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stopped := make(chan struct{})
	go func() { defer close(stopped); p.Run(ctx) }()

	require.Eventually(t, func() bool { return f.frames() == 1 }, time.Second, 5*time.Millisecond)
	p.Hold()
	// The hold reaches the loop on its channel; ten rounds later it has
	// been held for at least nine of them.
	from := rounds.Load()
	require.Eventually(t, func() bool { return rounds.Load() >= from+10 }, time.Second, time.Millisecond)
	require.Equal(t, 1, f.frames(), "a frame was sent over somebody else's picture")

	p.Release()
	require.Eventually(t, func() bool { return f.frames() == 2 }, time.Second, 5*time.Millisecond)

	cancel()
	<-stopped
}

// A frame that fails is not counted as shown, and the next cycle sends it.
func TestOnFramesAFailedFrameIsSentAgain(t *testing.T) {
	f := &framing{err: context.DeadlineExceeded}
	r := reading()
	p, c := framed(f, &r)

	p.cycle(context.Background())
	require.Zero(t, f.frames())
	require.Zero(t, p.tick)

	f.err = nil
	c.pass(time.Second)
	p.cycle(context.Background())
	require.Equal(t, 1, f.frames())
	require.Equal(t, 1, p.tick)
}

// A cooler adapter has the frame methods whatever its panel is; one whose
// panel takes no frames is drawn on with GIFs, as before.
func TestAPanelThatTakesNoFramesGetsGIFs(t *testing.T) {
	f := &noFrames{}
	p := NewPusher(f, func(context.Context) Reading { return reading() })
	p.cycle(context.Background())
	require.Equal(t, 1, f.count())
	require.Zero(t, f.frames())
}

type noFrames struct{ framing }

func (*noFrames) Frames() bool { return false }
