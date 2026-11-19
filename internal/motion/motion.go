/*
Package motion draws a dashboard's change from one reading to the next on a
panel that takes motion frames (spec 081): the text that changed melts,
morphs or rolls into its new value, and the rings sweep or swing like a comet.

It works on layers (dashboard.Layers): transparent RGBA images, premultiplied,
640 pixels square. Only the regions of text that changed are touched, and the
work that a change needs once is done once, by Prepare, on those regions
alone.

The effects were chosen on the panel, with the maintainer watching, from a
prototype (spec 081, "What the prototype showed").
*/
package motion

import (
	"image"
	"math"
	"time"
)

// The text transitions.
const (
	Melt  = "melt"
	Morph = "morph"
	Roll  = "roll"
)

// Duration is how long a text transition takes: 700 ms to melt or morph,
// 500 ms to roll, as tried on the panel. A ring alone sweeps in 500 ms.
func Duration(kind string) time.Duration {
	switch kind {
	case Melt, Morph:
		return 700 * time.Millisecond
	}
	return 500 * time.Millisecond
}

// Ease is smooth in and out, 0 at 0 and 1 at 1.
func Ease(t float64) float64 {
	t = clamp(t)
	return t * t * (3 - 2*t)
}

// Spring overshoots and settles: the comet's sweep. It ends at 1.
func Spring(t float64) float64 {
	t = clamp(t)
	if t >= 1 {
		return 1
	}
	return 1 - math.Exp(-5*t)*math.Cos(9*t)
}

func clamp(t float64) float64 { return min(1, max(0, t)) }

/*
Change is a text transition prepared once: the old and new text layers, the
regions where they differ, and what each kind needs measured in them.
*/
type Change struct {
	kind    string
	old     *image.RGBA
	new     *image.RGBA
	regions []image.Rectangle
	shapes  []pair
}

// pair is one region of the old and new text, measured.
type pair struct{ old, new shape }

/*
Prepare measures a transition from old to new text. A kind this package does
not know is a jump: At gives the new text at once.
*/
func Prepare(kind string, old, new *image.RGBA) *Change {
	c := &Change{kind: kind, old: old, new: new, regions: Regions(old, new)}
	if kind == Melt || kind == Morph {
		c.shapes = make([]pair, len(c.regions))
		for i, r := range c.regions {
			c.shapes[i] = pair{measure(old, r, kind == Morph), measure(new, r, kind == Morph)}
		}
	}
	return c
}

// Regions are where the transition happens.
func (c *Change) Regions() []image.Rectangle { return c.regions }

/*
At is the text layer t of the way through: the old text at 0, the new at 1,
and outside the changed regions the new text throughout.
*/
func (c *Change) At(t float64) *image.RGBA {
	switch {
	case t >= 1 || len(c.regions) == 0:
		return c.new
	case t <= 0:
		return c.old
	}
	out := image.NewRGBA(c.new.Bounds())
	copy(out.Pix, c.new.Pix)
	for i, r := range c.regions {
		switch c.kind {
		case Melt:
			melt(out, r, c.shapes[i], t)
		case Morph:
			morph(out, r, c.shapes[i], t)
		case Roll:
			roll(out, c.old, c.new, r, t)
		default:
			return c.new
		}
	}
	return out
}

/*
Regions are where two text layers differ: every differing pixel, grown by a
few pixels so a glyph's whole shape is inside, in connected pieces.

A changed number is one piece; a label beside it, unchanged and more than the
growth away, is outside every piece, and stays as it is.
*/
func Regions(a, b *image.RGBA) []image.Rectangle {
	bounds := a.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	diff := make([]bool, w*h)
	changed := false
	for i := range diff {
		for c := range 4 {
			if d := int(a.Pix[i*4+c]) - int(b.Pix[i*4+c]); d > 24 || d < -24 {
				diff[i], changed = true, true
				break
			}
		}
	}
	if !changed {
		return nil
	}
	grown := dilate(diff, w, h, grow)

	seen := make([]bool, w*h)
	var out []image.Rectangle
	stack := make([]int, 0, 1024)
	for i, on := range grown {
		if !on || seen[i] {
			continue
		}
		r := image.Rect(i%w, i/w, i%w+1, i/w+1)
		seen[i] = true
		stack = append(stack[:0], i)
		for len(stack) > 0 {
			j := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			x, y := j%w, j/w
			r = r.Union(image.Rect(x, y, x+1, y+1))
			if x > 0 && grown[j-1] && !seen[j-1] {
				seen[j-1] = true
				stack = append(stack, j-1)
			}
			if x < w-1 && grown[j+1] && !seen[j+1] {
				seen[j+1] = true
				stack = append(stack, j+1)
			}
			if y > 0 && grown[j-w] && !seen[j-w] {
				seen[j-w] = true
				stack = append(stack, j-w)
			}
			if y < h-1 && grown[j+w] && !seen[j+w] {
				seen[j+w] = true
				stack = append(stack, j+w)
			}
		}
		out = append(out, r)
	}
	return out
}

// grow is how far a region reaches past the pixels that changed.
const grow = 6

// dilate grows a mask by r in every direction: a square, rows then columns.
func dilate(m []bool, w, h, r int) []bool {
	rows := make([]bool, len(m))
	for y := range h {
		last := -r - 1
		for x := range w {
			if m[y*w+x] {
				last = x
			}
			if x-last <= r {
				rows[y*w+x] = true
			}
		}
		last = w + r + 1
		for x := w - 1; x >= 0; x-- {
			if m[y*w+x] {
				last = x
			}
			if last-x <= r {
				rows[y*w+x] = true
			}
		}
	}
	out := make([]bool, len(m))
	for x := range w {
		last := -r - 1
		for y := range h {
			if rows[y*w+x] {
				last = y
			}
			if y-last <= r {
				out[y*w+x] = true
			}
		}
		last = h + r + 1
		for y := h - 1; y >= 0; y-- {
			if rows[y*w+x] {
				last = y
			}
			if last-y <= r {
				out[y*w+x] = true
			}
		}
	}
	return out
}
