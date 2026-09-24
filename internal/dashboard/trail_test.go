package dashboard

import (
	"image"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/readings"
)

// stacked is a dashboard with a headline the trail belongs to, on a flat
// background so that what is drawn in the band is what this drew.
func stacked(source readings.Source) Dashboard {
	d := Shipped()[1]
	d.Arrangement = Stacked
	d.Background = Background{Kind: Plain}
	d.Headline = Slot{Source: source, Label: "CPU"}
	return d
}

// inked is the rows a picture has ink on, within a band.
func inked(img *image.Paletted, top, bottom int) []int {
	background := img.Pix[0]
	var rows []int
	for y := top; y < bottom; y++ {
		for x := range img.Bounds().Dx() {
			if img.Pix[y*img.Stride+x] != background {
				rows = append(rows, y)
				break
			}
		}
	}
	return rows
}

// level is a trail that says the same thing sixty times.
func level(at float64) Trail {
	t := make(Trail, readings.Points)
	for i := range t {
		t[i] = at
	}
	return t
}

func TestTheTrailIsDrawnInTheBandAndNowhereElse(t *testing.T) {
	/*
		The stacked arrangement leaves 94 pixels between the headline's ink
		and the first row's, measured on a frame rather than derived from the
		constants that place them. The trail goes there, and the rest of the
		panel draws exactly what it drew.
	*/
	d := stacked(readings.CPULoad)
	r := reading()
	r.Set(readings.CPULoad, 50)

	was := decode(t, Render(d, r, 0, nil, nil))
	now := decode(t, Render(d, r, 0, nil, level(50)))

	// The band, up to where this dashboard's own labels begin.
	require.Empty(t, inked(was, 202, 294), "the band was not empty to begin with")

	drawn := inked(now, 202, 294)
	require.NotEmpty(t, drawn, "nothing was drawn in the band")
	require.GreaterOrEqual(t, drawn[0], trailTop-1, "the trace is above its band")
	require.LessOrEqual(t, drawn[len(drawn)-1], trailTop+trailHeight,
		"the trace is below its band")

	// And nothing moved above it or below it.
	require.Equal(t, was.Pix[:trailTop*was.Stride], now.Pix[:trailTop*now.Stride],
		"the trail changed the headline")
	bottom := (trailTop + trailHeight + 1) * was.Stride
	require.Equal(t, was.Pix[bottom:], now.Pix[bottom:], "the trail changed the rows")
}

func TestOnlyTheStackedArrangementDrawsATrail(t *testing.T) {
	// The others have a ring, or a number filling the panel. A trail they
	// were handed must draw nothing at all, byte for byte.
	for _, arrangement := range []string{Ring, Grid, Big} {
		d := stacked(readings.CPULoad)
		d.Arrangement = arrangement

		require.Equal(t,
			Render(d, reading(), 0, nil, nil).GIF,
			Render(d, reading(), 0, nil, level(50)).GIF,
			"%s drew something for a trail", arrangement)
	}
}

func TestAFlatTrailDoesNotDefeatThePushGate(t *testing.T) {
	/*
		The gate compares what the frame *says*, and a panel written to for a
		picture nobody could tell apart costs the device a settling period
		for nothing.

		A trail that advances while the reading holds still is the same
		picture shifted along a line of equal points, which is the same
		picture. A trail whose values move is not, and must not be.
	*/
	d := stacked(readings.CPULoad)
	r := reading()
	r.Set(readings.CPULoad, 50)

	held := Render(d, r, 0, nil, level(50)).Content
	require.Equal(t, held, Render(d, r, 0, nil, advance(level(50), 50)).Content,
		"a reading that did not move drew a different picture")
	require.NotEqual(t, held, Render(d, r, 0, nil, advance(level(50), 80)).Content,
		"a reading that moved drew the same picture")
}

// advance is the trail one bucket later: the oldest point falls off the left
// and a new one arrives at the right, which is what every five seconds does
// to it.
func advance(t Trail, next float64) Trail {
	out := make(Trail, 0, len(t))
	return append(append(out, t[1:]...), next)
}

