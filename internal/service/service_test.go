package service_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/config"
	"github.com/ushineko/hotaru/internal/devices"
	"github.com/ushineko/hotaru/internal/openrgb"
	"github.com/ushineko/hotaru/internal/service"
)

// A board that advertises Static and Direct. On the real Aura, Static is
// accepted and leaves the addressable headers dark.
func board() devices.Device {
	return devices.Device{
		Name:     "ASUS ROG MAXIMUS Z790 HERO",
		LEDCount: 4,
		Modes: []devices.Mode{
			{Name: "Static", PerLED: true},
			{Name: "Direct", PerLED: true},
			{Name: "Rainbow Wave"},
		},
		Zones:      []devices.Zone{{Name: "Addressable 1", First: 0, Count: 4}},
		ActiveMode: "Rainbow Wave",
	}
}

func keyboard() devices.Device {
	return devices.Device{
		Name:     "Keychron K4 HE",
		LEDCount: 2,
		Modes: []devices.Mode{
			{Name: "Direct", PerLED: true},
			{Name: "Solid Color"},
		},
		Zones:      []devices.Zone{{Name: "Keyboard", First: 0, Count: 2}},
		ActiveMode: "Solid Color",
	}
}

func strip() devices.Device {
	return devices.Device{
		Name:       "Generic Strip",
		LEDCount:   3,
		Modes:      []devices.Mode{{Name: "Static"}, {Name: "Off"}},
		Zones:      []devices.Zone{{Name: "Strip", First: 0, Count: 3}},
		ActiveMode: "Off",
	}
}

func solid(target, name string) []devices.Assignment {
	t, err := devices.ParseTarget(target)
	if err != nil {
		panic(err)
	}
	return []devices.Assignment{{Target: t, Colour: colour.MustParse(name)}}
}

func TestAMachineWithNoConfigurationGetsItsLightsSet(t *testing.T) {
	// The stranger's machine: no rules file, hardware nobody wrote a line
	// about, and it works.
	server := openrgb.NewFake(board(), keyboard(), strip())
	svc := service.New(nil, server, "")

	results, err := svc.Apply(t.Context(), service.Request{Assignments: append(
		solid("ASUS", "red"), append(solid("Keychron", "red"), solid("Generic", "red")...)...)})
	require.NoError(t, err)
	require.Len(t, results, 3)
	for _, r := range results {
		require.True(t, r.Applied, "%s: %s", r.Device, r.Skipped)
	}

	showing, _ := server.Showing("Generic Strip")
	require.Equal(t, colour.MustParse("red"), showing.Colours[0])
}

func TestADeviceThatAcceptsAModeWithoutHonouringItFallsThrough(t *testing.T) {
	// The Aura failure, discovered rather than configured: Static is taken,
	// the server says yes, and the device stays where it was. Only the
	// read-back notices, and the next candidate is tried.
	server := openrgb.NewFake(board())
	server.Lies["ASUS ROG MAXIMUS Z790 HERO"] = "Static"
	svc := service.New(nil, server, "")

	results, err := svc.Apply(t.Context(), service.Request{Assignments: solid("ASUS", "blue")})
	require.NoError(t, err)
	require.Len(t, results, 1)

	got := results[0]
	require.True(t, got.Applied)
	require.Equal(t, "Direct", got.Mode, "the mode that actually took")
	require.Len(t, got.Attempts, 2, "and the one that did not is on the record")
	require.Equal(t, "Static", got.Attempts[0].Mode)
	require.True(t, got.Attempts[0].Accepted, "the server said yes")
	require.Equal(t, "Rainbow Wave", got.Attempts[0].Active, "and the device disagreed")
}

func TestARuleSparesTheFallThroughRatherThanBeingRequiredForIt(t *testing.T) {
	// With the correction written down, the first attempt is the right one.
	server := openrgb.NewFake(board())
	server.Lies["ASUS ROG MAXIMUS Z790 HERO"] = "Static"
	cfg := &config.Config{Devices: []config.DeviceRule{
		{Match: "maximus", SolidModes: []string{"direct", "static"}},
	}}
	svc := service.New(cfg, server, "")

	results, err := svc.Apply(t.Context(), service.Request{Assignments: solid("ASUS", "blue")})
	require.NoError(t, err)
	require.True(t, results[0].Applied)
	require.Len(t, results[0].Attempts, 1, "no wasted write")
}

