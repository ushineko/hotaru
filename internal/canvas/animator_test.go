package canvas_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/canvas"
	"github.com/ushineko/hotaru/internal/canvas/canvastest"
	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/render"
	"github.com/ushineko/sanshoku"
)

const (
	soon = 2 * time.Second
	poll = 2 * time.Millisecond
)

// running is an animator on a fake canvas, started, with what it reported.
type running struct {
	canvas *canvastest.Device
	device *canvas.Device
	clock  *canvastest.Clock
	anim   *canvas.Animator
	done   chan error

	mu   sync.Mutex
	said []string
}

func start(t *testing.T, fake *canvastest.Device) *running {
	t.Helper()
	device, ok := canvas.New(fake)
	require.True(t, ok, "the fake is not a canvas")
	r := &running{canvas: fake, device: device, clock: canvastest.NewClock(), done: make(chan error, 1)}
	r.anim = canvas.NewAnimator(device, r.clock, func(format string, _ ...any) {
		r.mu.Lock()
		r.said = append(r.said, format)
		r.mu.Unlock()
	})
	ctx, cancel := context.WithCancel(t.Context())
	go func() { r.done <- r.anim.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		<-r.done
	})
	return r
}

func (r *running) reports() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.said)
}

func show(name string, p render.Params, interval time.Duration) canvas.Show {
	effect, _ := render.Lookup(name)
	return canvas.Show{Effect: effect, Params: p, Interval: interval}
}

func blue(n int) render.Params {
	p := render.Params{}
	for range n {
		p.Colours = append(p.Colours, colour.MustParse("blue"))
	}
	return p
}

func TestAStaticSceneSendsOneFrameAndThenNothing(t *testing.T) {
	// R3.2: the device holds its last frame, so a scene that does not move
	// is one frame and no ticker.
	r := start(t, canvastest.New(8))
	r.anim.Show(show(render.Static, blue(8), 0))

	require.Eventually(t, func() bool { return r.canvas.Frames() == 1 }, soon, poll)
	time.Sleep(50 * time.Millisecond)
	require.Equal(t, 1, r.canvas.Frames(), "a static scene sent more than one frame")
	require.Zero(t, r.clock.Running(), "a static scene left a ticker running")
	require.False(t, r.clock.Tick(t.Context()), "something was still ticking")

	held, ok := r.canvas.Showing(3)
	require.True(t, ok)
	require.Equal(t, uint8(255), held.B, "the frame the device holds is not the scene")

	status := r.anim.Status()
	require.Equal(t, render.Static, status.Effect)
	require.False(t, status.Drawing, "a static scene is reported as streaming rather than holding")
	require.Equal(t, 1, status.Frames)
}

func TestTheSameStaticSceneAgainSendsNothing(t *testing.T) {
	// R3.4, and the case reconcile makes: the frame it would send is the
	// one already showing.
	r := start(t, canvastest.New(4))
	r.anim.Show(show(render.Static, blue(4), 0))
	require.Eventually(t, func() bool { return r.canvas.Frames() == 1 }, soon, poll)

	r.anim.Show(show(render.Static, blue(4), 0))
	time.Sleep(50 * time.Millisecond)
	require.Equal(t, 1, r.canvas.Frames(), "an identical frame was sent again")

	// A different one is sent.
	r.anim.Show(show(render.Off, render.Params{}, 0))
	require.Eventually(t, func() bool { return r.canvas.Frames() == 2 }, soon, poll)
}

func TestAnAnimatedEffectTicksAtItsInterval(t *testing.T) {
	r := start(t, canvastest.New(6))
	r.anim.Show(show(render.Spectrum, render.Params{}, 40*time.Millisecond))

	require.Eventually(t, func() bool { return r.canvas.Frames() == 1 && r.clock.Running() == 1 }, soon, poll,
		"an animated effect did not draw its first frame at once")
	tickers := r.clock.Tickers()
	require.Len(t, tickers, 1)
	require.Equal(t, 40*time.Millisecond, tickers[0].Every, "it did not tick at the interval it was given")

	for i := 2; i <= 5; i++ {
		require.True(t, r.clock.Tick(t.Context()))
		require.Eventually(t, func() bool { return r.canvas.Frames() == i }, soon, poll,
			"tick %d did not send a frame", i)
	}
	status := r.anim.Status()
	require.True(t, status.Drawing)
	require.Equal(t, 40*time.Millisecond, status.Interval)
	require.InDelta(t, 25, status.Rate, 0.01, "the rate is not frames a second at this interval")
}

func TestTheIntervalIsNeverUnderTheFloorAndDefaultsToTheVendorsPace(t *testing.T) {
	fake := canvastest.New(6)
	fake.FloorAt = 16 * time.Millisecond
	r := start(t, fake)

	r.anim.Show(show(render.Wave, render.Params{}, 5*time.Millisecond))
	require.Eventually(t, func() bool { return len(r.clock.Tickers()) == 1 }, soon, poll)
	require.Equal(t, 16*time.Millisecond, r.clock.Tickers()[0].Every, "an interval under the floor was used")

	r.anim.Show(show(render.Breathing, render.Params{}, 0))
	require.Eventually(t, func() bool { return len(r.clock.Tickers()) == 2 }, soon, poll)
	require.Equal(t, canvas.DefaultInterval, r.clock.Tickers()[1].Every)
	require.Equal(t, 56*time.Millisecond, canvas.DefaultInterval, "the default is GG's measured median")
	require.True(t, r.clock.Tickers()[0].Stopped(), "the last effect's ticker kept running")
	require.Equal(t, 1, r.clock.Running())
}

