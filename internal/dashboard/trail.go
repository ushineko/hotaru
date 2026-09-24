package dashboard

import (
	"image/color"
	"math"

	"github.com/ushineko/hotaru/internal/readings"
)

/*
Trail is where a reading has been, oldest first, as the stacked arrangement
draws it under the headline.

NaN is a bucket the machine had nothing to put in, and it draws as a gap. A
trail shorter than its window is drawn against the right-hand edge, so a
service that started a minute ago shows a minute of trace and grows leftwards
as the rest arrives.

Nil draws nothing, which is what every arrangement but Stacked does with it.
*/
type Trail []float64

/*
The band the trail is drawn in.

The stacked arrangement's headline ends at y=201 and its first row begins at
y=296, measured on a rendered frame rather than derived from the constants
that place them. The trace takes the middle of that and leaves air on both
sides, because a line that reaches either neighbour reads as part of it.

The sides are the rows' own inset, so the trace starts and ends where the
table above and below it does.
*/
const (
	trailTop    = 216
	trailHeight = 64
)

/*
drawTrail draws the trace and the area under it.

**Dithered rather than blended.** The panel takes a paletted GIF, and a
colour mixed with whatever is behind it lands on whichever palette entry is
nearest -- which over a photograph is one of the picture's own colours and
over a plain theme is a step of the starfield's ramp. A checkerboard of a
colour the palette already holds reads as a wash at arm's length and needs no
entry of its own.

The line itself is solid, and carries the dark edge the text carries when the
background is a photograph, for the same reason: a bright line over a bright
pixel is a line somebody cannot follow.
*/
func (p *paint) drawTrail(points Trail, source readings.Source, c color.Color) {
	if len(points) < 2 {
		return
	}
	lo, hi := domain(points, source)
	if hi <= lo {
		return
	}

	left, right := rowInset, Size-rowInset-1
	bottom := trailTop + trailHeight

	at := func(i int) (int, int, bool) {
		// Right-aligned: a trail shorter than its window ends where a full
		// one ends, and starts further in.
		slot := readings.Points - len(points) + i
		x := left + slot*(right-left)/(readings.Points-1)
		v := points[i]
		if math.IsNaN(v) {
			return x, 0, false
		}
		at := (v - lo) / (hi - lo)
		return x, bottom - int(math.Round(at*float64(trailHeight))), true
	}

	for i := range len(points) - 1 {
		x0, y0, ok := at(i)
		x1, y1, next := at(i + 1)
		if !ok || !next {
			continue
		}
		p.segment(x0, y0, x1, y1, bottom, c)
	}
}

/*
segment draws one step of the trace, with the area under it.

A column at a time, so the fill and the line are the same shape and a steep
step has no gap in it: interpolating y per column is what a line drawn as
points between two ends does not do.
*/
func (p *paint) segment(x0, y0, x1, y1, bottom int, c color.Color) {
	if x1 <= x0 {
		return
	}
	for x := x0; x <= x1; x++ {
		y := y0 + (y1-y0)*(x-x0)/(x1-x0)

		// The wash. Half the pixels, in a fixed pattern, so the same reading
		// draws the same picture and the push gate still means something.
		for fill := y + trailLine; fill < bottom; fill++ {
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

// trailLine is how thick the trace is. Two pixels rather than one: a hairline
// on a 640-pixel panel seen across a desk is a smudge.
const trailLine = 2

/*
domain is the range the band is drawn against.

**A share is drawn against the whole of it.** Per cent has a scale everybody
already knows, and a processor idling at 3% should look low rather than
filling the band because it spent five minutes between 2 and 4.

Everything else is drawn against its own lowest and highest, because a
coolant temperature lives in a few degrees and a pump in a few thousand
revolutions. The minimum span stops a reading that did not move from being
drawn as though it did: a tenth of the highest value, which is scale-free and
needs no table of what each source does.
*/
func domain(points Trail, source readings.Source) (lo, hi float64) {
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

	if span := math.Abs(hi) / 10; hi-lo < span {
		middle := (lo + hi) / 2
		lo, hi = middle-span/2, middle+span/2
	}
	if hi <= lo {
		return lo, lo + 1
	}
	return lo, hi
}
