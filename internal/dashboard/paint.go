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
wordsAt is the size a set of labels is drawn at in the column they were left.

Their own size, unless the widest of them does not fit it -- in which case as
much of it as does. Shrink only: a label that fits is drawn at exactly what
the dashboard asked for, so this is invisible until the numbers have taken the
room (spec 045), and it is what stops a squeezed word being drawn through the
figure beside it.

The walk down is the one the column uses: advances are near enough linear in
the point size, so an estimate and a short walk settle it.
*/
func (p *paint) wordsAt(words []string, w int, pt float64) float64 {
	for {
		widest := 0
		for _, s := range words {
			widest = max(widest, p.textWidth(s, pt, false, p.letters.Labels))
		}
		if widest <= w || w <= 0 || pt <= minLabelPt {
			return pt
		}
		next := math.Floor(pt * float64(w) / float64(widest))
		if next >= pt {
			next = pt - 1
		}
		pt = max(next, minLabelPt)
	}
}

// minLabelPt is the smallest a squeezed label is drawn. Below it the word is
// not read across a desk, which is the whole of what a label is for: better a
// word that reaches the numbers than one nobody can make out.
const minLabelPt = 12

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
	p.fieldsSized(fs, x, y, w, h, p.fitted(fs, w, pt*p.letters.Values.Scale(), align),
		c, graded, align)
}

/*
fitted is the largest size at which an assembly fits the room it is given.

**Measured on the layout it will be drawn in**, which is the whole of this.
A pair anchored on its separator is not the same shape as the same fields laid
end to end: the separator sits at the centre of the space and each half grows
outward from it, so the wider half decides both sides. Fitting one shape and
drawing the other is how "48% · 100°C" came to hang five pixels past the inset
on a 640-pixel panel, while "48% · 38°C" sat well inside it (#140).
*/
func (p *paint) fitted(fs []Field, w int, pt float64, align Align) float64 {
	at, paired := dividerAt(fs)
	if !paired || align == Right {
		_, _, size := p.fittedBoxes(fs, w, pt)
		return size
	}

	for size := pt; ; size-- {
		if p.spread(fs, at, size) <= w || size <= minValuePt {
			return max(size, minValuePt)
		}
	}
}

/*
spread is how wide a pair is drawn when it is anchored on its separator.

The separator is centred, so each half has the same room and the wider of the
two decides how much that is. Reserved widths rather than measured ones,
because the reservation is what makes the assembly the same width from one
frame to the next: a headline that shrank as its temperature passed a hundred
would be spec 042 undone.
*/
func (p *paint) spread(fs []Field, divider int, pt float64) int {
	var left, right int
	for i, f := range fs {
		switch {
		case i < divider:
			left += p.fieldWidth(f, pt)
		case i > divider:
			right += p.fieldWidth(f, pt)
		}
	}
	return p.textWidth(fs[divider].Text, pt, true, Text{}) + 2*max(left, right)
}

/*
column is the field widths a set of rows share, and the size they all fit at.

**Per field index, across every row.** The widest first number decides the
first box for all of them, so the separators after those boxes land in one
column -- which is the whole of what makes four rows read as a table rather
than as four separate lines.

A row with fewer fields than the widest contributes to the boxes it has and
leaves the rest alone, which is also how it is drawn: from the left, its
number under the other rows' first numbers.
*/
func (p *paint) column(rows [][]Field, w int, pt float64) ([]int, int, float64) {
	for {
		widths, total := p.columnBoxes(rows, pt)
		if total <= w || w <= 0 || pt <= minValuePt {
			return widths, total, pt
		}
		// Advances are near enough linear in the point size, so one estimate
		// and a short walk down, the way a single assembly is fitted.
		next := math.Floor(pt * float64(w) / float64(total))
		if next >= pt {
			next = pt - 1
		}
		pt = max(next, minValuePt)
	}
}

// columnBoxes is the widest each field is across the rows, and their total.
func (p *paint) columnBoxes(rows [][]Field, pt float64) ([]int, int) {
	var widths []int
	for _, row := range rows {
		for i, f := range row {
			w := p.fieldWidth(f, pt)
			switch {
			case i < len(widths):
				widths[i] = max(widths[i], w)
			default:
				widths = append(widths, w)
			}
		}
	}

	total := 0
	for _, w := range widths {
		total += w
	}
	return widths, total
}

/*
inColumn draws one row into widths the column settled.

Left to right through the boxes, so a row with fewer fields sits under the
first of everything rather than the last. Aligning a lone value from the right
would put it under the other rows' *second* numbers, which is a lie about what
it is.
*/
func (p *paint) inColumn(fs []Field, widths []int, x, y, h int, pt float64,
	c color.Color, graded bool,
) {
	style := p.letters.Values
	if graded {
		style.Colour = ""
	}
	style.Size = 0 // the column applied it

	at := x
	for i, f := range fs {
		w := widths[i]
		p.writeField(f, at, y, w, h, pt, style, c)
		at += w
	}
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

	if at, ok := dividerAt(fs); ok && align != Right {
		p.aroundDivider(fs, at, x, y, w, h, pt, style, c)
		return
	}

	widths, total := p.boxes(fs, pt)
	place := x + offset(w, total, align)
	for i, f := range fs {
		p.writeField(f, place, y, widths[i], h, pt, style, c)
		place += widths[i]
	}
}

