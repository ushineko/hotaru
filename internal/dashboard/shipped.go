package dashboard

import "github.com/ushineko/hotaru/internal/readings"

// Arrangements. Each draws the headline; they differ in what is under it.
const (
	// Ring is spec 013's layout: a graded ring, the headline inside it, and
	// three columns along the bottom.
	Ring = "ring"
	// Grid is the headline over four readings in two rows of two.
	Grid = "grid"
	// Stacked is the headline over four rows, with no ring: the arrangement
	// for somebody who wants numbers rather than an instrument.
	Stacked = "stacked"
	// Big is the headline alone, as large as the panel will take.
	Big = "big"
)

/*
Rings is how many arcs an arrangement has room for.

Geometry, not taste. A circle small enough to sit inside another one is, at
the bottom of the panel, a circle that crosses whatever is written there: at
radius 247 the arc passes through x=210 and x=430 at the foot of the metric
row, which is straight through two of its three columns. Only the outermost
ring skirts that row, which is why spec 013's inset clears one ring and could
never have cleared four.

So an arrangement with a row of readings along the bottom gets two, and the
one with nothing below the headline gets four -- which is the arrangement to
choose for a panel of gauges.
*/
func Rings(arrangement string) int {
	switch arrangement {
	case Big:
		return MostRings
	case Stacked:
		return 0
	}
	return 2
}

// Slots is how many small readings an arrangement draws. Extra slots in a
// saved dashboard are dropped rather than drawn over each other.
func Slots(arrangement string) int {
	switch arrangement {
	case Grid, Stacked:
		return 4
	case Big:
		return 0
	}
	return 3
}

// Arrangements is every arrangement, in the order the editor offers them.
var Arrangements = []string{Ring, Grid, Stacked, Big}

/*
Shipped are the dashboards every machine has before anybody saves one.

Code rather than a file, exactly as the shipped scenes are: a fresh install
has these and has written nothing. Saving over one of these names replaces it
for as long as the saved one exists, and deleting that override brings this
one back.

`coolant` is spec 013's screen, constant for constant. A machine that upgrades
and touches nothing sees exactly what it saw.
*/
func Shipped() []Dashboard {
	return []Dashboard{
		{
			Name:        "coolant",
			Arrangement: Ring,
			Headline:    Slot{Source: readings.Coolant, Label: "COOLANT"},
			Rings:       []readings.Source{readings.Coolant},
			Slots: []Slot{
				{Source: readings.CPUTemp, Label: "CPU"},
				{Source: readings.GPUTemp, Label: "GPU"},
				{Source: readings.PumpRPM, Label: "PUMP"},
			},
		},
		{
			Name:        "load",
			Arrangement: Grid,
			Theme:       "ice",
			Headline:    Slot{Source: readings.Coolant, Label: "COOLANT"},
			Rings:       []readings.Source{readings.Coolant, readings.CPULoad},
			Slots: []Slot{
				{Source: readings.CPULoad, Label: "CPU"},
				{Source: readings.GPULoad, Label: "GPU"},
				{Source: readings.CPUTemp, Label: "CPU"},
				{Source: readings.GPUTemp, Label: "GPU"},
			},
		},
		{
			Name:        "cooling",
			Arrangement: Stacked,
			Theme:       "amber",
			Headline:    Slot{Source: readings.Coolant, Label: "COOLANT"},
			Slots: []Slot{
				{Source: readings.PumpRPM, Label: "PUMP"},
				{Source: readings.PumpDuty, Label: "DUTY"},
				{Source: readings.FanRPM, Label: "FANS"},
				{Source: readings.FanDuty, Label: "DUTY"},
			},
		},
		{
			Name:        "quiet",
			Arrangement: Big,
			Theme:       "mono",
			Background:  Background{Kind: Plain},
			Headline:    Slot{Source: readings.Coolant, Label: "COOLANT"},
		},
	}
}

// Default is the dashboard a machine draws when it has been told nothing.
const Default = "coolant"
