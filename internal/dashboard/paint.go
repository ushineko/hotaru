package dashboard

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"sort"

	"golang.org/x/image/font"
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
	img   *image.Paletted
	theme Theme
	// picture says the background is a photograph, which is what decides
	// whether text is outlined when the dashboard has not said.
	picture bool
	// letters is the dashboard's own typography, applied on top of what the
	// arrangement and the theme decided. See Lettering.
	letters Lettering
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
		return &paint{
			img:   dimmed(behind, d.Background.Darkness(), theme),
			theme: theme, picture: true, letters: d.Lettering,
		}
	case Plain:
		img := image.NewPaletted(image.Rect(0, 0, Size, Size),
			palette(theme, nil, d.Lettering.chosen()...))
		draw.Draw(img, img.Bounds(), image.NewUniform(theme.BG), image.Point{}, draw.Src)
		return &paint{img: img, theme: theme, letters: d.Lettering}
	default:
		sky := background(theme)
		img := image.NewPaletted(sky.Bounds(), sky.Palette)
		copy(img.Pix, sky.Pix)
		return &paint{img: img, theme: theme, letters: d.Lettering}
	}
}

/*
dimmed draws a picture darkened, on a palette that has room for both.

The picture gets what is left after the interface's own colours: a photograph
through 240 entries is indistinguishable from one through 256, and text drawn
in the nearest available colour to white is not white.
*/
func dimmed(behind image.Image, keep float64, theme Theme, chosen ...color.RGBA) *image.Paletted {
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

	img := image.NewPaletted(bounds, palette(theme, fromPicture(rgba), chosen...))
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
textWidth is how wide a string is as this dashboard draws it: the
arrangement's size through the author's scale, in the author's face.

The rows need it because they set themselves rather than being given bands.
Everything else is told where to go by the arrangement.
*/
func (p *paint) textWidth(s string, pt float64, bold bool, style Text) int {
	drawing.Lock()
	defer drawing.Unlock()
	return font.MeasureString(face(pt*style.Scale(), bold, p.letters.Font), s).Round()
}

/*
label draws one of the words: the dashboard's lettering for labels, and the
theme's muted colour unless it named one.
*/
func (p *paint) label(s string, x, y, w, h int, pt float64) {
	p.labelAt(s, x, y, w, h, pt, Centre)
}

// labelAt is label, placed. See Align.
func (p *paint) labelAt(s string, x, y, w, h int, pt float64, align Align) {
	p.write(s, x, y, w, h, pt, false, false, p.letters.Labels, p.theme.Muted, align)
}

/*
value draws one of the readings, shrunk to its band if it has to be.

A pair makes overflow certain where it used to be a four-digit edge case:
"1450 / 1450" is eleven characters in a band that was sized for four. The
column comment records the last fix for that, which was to make the font
smaller for every dashboard; this one costs nothing to the values that fit.

**A graded colour is not overridden.** Coolant is green, amber or red because
that is what the alerts say, and a screen showing calm while a notification
says critical is worse than either alone (spec 013). So a dashboard's own
colour applies to the readings that carry no grade, and the one that means
something keeps meaning it.
*/
func (p *paint) value(s string, x, y, w, h int, pt float64, c color.Color, graded bool) {
	p.valueAt(s, x, y, w, h, pt, c, graded, Centre)
}

/*
valueSized draws a value at a point size the caller has already settled.

The stacked rows work out one size for the whole column -- the author's scale
applied, then shrunk until the widest number fits -- and scaling or shrinking
it again per row is how a column that was measured as a table goes back to
three sizes. So neither happens here: what is passed is what is drawn.
*/
func (p *paint) valueSized(s string, x, y, w, h int, pt float64, c color.Color,
	graded bool, align Align,
) {
	style := p.letters.Values
	if graded {
		style.Colour = ""
	}
	style.Size = 0 // the caller applied it
	p.write(s, x, y, w, h, pt, true, false, style, c, align)
}

// valueAt is value, placed. See Align.
func (p *paint) valueAt(s string, x, y, w, h int, pt float64, c color.Color,
	graded bool, align Align,
) {
	style := p.letters.Values
	if graded {
		style.Colour = ""
	}
	p.write(s, x, y, w, h, pt, true, true, style, c, align)
}

/*
write draws a string at a style, with a dark outline where one is called for.

Eight offsets and then the fill. Four would leave the diagonals thin, and a
glyph whose corner blends into a bright pixel is a digit somebody misreads --
which on this panel means misreading a temperature.

`fit` shrinks the text to its band rather than letting it overrun. Values ask
for it and labels do not: a label too long is the author's sentence and theirs
to shorten, and a value is the machine's and cannot be edited. The size is
settled before the outline loop so all nine draws are the same glyphs.
*/
func (p *paint) write(s string, x, y, w, h int, pt float64, bold, fit bool,
	style Text, c color.Color, align Align,
) {
	if chosen, ok := parseColour(style.Colour); ok {
		c = chosen
	}
	pt *= style.Scale()
	family := p.letters.Font
	if fit {
		pt = fitted(s, w, pt, bold, family)
	}

	if edge := style.Edge(p.picture); edge > 0 {
		for _, d := range []image.Point{
			{X: -edge}, {X: edge}, {Y: -edge}, {Y: edge},
			{X: -edge, Y: -edge}, {X: edge, Y: -edge},
			{X: -edge, Y: edge}, {X: edge, Y: edge},
		} {
			centred(p.img, s, x+d.X, y+d.Y, w, h, pt, bold, family, colOutline, align)
		}
	}
	centred(p.img, s, x, y, w, h, pt, bold, family, c, align)
}

/*
parseColour reads "#rrggbb".

Anything else is not a colour and is ignored, which leaves the theme's own --
a dashboard edited by hand into a colour that does not parse draws the way it
did before, rather than in black on black.
*/
func parseColour(s string) (color.RGBA, bool) {
	if len(s) != 7 || s[0] != '#' {
		return color.RGBA{}, false
	}
	var r, g, b uint8
	if _, err := fmt.Sscanf(s[1:], "%02x%02x%02x", &r, &g, &b); err != nil {
		return color.RGBA{}, false
	}
	return color.RGBA{R: r, G: g, B: b, A: 255}, true
}
