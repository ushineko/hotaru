package dashboard

import (
	"image"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/readings"
	"golang.org/x/image/font"
)

/*
The digits are tabular, in every face this package offers.

The whole fix rests on it: if two digits were different widths, a value would
move as the *digits* changed and not only as their number did, and no amount
of reserving room would hold it still. A font that lost the property would
bring the wobble back somewhere nobody would think to look, so it is measured
rather than believed.
*/
func TestEveryDigitIsTheSameWidth(t *testing.T) {
	for _, family := range append([]string{""}, Fonts()...) {
		for _, bold := range []bool{true, false} {
			drawing.Lock()
			f := face(118, bold, family)
			want := font.MeasureString(f, "0").Round()
			for _, digit := range "123456789" {
				got := font.MeasureString(f, string(digit)).Round()
				require.Equal(t, want, got,
					"%q is not the width of 0 in %q bold=%v", digit, family, bold)
			}
			drawing.Unlock()
			require.Positive(t, want)
		}
	}
}

/*
window is the patch of the panel a value is drawn in.

A rectangle rather than a band of rows, because the arrangements that draw a
ring draw it *through* the rows a value sits in -- and a ring is a gauge, so
it changes with the reading. Sampling a whole row would be measuring the
instrument that is supposed to move.
*/
type window struct{ top, height, left, right int }

/*
inkColumns is which columns have something drawn in them inside a window.

The set rather than its first and last member: a value that shifted two pixels
is a different set, and so is one that grew a digit. What this proves is that
neither happens.
*/
func inkColumns(img *image.Paletted, at window) []int {
	background := img.Pix[0]
	var out []int
	for x := at.left; x < at.right; x++ {
		for y := at.top; y < at.top+at.height; y++ {
			if img.Pix[y*img.Stride+x] != background {
				out = append(out, x)
				break
			}
		}
	}
	return out
}

/*
edges is the first and last column a value is drawn in.

The last one is the anchor. A number that grows a digit *must* put ink
somewhere it did not before -- that is the digit -- so what "does not move" can
mean is that the digits already there stay where they are, which for a
right-aligned field means its right edge never shifts.
*/
func edges(drawn []int) (first, last int) {
	if len(drawn) == 0 {
		return -1, -1
	}
	return drawn[0], drawn[len(drawn)-1]
}

/*
steady renders a dashboard at a series of readings and returns the columns the
value occupied each time.

The ring is pinned to a reading that does not change, so anything moving in
the window is the number.
*/
func steady(t *testing.T, d Dashboard, at window, source readings.Source,
	values ...float64,
) [][]int {
	t.Helper()
	var out [][]int
	for _, v := range values {
		r := reading()
		r.Set(source, v)
		drawn := inkColumns(decode(t, Render(d, r, 0, nil, nil)), at)
		require.NotEmpty(t, drawn, "nothing was drawn at %v", v)
		out = append(out, drawn)
	}
	return out
}

/*
anchored is the assertion this spec exists for.

**The readings all end in the same digit**, which is the only way to say this
in pixels. Tabular means every digit has the same *advance*, not the same ink:
"1" is a narrow mark inside its box and "9" nearly fills one, so two readings
of the same width can differ in which columns they touch while occupying
exactly the same space. Sharing a last digit removes that -- whatever moves
the right edge now is the layout, which is the thing under test.

Growing a digit must put ink somewhere it did not before; that is the digit.
What must not happen is the rest of the number moving to make room for it.
*/
func anchored(t *testing.T, at [][]int, what string) {
	t.Helper()
	require.NotEmpty(t, at)

	_, want := edges(at[0])
	for i, got := range at {
		_, last := edges(got)
		require.Equal(t, want, last, "%s moved its right edge (reading %d)", what, i)
	}
}

