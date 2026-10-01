package cli_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/api"
	"github.com/ushineko/hotaru/internal/canvas"
	"github.com/ushineko/hotaru/internal/canvas/canvastest"
	"github.com/ushineko/hotaru/internal/devices"
	"github.com/ushineko/hotaru/internal/openrgb"
	"github.com/ushineko/hotaru/internal/service"
)

// drawnSocket is a service with one canvas device attached and its twin on
// the OpenRGB server, over a socket.
func drawnSocket(t *testing.T) (string, *canvastest.Device) {
	t.Helper()
	fake := canvastest.New(3)
	fake.Name, fake.Path = "Test Canvas Board", "/dev/hidraw-test9"
	device, _ := canvas.New(fake)
	anim := canvas.NewAnimator(device, canvastest.NewClock(), nil)
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	go func() { _ = anim.Run(ctx) }()

	svc := service.New(nil, openrgb.NewFake(devices.Device{
		Name: "Test Canvas Board (OpenRGB)", Location: "HID: /dev/hidraw-test9", LEDCount: 3,
		Modes: []devices.Mode{{Name: "Direct", PerLED: true}},
	}), "")
	svc.AttachCanvas(canvas.Attached{Device: device, Animator: anim})

	socket := filepath.Join(t.TempDir(), "s")
	listener, err := api.Listen(ctx, socket)
	require.NoError(t, err)
	go func() { _ = api.Serve(ctx, listener, svc) }()
	return socket, fake
}

func TestTheListingSaysWhatACanvasIsDoingAndWhyItsTwinIsLeftAlone(t *testing.T) {
	socket, _ := drawnSocket(t)

	out, err := run(t, socket, "light", "list")
	require.NoError(t, err)
	require.Contains(t, out, "Test Canvas Board (OpenRGB)")
	require.Contains(t, out, "handed")
	require.Contains(t, out, "Test Canvas Board (OpenRGB) is drawn by hotaru as Test Canvas Board")
	require.Contains(t, out, "Static, Breathing, Spectrum, Rainbow Wave, Off", "the canvas does not offer its renderers")

	_, err = run(t, socket, "light", "set", "red")
	require.NoError(t, err)
	out, err = run(t, socket, "light", "list")
	require.NoError(t, err)
	require.Contains(t, out, "Static, holding", "a canvas holding its frame does not say so")

	out, _ = run(t, socket, "light", "health")
	require.Contains(t, out, "drawing on Test Canvas Board: Static, holding")
}

func TestReleaseSaysTheDeviceReenumerates(t *testing.T) {
	socket, fake := drawnSocket(t)

	out, err := run(t, socket, "light", "release", "canvas board")
	require.NoError(t, err)
	require.Contains(t, out, "re-enumerates")
	require.Equal(t, 1, fake.Released())

	_, err = run(t, socket, "light", "release", "nothing like it")
	require.Error(t, err)
}
