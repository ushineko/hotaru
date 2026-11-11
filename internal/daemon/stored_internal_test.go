package daemon

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/canvas"
	"github.com/ushineko/hotaru/internal/canvas/canvastest"
	"github.com/ushineko/hotaru/internal/service"
	"github.com/ushineko/hotaru/internal/state"
	"github.com/ushineko/sanshoku/lighting"
)

// watchedKeyboard is a fake keyboard as the watcher sees it, with a status
// a test sets.
type watchedKeyboard struct {
	*canvas.Device
	fake    *canvastest.Keyboard
	drawing atomic.Value // string
}

func (w *watchedKeyboard) Status() canvas.Status {
	effect, _ := w.drawing.Load().(string)
	return canvas.Status{Effect: effect}
}

// host is a service as the watcher sees it: a recorded mode, and put-backs
// counted.
type host struct {
	mu       sync.Mutex
	recorded string
	putBacks int
	device   *watchedKeyboard
}

func (h *host) Desired() state.Snapshot {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.recorded == "" {
		return state.Snapshot{}
	}
	return state.Snapshot{Devices: map[string]state.Device{h.device.Name(): {Mode: h.recorded}}}
}

func (h *host) PutBack(ctx context.Context, _ string) (service.Restore, error) {
	h.mu.Lock()
	h.putBacks++
	h.mu.Unlock()
	_, err := h.device.SetFirmware(ctx, h.recorded, &lighting.Colour{R: 0x80, B: 0xff})
	return service.Restore{Applied: 1}, err
}

func (h *host) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.putBacks
}

// watching starts the watcher on a keyboard recorded on Ripples shining in
// purple, and returns the tick to drive it with.
func watching(t *testing.T) (*host, *watchedKeyboard, chan<- time.Time, func()) {
	t.Helper()
	return watchingWith(t, "Ripples shining", "")
}

// watchingWith starts the watcher with a recorded mode and a drawn effect,
// each "" for none, set before the watcher's first tick.
func watchingWith(t *testing.T, recorded, drawing string) (*host, *watchedKeyboard, chan<- time.Time, func()) {
	t.Helper()
	fake := canvastest.NewKeyboard(6)
	device, ok := canvas.New(fake)
	require.True(t, ok)
	w := &watchedKeyboard{Device: device, fake: fake}
	w.drawing.Store(drawing)
	h := &host{recorded: recorded, device: w}
	_, err := device.SetFirmware(t.Context(), "Ripples shining", &lighting.Colour{R: 0x80, B: 0xff})
	require.NoError(t, err)

	tick := make(chan time.Time)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		watchStored(ctx, h, w, tick, time.Now, func(string, ...any) {})
	}()
	stop := func() { cancel(); <-done }
	t.Cleanup(stop)
	return h, w, tick, stop
}

// ticks delivers n ticks and waits for the watcher to take each.
func ticks(t *testing.T, tick chan<- time.Time, n int) {
	t.Helper()
	for range n {
		select {
		case tick <- time.Now():
		case <-time.After(2 * time.Second):
			t.Fatal("the watcher did not take a tick")
		}
	}
}

func TestAPowerCycleIsPutBackOnceTheKeyboardAnswers(t *testing.T) {
	// Spec 071 R6.1, R6.2: silent, then answering on the factory effect.
	h, w, tick, _ := watching(t)
	ticks(t, tick, 2)
	require.True(t, w.Watch().Answering)

	w.fake.SwitchOff()
	ticks(t, tick, 2)
	require.False(t, w.Watch().Answering)
	w.fake.PowerCycle()
	ticks(t, tick, 2) // the second waits for the first to finish
	require.Equal(t, 1, h.count(), "the factory settings were not put back")
	require.False(t, w.Watch().PutBack.IsZero())
	effect, _ := w.fake.Effect(t.Context())
	require.Equal(t, "Ripples shining", effect.Name)
}

func TestWakingFromSleepIsNotAPowerCycle(t *testing.T) {
	// The keyboard keeps its settings through a nap, and a change made with
	// its own keys before the nap is kept (R6.3).
	h, w, tick, _ := watching(t)
	ticks(t, tick, 1)
	w.fake.Press("Reaction", nil)
	w.fake.SwitchOff()
	ticks(t, tick, 2)
	w.fake.SwitchOn()
	ticks(t, tick, 2)
	require.Zero(t, h.count(), "a keyboard that woke on a person's choice was overwritten")
}

func TestAChangeWhileAnsweringIsKept(t *testing.T) {
	// R6.3.
	h, w, tick, _ := watching(t)
	ticks(t, tick, 1)
	w.fake.Press(canvastest.FactoryEffect, nil)
	ticks(t, tick, 3)
	require.Zero(t, h.count())
}

func TestTheWatcherAsksNothingWhileFramesAreDrawn(t *testing.T) {
	// R6.4.
	_, w, tick, _ := watchingWith(t, "Ripples shining", "Static")
	ticks(t, tick, 3)
	require.Zero(t, w.fake.Asked(), "the watcher asked while frames were drawn")
}

func TestTheWatcherAsksNothingWithNothingStored(t *testing.T) {
	// R6.1's condition on the recorded state.
	_, w, tick, _ := watchingWith(t, "", "")
	ticks(t, tick, 3)
	require.Zero(t, w.fake.Asked(), "the watcher asked with nothing stored to keep")
}

func TestADeviceWithoutStoredEffectsIsNotWatched(t *testing.T) {
	device, ok := canvas.New(canvastest.New(6))
	require.True(t, ok)
	done := make(chan struct{})
	go func() {
		defer close(done)
		watchStored(t.Context(), &host{}, &watchedKeyboard{Device: device}, make(chan time.Time),
			time.Now, func(string, ...any) {})
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the watcher runs on a device with no stored effect")
	}
}

func TestAFailedFirstReadIsTriedAgain(t *testing.T) {
	// Measured: the first read after a power-on can lose packets. The next
	// tick reads again, and the effect is put back then.
	h, w, tick, _ := watching(t)
	ticks(t, tick, 1)
	w.fake.SwitchOff()
	ticks(t, tick, 2)
	w.fake.PowerCycle()
	w.fake.FailReads(1)
	ticks(t, tick, 3) // the first read fails, the second reads Neon stream
	require.Equal(t, 1, h.count(), "the failed read was not tried again")
}
