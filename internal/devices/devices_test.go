package devices_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/config"
	"github.com/ushineko/hotaru/internal/devices"
)

/*
The fixtures are this desk's hardware as it actually reports itself, because
that is where the awkwardness was learned. They are not what hotaru requires:
every test that matters here is about deriving behaviour from the report, so an
invented device works the same way.
*/

// The RTX 4090: accepts direct/breathing/flashing/off and does not advertise
// static at all. Sending static is what switched it off during the original
// investigation.
func gpu() *devices.Device {
	return &devices.Device{
		Name:     "MSI GeForce RTX 4090 Suprim Liquid X",
		LEDCount: 4,
		Modes: []devices.Mode{
			{Name: "Direct", PerLED: true},
			{Name: "Breathing"},
			{Name: "Flashing"},
			{Name: "Off"},
		},
		Zones: []devices.Zone{{Name: "Logo", First: 0, Count: 4}},
	}
}

// The Aura board: advertises both, but static drives only the onboard LED and
// leaves the addressable headers dark. Nothing in the report says so.
func aura() *devices.Device {
	return &devices.Device{
		Name:     "ASUS ROG MAXIMUS Z790 HERO",
		LEDCount: 8,
		Modes: []devices.Mode{
			{Name: "Static", PerLED: true},
			{Name: "Direct", PerLED: true},
			{Name: "Rainbow Wave"},
		},
		Zones: []devices.Zone{
			{Name: "Onboard", First: 0, Count: 2},
			{Name: "Addressable 1", First: 2, Count: 6},
		},
	}
}

// The Keychron: no Off mode at all, so "off" would resolve to black.
func keychron() *devices.Device {
	return &devices.Device{
		Name:     "Keychron K4 HE",
		LEDCount: 6,
		Modes: []devices.Mode{
			{Name: "Direct", PerLED: true},
			{Name: "Solid Color"},
			{Name: "Solid Splash"},
		},
		Zones: []devices.Zone{{Name: "Keyboard", First: 0, Count: 6}},
	}
}

// The cooler, whose radiator fans daisy-chain into it as one addressable zone.
func kraken() *devices.Device {
	return &devices.Device{
		Name:     "NZXT Kraken 2024 ELITE Series RGB",
		LEDCount: 40,
		Modes: []devices.Mode{
			{Name: "Static", PerLED: true},
			{Name: "Direct", PerLED: true},
			{Name: "Off"},
		},
		Zones: []devices.Zone{
			{Name: "Ring", First: 0, Count: 36},
			{Name: "Logo", First: 36, Count: 4},
		},
	}
}

// A device that takes one colour for the whole of itself, which is most cheap
// hardware and some expensive hardware.
func oneColour() *devices.Device {
	return &devices.Device{
		Name:     "Generic Strip",
		LEDCount: 10,
		Modes:    []devices.Mode{{Name: "Static"}, {Name: "Breathing"}},
		Zones:    []devices.Zone{{Name: "Strip", First: 0, Count: 10}},
	}
}

func TestTheGPUIsNeverOfferedAModeItDoesNotAdvertise(t *testing.T) {
	// Static is first in the default order and this device does not have it.
	// Resolution moves on rather than sending it and hoping.
	mode, err := gpu().Resolve(devices.Rule{}, devices.Want{})
	require.NoError(t, err)
	require.Equal(t, "Direct", mode)
}

func TestAuraTakesDirectFirstWhenARuleSaysTo(t *testing.T) {
	// Both modes are advertised and static is the default preference, so
	// without the rule hotaru would pick the one that leaves the headers dark.
	// This is the correction a rule exists to make.
	plain, err := aura().Resolve(devices.Rule{}, devices.Want{})
	require.NoError(t, err)
	require.Equal(t, "Static", plain)

	corrected, err := aura().Resolve(devices.Rule{SolidModes: []string{"direct", "static"}}, devices.Want{})
	require.NoError(t, err)
	require.Equal(t, "Direct", corrected)
}