/*
hinged is anchored for a field that grows the other way.

The second half of a centred pair hugs the separator and grows outward, so its
*left* edge is the fixed one and its right edge is where the new digit goes.
Asserting the right edge there would be asserting that a number cannot get
longer.
*/
func hinged(t *testing.T, at [][]int, what string) {
	t.Helper()
	require.NotEmpty(t, at)

	want, _ := edges(at[0])
	for i, got := range at {
		first, _ := edges(got)
		require.Equal(t, want, first, "%s moved its left edge (reading %d)", what, i)
	}
}

// plain is a dashboard with nothing in it that moves but the readings: a flat
// background, and a ring pinned to a constant.
func plain(arrangement string, headline Slot, slots ...Slot) Dashboard {
	return Dashboard{
		Arrangement: arrangement, Theme: "mono",
		Background: Background{Kind: Plain},
		Rings:      []readings.Source{readings.Coolant},
		Headline:   headline, Slots: slots,
	}
}

/*
headlineWindow is where the Stacked arrangement draws its number.

**Stacked, because it is the one headline with no ring around it.** A ring is
a gauge: it moves with the reading, and it is drawn through the rows the
number sits in, so watching a Ring or Big headline would be watching the
instrument that is supposed to move. The headline code is the same function
either way.
*/
var headlineWindow = window{top: 98, height: 116, left: headlineInset, right: Size - headlineInset}

func TestTheBigNumberDoesNotMoveAsItChanges(t *testing.T) {
	/*
		One centred string moves every character when the character count
		changes: 9 to 10, 99 to 100. The panel redraws every couple of
		seconds, and what that reads as is the number wobbling rather than
		updating.

		Asserted across a digit boundary in both directions, because one
		digit either side of ten is the case that used to move furthest.
	*/
	d := plain(Stacked, Slot{Source: readings.CPULoad, Label: "CPU"})

	anchored(t, steady(t, d, headlineWindow, readings.CPULoad, digitBoundary...),
		"the headline")
}

/*
digitBoundary is one, two and three digits, all ending in the same one.

119% of a processor is not a reading anybody will see. What it is, is three
characters where there were two, which is the whole of what used to move the
number -- and ending in 9 each time means the right edge is comparable.
*/
var digitBoundary = []float64{9, 19, 119}

func TestAPairHoldsBothHalvesStill(t *testing.T) {
	/*
		The harder case. A pair drawn as one string moves its *right* half
		when its left half changes width, which is the half nobody touched.
	*/
	d := plain(Stacked, Slot{
		Source: readings.CPULoad, Second: readings.CPUTemp, Label: "CPU % · °C",
	})

	anchored(t, steady(t, d, headlineWindow, readings.CPULoad, digitBoundary...),
		"a pair whose first half changed width")

	/*
		And the half nobody touched is drawn in exactly the same columns.

		This is the failure that made the wobble worth fixing: as one string,
		a pair moves its *right* half when its left half gains a digit, so a
		stable reading appeared to twitch because the one beside it changed.
	*/
	right := window{top: headlineWindow.top, height: headlineWindow.height,
		left: Size / 2, right: headlineWindow.right}
	untouched := steady(t, d, right, readings.CPULoad, digitBoundary...)
	for i, got := range untouched {
		require.Equal(t, untouched[0], got,
			"the second half moved when the first one changed (reading %d)", i)
	}

	// And the other way round: the left half must not move when the right
	// one changes.
	d.Headline = Slot{Source: readings.CPUTemp, Second: readings.CPULoad, Label: "CPU"}
	/*
		Growing the second half: it hugs the separator, so its left edge is
		the anchor and the digits appear on the outside.
	*/
	hinged(t, steady(t, d, right, readings.CPULoad, digitBoundary...),
		"a pair whose second half changed width")

	// And the half nobody touched -- the first one -- does not move at all.
	left := window{top: headlineWindow.top, height: headlineWindow.height,
		left: headlineWindow.left, right: Size / 2}
	still := steady(t, d, left, readings.CPULoad, digitBoundary...)
	for i, got := range still {
		require.Equal(t, still[0], got,
			"the first half moved when the second one changed (reading %d)", i)
	}
}

