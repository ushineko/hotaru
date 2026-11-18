package service_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/canvas/canvastest"
	"github.com/ushineko/hotaru/internal/service"
	"github.com/ushineko/hotaru/internal/service/servicetest"
)

const hero = "ASUS ROG MAXIMUS Z790 HERO"

// heroBoard is headers() as a canvas: the board's light and two headers of
// 16, each 0 to 500.
func heroBoard() *canvastest.Board {
	b := canvastest.NewBoard(hero, canvastest.Zone("Aura Mainboard", 1),
		canvastest.Header("Addressable RGB Header 1", 0, 500), canvastest.Header("Addressable RGB Header 2", 0, 500))
	_ = b.Resize("Addressable RGB Header 1", 16)
	_ = b.Resize("Addressable RGB Header 2", 16)
	return b
}

func TestASceneRecordsTheSizeOfEveryZone(t *testing.T) {
	// AC3.
	svc := servicetest.New(t, nil, heroBoard())
	svc.SetRecorder(recorder(t))

	_, err := svc.Apply(t.Context(), service.Request{Assignments: solid(hero, "teal")})
	require.NoError(t, err)

	require.Equal(t, map[string]int{
		"Aura Mainboard": 1, "Addressable RGB Header 1": 16, "Addressable RGB Header 2": 16,
	}, svc.Desired().Devices[hero].Zones)
}

func TestASceneDoesNotResizeAZone(t *testing.T) {
	// AC8: a header given 8 lights stays that size, and a target past its
	// end is a problem rather than a resize.
	board := heroBoard()
	svc := service.New(nil)
	svc.SetRecorder(recorder(t))
	drawn := servicetest.Attach(t, svc, board)
	_, err := svc.Apply(t.Context(), service.Request{Assignments: solid(hero, "teal")})
	require.NoError(t, err)

	require.NoError(t, drawn.Device.Resize("Addressable RGB Header 1", 8))

	results, err := svc.Apply(t.Context(), service.Request{Assignments: append(
		solid(hero, "teal"), solid(hero+"/Addressable RGB Header 1[15]", "red")...)})
	require.NoError(t, err)
	require.True(t, results[0].Applied)
	require.NotEmpty(t, results[0].Problems)
	require.Equal(t, 8, board.Zones()[1].Count)
	require.Equal(t, 8, svc.Desired().Devices[hero].Zones["Addressable RGB Header 1"])
}
