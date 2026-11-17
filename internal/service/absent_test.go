package service_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/service"
	"github.com/ushineko/hotaru/internal/state"
)

// A canvas works on its own: listed, lit by a colour and a scene,
// and restored from what is recorded (spec 074 R1).
func TestACanvasWorksWithNoServer(t *testing.T) {
	svc := service.New(nil)
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
	svc := service.New(nil)
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
