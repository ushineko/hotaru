package daemon_test

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/canvas"
	"github.com/ushineko/hotaru/internal/canvas/canvastest"
	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/daemon"
	"github.com/ushineko/hotaru/internal/openrgb"
	"github.com/ushineko/hotaru/internal/service"
	"github.com/ushineko/hotaru/internal/state"
)

// boards is a sequence of canvas devices for an opener to hand out, one per
// look, with nothing found between them as a rebooting device is.
type boards struct {
	mu    sync.Mutex
	queue []*canvastest.Device
	looks int
}

func (b *boards) open(_ context.Context, skip func(string) bool) ([]*canvas.Device, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.looks++
	if len(b.queue) == 0 || skip(b.queue[0].Path) {
		return nil, canvas.ErrNoCanvas
	}
	next := b.queue[0]
	b.queue = b.queue[1:]
	device, _ := canvas.New(next)
	return []*canvas.Device{device}, nil
}

func canvasBoard() *canvastest.Device {
	fake := canvastest.New(4)
	fake.Name, fake.Path = "Test Canvas Board", "fake:canvas"
	return fake
}

func serving(t *testing.T, svc *service.Service, b *boards) func() {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		daemon.AttachCanvases(ctx, svc, b.open, quick, func(string, ...any) {})
	}()
	return func() { cancel(); <-done }
}

func TestACanvasThatGoesIsAttachedAgainAndItsScenePutBack(t *testing.T) {
	/*
		R1.4. OpenRGB exiting reboots the board, and the next frame is
		ErrGone. The attachment ends, the device is looked for with the
		cooler's backoff, and the attachment that finds it again sends the
		scene that was recorded, because a rebooted board shows its
		firmware's effect.
	*/
	store, err := state.Open(filepath.Join(t.TempDir(), "state.yml"))
	require.NoError(t, err)
	svc := service.New(nil, openrgb.NewFake(), "")
	svc.SetRecorder(store)

	first, second := canvasBoard(), canvasBoard()
	b := &boards{queue: []*canvastest.Device{first, second}}
	stop := serving(t, svc, b)

	require.Eventually(t, func() bool { return len(svc.Canvases()) == 1 }, 2*time.Second, time.Millisecond)
	red := colour.MustParse("red")
	results, err := svc.Apply(t.Context(), service.Request{Colour: &red})
	require.NoError(t, err)
	require.True(t, results[0].Applied, "%+v", results[0])
	require.Eventually(t, func() bool { return first.Frames() == 1 }, 2*time.Second, time.Millisecond)

	// The board reboots; the scene's next frame finds it gone.
	first.GoneNext = true
	blue := colour.MustParse("blue")
	_, err = svc.Apply(t.Context(), service.Request{Colour: &blue})
	require.NoError(t, err)

	require.Eventually(t, first.Closed, 2*time.Second, time.Millisecond, "a canvas that had gone was not closed")
	require.Eventually(t, func() bool { return second.Frames() == 1 }, 2*time.Second, time.Millisecond,
		"the canvas that came back was not sent its scene")
	last := second.Last()
	require.Equal(t, uint8(255), last[0].B, "the canvas that came back was sent something other than the scene")

	stop()
	require.True(t, second.Closed(), "stopping the service left the canvas open")
	require.Zero(t, second.Released(), "stopping the service handed the lighting back")
	require.Equal(t, 1, second.Frames(), "stopping the service sent a frame")
	require.Empty(t, svc.Canvases(), "a stopped attachment left the canvas attached")
}

func TestAReleasedCanvasIsAttachedAgainAndSentNothing(t *testing.T) {
	// R4.2: released, the board re-enumerates, and hotaru attaches it again
	// without writing to it until a scene asks.
	store, err := state.Open(filepath.Join(t.TempDir(), "state.yml"))
	require.NoError(t, err)
	svc := service.New(nil, openrgb.NewFake(), "")
	svc.SetRecorder(store)

	first, second := canvasBoard(), canvasBoard()
	b := &boards{queue: []*canvastest.Device{first, second}}
	stop := serving(t, svc, b)
	defer stop()

	require.Eventually(t, func() bool { return len(svc.Canvases()) == 1 }, 2*time.Second, time.Millisecond)
	red := colour.MustParse("red")
	_, err = svc.Apply(t.Context(), service.Request{Colour: &red})
	require.NoError(t, err)
	require.Eventually(t, func() bool { return first.Frames() == 1 }, 2*time.Second, time.Millisecond)

	_, err = svc.ReleaseCanvas(t.Context(), "canvas")
	require.NoError(t, err)
	require.Equal(t, 1, first.Released())

	require.Eventually(t, func() bool {
		b.mu.Lock()
		defer b.mu.Unlock()
		return len(b.queue) == 0
	}, 2*time.Second, time.Millisecond, "the released canvas was not looked for again")
	require.Eventually(t, func() bool { return len(svc.Canvases()) == 1 }, 2*time.Second, time.Millisecond)
	time.Sleep(50 * time.Millisecond)
	require.Zero(t, second.Frames(), "a released canvas was written to before a scene asked")
}
