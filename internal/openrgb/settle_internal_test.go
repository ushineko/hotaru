package openrgb

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/devices"
)

/*
A mode change and the frame after it (spec 068).

OpenRGB 1.0 writes a mode change from the controller's own thread after it
has answered, and the frame that follows from the network thread, so the
frame has to wait for the mode to settle. The race itself is the hardware's
and is checked live; these say what the wait does.
*/

// timers is a clock for holds: it remembers what was due, and runs it when
// the test says time has passed.
type timers struct {
	mu   sync.Mutex
	due  []func()
	asks []time.Duration
}

func (c *timers) after(d time.Duration, f func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.asks = append(c.asks, d)
	c.due = append(c.due, f)
}

// pass fires everything that is due.
func (c *timers) pass() {
	c.mu.Lock()
	due := c.due
	c.due = nil
	c.mu.Unlock()
	for _, f := range due {
		f()
	}
}

func heldBy(clock *timers) *holds {
	h := newHolds()
	h.after = clock.after
	return h
}

// waiting starts a wait and reports when it ends.
func waiting(ctx context.Context, h *holds, device string) <-chan error {
	done := make(chan error, 1)
	go func() { done <- h.wait(ctx, device) }()
	return done
}

const stillHeld = 50 * time.Millisecond

func TestAFrameAfterAModeChangeWaitsForItToSettle(t *testing.T) {
	clock := &timers{}
	h := heldBy(clock)

	h.start("NZXT Kraken")
	done := waiting(t.Context(), h, "NZXT Kraken")

	select {
	case <-done:
		t.Fatal("the frame went before the mode change had settled")
	case <-time.After(stillHeld):
	}
	require.Equal(t, []time.Duration{SettleDelay}, clock.asks)

	clock.pass()
	require.NoError(t, <-done)
}

func TestAFrameToAnotherDeviceIsNotHeld(t *testing.T) {
	h := heldBy(&timers{})
	h.start("NZXT Kraken")

	require.NoError(t, h.wait(t.Context(), "Keychron K4 HE"))
}

func TestAFrameAfterTheModeHasSettledDoesNotWait(t *testing.T) {
	clock := &timers{}
	h := heldBy(clock)
	h.start("NZXT Kraken")
	clock.pass()

	require.NoError(t, h.wait(t.Context(), "nzxt kraken"), "names are matched as the server matches them")
}

func TestTheHoldRunsFromTheLastModeChange(t *testing.T) {
	clock := &timers{}
	h := heldBy(clock)
	h.start("NZXT Kraken")
	clock.pass()
	h.start("NZXT Kraken")

	done := waiting(t.Context(), h, "NZXT Kraken")
	select {
	case <-done:
		t.Fatal("a second mode change did not hold the frame")
	case <-time.After(stillHeld):
	}
	clock.pass()
	require.NoError(t, <-done)
}

func TestACallerThatGivesUpIsNotHeld(t *testing.T) {
	h := heldBy(&timers{})
	h.start("NZXT Kraken")

	ctx, cancel := context.WithCancel(t.Context())
	done := waiting(ctx, h, "NZXT Kraken")
	cancel()

	require.ErrorIs(t, <-done, context.Canceled)
}

func TestAHeldFrameThatIsGivenUpOnWritesNothing(t *testing.T) {
	socket, asked := serve(t, true, func(net.Conn, func(uint32)) {})
	conn := connect(t, socket)
	conn.holds = heldBy(&timers{})
	conn.holds.start("NZXT Kraken")

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := conn.SetFrame(ctx, "NZXT Kraken", framed(24))

	require.ErrorIs(t, err, context.Canceled)
	select {
	case <-asked:
		t.Fatal("the server was asked for something while the frame was held")
	default:
	}
}

func TestAWriteToAnotherDeviceGoesAheadWhileOneIsHeld(t *testing.T) {
	socket, asked := serve(t, true, func(_ net.Conn, reply func(uint32)) { reply(0) })
	conn := connect(t, socket)
	conn.holds = heldBy(&timers{})
	conn.holds.start("NZXT Kraken")

	// The server has no devices, so the write fails; that it reached the
	// server at all is the point.
	_ = conn.SetFrame(t.Context(), "Keychron K4 HE", framed(24))

	select {
	case <-asked:
	case <-time.After(brief):
		t.Fatal("a write to another device was held")
	}
}

func framed(n int) devices.Frame {
	return devices.Frame{Colours: make([]colour.Colour, n)}
}
