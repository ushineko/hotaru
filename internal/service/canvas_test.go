package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/canvas"
	"github.com/ushineko/hotaru/internal/canvas/canvastest"
	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/config"
	"github.com/ushineko/hotaru/internal/devices"
	"github.com/ushineko/hotaru/internal/scenes"
	"github.com/ushineko/hotaru/internal/service"
	"github.com/ushineko/hotaru/internal/service/servicetest"
	"github.com/ushineko/sanshoku/lighting"
)

const (
	drawnName  = "Test Canvas Board"
	drawnPath  = "/dev/hidraw-test9"
	twinName   = "Test Canvas Board (OpenRGB)"
	eventually = 2 * time.Second
	every      = 2 * time.Millisecond
)

// drawing is a canvas attached to a service, with its animator running.
type drawing struct {
	fake   *canvastest.Device
	clock  *canvastest.Clock
	attach canvas.Attached
}

func attachDrawn(t *testing.T, svc *service.Service) *drawing {
	t.Helper()
	fake := canvastest.New(6)
	fake.Name, fake.Path = drawnName, drawnPath
	device, ok := canvas.New(fake)
	require.True(t, ok)
	clock := canvastest.NewClock()
	anim := canvas.NewAnimator(device, clock, nil)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { defer close(done); _ = anim.Run(ctx) }()
	t.Cleanup(func() { cancel(); <-done })

	d := &drawing{fake: fake, clock: clock, attach: canvas.Attached{Device: device, Animator: anim}}
	svc.AttachCanvas(d.attach)
	return d
}

func (d *drawing) frames(t *testing.T, n int) {
	t.Helper()
	require.Eventually(t, func() bool { return d.fake.Frames() == n }, eventually, every,
		"the canvas has %d frames, not %d", d.fake.Frames(), n)
}

func viewOf(t *testing.T, svc *service.Service, name string) service.View {
	t.Helper()
	views, err := svc.List(t.Context())
	require.NoError(t, err)
	for _, v := range views {
		if v.Device.Name == name {
			return v
		}
	}
	t.Fatalf("%s is not listed", name)
	return service.View{}
}

func TestACanvasIsListedAsADeviceWithItsRenderersAsModes(t *testing.T) {
	// R1.2: one zone, a light per key named by the key, and hotaru's
	// renderers where a firmware device has modes.
	svc := service.New(nil)
	attachDrawn(t, svc)

	view := viewOf(t, svc, drawnName)
	device := view.Device
	require.True(t, view.InScope)
	require.NotNil(t, device.Canvas, "a canvas device is not marked as drawn")
	require.Equal(t, 6, device.LEDCount)
	require.Equal(t, []devices.Zone{{Name: "Keys", Shape: devices.ShapeGrid, First: 0, Count: 6}}, device.Zones)
	require.Equal(t, "L1", device.LEDNames[0])
	require.Equal(t, []string{"Static", "Breathing", "Spectrum", "Rainbow Wave", "Off"}, device.ModeNames()[:5])
	require.Contains(t, device.ModeNames(), "Water Cooler", "the Kraken's effects are offered (spec 077)")

	breathing, _ := device.Mode("breathing")
	require.True(t, breathing.ModeColour, "Breathing does not say it shows one colour")
	require.NotNil(t, breathing.Speed)
	static, _ := device.Mode("static")
	require.True(t, static.PerLED)
	require.True(t, static.Brightness)
}

