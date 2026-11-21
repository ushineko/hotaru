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

// What a device would not take is refused and nothing written: a length for
// a zone it counts, a shape for fans it counts, a zone it does not have, and
// a name the rules do not take (spec 078).
func TestZoneChangesTheDeviceWouldNotTakeAreRefused(t *testing.T) {
	cooler := canvastest.NewBoard("NZXT Kraken Elite V2", canvastest.Ring("Ring", 24), canvastest.Ring("Fan", 8, 8, 8))
	header := canvastest.NewBoard("ASUS ROG MAXIMUS Z790 HERO", canvastest.Zone("Aura Mainboard", 1),
		canvastest.Header("Addressable RGB Header 3", 20, 120))
	svc, path := withRules(t, "", cooler, header)
	var bad *service.BadRule
	for name, change := range map[string]struct {
		device string
		change service.RuleChange
	}{
		"a length the device counts": {"maximus", service.RuleChange{Lengths: map[string]int{"Aura Mainboard": 3}}},
		"a length past its most":     {"maximus", service.RuleChange{Lengths: map[string]int{"Addressable RGB Header 3": 500}}},
		"a shape on counted fans":    {"kraken", service.RuleChange{Shapes: map[string]string{"Fan": "ring"}}},
		"a shape nobody draws":       {"maximus", service.RuleChange{Shapes: map[string]string{"Addressable RGB Header 3": "grid"}}},
		"a zone it does not have":    {"maximus", service.RuleChange{Shapes: map[string]string{"Header 9": "ring"}}},
		"a name with spaces":         {"maximus", service.RuleChange{Segments: map[string]*config.Segment{"Rear Fan": {Zone: "Addressable RGB Header 3"}}}},
	} {
		_, err := svc.SetRule(t.Context(), change.device, change.change)
		assert.ErrorAs(t, err, &bad, name)
	}
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Empty(t, raw, "a refused change was written")

	_, err = svc.SetRule(t.Context(), "maximus", service.RuleChange{
		Lengths:  map[string]int{"Addressable RGB Header 3": 18},
		Shapes:   map[string]string{"Addressable RGB Header 3": "ring"},
		Segments: map[string]*config.Segment{"rear-fan": {Zone: "Addressable RGB Header 3"}},
	})
	require.NoError(t, err)
	assert.Equal(t, 18, viewOf(t, svc, "ASUS ROG MAXIMUS Z790 HERO").Device.Zones[1].Count, "the length is not in use")
}

// One fan on a header named whole keeps its name (the rear fan, on the live
// run): divided, it is the same lights.
func TestOneFanKeepsAWholeZonesName(t *testing.T) {
	header := canvastest.NewBoard("ASUS ROG MAXIMUS Z790 HERO", canvastest.Zone("Aura Mainboard", 1),
		canvastest.Header("Addressable RGB Header 3", 20, 120))
	svc, path := withRules(t, `devices:
  - match: "maximus"
    segments:
      rear-fan: {zone: "Addressable RGB Header 3"}
`, header)
	_, err := svc.SetRule(t.Context(), "maximus", service.RuleChange{Fans: map[string]int{"Addressable RGB Header 3": 1}})
	require.NoError(t, err)
	written, _, err := config.Load(path)
	require.NoError(t, err)
	assert.Contains(t, written.Devices[0].Segments, "rear-fan")
	assert.Len(t, written.Devices[0].Segments, 1)
}

/*
Spec 084. Leading is written to the rules and in use at once: the rest start
on the leader's effect. One device leads, so a second takes it from the
first, and a leader follows nothing.
*/
func TestLeadIsSetMovedAndStartsNow(t *testing.T) {
	board := canvastest.NewBoard("ASUS ROG MAXIMUS Z790 HERO", canvastest.Zone("Addressable 1", 4))
	gpu := msiCard()
	svc, path := withRules(t, `devices:
  - match: "maximus"
    follow: "msi"
`, board, gpu)
	_, err := svc.Apply(t.Context(), service.Request{
		Effects: map[string]scenes.Effect{"msi": {Mode: "Breathing", Colours: []string{"#ff0000"}}},
		Devices: []string{"MSI GeForce RTX 4090"},
	})
	require.NoError(t, err)

	on := true
	_, err = svc.SetRule(t.Context(), "msi", service.RuleChange{Lead: &on})
	require.NoError(t, err)
	_, err = svc.SetRule(t.Context(), "maximus", service.RuleChange{Lead: &on})
	require.NoError(t, err)

	written, _, err := config.Load(path)
	require.NoError(t, err)
	leads := map[string]bool{}
	for _, r := range written.Devices {
		leads[r.Match] = r.Lead
		if r.Match == "maximus" {
			assert.Empty(t, r.Follow, "a leader still follows")
		}
	}
	assert.Equal(t, map[string]bool{"maximus": true, "msi geforce rtx 4090": false}, leads)
	assert.True(t, viewOf(t, svc, "ASUS ROG MAXIMUS Z790 HERO").Rule.Lead)

	off := false
	_, err = svc.SetRule(t.Context(), "maximus", service.RuleChange{Lead: &off})
	require.NoError(t, err)
	written, _, err = config.Load(path)
	require.NoError(t, err)
	for _, r := range written.Devices {
		assert.False(t, r.Lead, r.Match)
	}
}
