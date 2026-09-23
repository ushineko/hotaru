package dashboard

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/readings"
)

func paired() []Slot {
	return []Slot{
		{Source: readings.MemUsed, Second: readings.MemBytes},
		{Source: readings.PumpRPM, Second: readings.PumpDuty},
	}
}

func TestUnitsOffDrawNothingExtra(t *testing.T) {
	// Every dashboard written before this existed. The shipped screen's
	// golden frame is the other half of this, next door.
	slot := Slot{Source: readings.CPULoad, Second: readings.CPUTemp}
	require.Len(t, slot.Fields(reading(), false), 3, "units off drew a unit")
}

func TestEachHalfOfAPairCarriesItsOwnUnit(t *testing.T) {
	/*
		Which is the case pairs exist for: CPU load with CPU temperature is
		two readings measured in different things, and one unit at the end
		would say nothing about the first number.
	*/
	fields := Slot{Source: readings.CPULoad, Second: readings.CPUTemp}.Fields(reading(), true)

	var text []string
	for _, f := range fields {
		text = append(text, f.Text)
	}
	require.Equal(t, []string{"--", "%", " · ", "52", "°C"}, text)

	require.True(t, fields[1].Small, "the unit is not drawn small")
	require.True(t, fields[4].Small)
	require.False(t, fields[0].Small, "the number is drawn small")
	require.True(t, fields[2].Divider)
}

func TestASourceMeasuredInNothingReservesNothing(t *testing.T) {
	// A unit that is an empty string would otherwise be a gap after a number
	// for a word that is not there.
	for _, f := range unitOf(readings.Source("made_up"), true) {
		require.NotEmpty(t, f.Text, "an empty unit was given a field")
	}
	require.Empty(t, unitOf(readings.CPULoad, false))
}

func TestAUnitIsDrawnSmallerThanItsNumber(t *testing.T) {
	/*
		At the headline's size a full-size "%" is as large as the figure it
		qualifies and reads as a second number.
	*/
	number := Field{Text: "63", Chars: 3}
	unit := Field{Text: "°C", Small: true}

	require.Equal(t, 170.0, sizeOf(number, 170))
	require.Less(t, sizeOf(unit, 170), 170.0)
	require.InDelta(t, 170*UnitScale, sizeOf(unit, 170), 0.01)

	// And it stops shrinking before it becomes a smudge.
	require.Equal(t, float64(minUnitPt), sizeOf(unit, 4))
}

func TestTheGeneratedLabelDropsTheUnitWhenUnitsAreShown(t *testing.T) {
	/*
		The unit is beside the number now, and a label repeating it is the
		redundancy spec 041 removed the unit line for.
	*/
	slot := Slot{Source: readings.CPUTemp}
	require.Equal(t, "CPU °C", slot.Words(false))
	require.Equal(t, "CPU", slot.Words(true))

	pair := Slot{Source: readings.CPULoad, Second: readings.GPUTemp}
	require.Equal(t, "CPU % · GPU °C", pair.Words(false))
	require.Equal(t, "CPU · GPU", pair.Words(true))

	// Two readings of one thing, named once and measured in nothing worth
	// saying, is one word rather than a word and a dangling separator.
	same := Slot{Source: readings.CPULoad, Second: readings.CPUTemp}
	require.Equal(t, "CPU", same.Words(true))

	// An author's own label is never touched.
	mine := Slot{Source: readings.CPUTemp, Label: "PROC °C"}
	require.Equal(t, "PROC °C", mine.Words(true))
	require.Equal(t, "PROC °C", mine.Words(false))
}

func TestTheDotsLandInOneColumn(t *testing.T) {
	/*
		The rows are a table and the dot is its third column. Per row, the
		pump's four digits and the processor's two put their separators in
		different places, and four rows of that is a table with a bend in it.
	*/
	var r Reading
	r.Set(readings.MemUsed, 59)
	r.Set(readings.MemBytes, 73)
	r.Set(readings.PumpRPM, 2709)
	r.Set(readings.PumpDuty, 90)

	p := &paint{}
	rows := make([][]Field, 0, 2)
	for _, slot := range paired() {
		rows = append(rows, slot.Fields(r, true))
	}
	widths, _, _ := p.column(rows, 400, rowValuePt)

	var dots []int
	for _, row := range rows {
		at := 0
		for i, f := range row {
			if f.Divider {
				dots = append(dots, at)
				break
			}
			at += widths[i]
		}
	}
	require.Len(t, dots, 2)
	require.Equal(t, dots[0], dots[1], "the separators are not in one column")

	// And the rows are genuinely different widths, or this proves nothing.
	require.NotEqual(t,
		p.textWidth(rows[0][0].Text, rowValuePt, true, Text{}),
		p.textWidth(rows[1][0].Text, rowValuePt, true, Text{}))
}

func TestARowWithFewerFieldsAlignsFromTheLeft(t *testing.T) {
	/*
		A single value among pairs goes under the other rows' *first* numbers.
		Aligning it from the right would put it under their second ones, which
		is a lie about what it is.
	*/
	var r Reading
	r.Set(readings.MemUsed, 59)
	r.Set(readings.MemBytes, 73)
	r.Set(readings.Coolant, 40.1)

	p := &paint{}
	rows := [][]Field{
		Slot{Source: readings.MemUsed, Second: readings.MemBytes}.Fields(r, false),
		Slot{Source: readings.Coolant}.Fields(r, false),
	}
	widths, _, _ := p.column(rows, 400, rowValuePt)

	require.Len(t, rows[1], 1, "the lone row grew fields")
	require.GreaterOrEqual(t, widths[0], p.textWidth("40.1", rowValuePt, true, Text{}),
		"the first column is too narrow for the value that has to sit in it")
}
