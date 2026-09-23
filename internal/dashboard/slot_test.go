package dashboard

import (
	"image"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/readings"
)

func TestASlotWithOneReadingSaysWhatItAlwaysSaid(t *testing.T) {
	// Every dashboard written before spec 041 is this case, and none of them
	// should have changed shape.
	slot := Slot{Source: readings.CPUTemp}
	require.Equal(t, "52", slot.Text(reading()))
}

func TestASlotWithTwoReadingsJoinsThem(t *testing.T) {
	/*
		The thought the feature is for: CPU load and CPU temperature are one
		question, and "11 / 59" under "CPU % / C" answers it in the room one
		number used to take.
	*/
	r := reading()
	r.Set(readings.CPULoad, 11)
	r.Set(readings.CPUTemp, 59)

	slot := Slot{Source: readings.CPULoad, Second: readings.CPUTemp}
	require.Equal(t, "11 · 59", slot.Text(r))
}

func TestTheSeparatorIsTheAuthorsWithAMiddleDotAsTheDefault(t *testing.T) {
	r := reading()
	r.Set(readings.CPULoad, 11)
	r.Set(readings.CPUTemp, 59)
	slot := Slot{Source: readings.CPULoad, Second: readings.CPUTemp}

	require.Equal(t, "11 · 59", slot.Text(r), "no separator did not draw the default")

	/*
		And an author who set one keeps it, including the slash that used to
		be the default. A default is what happens when nobody said anything;
		changing it must not reach into the dashboards of people who did.
	*/
	slot.Separator = " / "
	require.Equal(t, "11 / 59", slot.Text(r))

	// Including one with no spaces in it, which is a thing somebody will want
	// in a narrow column.
	slot.Separator = "/"
	require.Equal(t, "11/59", slot.Text(r))
}

func TestOneMissingSensorDoesNotTakeTheOtherWithIt(t *testing.T) {
	/*
		A pair with one sensor gone reads "-- · 59", which says which half
		went away. Collapsing to a single placeholder would report both as
		absent on the evidence of one, and the other one is still true --
		which is the whole shape of this package.
	*/
	r := reading()
	r.Set(readings.CPUTemp, 59)

	slot := Slot{Source: readings.CPULoad, Second: readings.CPUTemp}
	require.Equal(t, readings.Placeholder+" · 59", slot.Text(r))
}

func TestADefaultLabelCarriesTheUnit(t *testing.T) {
	/*
		The unit line is gone, so the label has to say it. A slot nobody has
		edited must still say whether it is degrees or percent -- otherwise
		removing the line costs information rather than a redundancy.
	*/
	require.Equal(t, "CPU °C", Slot{Source: readings.CPUTemp}.Words())

	// Two readings of the same thing are named once: "CPU % / CPU °C" makes
	// the eye read a word to learn nothing.
	require.Equal(t, "CPU % · °C",
		Slot{Source: readings.CPULoad, Second: readings.CPUTemp}.Words())

	// Two different things are both named.
	require.Equal(t, "CPU % · GPU °C",
		Slot{Source: readings.CPULoad, Second: readings.GPUTemp}.Words())

	// The label is divided by whatever divides the numbers: two answers to
	// one question is one answer too many.
	require.Equal(t, "CPU % / °C",
		Slot{Source: readings.CPULoad, Second: readings.CPUTemp, Separator: " / "}.Words())

	// And the author's own words beat all of it.
	require.Equal(t, "PROC",
		Slot{Source: readings.CPULoad, Second: readings.CPUTemp, Label: "PROC"}.Words())
}

func TestNothingDrawsAUnitLine(t *testing.T) {
	/*
		The line under every value, taken from the readings package and
		editable nowhere. Asserted by drawing a dashboard whose label says
		nothing a unit would say, and looking for the unit's own glyphs
		below the number.

		Pixels rather than a string, because there is no string: the renderer
		draws, and what this is about is what is on the panel.
	*/
	d := Dashboard{
		Arrangement: Big,
		Theme:       "mono",
		Background:  Background{Kind: Plain},
		Headline:    Slot{Source: readings.Coolant, Label: "X"},
	}
	plain := decode(t, Render(d, reading(), 0, nil))

	// The same screen with the unit spelled into the label draws more ink,
	// which is the control: if the label were not what is drawn, this would
	// match.
	d.Headline.Label = "X °C"
	spelled := decode(t, Render(d, reading(), 0, nil))
	require.NotEqual(t, ink(plain), ink(spelled),
		"the label is not what is drawn")

	/*
		The unit line lived at y=434 in Big. Every row of it is background
		now: whatever the arrangement draws below the number, it is not a
		unit nobody asked for.
	*/
	require.Zero(t, inkInBand(plain, 424, 460),
		"something is still drawn where the unit line used to be")
}

// ink is how many pixels are not the background, which on a plain theme is
// whatever the corner is.
func ink(img *image.Paletted) int {
	background := img.Pix[0]
	n := 0
	for _, p := range img.Pix {
		if p != background {
			n++
		}
	}
	return n
}

