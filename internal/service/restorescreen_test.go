package service_test

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	kraken "github.com/ushineko/hotaru/internal/cooler"
	"github.com/ushineko/hotaru/internal/cooler/coolertest"
	"github.com/ushineko/hotaru/internal/dashboard"
	"github.com/ushineko/hotaru/internal/openrgb"
	"github.com/ushineko/hotaru/internal/scenes"
	"github.com/ushineko/hotaru/internal/service"
	"github.com/ushineko/hotaru/internal/state"
)

/*
The screen comes back after a restart, as the lights do (#184).

What the panel was asked to show was held nowhere, so every start drew the
dashboard, and the window's Screen read empty until somebody changed it.
Each test runs a service, stops it as stopping does (the state file is
flushed), and starts another on the same files with a fresh cooler.
*/

// restarted is a service on a given state file, with a cooler that has a panel.
type restarted struct {
	svc   *service.Service
	panel *coolertest.Panel
	state *state.Store
}

func boot(t *testing.T, statePath string, saved *scenes.Store) restarted {
	t.Helper()
	desired, err := state.Open(statePath)
	require.NoError(t, err)
	svc := service.New(nil, openrgb.NewFake(board()), "")
	svc.SetRecorder(desired)
	if saved != nil {
		svc.SetScenes(saved)
	}
	panel := coolertest.NewPanel()
	svc.SetCooler(kraken.New(panel))
	return restarted{svc: svc, panel: panel, state: desired}
}

// stop is what stopping the service does to the state file.
func (m restarted) stop(t *testing.T) {
	t.Helper()
	require.NoError(t, m.state.Flush())
}

// still is a one-frame GIF.
func still(t *testing.T) []byte {
	t.Helper()
	frame := image.NewPaletted(image.Rect(0, 0, 8, 8), color.Palette{color.Black, color.White})
	var out bytes.Buffer
	require.NoError(t, gif.Encode(&out, frame, nil))
	return out.Bytes()
}

func TestAScenesPictureIsOnTheScreenAfterARestart(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.yml")
	picture := filepath.Join(dir, "sunset.gif")
	require.NoError(t, os.WriteFile(picture, still(t), 0o600))
	saved, err := scenes.Open(filepath.Join(dir, "scenes.yml"))
	require.NoError(t, err)
	require.NoError(t, saved.Save(scenes.Scene{Name: "dusk", Colour: "red", Screen: picture}))

	before := boot(t, statePath, saved)
	_, err = before.svc.ApplyScene(t.Context(), "dusk")
	require.NoError(t, err)
	require.Equal(t, 1, before.panel.Images())
	before.stop(t)

	after := boot(t, statePath, saved)
	require.Equal(t, "dusk", after.svc.Applied(), "the window's Scene was empty after a restart")
	require.Equal(t, "picture: sunset", after.svc.Showing(), "the window's Screen was empty after a restart")

	require.NoError(t, after.svc.RestoreScreen(t.Context()))
	require.Equal(t, 1, after.panel.Images(), "the picture was not put back on the panel")
}

// A picture sent as bytes has no file to name, so the bytes are kept.
func TestAPictureSentAsBytesIsPutBack(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.yml")

	before := boot(t, statePath, nil)
	require.NoError(t, before.svc.Draw(t.Context(), service.Screen{Image: still(t)}))
	before.stop(t)

	after := boot(t, statePath, nil)
	require.Equal(t, "a picture", after.svc.Showing())
	require.NoError(t, after.svc.RestoreScreen(t.Context()))
	require.Equal(t, 1, after.panel.Images())
}

func TestTheReadoutIsPutBack(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.yml")

	before := boot(t, statePath, nil)
	require.NoError(t, before.svc.Draw(t.Context(), service.Screen{Readout: true}))
	before.stop(t)

	after := boot(t, statePath, nil)
	require.Equal(t, "the cooler's own readout", after.svc.Showing())
	require.NoError(t, after.svc.RestoreScreen(t.Context()))
	require.Equal(t, 1, after.panel.Readouts())
	require.Zero(t, after.panel.Images())
}

// The dashboard asked for after a picture is the dashboard after a restart,
// not the picture before it.
func TestTheDashboardAfterAPictureStaysTheDashboard(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.yml")

	boards, err := dashboard.Open(filepath.Join(t.TempDir(), "dashboards.yml"))
	require.NoError(t, err)
	before := boot(t, statePath, nil)
	before.svc.SetDashboards(boards)
	shipped := boards.Active().Name
	require.NoError(t, before.svc.Draw(t.Context(), service.Screen{Image: still(t)}))
	require.NoError(t, before.svc.UseDashboard(shipped))
	before.stop(t)

	after := boot(t, statePath, nil)
	require.Equal(t, "dashboard: "+shipped, after.svc.Showing())
	require.NoError(t, after.svc.RestoreScreen(t.Context()))
	require.Zero(t, after.panel.Images(), "a picture replaced since was put back")
	require.Zero(t, after.panel.Readouts())
}

// The inert rule: a restarted that was never asked about its screen gets the
// dashboard and nothing else.
func TestAFreshInstallPutsNothingOnTheScreen(t *testing.T) {
	m := boot(t, filepath.Join(t.TempDir(), "state.yml"), nil)
	require.Empty(t, m.svc.Showing())
	require.NoError(t, m.svc.RestoreScreen(t.Context()))
	require.Zero(t, m.panel.Images())
	require.Zero(t, m.panel.Readouts())
}

// Showing a stored picture says so, as a scene's picture does. It changed the
// panel and left the window's Screen naming what was there before.
func TestShowingAStoredPictureSaysWhatIsShowing(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.yml")
	before := boot(t, statePath, nil)
	withPictures(t, before.svc)
	_, err := before.svc.AddImage("halves", encoded(t))
	require.NoError(t, err)

	require.NoError(t, before.svc.ShowImage(t.Context(), "halves"))
	require.Equal(t, "picture: halves", before.svc.Showing())
	before.stop(t)

	after := boot(t, statePath, nil)
	require.NoError(t, after.svc.RestoreScreen(t.Context()))
	require.Equal(t, 1, after.panel.Images())
}

// held is a dashboard that only says whether it holds the screen.
type held struct{ holding bool }

func (h *held) Hold()    { h.holding = true }
func (h *held) Release() { h.holding = false }
func (h *held) Redraw()  {}

// A scene that asks for the dashboard after a picture is the dashboard after
// a restart: the record follows every route, not only choosing a dashboard.
func TestASceneThatAsksForTheDashboardReplacesThePicture(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.yml")
	before := boot(t, statePath, nil)
	before.svc.SetDashboard(&held{})
	require.NoError(t, before.svc.Draw(t.Context(), service.Screen{Image: still(t)}))
	require.NoError(t, before.svc.Draw(t.Context(), service.Screen{Dashboard: true}))
	before.stop(t)

	after := boot(t, statePath, nil)
	require.NoError(t, after.svc.RestoreScreen(t.Context()))
	require.Zero(t, after.panel.Images(), "the picture the dashboard replaced was put back")
}