func TestAShareIsDrawnAgainstTheWholeOfIt(t *testing.T) {
	/*
		Per cent has a scale everybody knows. A processor idling between 2
		and 4 must look low rather than filling the band, which is what
		drawing it against its own range would do.
	*/
	lo, hi := domain(level(3), readings.CPULoad)
	require.Equal(t, 0.0, lo)
	require.Equal(t, 100.0, hi)

	// Half way up the band, for a reading half way up its scale.
	d := stacked(readings.CPULoad)
	rows := inked(decode(t, Render(d, reading(), 0, nil, level(50))), 202, 294)
	require.NotEmpty(t, rows)
	require.InDelta(t, trailTop+trailHeight/2, rows[0], 3,
		"50%% was not drawn half way up the band")
}

func TestAReadingThatDidNotMoveIsNotDrawnAsThoughItDid(t *testing.T) {
	/*
		A coolant temperature lives in a few degrees, so its own range is a
		fraction of a degree and a trace against it is noise at full height.
		The minimum span is a tenth of the highest value, which needs no
		table of what each source does.
	*/
	steady := Trail{39.0, 39.1, 39.0, 39.1}
	lo, hi := domain(steady, readings.Coolant)
	require.InDelta(t, 3.9, hi-lo, 0.01, "the span is not the minimum")
	require.Less(t, lo, 39.0)
	require.Greater(t, hi, 39.1)

	// And a reading that did move is drawn against what it did.
	lo, hi = domain(Trail{1000, 3000}, readings.PumpRPM)
	require.Equal(t, 1000.0, lo)
	require.Equal(t, 3000.0, hi)
}

func TestAGapIsDrawnAsAGap(t *testing.T) {
	/*
		A bucket the machine had nothing to put in is not a zero. The trace
		stops, and starts again where the readings do.
	*/
	gapped := level(50)
	for i := 20; i < 40; i++ {
		gapped[i] = math.NaN()
	}

	img := decode(t, Render(stacked(readings.CPULoad), reading(), 0, nil, gapped))
	background := img.Pix[0]

	// The middle third of the band's width is empty, and the sides are not.
	column := func(x int) bool {
		for y := trailTop - 1; y <= trailTop+trailHeight; y++ {
			if img.Pix[y*img.Stride+x] != background {
				return true
			}
		}
		return false
	}
	require.True(t, column(rowInset+8), "the trace did not start")
	require.False(t, column(Size/2), "the gap was drawn through")
	require.True(t, column(Size-rowInset-8), "the trace did not resume")
}

func TestATrailTooShortToBeALineDrawsNothing(t *testing.T) {
	// One point is not a trace, and a frame that drew a dot for it would be
	// a panel saying something it does not know.
	d := stacked(readings.CPULoad)
	was := Render(d, reading(), 0, nil, nil).GIF

	require.Equal(t, was, Render(d, reading(), 0, nil, Trail{50}).GIF)
	require.Equal(t, was, Render(d, reading(), 0, nil, Trail{}).GIF)
	require.NotEqual(t, was, Render(d, reading(), 0, nil, Trail{50, 80}).GIF,
		"two points are a line and drew nothing")
}

func TestAPartialTrailIsDrawnAgainstTheRightHandEdge(t *testing.T) {
	/*
		A service that started a minute ago has a minute of history. It draws
		what it has, ending where a full trail ends, and grows leftwards as
		the rest arrives -- rather than stretching twelve points across five
		minutes it did not watch.
	*/
	short := make(Trail, 12)
	for i := range short {
		short[i] = 50
	}

	img := decode(t, Render(stacked(readings.CPULoad), reading(), 0, nil, short))
	background := img.Pix[0]
	column := func(x int) bool {
		for y := trailTop - 1; y <= trailTop+trailHeight; y++ {
			if img.Pix[y*img.Stride+x] != background {
				return true
			}
		}
		return false
	}
	require.False(t, column(rowInset+8), "a twelve-point trail reached the left edge")
	require.True(t, column(Size-rowInset-8), "the trail does not end at the right edge")
}