// inkInBand is the same count, restricted to rows [top, bottom).
func inkInBand(img *image.Paletted, top, bottom int) int {
	background := img.Pix[0]
	n := 0
	for y := top; y < bottom; y++ {
		for x := range img.Bounds().Dx() {
			if img.Pix[y*img.Stride+x] != background {
				n++
			}
		}
	}
	return n
}

func TestAPairGradesOnItsFirstReading(t *testing.T) {
	/*
		One colour cannot say two things, so it says the first one and the
		author chooses the order.

		Whichever-is-worse was the other candidate and is wrong on this
		screen: "11 / 59" turning red gives no clue which half went hot, so
		the warning costs a look at the machine to interpret -- which is the
		one thing a panel read from across a room must not do.
	*/
	hot := reading()
	hot.Set(readings.Coolant, 62) // past the threshold
	hot.Set(readings.CPULoad, 4)

	first := Dashboard{
		Arrangement: Big, Background: Background{Kind: Plain},
		Headline: Slot{Source: readings.Coolant, Second: readings.CPULoad},
	}
	require.Positive(t, pixels(decode(t, Render(first, hot, 0, nil)), colCrit),
		"a hot first reading was not drawn as one")

	second := first
	second.Headline = Slot{Source: readings.CPULoad, Second: readings.Coolant}
	require.Zero(t, pixels(decode(t, Render(second, hot, 0, nil)), colCrit),
		"the colour followed the second reading")
}

func TestAValueTooWideForItsBandIsDrawnSmaller(t *testing.T) {
	/*
		`centred` draws at the size it is given and lets the string overrun
		its box -- which `column`'s comment already records as a four-digit
		pump reading colliding with its neighbour, fixed at the time by
		making the font smaller for every dashboard.

		A pair makes it certain rather than an edge case, and reserving each
		half its widest (spec 042) makes it certainer: the assembly is wider
		than the characters in it, which is why the fitting measures boxes
		and not text.
	*/
	p := &paint{}
	pump := Slot{Source: readings.PumpRPM}.Fields(reading())
	pair := Slot{Source: readings.PumpRPM, Second: readings.FanRPM}.Fields(reading())

	_, total := p.boxes(pump, 34)
	_, _, kept := p.fittedBoxes(pump, total+10, 34)
	require.Equal(t, 34.0, kept, "a value that fits was shrunk anyway")

	_, wide := p.boxes(pair, 34)
	_, fitted, size := p.fittedBoxes(pair, 200, 34)
	require.Less(t, size, 34.0, "a pair far too wide for its band was not shrunk")
	require.GreaterOrEqual(t, size, float64(minValuePt))
	require.LessOrEqual(t, fitted, 200, "the shrunk assembly still does not fit")
	require.Greater(t, wide, 200, "the band was not too small; this proves nothing")
}

func TestAValueIsNotShrunkIntoIllegibility(t *testing.T) {
	// Below the floor it overruns instead. Overrunning visibly is better
	// than being unreadable quietly, on a screen whose whole job is being
	// read from the other side of a desk.
	p := &paint{}
	pair := Slot{Source: readings.PumpRPM, Second: readings.FanRPM}.Fields(reading())

	_, _, size := p.fittedBoxes(pair, 20, 34)
	require.Equal(t, float64(minValuePt), size)
}

func TestAStackedPairStaysInsideItsRow(t *testing.T) {
	/*
		The row the unit line was taken out of. Its value band used to stop
		at 200 pixels with the unit drawn beyond it; the band is the rest of
		the row now, and the worst pair this machine can report has to sit
		inside it.
	*/
	p := &paint{}
	band := Size - 2*rowInset - rowLabelWidth

	// The worst pair this machine can report, at the widths it reserves.
	worst := reading()
	worst.Set(readings.PumpRPM, 9999)
	worst.Set(readings.FanRPM, 9999)
	fields := Slot{Source: readings.PumpRPM, Second: readings.FanRPM}.Fields(worst)

	_, total, _ := p.fittedBoxes(fields, band, rowValuePt)
	require.LessOrEqual(t, total, band)
}

