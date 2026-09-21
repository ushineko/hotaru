package dashboard

import (
	"image/color"
	"math"

	"github.com/ushineko/hotaru/internal/readings"
)

/*
The rings around the headline.

One arc was spec 013's, carrying the coolant's severity so the panel could be
read across a room without reading the number. More than one is worth having
for the same reason: a second arc says what the machine is *doing* while the
first says how hot it is, and neither costs a glance.

They are drawn outermost first and progressively thinner. That is what keeps
four of them apart at arm's length -- the eye reads thinning as depth, where
four arcs of one width are four things to tell apart by radius.
*/

// MostRings is how many the panel has room for before the innermost would
// reach the headline number.
const MostRings = 4

const (
	// ringGap is the space between one ring and the next.
	ringGap = 5
	// ringThinning is how much of its width each ring keeps from the one
	// outside it.
	ringThinning = 0.72
	// thinnestRing is where the thinning stops: below this an arc reads as a
	// scratch rather than a gauge.
	thinnestRing = 5.0
)

/*
ringsOf is the arcs this dashboard draws.

Naming none means none. The alternative -- an empty list meaning "one, like it
used to" -- makes a dashboard with no rings impossible to write down, and the
shipped `coolant` says `[coolant]` in as many words instead.
*/
func ringsOf(d Dashboard) []readings.Source {
	room := Rings(d.Arrangement)
	if len(d.Rings) > room {
		return d.Rings[:room]
	}
	return d.Rings
}

// drawRings puts each arc round the centre, outermost first.
func drawRings(p *paint, d Dashboard, r Reading) {
	centre := float64(Size) / 2
	radius := float64(Size-2*margin) / 2
	width := float64(ringWidth)

	for i, source := range ringsOf(d) {
		value, known := r.Value(source)
		arc(p.img, centre, centre, radius, width, 0, 2*math.Pi, p.theme.Edge)
		if known {
			arc(p.img, centre, centre, radius, width,
				math.Pi/2, -2*math.Pi*fraction(source, value), ringColour(source, value, known, p.theme, i))
		}

		radius -= width/2 + ringGap + math.Max(thinnestRing, width*ringThinning)/2
		width = math.Max(thinnestRing, width*ringThinning)
	}
}

/*
ringColour is what one arc is drawn in.

A reading that carries a grade keeps it, because the ring exists to say
"something is wrong" without being read. The rest are the theme's accent,
dimmed towards the muted colour as they go inwards: four arcs in one colour
are one arc with stripes.
*/
func ringColour(source readings.Source, value float64, known bool, theme Theme, depth int) color.RGBA {
	graded := gradeOf(source, value, known, theme)
	if graded != theme.Accent || depth == 0 {
		return graded
	}
	f := math.Min(1, float64(depth)*0.28)
	return color.RGBA{
		R: blend(graded.R, theme.Muted.R, f),
		G: blend(graded.G, theme.Muted.G, f),
		B: blend(graded.B, theme.Muted.B, f),
		A: 255,
	}
}

func blend(from, to uint8, f float64) uint8 {
	return uint8(float64(from)*(1-f) + float64(to)*f)
}