func TestTheModeSentIsTheDeviceSpellingSoAReadBackCanBeCompared(t *testing.T) {
	mode, err := kraken().Resolve(devices.Rule{SolidModes: []string{"STATIC"}}, devices.Want{})
	require.NoError(t, err)
	require.Equal(t, "Static", mode, "the device's own spelling, not the rule's")
}

func TestTheKeyboardIsColouredButNeverBlanked(t *testing.T) {
	rule := devices.Rule{NeverBlank: true}

	_, err := keychron().Resolve(rule, devices.Want{Off: true})
	var unsupported *devices.Unsupported
	require.ErrorAs(t, err, &unsupported)
	require.Contains(t, err.Error(), "backlight", "the reason is the one a person would give")

	// Colour scenes still reach it: never-blanked is about off, not about the
	// device.
	mode, err := keychron().Resolve(rule, devices.Want{})
	require.NoError(t, err)
	require.Equal(t, "Direct", mode)
}

func TestADeviceWithNoOffModeFallsBackToDirect(t *testing.T) {
	// Without the rule, this is what happens — and why the rule exists.
	mode, err := keychron().Resolve(devices.Rule{}, devices.Want{Off: true})
	require.NoError(t, err)
	require.Equal(t, "Direct", mode)
}

func TestADeviceThatCannotShowTwoColoursIsReportedNotApproximated(t *testing.T) {
	_, err := oneColour().Resolve(devices.Rule{}, devices.Want{PerLED: true})

	var unsupported *devices.Unsupported
	require.ErrorAs(t, err, &unsupported)
	require.Contains(t, err.Error(), "per LED")
	require.Contains(t, err.Error(), "Static", "and says what it does have")

	// The same device is fine with one colour.
	mode, err := oneColour().Resolve(devices.Rule{}, devices.Want{})
	require.NoError(t, err)
	require.Equal(t, "Static", mode)
}

func TestTheFallThroughOffersEveryModeThatCouldWorkAndNoAnimations(t *testing.T) {
	// A write can be accepted and not honoured, so the caller needs somewhere
	// to go next. Rainbow Wave is not somewhere to go: a device that cannot do
	// static should not quietly end up animated.
	candidates := aura().SolidCandidates(devices.Rule{}, devices.Want{})
	require.Equal(t, []string{"Static", "Direct"}, candidates)

	perLED := gpu().SolidCandidates(devices.Rule{}, devices.Want{PerLED: true})
	require.Equal(t, []string{"Direct"}, perLED, "modes that take one colour cannot carry the frame")
}

func TestAMachineWithNoRulesStillResolvesEveryDevice(t *testing.T) {
	for _, d := range []*devices.Device{gpu(), aura(), keychron(), kraken(), oneColour()} {
		mode, err := d.Resolve(devices.Rule{}, devices.Want{})
		require.NoError(t, err, d.Name)
		require.NotEmpty(t, mode, d.Name)
	}
}

func TestRulesMergeInFileOrderWithLaterValuesWinning(t *testing.T) {
	fifty, hundred := 50, 100
	merged := devices.MergeRules([]config.DeviceRule{
		{Match: "kraken", SolidModes: []string{"static"}, Brightness: &fifty,
			Segments: map[string]config.Segment{"fan-top": {Zone: "Ring"}}},
		{Match: "nzxt", SolidModes: []string{"direct"}, Brightness: &hundred, NeverBlank: true,
			Reassert: config.Duration(90 * time.Second),
			Segments: map[string]config.Segment{"pump": {Zone: "Logo"}}},
	})

	require.Equal(t, []string{"direct"}, merged.SolidModes)
	require.Equal(t, 100, *merged.Brightness)
	require.True(t, merged.NeverBlank)
	require.Equal(t, 90*time.Second, merged.Reassert)
	require.Len(t, merged.Segments, 2, "segments accumulate rather than replacing")
}
