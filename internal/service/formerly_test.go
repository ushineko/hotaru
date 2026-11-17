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
	"github.com/ushineko/hotaru/internal/openrgb"
	"github.com/ushineko/hotaru/internal/service"
	"github.com/ushineko/hotaru/internal/state"
)

/*
The cooler's lights as a canvas (spec 073): three ring lights and three fan
lights, named as sanshoku names the Kraken's, and the name and zones OpenRGB
listed them under before its detector was switched off.
*/
const (
	coolerName = "Test Cooler"
	coolerPath = "/dev/hidraw-test8"
	formerName = "Test Cooler Series RGB"
)

func formerRule() *config.Config {
	return &config.Config{Devices: []config.DeviceRule{{
		Match: "test cooler",
		Formerly: &config.Former{
			Name:  formerName,
			Zones: map[string]string{"Channel 1": "Ring", "Channel 2": "Fan"},
		},
		Segments: map[string]config.Segment{
			"fans-back": {Zone: "Channel 2", LEDs: &config.LEDs{First: 1, Last: 2}},
		},
	}}}
}

// attachCooler attaches the cooler's lights as a borrowed canvas.
func attachCooler(t *testing.T, svc *service.Service) *drawing {
	t.Helper()
	fake := canvastest.New(6)
	fake.Name, fake.Path = coolerName, coolerPath
	fake.Names = []string{"ring 1", "ring 2", "ring 3", "fan 1", "fan 2", "fan 3"}
	device, ok := canvas.Borrow(fake)
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

// red is whether a light (by key id, from 1) shows red.
func red(t *testing.T, d *drawing, id byte) bool {
	t.Helper()
	p, ok := d.fake.Showing(id)
	return ok && p.R == 255 && p.G == 0 && p.B == 0
}

func TestTheCoolersZonesComeFromItsKeyNames(t *testing.T) {
	svc := service.New(nil, openrgb.NewFake(board()), "")
	attachCooler(t, svc)
	got := viewOf(t, svc, coolerName).Device
	require.Equal(t, []devices.Zone{
		{Name: "Ring", Shape: devices.ShapeLine, First: 0, Count: 3},
		{Name: "Fan", Shape: devices.ShapeLine, First: 3, Count: 3},
	}, got.Zones)
}

func TestTheFormerNameIsListedAsTheCanvassTwin(t *testing.T) {
	svc := service.New(formerRule(), openrgb.NewFake(board()), "")
	attachCooler(t, svc)
	former := viewOf(t, svc, formerName).Device
	require.Equal(t, coolerName, former.HandedTo)
	require.Equal(t, "Channel 1", former.Zones[0].Name)
	require.Equal(t, "Channel 2", former.Zones[1].Name)
}

/*
A scene written when OpenRGB listed the cooler lands on the same lights of the
canvas: one former zone's light, a range of them, a segment, and the whole
device.
*/
func TestAScenesFormerTargetsLightTheCanvas(t *testing.T) {
	cases := map[string]struct {
		target string
		lit    []byte
	}{
		"one light of a former zone":  {formerName + "/Channel 1[1]", []byte{2}},
		"a range in the second zone":  {formerName + "/Channel 2[0:1]", []byte{4, 5}},
		"a segment, by the former":    {formerName + "/fans-back", []byte{5, 6}},
		"a segment, by the canvas":    {coolerName + "/fans-back", []byte{5, 6}},
		"the whole device, by former": {formerName, []byte{1, 2, 3, 4, 5, 6}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			svc := service.New(formerRule(), openrgb.NewFake(board()), "")
			svc.SetRecorder(recorder(t))
			d := attachCooler(t, svc)

			_, err := svc.Apply(t.Context(), service.Request{Assignments: solid(c.target, "red")})
			require.NoError(t, err)
			d.frames(t, 1)
			want := map[byte]bool{}
			for _, id := range c.lit {
				want[id] = true
			}
			for id := byte(1); id <= 6; id++ {
				require.Equal(t, want[id], red(t, d, id), "light %d", id)
			}
		})
	}
}

// The cooler's colours recorded under its former name restore the canvas,
// when there is one for each light; the former record is left as it is.
func TestTheFormerRecordRestoresTheCanvas(t *testing.T) {
	svc := service.New(formerRule(), openrgb.NewFake(board()), "")
	store := recorder(t)
	svc.SetRecorder(store)
	reds := make([]colour.Colour, 6)
	for i := range reds {
		reds[i] = colour.MustParse("red")
	}
	require.NoError(t, store.Record(formerName, state.Device{Mode: "Direct", Colours: reds}))
	d := attachCooler(t, svc)

	_, err := svc.Reconcile(t.Context(), nil)
	require.NoError(t, err)
	for id := byte(1); id <= 6; id++ {
		require.Eventually(t, func() bool { return red(t, d, id) }, eventually, every, "light %d was not restored", id)
	}
	_, kept := store.Snapshot().Devices[formerName]
	require.True(t, kept, "the former record was removed")
}

func TestAFormerRecordOfAnotherSizeIsNotAdopted(t *testing.T) {
	svc := service.New(formerRule(), openrgb.NewFake(board()), "")
	store := recorder(t)
	svc.SetRecorder(store)
	require.NoError(t, store.Record(formerName, state.Device{Mode: "Direct", Colours: []colour.Colour{colour.MustParse("red")}}))
	d := attachCooler(t, svc)

	_, err := svc.Reconcile(t.Context(), nil)
	require.NoError(t, err)
	time.Sleep(50 * time.Millisecond)
	require.Zero(t, d.fake.Frames(), "a record that does not fit was drawn")
}

/*
Releasing the cooler's lights keeps them attached, and an animated scene stops
before the release, so the frame the cooler keeps is the last one (R2.1).
*/
func TestReleasingTheCoolerKeepsItAttached(t *testing.T) {
	svc := service.New(formerRule(), openrgb.NewFake(board()), "")
	store := recorder(t)
	svc.SetRecorder(store)
	require.NoError(t, store.Record(formerName, state.Device{Mode: "Direct", Colours: make([]colour.Colour, 6)}))
	d := attachCooler(t, svc)
	_, err := svc.Apply(t.Context(), service.Request{Colour: ptr(colour.MustParse("blue"))})
	require.NoError(t, err)
	d.frames(t, 1)

	name, err := svc.ReleaseCanvas(t.Context(), coolerName)
	require.NoError(t, err)
	require.Equal(t, coolerName, name)
	require.Equal(t, 1, d.fake.Released())
	require.False(t, d.fake.Closed())
	viewOf(t, svc, coolerName) // still attached
	require.True(t, svc.Stays(coolerName))
	_, kept := store.Snapshot().Devices[formerName]
	require.False(t, kept, "the former record would be adopted again and undo the release")
}
