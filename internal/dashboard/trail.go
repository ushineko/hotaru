package dashboard

import (
	"image/color"
	"math"

	"github.com/ushineko/hotaru/internal/readings"
)

/*
Series is where a reading has been, oldest first, as an arrangement draws it
in its band.

NaN is a bucket the machine had nothing to put in, and it draws as a gap. A
series shorter than its window is drawn against the right-hand edge, so a
service that started a minute ago shows a minute of trace and grows leftwards
as the rest arrives.

Nil draws nothing.
*/
type Series []float64

/*
Trails are the two halves of the band: one growing from the bottom, and one
hanging from the top.

The one below takes the whole band when there is nothing above it, which is
what a dashboard that has said nothing about its trail draws.
*/
type Trails struct{ Below, Above Series }

/*
bandOf is where an arrangement draws its trail.

**Measured on rendered frames** rather than derived from the constants that
place the numbers. Each arrangement was drawn with every reading it has room
for, and these are the rows with no ink in the middle of the panel:

	stacked   202..295   the headline to the first row
	ring      309..421   inside the arc, under the headline
	grid      218..297   the headline to the first cell
	big       400..512   under the number

The trace takes the middle of each and leaves air on both sides, because a
line that reaches either neighbour reads as part of it.
*/
func bandOf(arrangement string) (top, height int) {
	switch arrangement {
	case Ring:
		return 325, 80
	case Grid:
		return 230, 56
	case Big:
		return 416, 80
	default:
		return 216, 64
	}
}

/*
drawTrails draws the band: one trace growing from the bottom, and one hanging
from the top when the dashboard asked for a second.

**The band is split only when there are two.** One trace takes the whole of
it, which is what a dashboard that has said nothing about its trail draws, and
what the panel looked like before there was anything to say.

The same reading twice is two identical shapes in one band, so the second is
dropped rather than drawn. It would cost the first half its height and say
nothing the first did not.
*/
func (p *paint) drawTrails(d Dashboard, r Reading, t Trails) {
	below, above := d.Trail.Sources(d.Headline.Source)
	if above == below {
		above, t.Above = "", nil
	}

	top, height := bandOf(d.Arrangement)
	if len(t.Above) >= 2 && above != "" {
		// Half each, with the gap between them taken out of both.
		half := (height - trailGap) / 2
		p.drawTrail(t.Above, above, p.grade(above, r), top, half, true)
		p.drawTrail(t.Below, below, p.grade(below, r), top+height-half, half, false)
		return
	}
	p.drawTrail(t.Below, below, p.grade(below, r), top, height, false)
}

// trailGap keeps the two traces apart, so a busy machine does not draw them
// into each other and leave a solid band.
const trailGap = 8

// grade is the colour a reading is drawn in: the one its own value earns,
// which is the colour of the number it is a history of.
func (p *paint) grade(source readings.Source, r Reading) color.Color {
	value, known := r.Value(source)
	return gradeOf(source, value, known, p.theme)
}

/*
drawTrail draws one trace and the area under it.

**Dithered rather than blended.** The panel takes a paletted GIF, and a colour
mixed with whatever is behind it lands on whichever palette entry is nearest
-- which over a photograph is one of the picture's own colours and over a
plain theme is a step of the starfield's ramp. A checkerboard of a colour the
palette already holds reads as a wash at arm's length and needs no entry of
its own.

The line itself is solid, and carries the dark edge the text carries when the
background is a photograph, for the same reason: a bright line over a bright
pixel is a line somebody cannot follow.

`up` is false for a trace that grows from the bottom of its band and true for
one that hangs from the top. Which way it hangs is what tells two readings
apart, on a panel where colour already means something else.
*/
func (p *paint) drawTrail(points Series, source readings.Source, c color.Color,
	top, height int, up bool,
) {
	if len(points) < 2 || source == "" {
		return
	}
	lo, hi := domain(points, source)
	if hi <= lo {
		return
	}

	left, right := rowInset, Size-rowInset-1
	edge := top + height
	if up {
		edge = top
	}

	at := func(i int) (int, int, bool) {
		// Right-aligned: a series shorter than its window ends where a full
		// one ends, and starts further in.
		slot := readings.Points - len(points) + i
		x := left + slot*(right-left)/(readings.Points-1)
		v := points[i]
		if math.IsNaN(v) {
			return x, 0, false
		}
		grown := int(math.Round((v - lo) / (hi - lo) * float64(height)))
		if up {
			return x, top + grown, true
		}
		return x, top + height - grown, true
	}

	for i := range len(points) - 1 {
		x0, y0, ok := at(i)
		x1, y1, next := at(i + 1)
		if !ok || !next {
			continue
		}
		p.segment(x0, y0, x1, y1, edge, c)
	}
}

/*
segment draws one step of the trace, and the area between it and the band's
edge.

A column at a time, so the fill and the line are the same shape and a steep
step has no gap in it: interpolating y per column is what a line drawn as
points between two ends does not do.

`edge` is the side the fill runs to, which is the bottom of the band for a
trace that grows and the top for one that hangs.
*/
func (p *paint) segment(x0, y0, x1, y1, edge int, c color.Color) {
	if x1 <= x0 {
		return
	}
	for x := x0; x <= x1; x++ {
		y := y0 + (y1-y0)*(x-x0)/(x1-x0)

		// The wash. Half the pixels, in a fixed pattern, so the same reading
		// draws the same picture and the push gate still means something.
		from, to := y+trailLine, edge
		if edge < y {
			from, to = edge, y
		}
		for fill := from; fill < to; fill++ {
			if (x+fill)%2 == 0 {
				p.img.Set(x, fill, c)
			}
		}

		if p.picture {
			p.img.Set(x, y-1, colOutline)
			p.img.Set(x, y+trailLine, colOutline)
		}
		for t := range trailLine {
			p.img.Set(x, y+t, c)
		}
	}
}

// trailLine is how thick a trace is. Two pixels rather than one: a hairline
// on a 640-pixel panel seen across a desk is a smudge.
const trailLine = 2

/*
domain is the range a trace is drawn against.

**A share is drawn against the whole of it.** Per cent has a scale everybody
already knows, and a processor idling at 3% should look low rather than
filling the band because it spent five minutes between 2 and 4.

Everything else is drawn against its own lowest and highest, because a coolant
temperature lives in a few degrees and a pump in a few thousand revolutions.
The minimum span stops a reading that did not move from being drawn as though
it did.
*/
func domain(points Series, source readings.Source) (lo, hi float64) {
	_, unit := readings.Describe(source)

	lo, hi = math.Inf(1), math.Inf(-1)
	for _, v := range points {
		if math.IsNaN(v) {
			continue
		}
		lo, hi = math.Min(lo, v), math.Max(hi, v)
	}
	if math.IsInf(lo, 1) {
		return 0, 0
	}
	if unit == "%" {
		return 0, 100
	}

	/*
		A reading that did not move has no range, so the band invents one --
		**upwards from the lowest value it saw**, rather than around it.

		Centred was the first answer and it draws a coolant sitting at 41.2
		as a line across the middle of the band, with the wash filling half
		of it: a quiet machine drew a solid bar on every screen. Anchored at
		the bottom, a steady reading is a thin line along the floor of the
		band and any movement lifts it off. Nothing happening looks like
		nothing happening.

		A tenth of the highest value, which is scale-free and needs no table
		of what each source does.
	*/
	if span := math.Abs(hi) / 10; hi-lo < span {
		hi = lo + span
	}
	if hi <= lo {
		return lo, lo + 1
	}
	return lo, hi
}