/*
aroundDivider draws a pair anchored on the thing between its halves.

**The separator is the fixed point**, at the centre of the space, with one
number growing left from it and the other growing right. Nothing here reserves
anything, and nothing needs to: an edge that does not move is an edge that
does not move, and each number has one against the divider.

That is also what puts a unit against the number it belongs to. Packing
outward from the middle means "12%" and "63°C" are each drawn as one run,
where laying them into reserved boxes left a blank column between a figure and
its own unit -- the number right-aligned in its box and the unit starting at
the far side.
*/
func (p *paint) aroundDivider(fs []Field, divider, x, y, w, h int, pt float64,
	style Text, c color.Color,
) {
	middle := x + w/2
	span := p.textWidth(fs[divider].Text, pt, true, Text{})
	p.write(fs[divider].Text, middle-span/2, y, span, h, pt, true, style, c, Left)

	// Leftwards from the separator, in reverse, so each piece ends where the
	// next one along begins.
	place := middle - span/2
	for i := divider - 1; i >= 0; i-- {
		width := p.run(fs[i], pt)
		place -= width
		p.writeField(fs[i], place, y, width, h, pt, style, c)
	}

	place = middle - span/2 + span
	for _, f := range fs[divider+1:] {
		width := p.run(f, pt)
		p.writeField(f, place, y, width, h, pt, style, c)
		place += width
	}
}

// dividerAt is where the separator sits, if there is one.
func dividerAt(fs []Field) (int, bool) {
	for i, f := range fs {
		if f.Divider {
			return i, true
		}
	}
	return 0, false
}

/*
hug is where a field sits inside its own box.

A number grows leftwards, so it is right-aligned: that is what keeps the
digits already drawn where they are when another arrives. A unit grows
rightwards from the number it belongs to, so it is left-aligned and lands
against the figure rather than at the far side of a column sized for "RPM".
*/
func hug(f Field) Align {
	if f.Small {
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
	widths = make([]int, len(fs))
	for i, f := range fs {
		widths[i] = p.fieldWidth(f, pt)
		total += widths[i]
	}
	return widths, total
}

/*
fieldWidth is the room one field takes at a size.

A reservation is in characters and the digits are tabular in every face here,
so a count is a width. A field wider than its reservation keeps its own: the
reservation is a floor, not a ceiling.

A small field -- a unit -- reserves nothing and is measured at its own size,
because it is a word rather than a number and has no digits to hold still.
*/
func (p *paint) fieldWidth(f Field, pt float64) int {
	size := sizeOf(f, pt)
	if f.Small {
		return unitGap(f, pt) + p.textWidth(f.Text, size, true, Text{})
	}
	digit := p.textWidth("0", size, true, Text{})
	return max(f.Chars*digit, p.textWidth(f.Text, size, true, Text{}))
}

// run is how much room a field takes packed against its neighbours, with no
// reservation: what it measures, and a unit's gap where there is one. The
// pair packed around a divider is drawn this way -- see aroundDivider.
func (p *paint) run(f Field, pt float64) int {
	return unitGap(f, pt) + p.textWidth(f.Text, sizeOf(f, pt), true, Text{})
}

// sizeOf is the point size one field is drawn at: the line's, or a unit's
// fraction of it.
func sizeOf(f Field, pt float64) float64 {
	if f.Small {
		return math.Max(pt*UnitScale, minUnitPt)
	}
	return pt
}

/*
unitGap is the space kept between a number and the unit that qualifies it.

Spec 044 packed the two as one run so that nothing sat between them, which was
right and a shade too tight: "12%" with the figure and the sign touching reads
as one token, and the eye takes longer to separate them than it should.

A fraction of the number's size rather than a count of pixels, so the gap is
the same gap on a stacked row and on the headline. Zero for anything that is
not a unit: the reserved boxes hold the digits still and a gap inside one
would be a digit moving.
*/
func unitGap(f Field, pt float64) int {
	if !f.Small {
		return 0
	}
	return int(math.Round(pt * UnitGap))
}

/*
writeField draws one field in the box the line gave it.

A unit is set in from the left edge of its box by the gap that belongs to it,
which is why the box was measured a gap wider: the space lands between the
figure and the unit rather than after the pair, wherever the field is drawn.
*/
func (p *paint) writeField(f Field, x, y, w, h int, pt float64, style Text, c color.Color) {
	if gap := unitGap(f, pt); gap > 0 {
		x, w = x+gap, w-gap
	}
	p.write(f.Text, x, y, w, h, sizeOf(f, pt), true, style, c, hug(f))
}

// minUnitPt keeps a unit readable when the line it belongs to has been shrunk
// a long way. Below this it is a smudge beside a number, which is worse than
// not drawing it.
const minUnitPt = 10

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
