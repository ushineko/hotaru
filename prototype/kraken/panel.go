package main

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"math/rand"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

/*
The monitor's dashboard, ported.

Every constant here is taken from aio_dashboard.py rather than re-chosen: the
layout was arrived at by looking at the panel in a case, and several of its
numbers are corrections to mistakes (the metric row's inset exists because the
coolant ring cut through "RPM" at the old margin).

Qt drew the Python's version. Go has no font rasteriser in the standard
library, so this uses x/image with the Go fonts embedded -- no system fonts, no
cgo, and nothing to install.
*/
const (
	panelSize   = 640
	panelMargin = 40
	metricInset = 140
	metricSlots = 3
	ringWidth   = 14
	warnC       = 50.0
	critC       = 60.0
	ptToPx      = 4.0 / 3.0 // Qt point sizes at 96 dpi
)

var (
	colBG     = color.RGBA{12, 14, 18, 255}
	colMuted  = color.RGBA{130, 140, 155, 255}
	colAccent = color.RGBA{120, 190, 255, 255}
	colOK     = color.RGBA{126, 200, 140, 255}
	colWarn   = color.RGBA{230, 180, 90, 255}
	colCrit   = color.RGBA{235, 110, 110, 255}
	colTrack  = color.RGBA{120, 132, 156, 70}
)

// nebulae are placed off-centre and kept dim: the middle of the panel carries
// the headline number and stays the darkest part of the image.
var nebulae = []struct {
	cx, cy, r float64
	c         color.RGBA
}{
	{0.20, 0.22, 0.46, color.RGBA{96, 60, 190, 255}},
	{0.82, 0.30, 0.40, color.RGBA{30, 120, 170, 255}},
	{0.68, 0.84, 0.44, color.RGBA{150, 45, 120, 255}},
	{0.30, 0.78, 0.34, color.RGBA{40, 90, 160, 255}},
}

// plainBackground drops the decoration while the size budget is unknown.
var plainBackground = false

var (
	faceCache = map[string]font.Face{}
	bgCache   *image.RGBA
)

func face(pt float64, bold bool) font.Face {
	key := "r"
	src := goregular.TTF
	if bold {
		key, src = "b", gobold.TTF
	}
	key += string(rune(int(pt)))
	if f, ok := faceCache[key]; ok {
		return f
	}
	parsed, err := opentype.Parse(src)
	if err != nil {
		panic(err)
	}
	f, err := opentype.NewFace(parsed, &opentype.FaceOptions{
		Size: pt * ptToPx, DPI: 72, Hinting: font.HintingFull,
	})
	if err != nil {
		panic(err)
	}
	faceCache[key] = f
	return f
}

// centred draws text centred in both axes within a rect, which is the whole
// reason the Python has a helper: top-aligned text lets a large glyph overrun
// its box and collide with the band beneath.
func centred(dst *image.RGBA, s string, x, y, w, h int, pt float64, bold bool, c color.Color) {
	f := face(pt, bold)
	adv := font.MeasureString(f, s)
	m := f.Metrics()
	tx := x + (w-adv.Round())/2
	ty := y + (h+m.Ascent.Round()-m.Descent.Round())/2
	d := &font.Drawer{
		Dst: dst, Src: image.NewUniform(c), Face: f,
		Dot: fixed.P(tx, ty),
	}
	d.DrawString(s)
}

