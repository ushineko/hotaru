package dashboard

import (
	"context"
	"image"
	"image/color"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/hotaru/internal/readings"
)

// mover is a panel that takes still and motion frames, as the Kraken Elite
// does through sanshoku, recording which kind arrived and when.
type mover struct {
	panel
	mu    sync.Mutex
	clock *clock
	sent  []sentFrame
}

type sentFrame struct {
	motion bool
	at     time.Time
	img    image.Image
}

func (*mover) Frames() bool             { return true }
func (*mover) Moves() bool              { return true }
func (*mover) FrameHold() time.Duration { return 25 * time.Second }

func (m *mover) Frame(_ context.Context, img image.Image) error { return m.record(false, img) }
func (m *mover) Move(_ context.Context, img image.Image) error  { return m.record(true, img) }

func (m *mover) record(motion bool, img image.Image) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, sentFrame{motion, m.clock.now, img})
	return nil
}

// count is how many of each kind arrived since index from.
func (m *mover) count(from int) (stills, moves int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, f := range m.sent[from:] {
		if f.motion {
			moves++
		} else {
			stills++
		}
	}
	return stills, moves
}

func (m *mover) n() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sent)
}

// scene is a pusher on a mover, with a dashboard, a reading and a trail the
// test changes, and a clock it moves.
type scene struct {
	t        *testing.T
	p        *Pusher
	m        *mover
	c        *clock
	d        Dashboard
	r        Reading
	trails   Trails
	backs    int
	backdrop Backdrop
}

func newScene(t *testing.T, d Dashboard) *scene {
	t.Helper()
	c := &clock{now: time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)}
	s := &scene{t: t, c: c, d: d, r: reading()}
	s.m = &mover{clock: c}
	s.p = NewPusher(s.m, func(context.Context) Reading { return s.r })
	s.p.Now = c.Now
	s.p.Look = func(context.Context) (Dashboard, image.Image) { return s.d, nil }
	s.p.Trails = func(context.Context, Dashboard) Trails { return s.trails }
	s.p.Backdrop = func(context.Context, Dashboard) Backdrop { s.backs++; return s.backdrop }
	return s
}

// run cycles the pusher for a span, as Run would, following its waits.
func (s *scene) run(span time.Duration) {
	end := s.c.now.Add(span)
	for s.c.now.Before(end) {
		wait := s.p.cycle(context.Background())
		s.c.pass(wait)
	}
}

func ringDashboard() Dashboard {
	d := Shipped()[0]
	d.Motion = Motion{Numbers: Melt, Ring: Comet}
	return d
}

/*
A change on a dashboard that melts is motion frames at 50 ms apart for its
700 ms, then one full-colour still; the tick advances once (R2, R2.5).
*/
func TestAChangeThatMeltsIsMotionThenAStill(t *testing.T) {
	s := newScene(t, ringDashboard())
	s.run(2 * time.Second)
	require.Equal(t, 1, s.m.n(), "a fresh start is one still and then quiet")
	tick := s.p.tick

	from := s.m.n()
	s.r.Set(readings.Coolant, 48.7)
	s.run(2 * time.Second)

	stills, moves := s.m.count(from)
	require.InDelta(t, 14, moves, 2, "700 ms at 50 ms a frame")
	require.Equal(t, 1, stills, "the transition ends on one full-colour still")
	require.Equal(t, tick+1, s.p.tick, "the tick advanced per frame, not per change")

	s.m.mu.Lock()
	defer s.m.mu.Unlock()
	last := s.m.sent[len(s.m.sent)-1]
	require.False(t, last.motion)
	for i := from + 1; i < len(s.m.sent)-1; i++ {
		require.Equal(t, motionPace, s.m.sent[i].at.Sub(s.m.sent[i-1].at), "motion frames at the motion pace")
	}
}

// A dashboard with no motion jumps: one still per change, as spec 080 sent.
func TestAChangeWithoutMotionJumps(t *testing.T) {
	s := newScene(t, Shipped()[0])
	s.run(2 * time.Second)
	from := s.m.n()
	s.r.Set(readings.Coolant, 48.7)
	s.run(2 * time.Second)
	stills, moves := s.m.count(from)
	require.Equal(t, 1, stills)
	require.Zero(t, moves)
}

// A change during a transition starts from what is on the panel: the rings
// set off from where the last sweep had reached, not from the old reading.
func TestAChangeMidTransitionStartsFromThePanel(t *testing.T) {
	d := Shipped()[0]
	d.Motion = Motion{Ring: Sweep}
	s := newScene(t, d)
	s.run(2 * time.Second)

	s.r.Set(readings.Coolant, 30)
	s.c.pass(time.Second) // the next reading
	s.p.cycle(context.Background())
	s.c.pass(250 * time.Millisecond) // halfway through a 500 ms sweep
	s.p.cycle(context.Background())
	mid, _ := s.p.anim.ringAt(0.5).Value(readings.Coolant)

	s.r.Set(readings.Coolant, 60)
	s.p.anim.nextRead = s.c.now // read now
	s.p.cycle(context.Background())
	from, _ := s.p.anim.ringFrom.Value(readings.Coolant)
	require.InDelta(t, mid, from, 0.5, "the new sweep did not start where the last one was")
	require.NotEqual(t, 30.0, from)
}

