package canvas_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/canvas"
	"github.com/ushineko/hotaru/internal/canvas/canvastest"
	"github.com/ushineko/hotaru/internal/render"
	"github.com/ushineko/sanshoku/lighting"
)

// startKeyboard is an animator on a fake keyboard that takes its lights back.
func startKeyboard(t *testing.T) (*canvastest.Keyboard, *canvas.Device, *canvastest.Clock, *canvas.Animator) {
	t.Helper()
	fake := canvastest.NewKeyboard(8)
	device, ok := canvas.New(fake)
	require.True(t, ok)
	clock := canvastest.NewClock()
	anim := canvas.NewAnimator(device, clock, nil)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { defer close(done); _ = anim.Run(ctx) }()
	t.Cleanup(func() { cancel(); <-done })
	return fake, device, clock, anim
}

func TestAStillFrameOnAFadingCanvasIsSentAgainAtItsHold(t *testing.T) {
	// Spec 071 R4.1: the same frame again, at Hold, because that keeps it up.
	fake, device, clock, anim := startKeyboard(t)
	require.Equal(t, 500*time.Millisecond, device.Hold())
	anim.Show(show(render.Static, blue(8), 0))

	require.Eventually(t, func() bool { return fake.Frames() == 1 }, soon, poll)
	require.Eventually(t, func() bool { return clock.Running() == 1 }, soon, poll, "nothing holds the frame")
	tickers := clock.Tickers()
	require.Equal(t, 500*time.Millisecond, tickers[len(tickers)-1].Every)

	for want := 2; want <= 4; want++ {
		require.True(t, clock.Tick(t.Context()))
		require.Eventually(t, func() bool { return fake.Frames() == want }, soon, poll,
			"tick %d did not send the frame again", want-1)
	}
	require.Equal(t, lighting.Pixel{ID: 1, B: 255}, fake.Last()[0], "the frame sent again is not the scene")
	require.False(t, anim.Status().Drawing, "holding a still frame is reported as streaming")
}

func TestAnAnimatedEffectOnAFadingCanvasRunsAtItsInterval(t *testing.T) {
	// R4.2: its own frames keep the canvas up.
	_, _, clock, anim := startKeyboard(t)
	anim.Show(show(render.Breathing, blue(8), 100*time.Millisecond))
	require.Eventually(t, func() bool { return clock.Running() == 1 }, soon, poll)
	tickers := clock.Tickers()
	require.Equal(t, 100*time.Millisecond, tickers[len(tickers)-1].Every)
}

func TestNothingToShowStopsSending(t *testing.T) {
	// R2.3 and R4.3: the zero Show stops the hold, and sends nothing.
	fake, _, clock, anim := startKeyboard(t)
	anim.Show(show(render.Static, blue(8), 0))
	require.Eventually(t, func() bool { return clock.Running() == 1 }, soon, poll)

	anim.Show(canvas.Show{})
	require.Eventually(t, func() bool { return clock.Running() == 0 }, soon, poll, "the hold kept ticking")
	sent := fake.Frames()
	time.Sleep(30 * time.Millisecond)
	require.Equal(t, sent, fake.Frames())
	require.Empty(t, anim.Status().Effect)
}

func TestAStillFrameOnACanvasThatKeepsItIsSentOnce(t *testing.T) {
	// R4.1, the other half: the Apex is sent one frame and then nothing.
	r := start(t, canvastest.New(8))
	require.Zero(t, r.device.Hold())
	r.anim.Show(show(render.Static, blue(8), 0))
	require.Eventually(t, func() bool { return r.canvas.Frames() == 1 }, soon, poll)
	time.Sleep(30 * time.Millisecond)
	require.Zero(t, r.clock.Running())
}

func TestAKeyboardsFirmwareIsCachedAndWritten(t *testing.T) {
	fake := canvastest.NewKeyboard(8)
	device, ok := canvas.New(fake)
	require.True(t, ok)
	_, known := device.Firmware()
	require.False(t, known, "the firmware is known before it is read")

	got, err := device.ReadFirmware(t.Context())
	require.NoError(t, err)
	require.Equal(t, canvastest.FactoryEffect, got.Name)

	wrote, err := device.SetFirmware(t.Context(), "reaction", &lighting.Colour{R: 1})
	require.NoError(t, err)
	require.True(t, wrote)
	now, _ := device.Firmware()
	require.Equal(t, "Reaction", now.Name)
	require.Equal(t, lighting.Colour{R: 1}, *now.Colour)

	wrote, err = device.SetFirmware(t.Context(), "Reaction", &lighting.Colour{R: 1})
	require.NoError(t, err)
	require.False(t, wrote, "an unchanged effect and colour were written")

	// Off takes no colour, and asking for one is not an error.
	_, err = device.SetFirmware(t.Context(), "Off", &lighting.Colour{R: 1})
	require.NoError(t, err)

	answered, askable := device.Answers(t.Context())
	require.True(t, askable)
	require.True(t, answered)
	fake.SwitchOff()
	answered, _ = device.Answers(t.Context())
	require.False(t, answered)
}
