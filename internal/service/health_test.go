package service_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/hotaru/internal/config"
	"github.com/ushineko/hotaru/internal/service"
	"github.com/ushineko/hotaru/internal/service/servicetest"
)

// Nothing found says what a person could check, naming the file (spec 076).
func TestHealthCarriesWhatAPersonCouldDoAboutIt(t *testing.T) {
	got := service.New(nil).Health(t.Context())
	require.Equal(t, service.StateNoDevices, got.State)
	require.False(t, got.OK())
	require.NotEmpty(t, got.Remedies)
	require.Contains(t, got.Remedies[len(got.Remedies)-1], "60-hotaru.rules",
		"a remedy nobody can act on is an observation")
}

func TestAHealthyMachineIsOfferedNothing(t *testing.T) {
	// Advice when everything works is noise, and noise is what people learn
	// to skip past.
	got := servicetest.New(t, nil, auraBoard()).Health(t.Context())
	require.Equal(t, service.StateHealthy, got.State)
	require.True(t, got.OK())
	require.Equal(t, 1, got.Devices)
	require.Len(t, got.Canvases, 1)
	require.Empty(t, got.Remedies)
}

// Hardware present and all of it excluded is a configuration problem, and
// says so.
func TestHealthTellsAScopeThatExcludesEverything(t *testing.T) {
	got := servicetest.New(t, &config.Config{Scope: []string{"nothing here"}}, auraBoard()).Health(t.Context())
	require.Equal(t, service.StateNoneInScope, got.State)
	require.Contains(t, got.Detail, "scope")
}
