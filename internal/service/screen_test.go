package service_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	hw "github.com/ushineko/hotaru/internal/cooler" // `cooler` is a fixture in this package
	"github.com/ushineko/hotaru/internal/openrgb"
	"github.com/ushineko/hotaru/internal/service"
)

// panel is a cooler that only has a screen, which is the part of it this file
// is about.
type panel struct{ shown, readouts int }

func (p *panel) Status(context.Context) (hw.Status, error)  { return hw.Status{}, nil }
func (p *panel) Device() hw.Device                          { return hw.Device{} }
func (p *panel) Show(context.Context, []byte) error         { p.shown++; return nil }
func (p *panel) Readout(context.Context) error              { p.readouts++; return nil }
func (p *panel) Appearance(context.Context, int, int) error { return nil }

// dashboard records whether it has been asked to stand down.
type dashboard struct{ held bool }

func (d *dashboard) Hold()    { d.held = true }
func (d *dashboard) Release() { d.held = false }

func drawn(t *testing.T) (*service.Service, *panel, *dashboard) {
	t.Helper()
	svc := service.New(nil, openrgb.NewFake(), "")
	screen, board := &panel{}, &dashboard{}
	svc.SetCooler(screen)
	svc.SetDashboard(board)
	return svc, screen, board
}

func TestShowingAPictureTakesTheScreenFromTheDashboard(t *testing.T) {
	/*
		The screen holds one picture, so it has one author. A dashboard that
		kept drawing would replace somebody's picture within two seconds,
		which is the same as never having shown it.
	*/
	svc, screen, board := drawn(t)

	require.NoError(t, svc.Draw(t.Context(), service.Screen{Image: []byte("GIF89a")}))
	require.Equal(t, 1, screen.shown)
	require.True(t, board.held, "the dashboard kept drawing over the picture it was asked to make way for")
}

func TestAskingForTheCoolersOwnDisplayAlsoTakesTheScreen(t *testing.T) {
	// Handing the panel to the firmware and then drawing over it half a
	// second later is the same fault in a different direction.
	svc, screen, board := drawn(t)

	require.NoError(t, svc.Draw(t.Context(), service.Screen{Readout: true}))
	require.Equal(t, 1, screen.readouts)
	require.True(t, board.held)
}

func TestTheDashboardIsAskedForBackExplicitly(t *testing.T) {
	svc, _, board := drawn(t)

	require.NoError(t, svc.Draw(t.Context(), service.Screen{Readout: true}))
	require.NoError(t, svc.Draw(t.Context(), service.Screen{Dashboard: true}))
	require.False(t, board.held)
}

func TestBrightnessLeavesTheAuthorshipAlone(t *testing.T) {
	// Brightness and orientation apply to whatever is showing. Turning the
	// panel down is not a claim on what it displays.
	svc, _, board := drawn(t)

	level := 60
	require.NoError(t, svc.Draw(t.Context(), service.Screen{Brightness: &level}))
	require.False(t, board.held)
}

func TestAMachineWithNoDashboardSaysSo(t *testing.T) {
	svc := service.New(nil, openrgb.NewFake(), "")
	svc.SetCooler(&panel{})

	require.ErrorContains(t, svc.Draw(t.Context(), service.Screen{Dashboard: true}), "no dashboard")
}
