package service_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/dashboard"
	"github.com/ushineko/hotaru/internal/openrgb"
	"github.com/ushineko/hotaru/internal/readings"
	"github.com/ushineko/hotaru/internal/scenes"
	"github.com/ushineko/hotaru/internal/service"
)

// withScreens gives a service somewhere to keep dashboards.
func withScreens(t *testing.T, svc interface {
	SetDashboards(*dashboard.Store)
}) *dashboard.Store {
	t.Helper()
	store, err := dashboard.Open(filepath.Join(t.TempDir(), "dashboards.yml"))
	require.NoError(t, err)
	svc.SetDashboards(store)
	return store
}

func TestASceneCanNameADashboard(t *testing.T) {
	/*
		The thing a scene could not do before: change the lights and the
		screen together. "dashboard" on its own still means whichever one the
		panel is set to, because a scene written before there was more than
		one must still mean what it meant.
	*/
	svc, _ := lit(t, scenes.Scene{
		Name: "evening", Screen: "dashboard:quiet",
		Assignments: []scenes.Assignment{{Target: "ASUS", Colour: "blue"}},
	})
	svc.SetCooler(&panel{})
	svc.SetDashboard(&screenAuthor{})
	store := withScreens(t, svc)

	done, err := svc.ApplyScene(t.Context(), "evening")
	require.NoError(t, err)
	require.Equal(t, "dashboard:quiet", done.Screen)
	require.Equal(t, "quiet", store.Active().Name,
		"the scene did not change what the panel draws")
}

func TestASceneNamingADashboardThatIsGoneCostsTheScreenNotTheColours(t *testing.T) {
	// The rule spec 015 set for a missing image, which holds here: the
	// colours are still what somebody asked for.
	svc, server := lit(t, scenes.Scene{
		Name: "evening", Screen: "dashboard:absent",
		Assignments: []scenes.Assignment{{Target: "ASUS", Colour: "blue"}},
	})
	svc.SetCooler(&panel{})
	svc.SetDashboard(&screenAuthor{})
	withScreens(t, svc)

	done, err := svc.ApplyScene(t.Context(), "evening")
	require.NoError(t, err)
	require.Empty(t, done.Screen)
	require.Contains(t, done.Problems[0], "absent")
	require.NotEmpty(t, showing(t, server, "ASUS ROG MAXIMUS Z790 HERO"))
}

func TestARenderedDashboardIsAFrameThePanelWouldTake(t *testing.T) {
	svc, _ := lit(t)
	withScreens(t, svc)

	frame, err := svc.RenderDashboard(t.Context(), dashboard.Dashboard{
		Name: "mine", Arrangement: dashboard.Big,
		Headline: dashboard.Slot{Source: readings.Coolant},
	})
	require.NoError(t, err)
	require.NotEmpty(t, frame)
	require.Equal(t, []byte("GIF"), frame[:3])
}

func TestChoosingADashboardTakesTheScreenBack(t *testing.T) {
	/*
		Reported as "Show it does nothing", and it was not the window: every
		scene that names a picture holds the dashboard, and only asking for
		the dashboard gives it up. Choosing one changed the store, the panel
		kept showing the picture, and nothing said why.
	*/
	svc, _ := lit(t)
	board := &screenAuthor{}
	svc.SetCooler(&panel{})
	svc.SetDashboard(board)
	withScreens(t, svc)

	// A picture on the screen, which is what a scene with an image does.
	require.NoError(t, svc.Draw(t.Context(), service.Screen{Image: []byte("GIF89a")}))
	require.True(t, board.held, "a picture did not take the panel from the dashboard")

	require.NoError(t, svc.UseDashboard("quiet"))
	require.False(t, board.held, "choosing a dashboard did not take the panel back")
}

func TestSavingADashboardDoesNotStealTheScreen(t *testing.T) {
	// The other half: editing a dashboard while a picture is up is not a
	// request for the picture to go away.
	svc, _ := lit(t)
	board := &screenAuthor{}
	svc.SetCooler(&panel{})
	svc.SetDashboard(board)
	withScreens(t, svc)

	require.NoError(t, svc.Draw(t.Context(), service.Screen{Image: []byte("GIF89a")}))
	require.NoError(t, svc.SaveDashboard(dashboard.Dashboard{
		Name: "mine", Arrangement: dashboard.Big,
		Headline: dashboard.Slot{Source: readings.Coolant},
	}))

	require.True(t, board.held, "saving a dashboard took the screen from a picture")
	require.Positive(t, board.redrawn, "saving a dashboard did not redraw it")
}

func TestADashboardsLetteringIsKept(t *testing.T) {
	/*
		Written to the file and read back from it, because the editor sends
		what it drew the form from: a field the store drops is a setting that
		silently reverts every time somebody saves.
	*/
	path := filepath.Join(t.TempDir(), "dashboards.yml")
	store, err := dashboard.Open(path)
	require.NoError(t, err)

	none := 0
	want := dashboard.Lettering{
		Font:   "mono",
		Labels: dashboard.Text{Size: 120, Colour: "#ff00ff", Outline: &none},
		Values: dashboard.Text{Size: 90, Colour: "#00ff88"},
	}
	require.NoError(t, store.Save(dashboard.Dashboard{
		Name: "lettered", Arrangement: dashboard.Ring, Lettering: want,
	}))

	again, err := dashboard.Open(path)
	require.NoError(t, err)
	got, err := again.Get("lettered")
	require.NoError(t, err, "the dashboard is not in the file")
	require.Equal(t, want, got.Lettering)
	require.NotNil(t, got.Lettering.Labels.Outline, "an outline of none came back as not set")
	require.Zero(t, *got.Lettering.Labels.Outline)
}

func TestThePreviewIsDrawnWithTheTrailThePanelHas(t *testing.T) {
	/*
		The editor draws what the panel draws. A preview rendered without the
		trail would show an empty band under the headline and somebody would
		save a dashboard believing that is what their screen looks like.

		The service records every reading it takes, so asking for readings is
		what fills the history: two buckets apart is a trail of one point,
		and the window is what the panel is handed.
	*/
	svc := service.New(nil, openrgb.NewFake(), "")

	require.Empty(t, svc.Trail(t.Context(), readings.CPULoad),
		"a service that has taken one reading has a trail")

	// A second bucket closes the first.
	require.Eventually(t, func() bool {
		return len(svc.Trail(t.Context(), readings.CPULoad)) > 0
	}, 3*readings.Bucket, readings.Bucket/5, "no point was ever closed")
}