func TestAnIdenticalFrameIsNotSent(t *testing.T) {
	// A moving effect at no brightness draws black on every tick: the first
	// is sent, the rest say what it said.
	r := start(t, canvastest.New(6))
	zero := 0
	r.anim.Show(show(render.Spectrum, render.Params{Brightness: &zero}, 0))
	require.Eventually(t, func() bool { return r.canvas.Frames() == 1 && r.clock.Running() == 1 }, soon, poll)

	for range 5 {
		require.True(t, r.clock.Tick(t.Context()))
	}
	time.Sleep(20 * time.Millisecond)
	require.Equal(t, 1, r.canvas.Frames(), "frames identical to the last were sent")
}

func TestTheSameMovingEffectAgainDoesNotRestartIt(t *testing.T) {
	// A reconcile sends what is showing. Breathing restarted at t=0 would go
	// dark mid-breath for a write that changed nothing.
	r := start(t, canvastest.New(3))
	r.anim.Show(show(render.Breathing, blue(3), 0))
	require.Eventually(t, func() bool { return len(r.clock.Tickers()) == 1 }, soon, poll)

	r.anim.Show(show(render.Breathing, blue(3), 0))
	time.Sleep(20 * time.Millisecond)
	require.Len(t, r.clock.Tickers(), 1, "the same effect was started again")
}

func TestAnUnacknowledgedFrameIsSaidOnceAndTheStreamCarriesOn(t *testing.T) {
	fake := canvastest.New(6)
	fake.SilentNext = 3
	r := start(t, fake)
	r.anim.Show(show(render.Spectrum, render.Params{}, 0))
	require.Eventually(t, func() bool { return r.clock.Running() == 1 }, soon, poll)

	for range 6 {
		require.True(t, r.clock.Tick(t.Context()))
	}
	require.Eventually(t, func() bool { return r.canvas.Frames() >= 3 }, soon, poll,
		"the stream stopped after a frame went unacknowledged: %d frames", r.canvas.Frames())
	require.Equal(t, 1, r.reports(), "an unacknowledged frame was reported more than once in a run")

	select {
	case err := <-r.done:
		t.Fatalf("an unacknowledged frame ended the run: %v", err)
	default:
	}
}

func TestAnUnacknowledgedStaticFrameIsSentAgainUntilItLands(t *testing.T) {
	// A static scene has no next tick of its own, so a lost frame is tried
	// again rather than leaving the device on the scene before it.
	fake := canvastest.New(4)
	fake.SilentNext = 2
	r := start(t, fake)
	r.anim.Show(show(render.Static, blue(4), 0))
	require.Eventually(t, func() bool { return r.clock.Running() == 1 }, soon, poll,
		"no retry was scheduled after the frame went unacknowledged")

	require.True(t, r.clock.Tick(t.Context()))
	require.True(t, r.clock.Tick(t.Context()))
	require.Eventually(t, func() bool { return r.canvas.Frames() == 1 }, soon, poll)
	require.Eventually(t, func() bool { return r.clock.Running() == 0 }, soon, poll,
		"the retry kept ticking after the frame landed")
	require.Equal(t, 1, r.reports())
}

func TestARedrawSendsTheSameFrameAgain(t *testing.T) {
	// A device behind a receiver can reboot without the animator hearing of
	// it, so a restore asks for the frame again although it has not changed.
	r := start(t, canvastest.New(4))
	r.anim.Show(show(render.Static, blue(4), 0))
	require.Eventually(t, func() bool { return r.canvas.Frames() == 1 }, soon, poll)

	r.anim.Redraw()
	require.Eventually(t, func() bool { return r.canvas.Frames() == 2 }, soon, poll,
		"a redraw did not send the frame again")
	time.Sleep(50 * time.Millisecond)
	require.Equal(t, 2, r.canvas.Frames(), "a redraw left something ticking")
	require.Zero(t, r.clock.Running())
}

func TestAGoneDeviceEndsTheRun(t *testing.T) {
	fake := canvastest.New(6)
	fake.GoneNext = true
	r := start(t, fake)
	r.anim.Show(show(render.Static, blue(6), 0))

	select {
	case err := <-r.done:
		require.ErrorIs(t, err, sanshoku.ErrGone)
		r.done <- err // for the cleanup
	case <-time.After(soon):
		t.Fatal("a device that had gone did not end the run")
	}
	select {
	case <-r.device.Gone():
	default:
		t.Fatal("the adapter did not say the device had gone")
	}
	require.True(t, fake.Closed(), "a device that had gone was not closed")
}

func TestStoppingSendsNothing(t *testing.T) {
	// R4.1: the animator stops and the device keeps its last frame. Release
	// is something a person asks for, never a side effect of stopping.
	fake := canvastest.New(4)
	device, _ := canvas.New(fake)
	anim := canvas.NewAnimator(device, canvastest.NewClock(), nil)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- anim.Run(ctx) }()

	anim.Show(show(render.Static, blue(4), 0))
	require.Eventually(t, func() bool { return fake.Frames() == 1 }, soon, poll)
	cancel()
	require.NoError(t, <-done)
	require.Equal(t, 1, fake.Frames())
	require.Zero(t, fake.Released(), "stopping handed the lighting back")
}
