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
	"github.com/ushineko/hotaru/internal/service"
	"github.com/ushineko/hotaru/internal/state"
)

const boardName = "fake board"

// attachBoard attaches a zoned board whose header has 4 lights.
func attachBoard(t *testing.T, svc *service.Service) *canvastest.Board {
	t.Helper()
	board := canvastest.NewBoard(boardName, canvastest.Zone("Board", 1), canvastest.Header("Header 1", 0, 500))
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
	svc := service.New(nil)
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
	svc := service.New(nil)
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
	assert.Equal(t, 4, board.Zones()[1].Count)
	require.Eventually(t, func() bool {
		p, ok := board.Showing(5)
		return ok && p.R == 255
	}, eventually, every)
}

/*
A record laid out for OpenRGB's sizes is fitted zone by zone: the header's two
recorded colours are stretched over its four lights, and the board's light
keeps its own (spec 075).
*/
func TestARecordIsFittedToTheZonesAsTheyAre(t *testing.T) {
	svc := service.New(nil)
	store := recorder(t)
	svc.SetRecorder(store)
	require.NoError(t, store.Record(boardName, state.Device{
		Mode:    "Direct",
		Colours: []colour.Colour{colour.MustParse("white"), colour.MustParse("red"), colour.MustParse("blue")},
		Zones:   map[string]int{"Header 1": 2},
	}))
	board := attachBoard(t, svc)

	restore, err := svc.Reconcile(t.Context(), nil)
	require.NoError(t, err)
	require.Len(t, restore.Results, 1)
	assert.Empty(t, restore.Results[0].Problems)
	want := [][3]uint8{{255, 255, 255}, {255, 0, 0}, {255, 0, 0}, {0, 0, 255}, {0, 0, 255}}
	require.Eventually(t, func() bool {
		for i, w := range want {
			p, ok := board.Showing(byte(i + 1)) // the fake's keys are 1 on
			if !ok || [3]uint8{p.R, p.G, p.B} != w {
				return false
			}
		}
		return true
	}, eventually, every)
}
