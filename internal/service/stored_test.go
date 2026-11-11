package service_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/canvas"
	"github.com/ushineko/hotaru/internal/canvas/canvastest"
	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/devices"
	"github.com/ushineko/hotaru/internal/openrgb"
	"github.com/ushineko/hotaru/internal/scenes"
	"github.com/ushineko/hotaru/internal/service"
	"github.com/ushineko/sanshoku/lighting"
)

const keyboardName = "Test Fading Keyboard"

// fadingKeyboard is a fading canvas with stored effects, attached to a service.
type fadingKeyboard struct {
	fake   *canvastest.Keyboard
	clock  *canvastest.Clock
	device *canvas.Device
	said   []string
	mu     sync.Mutex
}

func attachKeyboard(t *testing.T, svc *service.Service) *fadingKeyboard {
	t.Helper()
	fake := canvastest.NewKeyboard(6)
	fake.Name, fake.Path = keyboardName, "fake:keyboard"
	device, ok := canvas.New(fake)
	require.True(t, ok)
	k := &fadingKeyboard{fake: fake, clock: canvastest.NewClock(), device: device}
	device.SetReport(func(format string, _ ...any) {
		k.mu.Lock()
		defer k.mu.Unlock()
		k.said = append(k.said, format)
	})
	_, err := device.ReadFirmware(t.Context())
	require.NoError(t, err)
	anim := canvas.NewAnimator(device, k.clock, nil)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { defer close(done); _ = anim.Run(ctx) }()
	t.Cleanup(func() { cancel(); <-done })
	svc.AttachCanvas(canvas.Attached{Device: device, Animator: anim})
	return k
}

// noWaiting makes firmware writes count their waits instead of sleeping.
func noWaiting(t *testing.T) *[]time.Duration {
	t.Helper()
	var (
		now    = time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
		waited []time.Duration
	)
	t.Cleanup(service.SetFlashClock(
		func() time.Time { return now },
		func(_ context.Context, d time.Duration) bool {
			waited = append(waited, d)
			now = now.Add(d)
			return true
		}))
	return &waited
}

func writes(k *fadingKeyboard) int {
	effects, colours := k.fake.Writes()
	return effects + colours
}

func TestAKeyboardListsItsOwnEffectsAsStoredModesBeforeTheRenderers(t *testing.T) {
	// R2.1, R2.2, R7.2.
	svc := service.New(nil, openrgb.NewFake(), "")
	k := attachKeyboard(t, svc)
	k.fake.Press("Reaction", &lighting.Colour{R: 0x80, B: 0xff})
	_, err := k.device.ReadFirmware(t.Context())
	require.NoError(t, err)

	device := viewOf(t, svc, keyboardName).Device
	names := device.ModeNames()
	require.Equal(t, "Off", names[0])
	require.Equal(t, "Self-define", names[19])
	require.Equal(t, []string{"Static", "Breathing", "Spectrum", "Rainbow Wave"}, names[20:],
		"the renderers follow, and the firmware's Off takes the name")

	reaction, _ := device.Mode("Reaction")
	require.True(t, reaction.Stored)
	require.True(t, reaction.ModeColour)
	require.Equal(t, colour.MustParse("#8000ff"), reaction.Colour)
	off, _ := device.Mode("Off")
	require.True(t, off.Stored)
	require.False(t, off.ModeColour, "Off takes a colour")
	static, _ := device.Mode("Static")
	require.False(t, static.Stored)

	// Nothing drawn yet: the keyboard shows its own effect.
	require.True(t, device.Canvas.Own)
	require.Equal(t, "Reaction", device.ActiveMode)
}

