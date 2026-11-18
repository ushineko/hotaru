package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/hotaru/internal/canvas"
	"github.com/ushineko/hotaru/internal/canvas/canvastest"
	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/openrgb"
	"github.com/ushineko/hotaru/internal/service"
	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/lighting"
)

// holdingKeyboard is the fake keyboard's stored effects on a canvas that
// keeps its frame, as the Kraken does: it is not lighting.Fading.
type holdingKeyboard struct{ k *canvastest.Keyboard }

func (h holdingKeyboard) Identity() sanshoku.Identity                         { return h.k.Identity() }
func (h holdingKeyboard) Close() error                                        { return h.k.Close() }
func (h holdingKeyboard) Keys() []lighting.Key                                { return h.k.Keys() }
func (h holdingKeyboard) Floor() time.Duration                                { return h.k.Floor() }
func (h holdingKeyboard) Release(ctx context.Context) error                   { return h.k.Release(ctx) }
func (h holdingKeyboard) Effects() []string                                   { return h.k.Effects() }
func (h holdingKeyboard) Effect(ctx context.Context) (lighting.Effect, error) { return h.k.Effect(ctx) }
func (h holdingKeyboard) Frame(ctx context.Context, px []lighting.Pixel) error {
	return h.k.Frame(ctx, px)
}
func (h holdingKeyboard) SetEffect(ctx context.Context, name string) (bool, error) {
	return h.k.SetEffect(ctx, name)
}
func (h holdingKeyboard) SetColour(ctx context.Context, c lighting.Colour) (bool, error) {
	return h.k.SetColour(ctx, c)
}

/*
A solid colour on a canvas that keeps its frame, with no effect of its own
known to be running, is drawn, not handed to the firmware's first coloured
effect: on the Kraken that is Fading, and the colour came out fading (spec
074).
*/
func TestASolidColourOnAHoldingCanvasIsDrawn(t *testing.T) {
	svc := service.New(nil, openrgb.NewFake(board()), "")
	svc.SetRecorder(recorder(t))
	fake := canvastest.NewKeyboard(6)
	fake.Name, fake.Path = "Test Holding Keyboard", "fake:holding"
	device, ok := canvas.New(holdingKeyboard{k: fake})
	require.True(t, ok)
	require.Zero(t, device.Hold())
	anim := canvas.NewAnimator(device, canvastest.NewClock(), nil)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { defer close(done); _ = anim.Run(ctx) }()
	t.Cleanup(func() { cancel(); <-done })
	svc.AttachCanvas(canvas.Attached{Device: device, Animator: anim})

	results, err := svc.Apply(t.Context(), service.Request{
		Colour: ptr(colour.MustParse("blue")), Devices: []string{"Test Holding Keyboard"},
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "Static", results[0].Mode, "a solid colour went to a firmware effect")
}
