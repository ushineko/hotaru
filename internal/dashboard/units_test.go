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

/*
A unit stands a hair off the number it belongs to.

Spec 044 packed the value and its unit as one run so that nothing sat between
them, which was right and came out a shade too tight: "12%" with the figure
and the sign touching reads as one token. The gap is a fraction of the
number's size, so it is the same gap on a stacked row and on the headline.
*/
func TestAUnitDoesNotTouchItsNumber(t *testing.T) {
	p := &paint{}
	unit := Field{Text: "%", Small: true}

	require.Greater(t, p.fieldWidth(unit, rowValuePt),
		p.textWidth(unit.Text, sizeOf(unit, rowValuePt), true, Text{}),
		"the unit's box is exactly its text, so it is drawn against the number")

	// It grows with the text, and it belongs to units alone: a number's box
	// is its reservation, and a gap inside one would be a digit moving.
	require.Greater(t, unitGap(unit, 170), unitGap(unit, rowValuePt))
	require.Zero(t, unitGap(Field{Text: "12", Chars: 2}, rowValuePt))

	// The pair packed around a divider reserves nothing and measures its
	// own runs, so it carries the gap the same way or "12%" is spaced on a
	// stacked row and tight on the headline.
	require.Equal(t, unitGap(unit, rowValuePt),
		p.run(unit, rowValuePt)-p.textWidth(unit.Text, sizeOf(unit, rowValuePt), true, Text{}))
}

func TestAPairIsFittedToTheShapeItIsDrawnIn(t *testing.T) {
	/*
		`48% · 100°C` hung five pixels past the headline's inset and into the
		bezel, while `48% · 38°C` sat well inside it (#140).

		A pair was fitted with every field laid end to end and drawn anchored
		on its separator, and the two shapes agree only when the halves are
		the same width: the separator is centred, so the wider half decides
		both sides and the assembly needs `separator + 2 * max(left, right)`.
	*/
	p := &paint{}
	lopsided := Slot{Source: readings.CPULoad, Second: readings.CPUTemp}.
		Fields(cpu(48, 100), true)
	at, ok := dividerAt(lopsided)
	require.True(t, ok, "the pair has no separator")

	_, laid := p.boxes(lopsided, 88)
	require.Greater(t, p.spread(lopsided, at, 88), laid,
		"the drawn shape is no wider than the one that was fitted; this proves nothing")

	// So the fit is the stricter of the two, and what it settles on fits.
	size := p.fitted(lopsided, 200, 88, Centre)
	require.LessOrEqual(t, p.spread(lopsided, at, size), 200,
		"the pair was fitted to a room it does not fit")
}

func TestAPairIsTheSameSizeWhateverItSays(t *testing.T) {
	/*
		The reservation is what makes an assembly the same width from one
		frame to the next, so the fit uses it: a headline that shrank as its
		temperature passed a hundred would be spec 042 undone.

		Asserted on the drawn pixels, because the point size is not a number
		anybody can read off the panel -- the height of the digits is.
	*/
	d := Shipped()[1]
	d.Arrangement = Stacked
	d.Units = true
	d.Background = Background{Kind: Plain}
	d.Headline = Slot{Source: readings.CPULoad, Second: readings.CPUTemp, Label: "CPU"}

	heights := map[int]bool{}
	for _, pair := range [][2]float64{{5, 38}, {48, 38}, {48, 100}, {100, 38}, {100, 100}} {
		img := decode(t, Render(d, cpu(pair[0], pair[1]), 0, nil, Trails{}))

		rows := inked(img, 98, 214)
		require.NotEmpty(t, rows, "the headline drew nothing")
		heights[rows[len(rows)-1]-rows[0]] = true

		// And it is inside the panel's inset, which is what #140 was.
		left, right := inkEdges(img, 98, 214-98)
		require.GreaterOrEqual(t, left, headlineInset, "%v overflows to the left", pair)
		require.LessOrEqual(t, right, Size-headlineInset, "%v overflows to the right", pair)
	}
	require.Len(t, heights, 1, "the headline changed size with what it said: %v", heights)
}

// cpu is a reading of the two halves of a processor headline.
func cpu(share, degrees float64) Reading {
	var r Reading
	r.Set(readings.CPULoad, share)
	r.Set(readings.CPUTemp, degrees)
	return r
}
