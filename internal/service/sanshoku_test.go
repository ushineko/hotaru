package service_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hotaru/internal/canvas"
	"github.com/ushineko/hotaru/internal/canvas/canvastest"
	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/devices"
	"github.com/ushineko/hotaru/internal/openrgb"
	"github.com/ushineko/hotaru/internal/service"
	"github.com/ushineko/hotaru/internal/state"
)

const boardName = "fake board"

// attachBoard attaches a zoned board whose header has 4 lights.
func attachBoard(t *testing.T, svc *service.Service) *canvastest.Zoned {
	t.Helper()
	board := canvastest.NewZoned(1, 500)
	device, ok := canvas.New(board)
	require.True(t, ok)
	require.NoError(t, device.Resize("Header 1", 4))
	anim := canvas.NewAnimator(device, canvastest.NewClock(), nil)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { defer close(done); _ = anim.Run(ctx) }()
	t.Cleanup(func() { cancel(); <-done })
	svc.AttachCanvas(canvas.Attached{Device: device, Animator: anim})
	return board
}

// A zoned canvas is listed with the device's own zones (spec 075 R2.1).
func TestAZonedCanvasIsListedWithItsZones(t *testing.T) {
	svc := service.New(nil, nil, noServer)
	attachBoard(t, svc)
	got := viewOf(t, svc, boardName).Device
	assert.Equal(t, []devices.Zone{
		{Name: "Board", Shape: devices.ShapeSingle, First: 0, Count: 1},
		{Name: "Header 1", Shape: devices.ShapeLine, First: 1, Count: 4, Max: 500},
	}, got.Zones)
}

/*
A record made through OpenRGB, in its "Direct", restores a canvas of the same
name in Static, and the zone sizes OpenRGB recorded do not resize it (spec
075 R2.3, R3.1).
*/
func TestAnOpenRGBRecordRestoresTheCanvas(t *testing.T) {
	svc := service.New(nil, nil, noServer)
	store := recorder(t)
	svc.SetRecorder(store)
	reds := make([]colour.Colour, 5)
	for i := range reds {
		reds[i] = colour.MustParse("red")
	}
	require.NoError(t, store.Record(boardName, state.Device{
		Mode: "Direct", Colours: reds, Zones: map[string]int{"Header 1": 16},
	}))
	board := attachBoard(t, svc)

	restore, err := svc.Reconcile(t.Context(), nil)
	require.NoError(t, err)
	require.Len(t, restore.Results, 1)
	assert.Equal(t, "Static", restore.Results[0].Mode)
	assert.Empty(t, restore.Results[0].Resized, "a canvas was regrown to OpenRGB's size")
	assert.Equal(t, 4, board.Header)
	require.Eventually(t, func() bool {
		p, ok := board.Showing(5)
		return ok && p.R == 255
	}, eventually, every)
}

// While OpenRGB still runs, its device of the canvas's name is the canvas's
// twin: the graphics card's OpenRGB location is not a node (spec 075 R3.2).
func TestAnOpenRGBDeviceOfTheCanvassNameIsItsTwin(t *testing.T) {
	gpu := devices.Device{
		Name: boardName, Location: "I2C: NVIDIA i2c adapter 1, address 0x68", LEDCount: 1,
		Zones: []devices.Zone{{Name: "GPU", Shape: devices.ShapeSingle, Count: 1}},
		Modes: []devices.Mode{{Name: "Direct", PerLED: true}},
	}
	svc := service.New(nil, openrgb.NewFake(gpu), "")
	attachBoard(t, svc)
	views, err := svc.List(t.Context())
	require.NoError(t, err)
	handed := 0
	for _, v := range views {
		if v.Device.Name == boardName && v.Device.HandedTo == boardName {
			handed++
		}
	}
	assert.Equal(t, 1, handed)
}
