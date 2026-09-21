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
)

// Coolant colour bands, matching the alert thresholds in the reader so the
// screen and the notifications never disagree.
const (
	warnC = 50.0
	critC = 60.0
)

var (
	colBG     = color.RGBA{12, 14, 18, 255}
	colMuted  = color.RGBA{130, 140, 155, 255}
	colAccent = color.RGBA{120, 190, 255, 255}
	colBright = color.RGBA{236, 239, 244, 255}
	colOK     = color.RGBA{126, 200, 140, 255}
	colWarn   = color.RGBA{230, 180, 90, 255}
	colCrit   = color.RGBA{235, 110, 110, 255}
	colEdge   = color.RGBA{60, 68, 82, 255}
)

/*
palette is eight fixed colours and a short ramp.

A size decision as much as a visual one. GIF is LZW over palette indices, so a
long gradient gives nearly every pixel its own value and compresses to nothing:
the same picture through 24 steps is a third of the size, and dithering a
starfield turned a 21 KB frame into 105 KB. Frame size buys settling time on
this panel rather than nothing -- see the floor in push.go -- so it is a real
trade.
*/
func palette() color.Palette {
	out := color.Palette{colBG, colMuted, colAccent, colOK, colWarn, colCrit, colBright, colEdge}
	for i := range 24 {
		f := float64(i) / 23
		out = append(out, color.RGBA{
			R: clamp(12 + f*100),
			G: clamp(14 + f*80),
			B: clamp(18 + f*140),
			A: 255,
		})
	}
	return out
}

func clamp(v float64) uint8 {
	switch {
	case v < 0:
		return 0
	case v > 255:
		return 255
	}
	return uint8(v)
}

// coolantColour grades the headline by temperature. This is the one colour on
// the screen that carries meaning rather than style.
func coolantColour(c float64, known bool) color.RGBA {
	switch {
	case !known:
		return colMuted
	case c >= critC:
		return colCrit
	case c >= warnC:
		return colWarn
	}
	return colOK
}