// background is the starfield, rendered once. A field regenerated per frame
// would shimmer between updates, which on a screen that only redraws when
// something changes reads as a fault rather than decoration.
func background() *image.RGBA {
	if bgCache != nil {
		return bgCache
	}
	img := image.NewRGBA(image.Rect(0, 0, panelSize, panelSize))
	draw.Draw(img, img.Bounds(), image.NewUniform(colBG), image.Point{}, draw.Src)

	/*
		Flat, for now.

		The nebulae and the starfield are what this panel will not display: a
		background of smooth gradients and scattered points compresses badly,
		and the device silently ignores a frame above some size. Decoration
		gets added back once the budget is known, cheapest first.
	*/
	if !plainBackground {
		for _, n := range nebulae {
			cx, cy, r := n.cx*panelSize, n.cy*panelSize, n.r*panelSize
			for y := range panelSize {
				for x := range panelSize {
					dx, dy := float64(x)-cx, float64(y)-cy
					d := math.Hypot(dx, dy)
					if d > r {
						continue
					}
					f := (1 - d/r)
					f = f * f * 0.28
					o := img.RGBAAt(x, y)
					img.SetRGBA(x, y, color.RGBA{
						R: clamp8(float64(o.R) + float64(n.c.R)*f),
						G: clamp8(float64(o.G) + float64(n.c.G)*f),
						B: clamp8(float64(o.B) + float64(n.c.B)*f),
						A: 255,
					})
				}
			}
		}
		rng := rand.New(rand.NewSource(0x5EED))
		for range 150 {
			x, y := rng.Intn(panelSize-2), rng.Intn(panelSize-2)
			star := color.RGBA{200, 205, 230, 255}
			for dy := range 2 {
				for dx := range 2 {
					img.SetRGBA(x+dx, y+dy, star)
				}
			}
		}
	}

	bgCache = img
	return img
}

func clamp8(v float64) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

// arc strokes a circle segment, clockwise from twelve o'clock.
func arc(dst *image.RGBA, cx, cy, radius, width float64, from, sweep float64, c color.RGBA) {
	steps := int(math.Abs(sweep) * radius / 0.5)
	if steps < 2 {
		return
	}
	for i := range steps {
		a := from + sweep*float64(i)/float64(steps-1)
		for w := -width / 2; w <= width/2; w += 0.5 {
			r := radius + w
			x := int(cx + r*math.Cos(a))
			y := int(cy - r*math.Sin(a))
			if x < 0 || y < 0 || x >= panelSize || y >= panelSize {
				continue
			}
			if c.A == 255 {
				dst.SetRGBA(x, y, c)
				continue
			}
			o := dst.RGBAAt(x, y)
			f := float64(c.A) / 255
			dst.SetRGBA(x, y, color.RGBA{
				R: clamp8(float64(o.R)*(1-f) + float64(c.R)*f),
				G: clamp8(float64(o.G)*(1-f) + float64(c.G)*f),
				B: clamp8(float64(o.B)*(1-f) + float64(c.B)*f),
				A: 255,
			})
		}
	}
}

func coolantColour(v float64, ok bool) color.RGBA {
	switch {
	case !ok:
		return colMuted
	case v >= critC:
		return colCrit
	case v >= warnC:
		return colWarn
	}
	return colOK
}

// The paletted equivalents of the drawing helpers, so a frame composes onto a
// pre-quantised background instead of being converted whole.
func arcP(dst *image.Paletted, cx, cy, radius, width, from, sweep float64, c color.RGBA) {
	steps := int(math.Abs(sweep) * radius / 0.5)
	if steps < 2 {
		return
	}
	for i := range steps {
		a := from + sweep*float64(i)/float64(steps-1)
		for w := -width / 2; w <= width/2; w += 0.5 {
			r := radius + w
			x, y := int(cx+r*math.Cos(a)), int(cy-r*math.Sin(a))
			if x >= 0 && y >= 0 && x < panelSize && y < panelSize {
				dst.Set(x, y, c)
			}
		}
	}
}

func centredP(dst *image.Paletted, s string, x, y, w, h int, pt float64, bold bool, c color.Color) {
	f := face(pt, bold)
	adv := font.MeasureString(f, s)
	m := f.Metrics()
	d := &font.Drawer{
		Dst: dst, Src: image.NewUniform(c), Face: f,
		Dot: fixed.P(x+(w-adv.Round())/2, y+(h+m.Ascent.Round()-m.Descent.Round())/2),
	}
	d.DrawString(s)
}

func metricP(dst *image.Paletted, slot int, label, value, unit string, c color.RGBA) {
	width := (panelSize - 2*metricInset) / metricSlots
	x := metricInset + slot*width
	centredP(dst, label, x, 416, width, 28, 16, false, colMuted)
	centredP(dst, value, x, 446, width, 66, 34, true, c)
	centredP(dst, unit, x, 516, width, 26, 14, false, colMuted)
}