/*
The trail drifts: when a sample joins it the trace starts a step right and
moves left, a full-colour still each time it has moved a pixel, never a
motion frame on its own, and the tick does not move (R4).
*/
func TestTheTrailDriftsAsStills(t *testing.T) {
	s := newScene(t, Shipped()[0])
	s.trails = Trails{Below: rampSeries(60, 0), Points: 60}
	s.run(2 * time.Second)
	from, tick := s.m.n(), s.p.tick

	s.trails = Trails{Below: rampSeries(60, 1), Points: 60}
	s.run(6 * time.Second)
	stills, moves := s.m.count(from)
	require.Zero(t, moves, "a drift sent motion frames")
	step := float64(Size-2*rowInset-1) / 59
	require.InDelta(t, int(step), stills, 2, "a still per pixel of the step")
	require.Equal(t, tick, s.p.tick, "a drift advanced the tick")
}

// While drifting, a full window keeps its length: the sample that left is
// drawn at the left edge until it has moved out (R4.1).
func TestADriftingTrailKeepsItsLength(t *testing.T) {
	a := &animation{d: Shipped()[0]}
	a.prev = Trails{Below: rampSeries(60, 0), Points: 60}
	a.trails = Trails{Below: rampSeries(60, 1), Points: 60}
	tr := a.drifted(0.5)
	require.Len(t, tr.Below, 61, "the leaving sample is not drawn")
	require.Equal(t, a.prev.Below[0], tr.Below[0])
	require.InDelta(t, 0.5, tr.Shift, 1e-9)
	require.Len(t, a.drifted(0).Below, 60, "a settled trail has no leaving sample")
}

/*
An animated background plays: a motion frame per picture frame at its delay,
clamped to the motion pace, looping, and the frames are fetched once, not
per frame (R5).
*/
func TestAnAnimatedBackgroundPlays(t *testing.T) {
	d := Shipped()[0]
	d.Background = Background{Kind: Picture, Picture: "spin"}
	s := newScene(t, d)
	frames := []image.Image{solid(color.RGBA{200, 0, 0, 255}), solid(color.RGBA{0, 200, 0, 255}), solid(color.RGBA{0, 0, 200, 255})}
	s.backdrop = Backdrop{Frames: frames, Delays: []time.Duration{100 * time.Millisecond, 100 * time.Millisecond, 10 * time.Millisecond}}

	s.run(3 * time.Second)
	_, moves := s.m.count(0)
	require.Greater(t, moves, 40, "the picture did not play")
	require.Equal(t, 1, s.backs, "the frames were fetched more than once")
}

// Held, nothing is sent, transition or drift; released, the dashboard draws
// at once (R7).
func TestAHeldDashboardDoesNotMove(t *testing.T) {
	s := newScene(t, ringDashboard())
	var mu sync.Mutex
	coolant := 41.0
	s.p.Read = func(context.Context) Reading {
		mu.Lock()
		defer mu.Unlock()
		r := reading()
		r.Set(readings.Coolant, coolant)
		return r
	}
	s.p.Now = nil
	var rounds atomic.Int64
	s.p.after = func(time.Duration) <-chan time.Time {
		rounds.Add(1)
		done := make(chan time.Time, 1)
		done <- time.Time{}
		return done
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stopped := make(chan struct{})
	go func() { defer close(stopped); s.p.Run(ctx) }()
	require.Eventually(t, func() bool { return s.m.n() >= 1 }, time.Second, time.Millisecond)

	s.p.Hold()
	from := rounds.Load()
	require.Eventually(t, func() bool { return rounds.Load() >= from+5 }, time.Second, time.Millisecond)
	held := s.m.n()
	mu.Lock()
	coolant = 60
	mu.Unlock()
	from = rounds.Load()
	require.Eventually(t, func() bool { return rounds.Load() >= from+20 }, time.Second, time.Millisecond)
	require.Equal(t, held, s.m.n(), "frames were sent over somebody else's picture")

	s.p.Release()
	require.Eventually(t, func() bool { return s.m.n() > held }, time.Second, time.Millisecond)
	cancel()
	<-stopped
}

func rampSeries(n, offset int) Series {
	out := make(Series, n)
	for i := range out {
		out[i] = float64((i + offset) % 9)
	}
	return out
}

func solid(c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, Size, Size))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	return img
}

// Every one-second bucket gets a reading, even while transitions make the
// loop run late: a one-second trail draws a skipped bucket as a gap (found on
// the live run).
func TestEverySecondHasAReading(t *testing.T) {
	s := newScene(t, ringDashboard())
	seconds := map[int64]bool{}
	read := s.p.Read
	s.p.Read = func(ctx context.Context) Reading {
		seconds[s.c.now.Unix()] = true
		return read(ctx)
	}
	start := s.c.now.Unix()
	for i := range 30 {
		s.r.Set(readings.Coolant, 40+float64(i%7))
		s.run(time.Second)
	}
	for sec := start + 1; sec < s.c.now.Unix(); sec++ {
		require.True(t, seconds[sec], "no reading in second %d", sec-start)
	}
}