func TestEveryArrangementHoldsStill(t *testing.T) {
	// The columns and the rows, not only the big one.
	paired := Slot{Source: readings.CPULoad, Second: readings.CPUTemp, Label: "CPU"}
	head := Slot{Source: readings.Coolant, Label: "C"}
	ringColumn := (Size - 2*metricInset) / metricSlots
	gridColumn := (Size - 2*gridInset) / 2

	for _, one := range []struct {
		name string
		d    Dashboard
		at   window
	}{
		// The first of the three along the bottom, inside the inset the ring
		// already curves into.
		{"a ring column", plain(Ring, head, paired), window{
			top: 446, height: 66, left: metricInset, right: metricInset + ringColumn,
		}},
		{"a stacked row", plain(Stacked, head, paired), window{
			top: stackTop, height: 48, left: rowInset + rowLabelFloor, right: Size - rowInset,
		}},
		{"a grid cell", plain(Grid, head, paired), window{
			top: gridTop + 30, height: 66, left: gridInset, right: gridInset + gridColumn,
		}},
	} {
		t.Run(one.name, func(t *testing.T) {
			anchored(t, steady(t, one.d, one.at, readings.CPULoad, digitBoundary...), one.name)
		})
	}
}

func TestAReadingWiderThanItsFieldIsDrawnWhole(t *testing.T) {
	/*
		A field is a minimum. A coolant reserves four characters because
		"37.5" is what one says; a machine reading 100.0 gets the width it
		needs and shifts once, rather than losing a digit to a box that was
		sized for the ordinary case.
	*/
	p := &paint{}
	ordinary := Slot{Source: readings.Coolant}.Fields(reading(), false)
	require.Equal(t, 4, ordinary[0].Chars)

	hot := reading()
	hot.Set(readings.Coolant, 100)
	wide := Slot{Source: readings.Coolant}.Fields(hot, false)
	require.Equal(t, "100.0", wide[0].Text)

	_, narrow := p.boxes(ordinary, 118)
	_, broad := p.boxes(wide, 118)
	require.Greater(t, broad, narrow, "the wider reading was squeezed into the field")
	require.GreaterOrEqual(t, broad, p.textWidth("100.0", 118, true, Text{}),
		"a reading wider than its field lost room")
}

func TestEverySourceReservesAWidth(t *testing.T) {
	// A source with no width would be a value drawn to whatever it happens
	// to say, which is the bug.
	for _, source := range readings.All {
		require.Positive(t, readings.Width(source), "%s reserves nothing", source)

		// And it is wide enough for that source's own formatting of a
		// plausible reading.
		var r readings.Reading
		r.Set(source, 99)
		require.GreaterOrEqual(t, readings.Width(source), len(r.Text(source)),
			"%s reserves less than it draws", source)
	}
}

func TestTheBoxesDoNotChangeWithTheReading(t *testing.T) {
	/*
		The rule underneath the pixels: a field is as wide as the characters
		it reserved, so neither it nor the assembly around it depends on what
		the machine happened to say.

		Asserted here as well as on the drawn panel because the panel can only
		be asked about one glyph's right edge at a time, and this is the
		general statement -- every box, every total, every reading.
	*/
	p := &paint{}
	slot := Slot{Source: readings.CPULoad, Second: readings.CPUTemp}

	var widths [][]int
	var totals []int
	for _, v := range []float64{0, 9, 10, 99, 100, 119} {
		r := reading()
		r.Set(readings.CPULoad, v)
		r.Set(readings.CPUTemp, v)
		w, total := p.boxes(slot.Fields(r, false), 118)
		widths = append(widths, w)
		totals = append(totals, total)
	}

	for i := range widths {
		require.Equal(t, widths[0], widths[i], "a field changed width at reading %d", i)
		require.Equal(t, totals[0], totals[i], "the assembly changed width at reading %d", i)
	}

	// And the reservation is doing the work: the text alone is narrower than
	// the boxes it is drawn in, or there was nothing to hold still.
	require.Greater(t, totals[0], p.textWidth(slot.Text(reading()), 118, true, Text{}),
		"the boxes are no wider than the text, so nothing was reserved")
}
