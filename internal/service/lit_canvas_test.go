package service_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/hotaru/internal/canvas/canvastest"
	"github.com/ushineko/hotaru/internal/scenes"
	"github.com/ushineko/hotaru/internal/service"
	"github.com/ushineko/hotaru/internal/service/servicetest"
	"github.com/ushineko/hotaru/internal/state"
)

// litDesk is lit on canvases (spec 076): headerBoard and k4, with
// somewhere to keep scenes and somewhere to remember what was asked for.
func litDesk(t *testing.T, saved ...scenes.Scene) (*service.Service, *canvastest.Board, *canvastest.Firmware) {
	t.Helper()
	board, kb := headerBoard(), k4()
	svc := servicetest.New(t, nil, board, kb)

	desired, err := state.Open(filepath.Join(t.TempDir(), "state.yml"))
	require.NoError(t, err)
	svc.SetRecorder(desired)

	store, err := scenes.Open(filepath.Join(t.TempDir(), "scenes.yml"))
	require.NoError(t, err)
	for _, scene := range saved {
		require.NoError(t, store.Save(scene))
	}
	svc.SetScenes(store)
	return svc, board, kb
}

// headerBoard is the board as the scene tests had it: one zone of four
// lights, so a picture's two halves are two runs.
func headerBoard() *canvastest.Board {
	return canvastest.NewBoard("ASUS ROG MAXIMUS Z790 HERO", canvastest.Zone("Addressable 1", 4))
}

// lightsOn is whether a fake shows anything on its first light.
func lightsOn(d *canvastest.Device) bool {
	p, ok := d.Showing(1)
	return ok && (p.R != 0 || p.G != 0 || p.B != 0)
}
