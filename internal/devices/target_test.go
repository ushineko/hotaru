package devices_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/config"
	"github.com/ushineko/hotaru/internal/devices"
)

// The cooler's rules as someone would write them after looking at the fans.
func krakenRule() devices.Rule {
	return devices.Rule{Segments: map[string]config.Segment{
		"fan-top": {Zone: "Ring", LEDs: &config.LEDs{First: 0, Last: 11}},
		"fan-mid": {Zone: "Ring", LEDs: &config.LEDs{First: 12, Last: 23}},
		"fan-bot": {Zone: "Ring", LEDs: &config.LEDs{First: 24, Last: 35}},
		"pump":    {Zone: "Logo"},
	}}
}

func TestTheFormsAPersonWritesAllParse(t *testing.T) {
	for in, want := range map[string]string{
		"kraken":            "kraken",
		"kraken/ring":       "kraken/ring",
		"kraken/ring[0:11]": "kraken/ring[0:11]",
		"kraken/fan-top":    "kraken/fan-top",
		"  kraken / ring ":  "kraken/ring",
	} {
		got, err := devices.ParseTarget(in)
		require.NoError(t, err, in)
		require.Equal(t, want, got.String(), in)
	}
}

func TestWhatIsNotATargetIsRejected(t *testing.T) {
	for _, in := range []string{"", "   ", "/ring", "kraken/", "kraken/ring[11:0]"} {
		_, err := devices.ParseTarget(in)
		require.Error(t, err, in)
	}
}

func TestAWholeDeviceIsEveryLEDAndIsTheSameMechanism(t *testing.T) {
	target, err := devices.ParseTarget("kraken")
	require.NoError(t, err)

	span, err := kraken().ResolveTarget(target, krakenRule())
	require.NoError(t, err)
	require.Equal(t, devices.Span{First: 0, Count: 40}, span)
}

func TestAZoneResolvesToTheDevicesOwnRun(t *testing.T) {
	target, _ := devices.ParseTarget("kraken/Logo")
	span, err := kraken().ResolveTarget(target, devices.Rule{})
	require.NoError(t, err)
	require.Equal(t, devices.Span{First: 36, Count: 4}, span)
}

func TestANamedSegmentIsWhatAPersonWorkedOutOnce(t *testing.T) {
	target, _ := devices.ParseTarget("kraken/fan-mid")
	span, err := kraken().ResolveTarget(target, krakenRule())
	require.NoError(t, err)
	require.Equal(t, devices.Span{First: 12, Count: 12}, span)

	// A segment naming a whole zone carries no range of its own.
	pump, _ := devices.ParseTarget("kraken/pump")
	span, err = kraken().ResolveTarget(pump, krakenRule())
	require.NoError(t, err)
	require.Equal(t, devices.Span{First: 36, Count: 4}, span)
}

func TestASegmentNameWinsOverAZoneOfTheSameName(t *testing.T) {
	// The user named it; the vendor named the zone. On the user's machine the
	// user's name is the one they meant.
	rule := devices.Rule{Segments: map[string]config.Segment{
		"Logo": {Zone: "Ring", LEDs: &config.LEDs{First: 0, Last: 0}},
	}}
	target, _ := devices.ParseTarget("kraken/Logo")
	span, err := kraken().ResolveTarget(target, rule)
	require.NoError(t, err)
	require.Equal(t, devices.Span{First: 0, Count: 1}, span)
}

func TestARangeIsRelativeToWhatItIsWrittenAgainst(t *testing.T) {
	// ring[0:11] is the first twelve of the ring, not of the device.
	inZone, _ := devices.ParseTarget("kraken/Logo[1:2]")
	span, err := kraken().ResolveTarget(inZone, devices.Rule{})
	require.NoError(t, err)
	require.Equal(t, devices.Span{First: 37, Count: 2}, span)

	// And a range on a segment is relative to the segment.
	inSegment, _ := devices.ParseTarget("kraken/fan-mid[0:1]")
	span, err = kraken().ResolveTarget(inSegment, krakenRule())
	require.NoError(t, err)
	require.Equal(t, devices.Span{First: 12, Count: 2}, span)
}

func TestAnUnknownNameSaysWhatDoesExist(t *testing.T) {
	target, _ := devices.ParseTarget("kraken/fan-left")
	_, err := kraken().ResolveTarget(target, krakenRule())

	var unsupported *devices.Unsupported
	require.ErrorAs(t, err, &unsupported)
	require.Contains(t, err.Error(), "fan-left")
	require.Contains(t, err.Error(), "fan-top", "the segments that do exist")
	require.Contains(t, err.Error(), "Ring", "and the zones")
}