func TestTheStackedRowsAreSetLikeATable(t *testing.T) {
	/*
		Centred in their bands, every row started and ended somewhere
		different: "2709 · 90" is fifty pixels wider than "12 · 63", so four
		rows were four left edges and four right ones. A table has two.

		**Asserted on the drawn pixels**, not on the arithmetic that places
		them. A test that recomputes the geometry and checks its own answer
		passes whatever the renderer does with it -- which this one did,
		until flipping the alignment back to centred failed to fail it.
	*/
	d := Dashboard{
		Arrangement: Stacked,
		Theme:       "mono",
		Background:  Background{Kind: Plain},
		Headline:    Slot{Source: readings.Coolant, Label: "COOLANT °C"},
		Slots: []Slot{
			{Source: readings.CPULoad, Second: readings.CPUTemp, Label: "CPU % · °C"},
			{Source: readings.PumpRPM, Second: readings.PumpDuty, Label: "PUMP RPM · %"},
		},
	}
	r := reading()
	r.Set(readings.CPULoad, 12)
	r.Set(readings.CPUTemp, 63)
	r.Set(readings.PumpDuty, 90)

	img := decode(t, Render(d, r, 0, nil))
	rows := fit(d, Stacked)
	require.Len(t, rows, 2)

	var lefts, rights []int
	for i := range rows {
		left, right := inkEdges(img, stackTop+i*rowHeight, 48)
		require.NotEqual(t, -1, left, "row %d drew nothing", i)
		lefts = append(lefts, left)
		rights = append(rights, right)
	}

	/*
		Within a couple of pixels, because ink is not the pen: "C" and "P"
		start at the same position and put their first dark pixel a column
		apart, and the same is true of "3" and "0" at the other end. The
		misalignment this is about is twenty-five pixels, so two is a
		tolerance rather than a loophole.
	*/
	require.InDelta(t, lefts[0], lefts[1], 2, "the words do not start together")
	require.InDelta(t, rights[0], rights[1], 2, "the numbers do not end together")

	// Against the row's own edges, so "together" cannot mean "together in the
	// wrong place".
	require.InDelta(t, rowInset, lefts[0], 4, "the words do not start at the inset")
	require.InDelta(t, Size-rowInset, rights[0], 4, "the numbers do not reach the right edge")

	// And the two rows are genuinely different widths, or this proves
	// nothing: a table of identical strings lines up however it is set.
	p := &paint{}
	require.NotEqual(t,
		p.textWidth(rows[0].Text(r), rowValuePt, true, Text{}),
		p.textWidth(rows[1].Text(r), rowValuePt, true, Text{}),
		"both rows are the same width")
}

/*
inkEdges is the first and last column of a band that is not background.

The corner pixel is the background, which on a plain theme it is everywhere
nothing was drawn. -1 for a band with nothing in it.
*/
func inkEdges(img *image.Paletted, top, height int) (left, right int) {
	background := img.Pix[0]
	left, right = -1, -1
	for y := top; y < top+height; y++ {
		for x := range img.Bounds().Dx() {
			if img.Pix[y*img.Stride+x] == background {
				continue
			}
			if left == -1 || x < left {
				left = x
			}
			if x > right {
				right = x
			}
		}
	}
	return left, right
}

func TestOneSizeForTheWholeColumn(t *testing.T) {
	/*
		Letting each row shrink its own value put 34, 34, 34, 30 and 31 point
		down one column, which reads worse than the ragged edges the
		justifying was for. The widest number decides for all of them.
	*/
	p := &paint{}
	const band = 168

	narrow := Slot{Source: readings.CPULoad, Second: readings.CPUTemp}.Fields(reading())
	wide := Slot{Source: readings.PumpRPM, Second: readings.FanRPM}.Fields(reading())

	_, _, alone := p.fittedBoxes(narrow, band, rowValuePt)
	_, _, other := p.fittedBoxes(wide, band, rowValuePt)
	together := min(alone, other)
	require.Less(t, together, alone,
		"the widest number did not bring the column down with it")

	// Which is the point: both are drawn at one size, and both fit.
	for _, fields := range [][]Field{narrow, wide} {
		_, total := p.boxes(fields, together)
		require.LessOrEqual(t, total, band)
	}
}

func TestAWideLabelPushesTheNumberRatherThanMeetingIt(t *testing.T) {
	/*
		The collision justifying created. A label wider than its band
		overflowed both ways when it was centred and nobody noticed; hard
		left it overflows one way, into the number -- "PUMP RPM · %" is 208
		pixels and left three of them.
	*/
	p := &paint{}
	long := "PUMP RPM · %"
	require.Greater(t, p.textWidth(long, 22, false, Text{}), rowLabelWidth,
		"the label this is about now fits its band; pick a longer one")

	width := max(rowLabelWidth, p.textWidth(long, 22, false, Text{}))
	left := rowInset + width + rowGap
	require.GreaterOrEqual(t, left-(rowInset+p.textWidth(long, 22, false, Text{})), rowGap,
		"the gap between the words and the numbers was eaten")
}

func TestOnlyTheStackedRowsAreJustified(t *testing.T) {
	/*
		A Ring or Grid column is a label with one value under it -- its own
		unit, with nothing beside it to line up with -- so it stays centred.
		Asserted on the alignment helper rather than on pixels, because what
		is being fixed is a decision.
	*/
	require.Equal(t, 20, offset(100, 60, Centre))
	require.Equal(t, 0, offset(100, 60, Left))
	require.Equal(t, 40, offset(100, 60, Right))

	// A string wider than its box starts at zero when left-aligned and goes
	// negative when right-aligned, which is the overrun being visible rather
	// than being silently clipped.
	require.Equal(t, 0, offset(100, 140, Left))
	require.Equal(t, -40, offset(100, 140, Right))
}