func TestADeviceThatCannotBeBlankedIsSkippedWithItsReason(t *testing.T) {
	server := openrgb.NewFake(keyboard())
	cfg := &config.Config{Devices: []config.DeviceRule{{Match: "keychron", NeverBlank: true}}}
	svc := service.New(cfg, server, "")

	results, err := svc.Apply(t.Context(), service.Request{Off: true})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.False(t, results[0].Applied)
	require.Contains(t, results[0].Skipped, "backlight")
	require.Empty(t, server.Modes, "and nothing was written to it")
}

func TestOffUsesTheDevicesOwnOffModeWhereThereIsOne(t *testing.T) {
	server := openrgb.NewFake(strip(), keyboard())
	svc := service.New(nil, server, "")

	results, err := svc.Apply(t.Context(), service.Request{Off: true})
	require.NoError(t, err)
	require.Len(t, results, 2)

	byDevice := map[string]service.Result{}
	for _, r := range results {
		byDevice[r.Device] = r
	}
	require.Equal(t, "Off", byDevice["Generic Strip"].Mode)
	require.Equal(t, "Direct", byDevice["Keychron K4 HE"].Mode,
		"no Off mode, so direct with black -- which is why never_blank exists")

	showing, _ := server.Showing("Keychron K4 HE")
	require.Equal(t, colour.Black, showing.Colours[0])
}

func TestADeviceOutOfScopeIsLeftAloneEntirely(t *testing.T) {
	server := openrgb.NewFake(board(), keyboard())
	cfg := &config.Config{Scope: []string{"maximus"}}
	svc := service.New(cfg, server, "")

	results, err := svc.Apply(t.Context(), service.Request{Assignments: append(
		solid("ASUS", "red"), solid("Keychron", "red")...)})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "ASUS ROG MAXIMUS Z790 HERO", results[0].Device)

	for _, write := range server.Modes {
		require.NotEqual(t, "Keychron K4 HE", write.Device)
	}
}

func TestNamingADeviceNarrowsToItWithoutChangingScope(t *testing.T) {
	server := openrgb.NewFake(board(), keyboard())
	svc := service.New(nil, server, "")

	results, err := svc.Apply(t.Context(), service.Request{
		Assignments: append(solid("ASUS", "red"), solid("Keychron", "red")...),
		Devices:     []string{"keychron"},
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "Keychron K4 HE", results[0].Device)
}

func TestADeviceThatCannotShowTheFrameIsReportedNotApproximated(t *testing.T) {
	// The strip takes one colour for the whole of itself. A two-colour frame
	// is refused rather than reduced to whichever colour came first.
	server := openrgb.NewFake(strip())
	svc := service.New(nil, server, "")

	assignments := append(solid("Generic", "red"), devices.Assignment{
		Target: mustTarget(t, "Generic/Strip[0:0]"),
		Colour: colour.MustParse("blue"),
	})
	results, err := svc.Apply(t.Context(), service.Request{Assignments: assignments})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.False(t, results[0].Applied)
	require.Contains(t, results[0].Skipped, "per LED")
	require.Empty(t, server.Writes)
}

func TestListSaysWhichDevicesHotaruWouldDriveAndWhy(t *testing.T) {
	server := openrgb.NewFake(board(), keyboard())
	cfg := &config.Config{
		Scope:   []string{"maximus"},
		Devices: []config.DeviceRule{{Match: "maximus", SolidModes: []string{"direct"}}},
	}
	svc := service.New(cfg, server, "")

	list, err := svc.List(t.Context())
	require.NoError(t, err)
	require.Len(t, list, 2, "everything present is listed")

	require.True(t, list[0].InScope)
	require.Equal(t, []string{"direct"}, list[0].Rule.SolidModes)
	require.False(t, list[1].InScope, "and the reason it will not change is visible")
}

func TestAStoppedServerIsAnOrdinaryStateWithSomethingToDoAboutIt(t *testing.T) {
	svc := service.New(nil, nil, "127.0.0.1:6742")

	_, err := svc.List(t.Context())
	var down *service.Unreachable
	require.ErrorAs(t, err, &down)
	require.Equal(t, "127.0.0.1:6742", down.Address)
	require.Contains(t, err.Error(), "start it")

	_, err = svc.Apply(t.Context(), service.Request{Off: true})
	require.ErrorAs(t, err, &down)
}

func TestAServerThatFailsMidWayReportsAgainstTheDeviceItFailedOn(t *testing.T) {
	server := openrgb.NewFake(board())
	server.Unreachable = errors.New("connection reset")
	svc := service.New(nil, server, "")

	_, err := svc.Apply(t.Context(), service.Request{Assignments: solid("ASUS", "red")})
	require.ErrorContains(t, err, "connection reset")
}

func mustTarget(t *testing.T, s string) devices.Target {
	t.Helper()
	target, err := devices.ParseTarget(s)
	require.NoError(t, err)
	return target
}