func TestAStaticSceneIsOneFrameOnTheCanvas(t *testing.T) {
	svc := service.New(nil)
	svc.SetRecorder(recorder(t))
	d := attachDrawn(t, svc)

	results, err := svc.Apply(t.Context(), service.Request{
		Colour:      ptr(colour.MustParse("blue")),
		Assignments: solid(drawnName+"/keys[0:1]", "red"),
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.True(t, results[0].Applied, "%+v", results[0])
	require.Equal(t, "Static", results[0].Mode)
	require.Empty(t, results[0].Unconfirmed)

	d.frames(t, 1)
	time.Sleep(30 * time.Millisecond)
	require.Equal(t, 1, d.fake.Frames(), "a static scene was more than one frame")
	require.Zero(t, d.clock.Running(), "a static scene left something ticking")
	last := d.fake.Last()
	require.Equal(t, lighting.Pixel{ID: 1, R: 255}, last[0])
	require.Equal(t, lighting.Pixel{ID: 3, B: 255}, last[2])

	// R3.6: reconciling the device sends the recorded frame, which is the
	// one showing. (A full restore redraws once on purpose; see
	// TestAFullRestoreRedrawsACanvasAndANamedOneDoesNot.)
	restore, err := svc.Reconcile(t.Context(), []string{drawnName})
	require.NoError(t, err)
	require.Equal(t, 1, restore.Applied)
	time.Sleep(30 * time.Millisecond)
	require.Equal(t, 1, d.fake.Frames(), "a reconcile re-sent the frame the canvas is holding")

	view := viewOf(t, svc, drawnName)
	require.False(t, view.Device.Canvas.Drawing)
	require.Equal(t, "Static", view.Device.ActiveMode)
}

func TestASceneEffectIsARendererOnTheCanvas(t *testing.T) {
	// R5.1: a scene names a canvas's renderer as it names a firmware mode,
	// with its colour and speed.
	svc := service.New(nil)
	svc.SetRecorder(recorder(t))
	d := attachDrawn(t, svc)
	speed := 100
	results, err := svc.Apply(t.Context(), service.Request{
		Colour: ptr(colour.MustParse("white")),
		Effects: map[string]scenes.Effect{
			"canvas board": {Mode: "breathing", Colour: "#00ff00", Speed: &speed},
		},
	})
	require.NoError(t, err)
	require.True(t, results[0].Applied, "%+v", results[0])
	require.Equal(t, "Breathing", results[0].Mode)
	require.Empty(t, results[0].Problems)

	require.Eventually(t, func() bool { return d.clock.Running() == 1 }, eventually, every,
		"an animated effect is not ticking")
	require.Equal(t, canvas.DefaultInterval, d.clock.Tickers()[0].Every)

	// Half of the fastest period is 500 ms: the effect's own colour at full.
	for range 9 {
		require.True(t, d.clock.Tick(t.Context()))
	}
	require.Eventually(t, func() bool {
		last := d.fake.Last()
		return last != nil && last[0].G > 250 && last[0].R == 0
	}, eventually, every, "Breathing did not breathe in the effect's colour")

	kept := svc.Desired().Devices[drawnName]
	require.Equal(t, "Breathing", kept.Mode)
	require.Equal(t, "#00ff00", kept.ModeColour)
}

func TestTheDeviceRulesBrightnessIsAMultiply(t *testing.T) {
	// R2.4.
	half := 50
	cfg := &config.Config{Devices: []config.DeviceRule{{Match: "canvas", Brightness: &half}}}
	svc := service.New(cfg)
	d := attachDrawn(t, svc)

	_, err := svc.Apply(t.Context(), service.Request{Colour: ptr(colour.Colour{R: 200, G: 100, B: 50})})
	require.NoError(t, err)
	d.frames(t, 1)
	require.Equal(t, lighting.Pixel{ID: 1, R: 100, G: 50, B: 25}, d.fake.Last()[0])
}

func TestTheFrameIntervalIsTheRulesAndNeverUnderTheFloor(t *testing.T) {
	cfg := &config.Config{Devices: []config.DeviceRule{{Match: "canvas", FrameInterval: config.Duration(time.Millisecond)}}}
	svc := service.New(cfg)
	d := attachDrawn(t, svc)

	_, err := svc.Apply(t.Context(), service.Request{
		Colour:  ptr(colour.MustParse("white")),
		Effects: map[string]scenes.Effect{"canvas": {Mode: "Rainbow Wave"}},
	})
	require.NoError(t, err)
	require.Eventually(t, func() bool { return d.clock.Running() == 1 }, eventually, every)
	require.Equal(t, 16*time.Millisecond, d.clock.Tickers()[0].Every, "the interval went under the floor")
}

func TestOffIsABlackFrameAndForgetsTheDevice(t *testing.T) {
	svc := service.New(nil)
	svc.SetRecorder(recorder(t))
	d := attachDrawn(t, svc)

	_, err := svc.Apply(t.Context(), service.Request{Colour: ptr(colour.MustParse("red"))})
	require.NoError(t, err)
	d.frames(t, 1)
	require.Contains(t, svc.Desired().Names(), drawnName)

	results, err := svc.Apply(t.Context(), service.Request{Off: true})
	require.NoError(t, err)
	require.True(t, results[0].Applied, "%+v", results[0])
	d.frames(t, 2)
	require.Equal(t, lighting.Pixel{ID: 1}, d.fake.Last()[0])
	require.NotContains(t, svc.Desired().Names(), drawnName)
}

func TestReleaseHandsTheLightingBackAndForgetsTheScene(t *testing.T) {
	// R4.2, and the half of its criterion the service owns: once released,
	// nothing is recorded, so the device attached again is sent nothing
	// until a scene asks.
	svc := service.New(nil)
	svc.SetRecorder(recorder(t))
	d := attachDrawn(t, svc)
	_, err := svc.Apply(t.Context(), service.Request{Colour: ptr(colour.MustParse("red"))})
	require.NoError(t, err)

	_, err = svc.ReleaseCanvas(t.Context(), "no such board")
	require.ErrorIs(t, err, service.ErrNoCanvas)
	require.Contains(t, err.Error(), drawnName, "the refusal does not say what can be released")

	name, err := svc.ReleaseCanvas(t.Context(), "canvas")
	require.NoError(t, err)
	require.Equal(t, drawnName, name)
	require.Equal(t, 1, d.fake.Released())
	require.NotContains(t, svc.Desired().Names(), drawnName)
	select {
	case <-d.attach.Gone():
	default:
		t.Fatal("a released device is not gone, though it re-enumerates")
	}
}

func ptr[T any](v T) *T { return &v }

func TestAFullRestoreRedrawsACanvasAndANamedOneDoesNot(t *testing.T) {
	// Through a receiver the first canvas device reboots behind a node that
	// never changes, so a restore -- OpenRGB reconnecting, whose exit is
	// what reboots it -- sends the frame again even though it is unchanged.
	// A reconcile of named devices (re-assert, a preview ending) does not.
	svc := service.New(nil)
	svc.SetRecorder(recorder(t))
	d := attachDrawn(t, svc)
	other := genericStrip()
	servicetest.Attach(t, svc, other)
	_, err := svc.Apply(t.Context(), service.Request{Colour: ptr(colour.MustParse("blue"))})
	require.NoError(t, err)
	d.frames(t, 1)

	_, err = svc.Reconcile(t.Context(), []string{other.Name})
	require.NoError(t, err)
	time.Sleep(50 * time.Millisecond)
	require.Equal(t, 1, d.fake.Frames(), "a named reconcile redrew the canvas")

	_, err = svc.Reconcile(t.Context(), nil)
	require.NoError(t, err)
	d.frames(t, 2)
}
