package openrgb

import (
	"testing"

	sdk "github.com/csutorasa/go-openrgb-sdk"
	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/colour"
)

var (
	red  = colour.Colour{R: 255}
	blue = colour.Colour{B: 255}
	lime = colour.Colour{G: 255}
)

/*
The slot shapes below are what OpenRGB reported on the two machines spec 061
measured: one and one, one to eight with one slot filled, two and two, and
two to seven.
*/
func TestOneColourFillsEverySlotTheModeHas(t *testing.T) {
	// A stick of RAM whose pulse takes exactly two: both the new colour,
	// never the new one beside whatever the vendor left in the second.
	require.Equal(t, []colour.Colour{red, red}, Slots([]colour.Colour{red}, 2, 2, 2))
	require.Equal(t, []colour.Colour{red}, Slots([]colour.Colour{red}, 1, 1, 8))
}

func TestSeveralColoursGoOneToASlotUpToTheMost(t *testing.T) {
	require.Equal(t, []colour.Colour{red, blue}, Slots([]colour.Colour{red, blue}, 1, 1, 8))
	require.Equal(t, []colour.Colour{red, blue}, Slots([]colour.Colour{red, blue, lime}, 2, 2, 2))
}

func TestFewerColoursThanTheLeastRepeatTheLast(t *testing.T) {
	require.Equal(t, []colour.Colour{red, blue, blue}, Slots([]colour.Colour{red, blue}, 2, 3, 7))
}

func TestNoColoursFillNothing(t *testing.T) {
	require.Nil(t, Slots(nil, 2, 2, 2))
}

func TestAModeIsReadWithItsSlotsAndEveryColourItHolds(t *testing.T) {
	device := convert(&sdk.ControllerData{
		Name: "board",
		Modes: []*sdk.Mode{
			{ModeName: "Breathing", ModeFlags: flagHasModeSpecificColor, ModeColorsMin: 1, ModeColorsMax: 2,
				ModeColorMode: colourModeSpecific, ModeColors: []sdk.Color{{R: 255}, {B: 255}}},
			// A per-LED mode that reports one slot: not a colour of its own.
			{ModeName: "Direct", ModeFlags: flagHasPerLEDColor, ModeColorsMin: 1, ModeColorsMax: 1},
		},
	})
	breathing, _ := device.Mode("Breathing")
	require.Equal(t, 1, breathing.ColoursMin)
	require.Equal(t, 2, breathing.ColoursMax)
	require.Equal(t, []colour.Colour{red, blue}, breathing.Colours)
	require.Equal(t, red, breathing.Colour)

	direct, _ := device.Mode("Direct")
	require.Zero(t, direct.ColoursMax)
}

func TestAModeWithItsOwnColourAndNoReportedSlotsStillTakesOne(t *testing.T) {
	least, most := colourSlots(&sdk.Mode{ModeFlags: flagHasModeSpecificColor})
	require.Equal(t, 0, least)
	require.Equal(t, 1, most)
}

func TestTwoColoursAreWrittenIntoATwoColourMode(t *testing.T) {
	m := &sdk.Mode{ModeColorsMin: 1, ModeColorsMax: 2, ModeColors: []sdk.Color{{G: 255}}}
	require.Equal(t, []sdk.Color{{R: 255}, {B: 255}}, modeColours(m, []colour.Colour{red, blue}))
}
