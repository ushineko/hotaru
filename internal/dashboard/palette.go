/*
Package dashboard draws the cooler's screen.

peripheral-battery-monitor's design, ported constant for constant: a coolant
temperature as the headline with a severity-coloured ring, CPU, GPU and pump
across the bottom, over a starfield. It was arrived at by looking at the panel
in a case, and several of its numbers are corrections to mistakes -- the metric
row's inset exists because the ring cut through the word "RPM" at the old
margin. See spec 013.

Qt drew the original and Pillow encoded it. A service cannot carry a toolkit,
and Go has no font rasteriser of its own, so this uses golang.org/x/image with
the Go fonts embedded: one dependency, pure Go, no cgo, no system fonts.
*/
package dashboard

import "image/color"

// The panel, and the geometry the Python arrived at.
const (
	Size        = 640
	margin      = 40
	metricInset = 140
	metricSlots = 3
	ringWidth   = 14

	/*
		The insets the other arrangements need, for the reason the metric row
		needed its own: the ring curves inward, and a column placed by the
		panel's edge is drawn through by it.

		Grid sits wider than the metric row because its two columns are
		wider than three; the stacked rows clear the ring entirely by
		starting below where it ends.
	*/
	gridInset = 96
	rowInset  = 120

	/*
		The stacked row's word column, and the gap before the number.

		rowLabelWidth is a minimum rather than a width: a label wider than it
		pushes the number right instead of being drawn through it. The gap is
		what keeps "PUMP RPM / %" and "2709 / 90" from touching, which at a
		fixed band they did to within three pixels.
	*/
	rowLabelWidth = 160
	rowGap        = 24

	// rowValuePt is the size a stacked row's number is drawn at before the
	// column is fitted. Spec 013's, unchanged.
	rowValuePt = 34.0

	/*
		headlineInset keeps the big number off the panel's edges.

		It did not need one while the value was drawn to the width of
		whatever it said: "37.5" at 170 point is 508 pixels and sat well
		inside 640. Reserving each half of a pair its widest (spec 042) makes
		the assembly 945 before it is fitted, so without an inset it is
		squeezed to 639 and touches both edges.

		Wide enough to breathe, narrow enough that a single value is still
		drawn at the size spec 013 chose for it.
	*/
	headlineInset = 32

	/*
		Where the rows go, and how tall each one is.

		A column was 126 tall -- label, value, unit -- and the first version
		of the grid stepped by 112, which put one row's unit through the
		next row's label. Only visible by rendering it and looking.

		The unit is gone (spec 041) and the step is not: 126 is the height a
		grid of four readings was looked at with, and reclaiming the 26 pixels
		would move every row on every saved dashboard to buy back space
		nothing is asking for.
	*/
	columnHeight = 126
	gridTop      = 292
	rowHeight    = 66
	stackTop     = 288

	/*
		captionY is a band every arrangement keeps clear.

		The author's line is the one piece of text whose length nobody can
		predict, so it gets a reserved row rather than being fitted around
		the readings: an arrangement that ran into it would be an
		arrangement that looked right until somebody typed a caption.
	*/
	captionY = 556
)

// Coolant colour bands, matching the alert thresholds in the reader so the
// screen and the notifications never disagree.
const (
	warnC = 50.0
	critC = 60.0
)

/*
The colours no theme may change.

A theme is style; these three are meaning. The screen's grade has to agree
with what the alerts say, because a panel showing calm while a notification
says otherwise is worse than either alone -- so green, amber and red are the
same green, amber and red on every dashboard anybody builds.
*/
var (
	colOK   = color.RGBA{126, 200, 140, 255}
	colWarn = color.RGBA{230, 180, 90, 255}
	colCrit = color.RGBA{235, 110, 110, 255}
	/*
		colOutline is the ring of dark drawn around text over a picture.

		Black rather than the theme's background, which on a light theme
		would be no outline at all. The point is contrast with whatever
		somebody's photograph happens to put under a glyph, and only one
		colour is reliably darker than "anything".
	*/
	colOutline = color.RGBA{0, 0, 0, 255}
)

/*
palette is the theme's colours, the grade's, a short ramp, and whatever the
background needs.

A size decision as much as a visual one. GIF is LZW over palette indices, so a
long gradient gives nearly every pixel its own value and compresses to nothing:
the same picture through 24 steps is a third of the size, and dithering a
starfield turned a 21 KB frame into 105 KB. Frame size buys settling time on
this panel rather than nothing -- see the floor in push.go -- so it is a real
trade.
*/
func palette(theme Theme, extra color.Palette, chosen ...color.RGBA) color.Palette {
	out := color.Palette{
		theme.BG, theme.Muted, theme.Accent, colOK, colWarn, colCrit,
		theme.Bright, theme.Edge, colOutline,
	}
	/*
		A dashboard's own text colours, exactly.

		They go in the interface's part of the palette rather than competing
		with the picture's for what is left: text drawn in the nearest
		available colour to the one somebody chose is text in a colour
		nobody chose, which is the same argument as the comment on
		pictureColours.
	*/
	for _, c := range chosen {
		out = append(out, c)
	}
	/*
		A short ramp from the background towards the sky, for the starfield's
		gradients. Built from the theme rather than fixed, because a ramp
		into somebody else's blue is what makes an amber panel look wrong at
		the edges.
	*/
	for i := range 24 {
		f := float64(i) / 23
		out = append(out, color.RGBA{
			R: clamp(float64(theme.BG.R) + f*float64(theme.Ramp.R)),
			G: clamp(float64(theme.BG.G) + f*float64(theme.Ramp.G)),
			B: clamp(float64(theme.BG.B) + f*float64(theme.Ramp.B)),
			A: 255,
		})
	}
	out = append(out, extra...)

	/*
		**A GIF has 256 colours and no more.**

		The encoder refuses a block with more, and the renderer discards its
		error -- so a palette over the limit was a zero-byte frame: the panel
		showed nothing and nothing said why. It took a noisy photograph to
		reach it, which is why it survived until a dashboard could add
		colours of its own.

		The interface's colours are first and are kept; what is dropped is
		the tail of the background's, which are the least common ones in the
		picture.
	*/
	if len(out) > MaxColours {
		out = out[:MaxColours]
	}
	return out
}

// MaxColours is what a GIF frame holds, which is the limit the palette is
// built to rather than a number to check afterwards.
const MaxColours = 256

func clamp(v float64) uint8 {
	switch {
	case v < 0:
		return 0
	case v > 255:
		return 255
	}
	return uint8(v)
}

/*
coolantColour grades a temperature. This is the one colour on the screen that
carries meaning rather than style.

A reading nobody could take is drawn in the theme's muted colour: it is
absence, and absence is not a grade.
*/
func coolantColour(c float64, known bool, theme Theme) color.RGBA {
	switch {
	case !known:
		return theme.Muted
	case c >= critC:
		return colCrit
	case c >= warnC:
		return colWarn
	}
	return colOK
}
