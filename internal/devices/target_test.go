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
	// A list rather than a map: one of these deliberately carries the spacing
	// a person leaves behind, and a map key full of whitespace reads as a typo.
	for _, form := range []struct{ in, want string }{
		{"kraken", "kraken"},
		{"kraken/ring", "kraken/ring"},
		{"kraken/ring[0:11]", "kraken/ring[0:11]"},
		{"kraken/fan-top", "kraken/fan-top"},
		{"  kraken / ring ", "kraken/ring"},
	} {
		got, err := devices.ParseTarget(form.in)
		require.NoError(t, err, form.in)
		require.Equal(t, form.want, got.String(), form.in)
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

	spans, err := kraken().ResolveTarget(target, krakenRule())
	require.NoError(t, err)
	require.Equal(t, []devices.Span{{First: 0, Count: 40}}, spans)
}

func TestAZoneResolvesToTheDevicesOwnRun(t *testing.T) {
	target, _ := devices.ParseTarget("kraken/Logo")
	spans, err := kraken().ResolveTarget(target, devices.Rule{})
	require.NoError(t, err)
	require.Equal(t, []devices.Span{{First: 36, Count: 4}}, spans)
}

func TestANamedSegmentIsWhatAPersonWorkedOutOnce(t *testing.T) {
	target, _ := devices.ParseTarget("kraken/fan-mid")
	spans, err := kraken().ResolveTarget(target, krakenRule())
	require.NoError(t, err)
	require.Equal(t, []devices.Span{{First: 12, Count: 12}}, spans)

	// A segment naming a whole zone carries no range of its own.
	pump, _ := devices.ParseTarget("kraken/pump")
	spans, err = kraken().ResolveTarget(pump, krakenRule())
	require.NoError(t, err)
	require.Equal(t, []devices.Span{{First: 36, Count: 4}}, spans)
}

func TestASegmentNameWinsOverAZoneOfTheSameName(t *testing.T) {
	// The user named it; the vendor named the zone. On the user's machine the
	// user's name is the one they meant.
	rule := devices.Rule{Segments: map[string]config.Segment{
		"Logo": {Zone: "Ring", LEDs: &config.LEDs{First: 0, Last: 0}},
	}}
	target, _ := devices.ParseTarget("kraken/Logo")
	spans, err := kraken().ResolveTarget(target, rule)
	require.NoError(t, err)
	require.Equal(t, []devices.Span{{First: 0, Count: 1}}, spans)
}

func TestARangeIsRelativeToWhatItIsWrittenAgainst(t *testing.T) {
	// ring[0:11] is the first twelve of the ring, not of the device.
	inZone, _ := devices.ParseTarget("kraken/Logo[1:2]")
	spans, err := kraken().ResolveTarget(inZone, devices.Rule{})
	require.NoError(t, err)
	require.Equal(t, []devices.Span{{First: 37, Count: 2}}, spans)

	// And a range on a segment is relative to the segment.
	inSegment, _ := devices.ParseTarget("kraken/fan-mid[0:1]")
	spans, err = kraken().ResolveTarget(inSegment, krakenRule())
	require.NoError(t, err)
	require.Equal(t, []devices.Span{{First: 12, Count: 2}}, spans)
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

func TestAFrameOfManyColoursReducesToTheOneMostOfItIs(t *testing.T) {
	// What a mode that takes one colour is given when a scene names an effect
	// over a frame of many: the keyboard was mostly red, so the effect is red.
	red, blue := colour.MustParse("red"), colour.MustParse("blue")
	frame := devices.Solid(kraken(), red)
	frame.Colours[0] = blue
	frame.Colours[1] = blue

	got, ok := frame.Dominant()
	require.True(t, ok)
	require.Equal(t, red, got)
}

func TestAFrameWithNoColoursHasNoDominantOne(t *testing.T) {
	// A device hotaru has nothing for is an absent answer, not black: sending
	// black as a mode's colour would turn an effect off and call it applied.
	_, ok := devices.Frame{Device: "nothing"}.Dominant()
	require.False(t, ok)
}

func TestATieGoesToTheColourThatComesFirst(t *testing.T) {
	// Reducing a frame must not depend on the order a map is walked in.
	frame := devices.Frame{Device: "two", Colours: []colour.Colour{
		colour.MustParse("red"), colour.MustParse("blue"),
	}}
	for range 20 {
		got, ok := frame.Dominant()
		require.True(t, ok)
		require.Equal(t, colour.MustParse("red"), got)
	}
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

func TestLightsThatAreNotNextToEachOtherAreOneTarget(t *testing.T) {
	/*
		Pointing at four lights that are not adjacent is one intention, and
		hotaru had no way to say it: every run needed its own assignment, so a
		scene said four things where somebody meant one -- and said them in a
		form nobody would type.
	*/
	target, err := devices.ParseTarget("kraken/Logo[0,2:3]")
	require.NoError(t, err)
	require.Equal(t, "kraken/Logo[0,2:3]", target.String(),
		"the target did not come back the way it was written")

	spans, err := kraken().ResolveTarget(target, devices.Rule{})
	require.NoError(t, err)
	require.Equal(t, []devices.Span{{First: 36, Count: 1}, {First: 38, Count: 2}}, spans)
}

func TestOneLightIsWrittenAsItsNumber(t *testing.T) {
	// [7], not [7:7]. It is what somebody types, so it is what comes back.
	target, err := devices.ParseTarget("kraken/Ring[7]")
	require.NoError(t, err)
	require.Equal(t, "kraken/Ring[7]", target.String())

	spans, err := kraken().ResolveTarget(target, devices.Rule{})
	require.NoError(t, err)
	require.Equal(t, []devices.Span{{First: 7, Count: 1}}, spans)
}

func TestOneBadLightRefusesTheWholeTarget(t *testing.T) {
	/*
		All or nothing. Lighting two of three and reporting success would hide
		the mistake behind something that looked like it worked, which is the
		failure this project keeps finding in other people's code and its own.
	*/
	target, err := devices.ParseTarget("kraken/Logo[0,99]")
	require.NoError(t, err)

	_, err = kraken().ResolveTarget(target, devices.Rule{})
	require.ErrorContains(t, err, "99")
}

func TestNonsenseBetweenTheBracketsSaysSo(t *testing.T) {
	for _, target := range []string{
		"kraken/Ring[]",
		"kraken/Ring[1,]",
		"kraken/Ring[a]",
		"kraken/Ring[3:1]",
		"kraken/Ring[1:b]",
	} {
		_, err := devices.ParseTarget(target)
		require.Error(t, err, "%q parsed", target)
	}
}

func TestAListOfLightsComposesOntoTheFrame(t *testing.T) {
	// The end of the journey: what the picture points at, written down, and
	// applied to the LEDs it names.
	target, err := devices.ParseTarget("kraken/Logo[0,3]")
	require.NoError(t, err)

	frame, problems := devices.Compose(kraken(), devices.Rule{}, nil,
		[]devices.Assignment{{Target: target, Colour: colour.MustParse("red")}})

	require.Empty(t, problems)
	require.Equal(t, colour.MustParse("red"), frame.Colours[36])
	require.Equal(t, colour.MustParse("red"), frame.Colours[39])
	require.Equal(t, colour.Colour{}, frame.Colours[37], "a light nobody named was lit")
}