func TestARangeOffTheEndIsRefusedRatherThanLightingWhateverIsThere(t *testing.T) {
	target, _ := devices.ParseTarget("kraken/Logo[0:9]")
	_, err := kraken().ResolveTarget(target, devices.Rule{})
	require.ErrorContains(t, err, "that has 4")
}

func TestASegmentNamingAMissingZoneIsReportedAgainstTheDevice(t *testing.T) {
	// The same rules file on a machine whose cooler has different zones.
	target, _ := devices.ParseTarget("kraken/fan-top")
	_, err := oneColour().ResolveTarget(target, krakenRule())
	require.ErrorContains(t, err, `zone "Ring"`)
	require.ErrorContains(t, err, "Strip", "and what it does have")
}

func TestAssignmentsComposeIntoOneFramePerDevice(t *testing.T) {
	red, blue, white := colour.MustParse("red"), colour.MustParse("blue"), colour.MustParse("white")

	assignments := []devices.Assignment{
		{Target: mustTarget(t, "kraken"), Colour: blue},
		{Target: mustTarget(t, "kraken/fan-top"), Colour: red},
		{Target: mustTarget(t, "kraken/pump"), Colour: white},
	}
	frame, problems := devices.Compose(kraken(), krakenRule(), nil, assignments)
	require.Empty(t, problems)

	require.Len(t, frame.Colours, 40, "a frame is the whole device")
	require.Equal(t, red, frame.Colours[0], "the top fan")
	require.Equal(t, red, frame.Colours[11])
	require.Equal(t, blue, frame.Colours[12], "and everything the later assignments did not cover")
	require.Equal(t, white, frame.Colours[36], "the pump")
	require.True(t, frame.PerLED(), "three colours need a mode that can show them")
}

func TestALaterAssignmentWinsWhichIsWhatMakesExceptAFan(t *testing.T) {
	blue, red := colour.MustParse("blue"), colour.MustParse("red")
	frame, problems := devices.Compose(kraken(), krakenRule(), nil, []devices.Assignment{
		{Target: mustTarget(t, "kraken"), Colour: blue},
		{Target: mustTarget(t, "kraken/fan-bot"), Colour: red},
	})
	require.Empty(t, problems)
	require.Equal(t, blue, frame.Colours[0])
	require.Equal(t, red, frame.Colours[24])
	require.Equal(t, red, frame.Colours[35])
	require.Equal(t, blue, frame.Colours[36], "the logo is not part of the ring")
}

func TestLEDsNobodyMentionedKeepTheColourTheyHad(t *testing.T) {
	green, red := colour.MustParse("green"), colour.MustParse("red")
	base := make([]colour.Colour, 40)
	for i := range base {
		base[i] = green
	}

	frame, problems := devices.Compose(kraken(), krakenRule(), base, []devices.Assignment{
		{Target: mustTarget(t, "kraken/fan-top"), Colour: red},
	})
	require.Empty(t, problems)
	require.Equal(t, red, frame.Colours[0])
	require.Equal(t, green, frame.Colours[12], "untouched, not blanked")
}

func TestOneBadAssignmentCostsThatAssignmentAndNothingElse(t *testing.T) {
	red, blue := colour.MustParse("red"), colour.MustParse("blue")
	frame, problems := devices.Compose(kraken(), krakenRule(), nil, []devices.Assignment{
		{Target: mustTarget(t, "kraken/fan-left"), Colour: blue},
		{Target: mustTarget(t, "kraken/fan-top"), Colour: red},
	})
	require.Len(t, problems, 1)
	require.ErrorContains(t, problems[0], "fan-left")
	require.Equal(t, red, frame.Colours[0], "the rest of the scene still applied")
}

func TestAUniformFrameIsRecognisedSoSimpleDevicesStillWork(t *testing.T) {
	frame := devices.Solid(oneColour(), colour.MustParse("teal"))
	got, uniform := frame.Uniform()
	require.True(t, uniform)
	require.Equal(t, colour.MustParse("teal"), got)
	require.False(t, frame.PerLED())
}

func TestTwoFramesAreEqualWhenTheyWouldLookTheSame(t *testing.T) {
	a := devices.Solid(kraken(), colour.MustParse("red"))
	b := devices.Solid(kraken(), colour.MustParse("red"))
	require.True(t, a.Equal(b), "reconciliation asks exactly this")

	b.Colours[7] = colour.MustParse("blue")
	require.False(t, a.Equal(b))
}

func mustTarget(t *testing.T, s string) devices.Target {
	t.Helper()
	target, err := devices.ParseTarget(s)
	require.NoError(t, err)
	return target
}
