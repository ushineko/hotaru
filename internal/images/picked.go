package images

import (
	"image"
	"image/color"
	"math"
	"sort"
)

/*
Palette is a picture's most prominent distinct colours, at most n of them,
the one covering most of the picture first (spec 061 R5).

What a scene made from a picture gives an effect that takes colours of its
own. The lights are already the picture, sampled across it; an effect such as
a Breathing between two colours wants the picture's two colours, not the ones
at either end of a keyboard.

**Prominent is colourful and large.** Each pixel counts by how much colour it
has, as mean does, so a planet on black space is the planet's colours and not
a dim white for the space around it. A picture with no colour in it at all
counts every pixel the same, and gives its greys.

**Distinct is distance.** Colours are lifted to what a light shows first, then
gathered: a colour nearer than distinctFor(distance) to one already found is
counted as that one. The slider that pushes a scene's lights apart therefore
also decides how far apart two picked colours have to be. At 1 a red and an
orange are two colours; at 3 they are one.

Fewer than n is the answer for a picture with fewer distinct colours. The
caller decides what fills the rest; a firmware mode repeats the last.

Deterministic: the same picture and settings give the same colours, which is
what lets the window show them before a scene is made and the service pick
the same ones when it is.
*/
func Palette(picture image.Image, n int, distance float64) []color.NRGBA {
	if n <= 0 {
		return nil
	}
	cells := tallied(picture)
	if len(cells) == 0 {
		return nil
	}

	type found struct {
		colour color.NRGBA
		weight float64
	}
	var groups []found
	apart := distinctFor(distance)
	for _, cell := range cells {
		c := Lit(cell.mean(), Brightness)
		joined := false
		for i := range groups {
			if gap(groups[i].colour, c) < apart {
				groups[i].weight += cell.weight
				joined = true
				break
			}
		}
		if !joined {
			groups = append(groups, found{colour: c, weight: cell.weight})
		}
	}
	sort.SliceStable(groups, func(i, j int) bool { return groups[i].weight > groups[j].weight })

	out := make([]color.NRGBA, 0, min(n, len(groups)))
	for _, g := range groups[:min(n, len(groups))] {
		out = append(out, g.colour)
	}
	return out
}

/*
distinctFor is how far apart two colours have to be to count as two, as a
straight line in RGB out of the 441 between black and white.

64 at a distance of 1: about where two lights side by side stop reading as
one colour across a room, chosen by eye rather than measured. It grows with
the distance, so pushing a scene's colours apart also asks for colours that
are further apart to begin with.
*/
func distinctFor(distance float64) float64 {
	return 64 * math.Max(1, distance)
}

// gap is the straight-line distance between two colours in RGB.
func gap(a, b color.NRGBA) float64 {
	dr, dg, db := float64(a.R)-float64(b.R), float64(a.G)-float64(b.G), float64(a.B)-float64(b.B)
	return math.Sqrt(dr*dr + dg*dg + db*db)
}

// cell is one bin of a picture's colours: what fell in it, and how much.
type cell struct {
	key           uint32
	r, g, b       float64
	count, weight float64
}

func (c cell) mean() color.NRGBA {
	return color.NRGBA{R: uint8(c.r / c.count), G: uint8(c.g / c.count), B: uint8(c.b / c.count), A: 255}
}

// pickedBins is the histogram's coarseness: 16 levels a channel. Finer bins
// are merged by distance afterwards anyway; these only keep the count small.
const pickedBins = 16

// pickedSamples is about how many pixels are read. A panel-sized picture is
// 409,600 pixels, and the colours of a 64-by-64 grid of them are the same
// colours.
const pickedSamples = 4096

/*
tallied is the picture's colours in bins, heaviest first.

Ties are broken by the bin itself, so two runs over one picture agree.
*/
func tallied(picture image.Image) []cell {
	bounds := picture.Bounds()
	if bounds.Empty() {
		return nil
	}
	stride := max(1, int(math.Ceil(math.Sqrt(float64(bounds.Dx()*bounds.Dy())/pickedSamples))))
	bins := map[uint32]*cell{}
	var colourful float64

	for y := bounds.Min.Y; y < bounds.Max.Y; y += stride {
		for x := bounds.Min.X; x < bounds.Max.X; x += stride {
			r, g, b, _ := picture.At(x, y).RGBA()
			fr, fg, fb := float64(r>>8), float64(g>>8), float64(b>>8)
			key := (r>>8/pickedBins)<<16 | (g>>8/pickedBins)<<8 | (b >> 8 / pickedBins)
			bin, ok := bins[key]
			if !ok {
				bin = &cell{key: key}
				bins[key] = bin
			}
			chroma := (math.Max(fr, math.Max(fg, fb)) - math.Min(fr, math.Min(fg, fb))) / 255
			bin.r, bin.g, bin.b = bin.r+fr, bin.g+fg, bin.b+fb
			bin.count++
			bin.weight += chroma
			colourful += chroma
		}
	}

	out := make([]cell, 0, len(bins))
	for _, bin := range bins {
		if colourful == 0 {
			bin.weight = bin.count // no colour anywhere: every pixel the same
		}
		if bin.weight > 0 {
			out = append(out, *bin)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].weight != out[j].weight {
			return out[i].weight > out[j].weight
		}
		return out[i].key < out[j].key
	})
	return out
}