func TestASolidColourKeepsTheRunningEffectAndSetsItsColour(t *testing.T) {
	// R3.1: light set purple on a keyboard running Reaction.
	waited := noWaiting(t)
	svc := service.New(nil, openrgb.NewFake(), "")
	svc.SetRecorder(recorder(t))
	k := attachKeyboard(t, svc)
	k.fake.Press("Reaction", nil)
	_, err := k.device.ReadFirmware(t.Context())
	require.NoError(t, err)

	results, err := svc.Apply(t.Context(), service.Request{Colour: ptr(colour.MustParse("#8000ff"))})
	require.NoError(t, err)
	require.True(t, results[0].Applied, "%+v", results[0])
	require.Equal(t, "Reaction", results[0].Mode)
	require.Empty(t, results[0].Unconfirmed)

	effect, err := k.fake.Effect(t.Context())
	require.NoError(t, err)
	require.Equal(t, "Reaction", effect.Name)
	require.Equal(t, lighting.Colour{R: 0x80, B: 0xff}, *effect.Colour)
	effects, colours := k.fake.Writes()
	require.Equal(t, 0, effects, "the effect was rewritten")
	require.Equal(t, 1, colours)
	require.Zero(t, k.fake.Frames(), "a frame was drawn over the effect")
	require.Empty(t, *waited)

	view := viewOf(t, svc, keyboardName)
	require.Equal(t, "Reaction", view.Device.ActiveMode)
	require.True(t, view.Device.Canvas.Own)
	require.Equal(t, []string{keyboardName + ": wrote the effect %q%s to its storage"}, k.said)

	// The same again writes nothing and does not wait.
	_, err = svc.Apply(t.Context(), service.Request{Colour: ptr(colour.MustParse("#8000ff"))})
	require.NoError(t, err)
	require.Equal(t, 1, writes(k))
}

func TestASolidColourOnAColourlessEffectGoesToFixedOn(t *testing.T) {
	// R3.2.
	noWaiting(t)
	svc := service.New(nil, openrgb.NewFake(), "")
	svc.SetRecorder(recorder(t))
	k := attachKeyboard(t, svc)
	k.fake.Press("Off", nil)
	_, err := k.device.ReadFirmware(t.Context())
	require.NoError(t, err)

	results, err := svc.Apply(t.Context(), service.Request{Colour: ptr(colour.MustParse("red"))})
	require.NoError(t, err)
	require.Equal(t, "Fixed on", results[0].Mode, "%+v", results[0])
	effect, _ := k.fake.Effect(t.Context())
	require.Equal(t, "Fixed on", effect.Name)
	require.Equal(t, lighting.Colour{R: 0xff}, *effect.Colour)
}

func TestOffIsTheFirmwaresOff(t *testing.T) {
	// R3.3.
	noWaiting(t)
	svc := service.New(nil, openrgb.NewFake(), "")
	svc.SetRecorder(recorder(t))
	k := attachKeyboard(t, svc)

	results, err := svc.Apply(t.Context(), service.Request{Off: true})
	require.NoError(t, err)
	require.True(t, results[0].Applied, "%+v", results[0])
	require.Equal(t, "Off", results[0].Mode)
	effect, _ := k.fake.Effect(t.Context())
	require.Equal(t, "Off", effect.Name)
	require.Zero(t, k.fake.Frames())
}

func TestARendererDrawsAndLeavesTheStoredEffectAlone(t *testing.T) {
	// R2.4, and then R2.3: a firmware mode stops the animator.
	noWaiting(t)
	svc := service.New(nil, openrgb.NewFake(), "")
	svc.SetRecorder(recorder(t))
	k := attachKeyboard(t, svc)

	results, err := svc.Apply(t.Context(), service.Request{
		Colour:  ptr(colour.MustParse("white")),
		Effects: map[string]scenes.Effect{"fading keyboard": {Mode: "Rainbow Wave"}},
	})
	require.NoError(t, err)
	require.Equal(t, "Rainbow Wave", results[0].Mode, "%+v", results[0])
	require.Eventually(t, func() bool { return k.clock.Running() == 1 }, eventually, every)
	require.True(t, k.clock.Tick(t.Context()))
	require.Eventually(t, func() bool { return k.fake.Frames() >= 1 }, eventually, every)
	require.Zero(t, writes(k), "a renderer wrote the keyboard's storage")

	results, err = svc.Apply(t.Context(), service.Request{
		Colour:  ptr(colour.MustParse("#00ff00")),
		Effects: map[string]scenes.Effect{"fading keyboard": {Mode: "ripples shining"}},
	})
	require.NoError(t, err)
	require.Equal(t, "Ripples shining", results[0].Mode, "%+v", results[0])
	require.Eventually(t, func() bool { return k.clock.Running() == 0 }, eventually, every,
		"choosing the firmware's effect left the animator drawing")
	effect, _ := k.fake.Effect(t.Context())
	require.Equal(t, "Ripples shining", effect.Name)
	require.Equal(t, lighting.Colour{G: 0xff}, *effect.Colour)
}

