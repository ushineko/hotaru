package dashboard

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/readings"
)

// panel records what it was shown. It is not a stand-in for the cooler: the
// screen itself is tested against the hardware (spec 012), and what is checked
// here is hotaru's decision about when to write, which is not the device's.
type panel struct {
	mu     sync.Mutex
	frames [][]byte
	err    error
}

func (p *panel) Show(_ context.Context, gif []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.err != nil {
		return p.err
	}
	p.frames = append(p.frames, append([]byte(nil), gif...))
	return nil
}

func (p *panel) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.frames)
}

func pusher(p *panel, r Reading) *Pusher {
	return NewPusher(p, func(context.Context) Reading { return r })
}

func TestAnIdleMachineIsWrittenOnceAndThenLeftAlone(t *testing.T) {
	/*
		Coolant sits at 37.5 C and the pump at 2600 rpm for hours. Every push
		costs the device a settling period during which it will not accept
		another, so writing a picture nobody could tell apart is not free.
	*/
	screen := &panel{}
	p := pusher(screen, reading())

	for range 5 {
		p.cycle(context.Background())
	}

	require.Equal(t, 1, screen.count(), "an unchanging machine was written to more than once")
}

func TestAChangedReadingIsPushed(t *testing.T) {
	screen := &panel{}
	r := reading()
	p := NewPusher(screen, func(context.Context) Reading { return r })

	p.cycle(context.Background())
	r.Set(readings.Coolant, 41.0)
	p.cycle(context.Background())

	require.Equal(t, 2, screen.count())
}

func TestTheTickOnlyAdvancesOnAnAcceptedPush(t *testing.T) {
	/*
		The indicator exists so that a screen which has stopped being written
		is distinguishable from a machine whose sensors are steady -- which is
		most of the time. If it advanced on every render it would also mark
		frames that were never sent, and the mark would mean nothing.
	*/
	screen := &panel{}
	p := pusher(screen, reading())

	p.cycle(context.Background())
	require.Equal(t, 1, p.tick)
	for range 3 {
		p.cycle(context.Background())
	}
	require.Equal(t, 1, p.tick, "the indicator advanced without anything being shown")
}

func TestAFailedPushIsNotCountedAsShown(t *testing.T) {
	// A frame that did not arrive must be sent again, not remembered as the
	// thing on the screen.
	screen := &panel{err: context.DeadlineExceeded}
	p := pusher(screen, reading())

	p.cycle(context.Background())
	screen.err = nil
	p.cycle(context.Background())

	require.Equal(t, 1, screen.count())
	require.Equal(t, 1, p.tick)
}

func TestTheSameFrameTwiceIsByteIdentical(t *testing.T) {
	// The gate compares what a frame says; this is what makes that safe. If
	// rendering were not deterministic the comparison would be a coin toss.
	require.Equal(t, Render(reading(), 3).GIF, Render(reading(), 3).GIF)
}

func TestSomebodyElseCanHaveTheScreen(t *testing.T) {
	/*
		A picture that is replaced two seconds later was not shown. So
		`hotaru screen show` takes the panel, and only asking for the
		dashboard back gives it up.
	*/
	screen := &panel{}
	p := pusher(screen, reading())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stopped := make(chan struct{})
	go func() { defer close(stopped); p.Run(ctx) }()

	require.Eventually(t, func() bool { return screen.count() == 1 }, time.Second, 5*time.Millisecond)
	p.Hold()

	// Releasing redraws at once rather than waiting for something to change:
	// the panel is showing somebody else's picture, which the gate cannot see.
	p.Release()
	require.Eventually(t, func() bool { return screen.count() == 2 }, time.Second, 5*time.Millisecond)

	cancel()
	<-stopped
}

func TestThePushFloorFollowsTheFrame(t *testing.T) {
	/*
		Measured, not chosen: a frame that never appears at one second appears
		reliably at two, and nothing in the protocol says so. The table lives
		in Floor's comment; this is the part that would notice somebody
		editing it by accident.
	*/
	require.Equal(t, time.Second, Floor(5*1024))
	require.Equal(t, 2*time.Second, Floor(8*1024))
	require.Equal(t, 2*time.Second, Floor(21*1024))
	require.Equal(t, 3*time.Second, Floor(64*1024))

	// And the floor that matters is the one this design actually asks for.
	require.Equal(t, 2*time.Second, Floor(len(Render(reading(), 0).GIF)))
}

func TestRenderingDoesNotGrowPerFrame(t *testing.T) {
	/*
		It runs for the life of the machine. A renderer whose per-frame cost
		creeps is a service that has to be restarted, which is the one thing
		a background dashboard must never need.
	*/
	r := reading()
	first := testing.AllocsPerRun(20, func() { _ = Render(r, 1) })
	later := testing.AllocsPerRun(20, func() { _ = Render(r, 1) })

	require.LessOrEqual(t, later, first*1.05,
		"allocation per frame grew between the first frames and the later ones")
}
