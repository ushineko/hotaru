package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/api"
	"github.com/ushineko/hotaru/internal/dashboard"
	"github.com/ushineko/hotaru/internal/service"
)

/*
A dashboard's motion and trail interval travel through the API and the store
(spec 081), and one this version does not know is the caller's mistake: 400,
naming the ones it does know, and nothing stored.
*/
func TestMotionAndTheTrailIntervalRoundTrip(t *testing.T) {
	svc := service.New(nil)
	store, err := dashboard.Open(filepath.Join(t.TempDir(), "dashboards.yml"))
	require.NoError(t, err)
	svc.SetDashboards(store)
	handler := api.Handler(svc)

	put := func(body api.Dashboard) *httptest.ResponseRecorder {
		data, err := json.Marshal(body)
		require.NoError(t, err)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodPut,
			"/"+api.Version+"/dashboards/"+body.Name, bytes.NewReader(data)))
		return rec
	}

	good := api.Dashboard{
		Name: "moving", Headline: api.DashboardSlot{Source: "coolant"},
		Motion: api.DashboardMotion{Numbers: "melt", Ring: "comet"},
		Trail:  api.DashboardTrail{Every: 1},
	}
	require.Equal(t, http.StatusOK, put(good).Code)

	saved, err := store.Get("moving")
	require.NoError(t, err)
	require.Equal(t, dashboard.Motion{Numbers: dashboard.Melt, Ring: dashboard.Comet}, saved.Motion)
	require.Equal(t, 1, saved.Trail.Every)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet,
		"/"+api.Version+"/dashboards", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"numbers": "melt"`)

	bad := good
	bad.Name, bad.Motion.Numbers = "wobbly", "wobble"
	got := put(bad)
	require.Equal(t, http.StatusBadRequest, got.Code)
	require.Contains(t, got.Body.String(), "melt, morph, roll")
	_, err = store.Get("wobbly")
	require.Error(t, err, "a refused dashboard was stored")
}
