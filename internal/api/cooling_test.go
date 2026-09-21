package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/api"
	"github.com/ushineko/hotaru/internal/cooler"
	"github.com/ushineko/hotaru/internal/openrgb"
	"github.com/ushineko/hotaru/internal/service"
)

func ask(t *testing.T, svc *service.Service, path string) api.Cooling {
	t.Helper()
	rec := httptest.NewRecorder()
	api.Handler(svc).ServeHTTP(rec, httptest.NewRequestWithContext(
		t.Context(), http.MethodGet, path, nil))
	require.Equal(t, http.StatusOK, rec.Code)

	var out api.Cooling
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&out))
	return out
}

func TestTheCoolerIsServedFromTheService(t *testing.T) {
	svc := service.New(nil, openrgb.NewFake(), "")
	svc.SetCooler(cooler.Own(cooler.NewWithFake(cooler.NewFake())))

	got := ask(t, svc, "/"+api.Version+"/cooling")
	require.False(t, got.Absent)
	require.InDelta(t, 37.5, got.Coolant, 0.05)
	require.Equal(t, 2608, got.PumpRPM)
	require.False(t, got.Taken.IsZero(), "a reading with no time on it cannot be judged stale")
}

func TestAMachineWithNoCoolerAnswersRatherThanFailing(t *testing.T) {
	/*
		200 with Absent set, not 404 and not an error.

		"This machine has no cooler" is a fact a consumer wants. Making it a
		failure means every caller writes the same special case to tell it
		apart from a broken socket, and a dashboard that cannot distinguish
		them shows an alarm for an ordinary desktop.
	*/
	svc := service.New(nil, openrgb.NewFake(), "")

	got := ask(t, svc, "/"+api.Version+"/cooling")
	require.True(t, got.Absent)
	require.Contains(t, got.Detail, "no supported liquid cooler")
	require.Zero(t, got.Coolant)
}

func TestACoolerThatWillNotAnswerIsNamedRatherThanHidden(t *testing.T) {
	// Present and silent is a different problem from absent, and the device
	// name is the first thing somebody needs in order to chase it.
	silent := cooler.NewFake()
	silent.Silent = true
	svc := service.New(nil, openrgb.NewFake(), "")
	svc.SetCooler(cooler.Own(cooler.NewWithFake(silent)))

	got := ask(t, svc, "/"+api.Version+"/cooling")
	require.True(t, got.Absent)
	require.Equal(t, "fake cooler", got.Device)
	require.NotEmpty(t, got.Detail)
}

func TestCoolingIsReachableThroughTheClient(t *testing.T) {
	// Over a real socket, as a shell meets it: the wire type and the client
	// have to agree, and a struct tag typo is invisible to the handler tests.
	socket := filepath.Join(t.TempDir(), "s")
	listener, err := api.Listen(t.Context(), socket)
	require.NoError(t, err)

	svc := service.New(nil, openrgb.NewFake(), "")
	svc.SetCooler(cooler.Own(cooler.NewWithFake(cooler.NewFake())))

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- api.Serve(ctx, listener, svc) }()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	got, err := api.NewClient(socket).Cooling(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1190, got.FanRPM)
	require.Equal(t, "fake cooler", got.Device)
}
