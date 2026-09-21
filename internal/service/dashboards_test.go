package service_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/dashboard"
	"github.com/ushineko/hotaru/internal/readings"
	"github.com/ushineko/hotaru/internal/scenes"
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
