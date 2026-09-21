package dashboard

import (
	"image"
	"image/color"
	"image/draw"
	"sort"
)

/*
paint is the canvas a dashboard is drawn on: the panel-sized paletted image,
the theme, and whether text needs an outline.

It exists so that every drawing call in this package goes through one place
that knows about the outline. A renderer where some strings are outlined and
others are not is a renderer where the one somebody forgot is the one over the
bright part of the photograph.
*/
type paint struct {
	img     *image.Paletted
	theme   Theme
	outline bool
}

// newPaint prepares the canvas with its background already drawn.
func newPaint(d Dashboard, theme Theme, behind image.Image) *paint {
	kind := d.Background.Kind
	if kind == "" {
		kind = Starfield
	}
	if kind == Picture && behind == nil {
		// A picture somebody deleted costs the background and not the frame.
		kind = Plain
	}

	switch kind {
	case Picture:
		return &paint{img: dimmed(behind, d.Background.Darkness(), theme), theme: theme, outline: true}
	case Plain:
		img := image.NewPaletted(image.Rect(0, 0, Size, Size), palette(theme, nil))
		draw.Draw(img, img.Bounds(), image.NewUniform(theme.BG), image.Point{}, draw.Src)
		return &paint{img: img, theme: theme}
	default:
		sky := background(theme)
		img := image.NewPaletted(sky.Bounds(), sky.Palette)
		copy(img.Pix, sky.Pix)
		return &paint{img: img, theme: theme}
	}
}

/*
dimmed draws a picture darkened, on a palette that has room for both.

The picture gets what is left after the interface's own colours: a photograph
through 240 entries is indistinguishable from one through 256, and text drawn
in the nearest available colour to white is not white.
*/
func dimmed(behind image.Image, keep float64, theme Theme) *image.Paletted {
	bounds := image.Rect(0, 0, Size, Size)
	rgba := image.NewRGBA(bounds)
	draw.Draw(rgba, bounds, image.NewUniform(theme.BG), image.Point{}, draw.Src)
	draw.Draw(rgba, bounds, behind, behind.Bounds().Min, draw.Src)

	for y := range Size {
		for x := range Size {
			c := rgba.RGBAAt(x, y)
			rgba.SetRGBA(x, y, color.RGBA{
				R: clamp(float64(c.R) * keep),
				G: clamp(float64(c.G) * keep),
				B: clamp(float64(c.B) * keep),
				A: 255,
			})
		}
	}

	img := image.NewPaletted(bounds, palette(theme, fromPicture(rgba)))
	draw.Draw(img, bounds, rgba, image.Point{}, draw.Src)
	return img
}

/*
fromPicture chooses colours for the picture out of what the picture contains.

Spec 019's finding, applied here: a photograph through a fixed palette spends
half of it on colours the picture does not have and speckles everywhere it
does. Coarse buckets rather than every distinct value, because the whole point
of a short palette is that LZW has runs to compress -- see the palette
comment.
*/
func fromPicture(src *image.RGBA) color.Palette {
	const step = 16 // 16 levels per channel: enough for a dimmed photograph

	counts := map[color.RGBA]int{}
	for y := 0; y < Size; y += 2 {
		for x := 0; x < Size; x += 2 {
			c := src.RGBAAt(x, y)
			counts[color.RGBA{
				R: c.R / step * step, G: c.G / step * step, B: c.B / step * step, A: 255,
			}]++
		}
	}
	return popular(counts, pictureColours)
}

// pictureColours is how much of the palette a background may take. The rest
// is the interface's, which must be exact: text in the nearest colour to the
// theme's white is text in a colour nobody chose.
const pictureColours = 236

// popular is the n most common colours, most first.
func popular(counts map[color.RGBA]int, n int) color.Palette {
	type seen struct {
		c color.RGBA
		n int
	}
	all := make([]seen, 0, len(counts))
	for c, count := range counts {
		all = append(all, seen{c, count})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].n != all[j].n {
			return all[i].n > all[j].n
		}
		// A stable order for equal counts, so the same picture gives the same
		// palette and the same bytes: the push gate compares frames.
		a, b := all[i].c, all[j].c
		return uint32(a.R)<<16|uint32(a.G)<<8|uint32(a.B) < uint32(b.R)<<16|uint32(b.G)<<8|uint32(b.B)
	})
	if len(all) > n {
		all = all[:n]
	}
	out := make(color.Palette, 0, len(all))
	for _, one := range all {
		out = append(out, one.c)
	}
	return out
}

/*
text draws a string, with a dark outline where the background is a picture.

Eight offsets and then the fill. Four would leave the diagonals thin, and a
glyph whose corner blends into a bright pixel is a digit somebody misreads --
which on this panel means misreading a temperature.
*/
func (p *paint) text(s string, x, y, w, h int, pt float64, bold bool, c color.Color) {
	if p.outline {
		for _, d := range []image.Point{
			{X: -outlineWidth}, {X: outlineWidth}, {Y: -outlineWidth}, {Y: outlineWidth},
			{X: -outlineWidth, Y: -outlineWidth}, {X: outlineWidth, Y: -outlineWidth},
			{X: -outlineWidth, Y: outlineWidth}, {X: outlineWidth, Y: outlineWidth},
		} {
			centred(p.img, s, x+d.X, y+d.Y, w, h, pt, bold, colOutline)
		}
	}
	centred(p.img, s, x, y, w, h, pt, bold, c)
}

// outlineWidth is how far the outline is drawn from the glyph. Two pixels at
// 640: enough to survive a bright background, small enough that the digits do
// not close up at the headline's size.
const outlineWidth = 2