func TestAReconcileWritesNoStorageAndKeepsAPersonsChange(t *testing.T) {
	// R5.1, R6.3.
	noWaiting(t)
	svc := service.New(nil, openrgb.NewFake(), "")
	svc.SetRecorder(recorder(t))
	k := attachKeyboard(t, svc)
	_, err := svc.Apply(t.Context(), service.Request{Colour: ptr(colour.MustParse("#8000ff"))})
	require.NoError(t, err)
	before := writes(k)

	// Unchanged: nothing written, nothing read.
	restore, err := svc.Reconcile(t.Context(), []string{keyboardName})
	require.NoError(t, err)
	require.Equal(t, 1, restore.Applied, "%+v", restore.Results)
	require.Equal(t, before, writes(k))

	// Changed with the keyboard's own keys: kept, and said.
	k.fake.Press("Reaction", &lighting.Colour{R: 0xff})
	_, err = k.device.ReadFirmware(t.Context())
	require.NoError(t, err)
	restore, err = svc.Reconcile(t.Context(), []string{keyboardName})
	require.NoError(t, err)
	require.Equal(t, before, writes(k), "a reconcile wrote the keyboard's storage")
	require.Contains(t, restore.Results[0].Skipped, "only when a person asks")
	require.Zero(t, k.fake.Frames(), "a reconcile drew over the keyboard's effect")
	effect, _ := k.fake.Effect(t.Context())
	require.Equal(t, "Reaction", effect.Name)

	// PutBack is the reconcile that may write it.
	restore, err = svc.PutBack(t.Context(), keyboardName)
	require.NoError(t, err)
	require.Equal(t, 1, restore.Applied, "%+v", restore.Results)
	effect, _ = k.fake.Effect(t.Context())
	require.Equal(t, neonOrRunning(t, svc), effect.Name)
}

// neonOrRunning is the effect hotaru recorded for the keyboard.
func neonOrRunning(t *testing.T, svc *service.Service) string {
	t.Helper()
	return svc.Desired().Devices[keyboardName].Mode
}

func TestAPreviewDrawsAColourAndSkipsAnotherEffect(t *testing.T) {
	// R5.2.
	noWaiting(t)
	svc := service.New(nil, openrgb.NewFake(), "")
	svc.SetRecorder(recorder(t))
	k := attachKeyboard(t, svc)
	_, err := svc.Apply(t.Context(), service.Request{Colour: ptr(colour.MustParse("#8000ff"))})
	require.NoError(t, err)
	before := writes(k)

	results, err := svc.Apply(t.Context(), service.Request{Preview: true, Colour: ptr(colour.MustParse("#00ff00"))})
	require.NoError(t, err)
	require.True(t, results[0].Applied, "%+v", results[0])
	require.Equal(t, before, writes(k), "a preview wrote the keyboard's storage")
	require.Eventually(t, func() bool { return k.fake.Frames() >= 1 }, eventually, every,
		"a preview of a colour was not drawn")
	require.Equal(t, lighting.Pixel{ID: 1, G: 0xff}, k.fake.Last()[0])

	results, err = svc.Apply(t.Context(), service.Request{
		Preview: true, Colour: ptr(colour.MustParse("white")),
		Effects: map[string]scenes.Effect{"fading keyboard": {Mode: "Reaction"}},
	})
	require.NoError(t, err)
	require.Contains(t, results[0].Skipped, "a preview does not write", "%+v", results[0])
	require.Equal(t, before, writes(k))
}

func TestFirmwareWritesAreTwoSecondsApart(t *testing.T) {
	// R5.3: the second write waits for the end of the first's two seconds.
	waited := noWaiting(t)
	svc := service.New(nil, openrgb.NewFake(), "")
	svc.SetRecorder(recorder(t))
	k := attachKeyboard(t, svc)

	_, err := svc.Apply(t.Context(), service.Request{Colour: ptr(colour.MustParse("red"))})
	require.NoError(t, err)
	_, err = svc.Apply(t.Context(), service.Request{Colour: ptr(colour.MustParse("blue"))})
	require.NoError(t, err)
	require.Equal(t, []time.Duration{2 * time.Second}, *waited)
	effect, _ := k.fake.Effect(t.Context())
	require.Equal(t, lighting.Colour{B: 0xff}, *effect.Colour)
}

func TestADeviceWithoutStoredModesResolvesAsBefore(t *testing.T) {
	// R3.5.
	device := devices.Device{
		Name:  "x",
		Modes: []devices.Mode{{Name: "Direct", PerLED: true}, {Name: "Static", ModeColour: true}},
	}
	require.Equal(t, []string{"Direct", "Static"}, device.SolidCandidates(devices.Rule{}, devices.Want{}))
}
