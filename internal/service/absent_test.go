package service_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/devices"
	"github.com/ushineko/hotaru/internal/service"
	"github.com/ushineko/hotaru/internal/state"
)

// noServer is the address of an OpenRGB server that is not running.
const noServer = "127.0.0.1:6742"

func unreachable(t *testing.T, err error) {
	t.Helper()
	var u *service.Unreachable
	require.True(t, errors.As(err, &u), "not the server being unreachable: %v", err)
}

// With no server and nothing drawn by hotaru, every call says the server is
// unreachable, as before spec 074.
func TestNoServerAndNoCanvasIsUnreachable(t *testing.T) {
	svc := service.New(nil, nil, noServer)
	_, err := svc.List(t.Context())
	unreachable(t, err)
}

// A canvas works with no OpenRGB server: listed, lit by a colour and a scene,
// and restored from what is recorded (spec 074 R1).
func TestACanvasWorksWithNoServer(t *testing.T) {
	svc := service.New(nil, nil, noServer)
	store := recorder(t)
	svc.SetRecorder(store)
	d := attachCooler(t, svc)

	views, err := svc.List(t.Context())
	require.NoError(t, err)
	require.Len(t, views, 1)
	require.Equal(t, coolerName, views[0].Device.Name)

	_, err = svc.Apply(t.Context(), service.Request{Colour: ptr(colour.MustParse("red"))})
	require.NoError(t, err)
	require.Eventually(t, func() bool { return red(t, d, 1) && red(t, d, 6) }, eventually, every)

	_, err = svc.Apply(t.Context(), service.Request{Assignments: solid(coolerName+"/Fan[0]", "blue")})
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		p, ok := d.fake.Showing(4)
		return ok && p.B == 255 && p.R == 0
	}, eventually, every, "a target on the canvas did not land")
}

func TestACanvasIsRestoredWithNoServer(t *testing.T) {
	svc := service.New(nil, nil, noServer)
	store := recorder(t)
	svc.SetRecorder(store)
	reds := make([]colour.Colour, 6)
	for i := range reds {
		reds[i] = colour.MustParse("red")
	}
	require.NoError(t, store.Record(coolerName, state.Device{Mode: "Static", Colours: reds}))
	require.NoError(t, store.Record("ASUS ROG MAXIMUS Z790 HERO", state.Device{Mode: "Direct", Colours: reds}))
	d := attachCooler(t, svc)

	restore, err := svc.Reconcile(t.Context(), nil)
	require.NoError(t, err)
	require.Contains(t, restore.Missing, "ASUS ROG MAXIMUS Z790 HERO", "an OpenRGB device with no server is missing")
	for id := byte(1); id <= 6; id++ {
		require.Eventually(t, func() bool { return red(t, d, id) }, eventually, every, "light %d", id)
	}
}

// What is OpenRGB's stays OpenRGB's: a write to its device and a probe still
// say there is no server (R2.1). Health does not, since OpenRGB is optional
// and hotaru draws the canvas (spec 075 R3.3).
func TestWithNoServerOpenRGBsPartsStillSaySo(t *testing.T) {
	svc := service.New(nil, nil, noServer)
	svc.SetRecorder(recorder(t))
	attachCooler(t, svc)

	results, err := svc.Apply(t.Context(), service.Request{
		Assignments: []devices.Assignment{{Target: devices.Target{Device: "ASUS ROG MAXIMUS Z790 HERO"}, Colour: colour.MustParse("red")}},
	})
	require.NoError(t, err)
	for _, r := range results {
		require.NotEqual(t, "ASUS ROG MAXIMUS Z790 HERO", r.Device, "a device no server lists was written")
	}

	health := svc.Health(t.Context())
	require.Equal(t, service.StateHealthy, health.State)
	require.Contains(t, health.Detail, "no OpenRGB server")
	require.NotEmpty(t, health.Canvases, "health no longer lists the canvas")

	bare := service.New(nil, nil, noServer)
	require.Equal(t, service.StateUnreachable, bare.Health(t.Context()).State, "nothing drawn and no server is healthy")

	_, err = svc.Probe(t.Context(), nil)
	unreachable(t, err)
}
