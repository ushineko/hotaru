package service_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/ushineko/sanshoku"

	"github.com/ushineko/hotaru/internal/canvas/canvastest"
	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/config"
	"github.com/ushineko/hotaru/internal/scenes"
	"github.com/ushineko/hotaru/internal/service"
	"github.com/ushineko/hotaru/internal/service/servicetest"
)

/*
The window and the CLI change a device's follow and pace through the service,
which writes the rules file (spec 077 R3.2, R3.9).
*/

// withRules is a service over a rules file holding rules, and the file.
func withRules(t *testing.T, rules string, devs ...sanshoku.Device) (*service.Service, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "hotaru.yml")
	require.NoError(t, os.WriteFile(path, []byte(rules), 0o600))
	cfg, _, err := config.Load(path)
	require.NoError(t, err)
	svc := servicetest.New(t, cfg, devs...)
	svc.SetRulesPath(path)
	svc.SetRecorder(recorder(t))
	return svc, path
}

// A device given a leader follows it now, on the leader's recorded effect;
// the rest of its rule and the other rules are kept, and the file before is
// kept beside it.
func TestFollowIsSetAndStartsNow(t *testing.T) {
	board := canvastest.NewBoard("ASUS ROG MAXIMUS Z790 HERO", canvastest.Zone("Addressable 1", 4))
	svc, path := withRules(t, `devices:
  - match: "maximus"
    lengths: {"Addressable 1": 4}
  - match: "kraken"
    segments:
      ring: {zone: "Ring"}
`, board, leaderCooler())
	red := colour.MustParse("red")
	_, err := svc.Apply(t.Context(), service.Request{
		Colour: &red, Effects: map[string]scenes.Effect{"kraken": {Mode: "Tai Chi"}},
		Devices: []string{"NZXT Kraken Elite V2"},
	})
	require.NoError(t, err)
	require.NotEqual(t, "Tai Chi", viewOf(t, svc, "ASUS ROG MAXIMUS Z790 HERO").Device.ActiveMode)

	leader := "kraken"
	match, err := svc.SetRule(t.Context(), "maximus", service.RuleChange{Follow: &leader})
	require.NoError(t, err)
	assert.Equal(t, "maximus", match, "a second rule was added")
	assert.Equal(t, "Tai Chi", viewOf(t, svc, "ASUS ROG MAXIMUS Z790 HERO").Device.ActiveMode,
		"it did not start following")

	written, _, err := config.Load(path)
	require.NoError(t, err)
	require.Len(t, written.Devices, 2)
	assert.Equal(t, "kraken", written.Devices[0].Follow)
	assert.Equal(t, map[string]int{"Addressable 1": 4}, written.Devices[0].Lengths)
	assert.Contains(t, written.Devices[1].Segments, "ring")
	_, err = os.Stat(path + ".bak")
	assert.NoError(t, err, "the file before was not kept")

	none := ""
	_, err = svc.SetRule(t.Context(), "maximus", service.RuleChange{Follow: &none})
	require.NoError(t, err)
	written, _, err = config.Load(path)
	require.NoError(t, err)
	assert.Empty(t, written.Devices[0].Follow)
}

// A pace is set per effect, replaced whatever its case, and removed at zero.
func TestPaceIsSetAndRemoved(t *testing.T) {
	board := canvastest.NewBoard("ASUS ROG MAXIMUS Z790 HERO", canvastest.Zone("Addressable 1", 4))
	svc, path := withRules(t, "devices:\n  - match: \"maximus\"\n    pace: {\"breathing\": 2}\n", board)

	_, err := svc.SetRule(t.Context(), "maximus", service.RuleChange{Pace: map[string]float64{"Breathing": 2.16, "Fading": 1.1}})
	require.NoError(t, err)
	written, _, err := config.Load(path)
	require.NoError(t, err)
	assert.Equal(t, map[string]float64{"Breathing": 2.16, "Fading": 1.1}, written.Devices[0].Pace)

	_, err = svc.SetRule(t.Context(), "maximus", service.RuleChange{Pace: map[string]float64{"Breathing": 0, "Fading": 0}})
	require.NoError(t, err)
	written, _, err = config.Load(path)
	require.NoError(t, err)
	assert.Nil(t, written.Devices[0].Pace)
}

// A device with no rule gets one; a leader that is not there, a pace out of
// range and a device that is not there are refused, and nothing is written.
func TestARuleIsMadeAndBadChangesAreRefused(t *testing.T) {
	board := canvastest.NewBoard("ASUS ROG MAXIMUS Z790 HERO", canvastest.Zone("Addressable 1", 4))
	svc, path := withRules(t, "", board)

	nobody := "kraken"
	_, err := svc.SetRule(t.Context(), "maximus", service.RuleChange{Follow: &nobody})
	var bad *service.BadRule
	require.ErrorAs(t, err, &bad)
	_, err = svc.SetRule(t.Context(), "maximus", service.RuleChange{Pace: map[string]float64{"Fading": 11}})
	require.ErrorAs(t, err, &bad)
	_, err = svc.SetRule(t.Context(), "keychron", service.RuleChange{Pace: map[string]float64{"Fading": 2}})
	var missing *service.NoDevice
	require.True(t, errors.As(err, &missing), "%v", err)
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Empty(t, raw, "a refused change was written")

	match, err := svc.SetRule(t.Context(), "maximus", service.RuleChange{Pace: map[string]float64{"Fading": 2}})
	require.NoError(t, err)
	assert.Equal(t, "asus rog maximus z790 hero", match)
}
