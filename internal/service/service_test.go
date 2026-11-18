package service_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/config"
	"github.com/ushineko/hotaru/internal/devices"
	"github.com/ushineko/hotaru/internal/service"
	"github.com/ushineko/hotaru/internal/service/servicetest"
)

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
	strip := genericStrip()
	svc := servicetest.New(t, nil, auraBoard(), k4(), strip)

	results, err := svc.Apply(t.Context(), service.Request{Assignments: append(
		solid("ASUS", "red"), append(solid("Keychron", "red"), solid("Generic", "red")...)...)})
	require.NoError(t, err)
	require.Len(t, results, 3)
	for _, r := range results {
		require.True(t, r.Applied, "%s: %s", r.Device, r.Skipped)
	}
	require.Eventually(t, func() bool { return shows(t, strip.Device, colour.MustParse("red")) }, eventually, every)
}

func TestADeviceThatCannotBeBlankedIsSkippedWithItsReason(t *testing.T) {
	kb := k4()
	cfg := &config.Config{Devices: []config.DeviceRule{{Match: "keychron", NeverBlank: true}}}
	svc := servicetest.New(t, cfg, kb)

	results, err := svc.Apply(t.Context(), service.Request{Off: true})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.False(t, results[0].Applied)
	require.Contains(t, results[0].Skipped, "backlight")
	require.Zero(t, kb.Frames(), "and nothing was written to it")
}

func TestADeviceOutOfScopeIsLeftAloneEntirely(t *testing.T) {
	kb := k4()
	cfg := &config.Config{Scope: []string{"maximus"}}
	svc := servicetest.New(t, cfg, auraBoard(), kb)

	results, err := svc.Apply(t.Context(), service.Request{Assignments: append(
		solid("ASUS", "red"), solid("Keychron", "red")...)})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "ASUS ROG MAXIMUS Z790 HERO", results[0].Device)
	require.Zero(t, kb.Frames())
}

func TestNamingADeviceNarrowsToItWithoutChangingScope(t *testing.T) {
	svc := servicetest.New(t, nil, auraBoard(), k4())

	results, err := svc.Apply(t.Context(), service.Request{
		Assignments: append(solid("ASUS", "red"), solid("Keychron", "red")...),
		Devices:     []string{"keychron"},
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "Keychron K4 HE", results[0].Device)
}

func TestListSaysWhichDevicesHotaruWouldDriveAndWhy(t *testing.T) {
	cfg := &config.Config{
		Scope:   []string{"maximus"},
		Devices: []config.DeviceRule{{Match: "maximus", SolidModes: []string{"static"}}},
	}
	svc := servicetest.New(t, cfg, auraBoard(), k4())

	list, err := svc.List(t.Context())
	require.NoError(t, err)
	require.Len(t, list, 2, "everything present is listed")
	byName := map[string]service.View{}
	for _, v := range list {
		byName[v.Device.Name] = v
	}
	board := byName["ASUS ROG MAXIMUS Z790 HERO"]
	require.True(t, board.InScope)
	require.Equal(t, []string{"static"}, board.Rule.SolidModes)
	require.False(t, byName["Keychron K4 HE"].InScope, "and the reason it will not change is visible")
}

func TestAMachineWithNothingFoundIsAnOrdinaryState(t *testing.T) {
	// Nothing plugged in, or nothing hotaru may open: not an error. The
	// listing is empty, a write changes nothing, and health says why.
	svc := service.New(nil)

	list, err := svc.List(t.Context())
	require.NoError(t, err)
	require.Empty(t, list)

	results, err := svc.Apply(t.Context(), service.Request{Off: true})
	require.NoError(t, err)
	require.Empty(t, results)
}

func TestABrightnessRuleIsAssertedOnEveryWrite(t *testing.T) {
	// Brightness is device state nobody owned: whatever was last written
	// stuck, so a value set once by hand became permanent and invisible.
	// Asserting it makes hotaru's picture of the device complete rather than
	// partial. On a canvas it is a multiply on every frame.
	kb, board := k4(), auraBoard()
	level := 50
	cfg := &config.Config{Devices: []config.DeviceRule{{Match: "keychron", Brightness: &level}}}
	svc := servicetest.New(t, cfg, kb, board)

	half := colour.Colour{R: 128}
	for range 2 {
		_, err := svc.Apply(t.Context(), service.Request{Assignments: append(solid("Keychron", "red"), solid("ASUS", "red")...)})
		require.NoError(t, err)
		require.Eventually(t, func() bool { return shows(t, kb.Device, half) }, eventually, every,
			"the rule's brightness never reached the device")
	}
	// A device with no such rule is not dimmed.
	require.Eventually(t, func() bool { return shows(t, board.Device, colour.MustParse("red")) }, eventually, every)
}

func TestAnAssignmentThatNamedNothingIsReportedEvenWhenTheWriteWorked(t *testing.T) {
	// "Everything blue, except the top fan" with the fan misspelled is a
	// device that went blue all over and reported success. Found on a machine
	// whose zone name contained a slash, where the typo was mine.
	svc := servicetest.New(t, nil, auraBoard())

	typo, err := devices.ParseTarget("ASUS/Addressable 1 but spelled wrong")
	require.NoError(t, err)

	blue := colour.MustParse("blue")
	results, err := svc.Apply(t.Context(), service.Request{
		Colour:      &blue,
		Assignments: []devices.Assignment{{Target: typo, Colour: colour.MustParse("red")}},
	})
	require.NoError(t, err)
	require.Len(t, results, 1)

	require.True(t, results[0].Applied, "the rest of the request still applied")
	require.Len(t, results[0].Problems, 1)
	require.Contains(t, results[0].Problems[0], "spelled wrong")
	require.Contains(t, results[0].Problems[0], "Addressable 1", "and what does exist")
}

/*
cooler is a device whose Static mode carries its own colour.

Modelled on the NZXT Kraken that turned three radiator fans red when asked for
purple: Static holds a colour of its own, the per-LED buffer it also accepts is
not what it displays, and the colour already in the mode is the vendor's. See
spec 009.
*/
func TestTheOrdinaryWritePathDoesNotWait(t *testing.T) {
	/*
		Devices are written one after another, so a pause per device is paid
		for every device on the machine. The reporter noticed hotaru's speed
		over the program it replaces; keeping it is a requirement, not a
		nicety. Spec 010 AC7.
	*/
	svc := servicetest.New(t, nil, auraBoard(), genericStrip(), k4())

	start := time.Now()
	red := colour.MustParse("red")
	_, err := svc.Apply(t.Context(), service.Request{Colour: &red})
	require.NoError(t, err)
	require.Less(t, time.Since(start), 50*time.Millisecond,
		"the ordinary write path is sleeping; the settle delay has leaked into it")
}
