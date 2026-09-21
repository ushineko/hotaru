package gui_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/gui"
)

func TestClickingTogglesALight(t *testing.T) {
	// Clicking twice takes it back out, which is what makes selecting four
	// lights something somebody can correct halfway through.
	var picked gui.Selection
	spot := gui.Lights("kraken", "ring", 3, 3)

	picked.Toggle(spot)
	require.True(t, picked.Has(spot))

	picked.Toggle(spot)
	require.False(t, picked.Has(spot))
	require.True(t, picked.Empty())
}

func TestAdjacentLightsBecomeOneRun(t *testing.T) {
	/*
		Four lights clicked one after another are one intention, and a scene
		that lists them separately says four things where somebody meant one
		-- and says them in a form nobody would have typed.
	*/
	var picked gui.Selection
	for _, at := range []int{5, 3, 4} {
		picked.Toggle(gui.Lights("kraken", "ring", at, at))
	}

	require.Equal(t, []string{"kraken/ring[3:5]"}, picked.Targets())
}

func TestAGapMakesTwoRuns(t *testing.T) {
	var picked gui.Selection
	for _, at := range []int{0, 1, 5, 6} {
		picked.Toggle(gui.Lights("kraken", "ring", at, at))
	}

	// One target, not two: the grammar says lists as well as runs, so what
	// somebody picked is one line in the scene.
	require.Equal(t, []string{"kraken/ring[0:1,5:6]"}, picked.Targets())
}

func TestRunsAreMergedPerZone(t *testing.T) {
	// Two zones' lights are two sets of runs, never one: the indices are
	// within the zone, so merging across them would address the wrong lights.
	var picked gui.Selection
	picked.Toggle(gui.Lights("kraken", "ring", 0, 0))
	picked.Toggle(gui.Lights("kraken", "fans", 0, 0))

	require.ElementsMatch(t,
		[]string{"kraken/ring[0]", "kraken/fans[0]"}, picked.Targets())
}

func TestBlocksThatAlreadyCoverRunsStillMerge(t *testing.T) {
	// A big zone is drawn as runs rather than lights, and two adjacent runs
	// are still one run.
	var picked gui.Selection
	picked.Toggle(gui.Lights("keychron", "keyboard", 0, 4))
	picked.Toggle(gui.Lights("keychron", "keyboard", 5, 9))

	require.Equal(t, []string{"keychron/keyboard[0:9]"}, picked.Targets())
}

func TestAWholeZoneOrDeviceIsAlreadyOneTarget(t *testing.T) {
	var picked gui.Selection
	picked.Toggle(gui.WholeDevice("kraken"))
	picked.Toggle(gui.WholeZone("kraken", "ring"))

	require.Equal(t, []string{"kraken", "kraken/ring"}, picked.Targets())
}

func TestASpotSaysWhatItIs(t *testing.T) {
	require.Equal(t, "ring, light 3", gui.Lights("kraken", "ring", 3, 3).Describe())
	require.Equal(t, "ring, lights 3 to 7", gui.Lights("kraken", "ring", 3, 7).Describe())
	require.Equal(t, "ring", gui.WholeZone("kraken", "ring").Describe())
	require.Equal(t, "kraken", gui.WholeDevice("kraken").Describe())
}

func TestScatteredLightsAreOneTarget(t *testing.T) {
	/*
		The shape somebody actually wants: pick the lights you mean, wherever
		they are, and get one rule for all of them.

		`kraken/ring[1,4,7:9]` -- one target, one assignment, one line in the
		scene, and exactly what they would have typed.
	*/
	var picked gui.Selection
	for _, at := range []int{7, 1, 9, 4, 8} {
		picked.Toggle(gui.Lights("kraken", "ring", at, at))
	}

	require.Equal(t, []string{"kraken/ring[1,4,7:9]"}, picked.Targets())
}

func TestOneLightIsWrittenAsItsNumber(t *testing.T) {
	// [7], not [7:7]: it is what somebody types.
	var picked gui.Selection
	picked.Toggle(gui.Lights("kraken", "ring", 7, 7))

	require.Equal(t, []string{"kraken/ring[7]"}, picked.Targets())
}
