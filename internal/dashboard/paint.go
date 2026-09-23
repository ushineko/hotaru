package dashboard

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
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
	p.write(s, x, y, w, h, pt, false, p.letters.Labels, p.theme.Muted, align)
}

/*
fields draws a value as reserved boxes rather than as one string, which is how
it stops moving between frames.

Each field is drawn right-aligned in a box as wide as the characters it
reserved -- numbers grow leftwards, the way a number should -- and the boxes
are laid end to end and then placed as a unit. Because every box's width comes
from the reservation rather than from the reading, the assembly is the same
width every frame and sits in the same place.

The whole assembly shrinks together when it will not fit, for the reason a
stacked column does: one size, or the pieces of one number are drawn at two.
*/
func (p *paint) fields(fs []Field, x, y, w, h int, pt float64, c color.Color,
	graded bool, align Align,
) {
	_, _, size := p.fittedBoxes(fs, w, pt*p.letters.Values.Scale())
	p.fieldsSized(fs, x, y, w, h, size, c, graded, align)
}

/*
fieldsSized is fields at a point size the caller has settled.

The stacked rows work one size out for the whole column and then draw every
row at it; fitting again per row is how a column measured as a table goes back
to three sizes.
*/
func (p *paint) fieldsSized(fs []Field, x, y, w, h int, pt float64, c color.Color,
	graded bool, align Align,
) {
	style := p.letters.Values
	if graded {
		style.Colour = ""
	}
	style.Size = 0 // the caller applied it

	widths, total := p.boxes(fs, pt)
	at := x + offset(w, total, align)
	for i, f := range fs {
		p.write(f.Text, at, y, widths[i], h, pt, true, style, c, within(i, len(fs), align))
		at += widths[i]
	}
}

/*
within is where a field sits inside the box reserved for it.

Numbers grow leftwards, so a field is right-aligned by default -- which is
what makes the digits already drawn stay put when another one arrives.

**The last field of a pair is the exception when the assembly is centred.**
Right-aligning both halves puts a blank character on each side of the
separator, and a dot floating in that much space stops reading as a divider
between two numbers and starts reading as a third thing. Hugging the separator
puts the slack on the outer edges instead, where a centred assembly has it
symmetrically and nobody sees it.

A right-aligned assembly keeps every field right-aligned: it is a column, and
what matters there is that the numbers end where the ones above and below them
end.
*/
func within(i, n int, align Align) Align {
	if i == n-1 && n > 1 && align != Right {
		return Left
	}
	return Right
}

/*
boxes is how wide each field is drawn and how wide they are together.

A field's reservation is in characters and the digits are tabular in every
face here, so one digit's advance is the unit. A field wider than its
reservation keeps its own width: the reservation is a floor, not a ceiling.
*/
func (p *paint) boxes(fs []Field, pt float64) (widths []int, total int) {
	digit := p.textWidth("0", pt, true, Text{})
	widths = make([]int, len(fs))
	for i, f := range fs {
		widths[i] = max(f.Chars*digit, p.textWidth(f.Text, pt, true, Text{}))
		total += widths[i]
	}
	return widths, total
}

/*
fittedBoxes is boxes at the largest size whose assembly fits the room given.

Measured on the *assembly*, not on its text. The reservations make it wider
than the characters in it, so shrinking until the text fits would leave the
boxes overrunning by exactly the padding -- which is the whole of what was
added. Advances are near enough linear in the point size for one estimate and
a short walk down, the way a single value is fitted.
*/
func (p *paint) fittedBoxes(fs []Field, w int, pt float64) ([]int, int, float64) {
	widths, total := p.boxes(fs, pt)
	if total <= w || w <= 0 || total <= 0 || pt <= minValuePt {
		return widths, total, pt
	}

	for size := math.Floor(pt * float64(w) / float64(total)); size > minValuePt; size-- {
		if widths, total = p.boxes(fs, size); total <= w {
			return widths, total, size
		}
	}
	widths, total = p.boxes(fs, minValuePt)
	return widths, total, minValuePt
}

/*
write draws a string at a style, with a dark outline where one is called for.

Eight offsets and then the fill. Four would leave the diagonals thin, and a
glyph whose corner blends into a bright pixel is a digit somebody misreads --
which on this panel means misreading a temperature.

The size is settled by the caller, so all nine draws are the same glyphs.
Values are sized by fittedBoxes before they get here; a label too long is the
author's sentence and theirs to shorten, so it is drawn as written.
*/
func (p *paint) write(s string, x, y, w, h int, pt float64, bold bool,
	style Text, c color.Color, align Align,
) {
	if chosen, ok := parseColour(style.Colour); ok {
		c = chosen
	}
	pt *= style.Scale()
	family := p.letters.Font

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
