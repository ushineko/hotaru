package gui

import (
	"image"
	"image/color"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
)

/*
Wheel is a colour wheel: hue around, saturation outward, brightness on a
slider beside it.

Fyne has one and keeps it unexported, so this is hotaru's. It exists rather
than a grid of named swatches because picking a colour for a light is a
*visual* act -- the thing being chosen is what the room will look like, and a
wheel is the shape that says so.

It reports every move, not just the last one. The editor uses that to show the
colour on the hardware while the pointer is still down, which is the whole
point of having the machine right there.
*/
type Wheel struct {
	widget.BaseWidget

	// OnPick is called on every tap and every drag step.
	OnPick func(color.Color)

	hue, sat float64
	value    float64

	disc   *canvas.Image
	marker *canvas.Circle
}

// NewWheel builds one at full brightness.
func NewWheel() *Wheel {
	w := &Wheel{value: 1}
	w.ExtendBaseWidget(w)
	return w
}

// Set moves the wheel to a colour without calling OnPick, for opening the
// editor on what a scene already says.
func (w *Wheel) Set(c color.Color) {
	r, g, b, _ := c.RGBA()
	w.hue, w.sat, w.value = toHSV(eighth(r), eighth(g), eighth(b))
	w.Refresh()
}

// Colour is what the wheel is pointing at.
func (w *Wheel) Colour() color.Color {
	r, g, b := fromHSV(w.hue, w.sat, w.value)
	return color.NRGBA{R: r, G: g, B: b, A: 255}
}

// Saturation is how far out the wheel is pointing, which a slider tunes when
// the rim is too coarse to hit.
func (w *Wheel) Saturation() float64 { return w.sat }

// SetSaturation moves in from the rim without moving the hue.
func (w *Wheel) SetSaturation(v float64) {
	w.sat = clamp01(v)
	w.Refresh()
	w.pick()
}

// Hue is the angle around the wheel, in degrees.
func (w *Wheel) Hue() float64 { return w.hue }

// SetValue sets the brightness, which is the slider's half of the picker.
func (w *Wheel) SetValue(v float64) {
	w.value = clamp01(v)
	w.Refresh()
	w.pick()
}

// Value is the current brightness.
func (w *Wheel) Value() float64 { return w.value }

/*
CreateRenderer builds the disc and the marker, and lays them out itself.

Its own renderer rather than a SimpleRenderer over an unlaid-out container,
because **what is drawn and what is picked have to be the same circle**. The
first version let Fyne place the image at its natural size inside a widget of a
different one, so the colour under the pointer was not the colour the pointer
was over -- and no amount of squinting at the conversion maths would have found
it, because the maths was right.
*/
func (w *Wheel) CreateRenderer() fyne.WidgetRenderer {
	w.disc = canvas.NewImageFromImage(w.image(wheelPixels))
	w.disc.FillMode = canvas.ImageFillStretch

	w.marker = canvas.NewCircle(color.Transparent)
	w.marker.StrokeColor = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	w.marker.StrokeWidth = 2

	return &wheelRenderer{wheel: w}
}

// wheelRenderer draws the disc square and centred, which is the geometry the
// picking maths assumes.
type wheelRenderer struct{ wheel *Wheel }

func (r *wheelRenderer) Layout(size fyne.Size) {
	side := min32(size.Width, size.Height)
	origin := fyne.NewPos((size.Width-side)/2, (size.Height-side)/2)

	r.wheel.disc.Resize(fyne.NewSize(side, side))
	r.wheel.disc.Move(origin)
	r.wheel.place()
}

func (r *wheelRenderer) MinSize() fyne.Size { return fyne.NewSize(wheelSize, wheelSize) }

func (r *wheelRenderer) Refresh() {
	r.wheel.disc.Refresh()
	r.wheel.marker.Refresh()
}

func (r *wheelRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.wheel.disc, r.wheel.marker}
}

func (r *wheelRenderer) Destroy() {}

// The wheel's drawn size, and the resolution of the image behind it. The
// image is generated once per brightness change, so it is worth more pixels
// than the widget strictly needs.
const (
	wheelSize   = 180
	wheelPixels = 180
)

// Tapped picks the colour under the pointer.
func (w *Wheel) Tapped(e *fyne.PointEvent) { w.at(e.Position) }

// Dragged picks continuously, which is what makes the hardware follow the
// pointer.
func (w *Wheel) Dragged(e *fyne.DragEvent) { w.at(e.Position) }

// DragEnd is required by the interface and has nothing to do: every step has
// already been reported.
func (w *Wheel) DragEnd() {}

/*
at converts a position on the disc to hue and saturation.

Outside the disc is clamped to its edge rather than ignored: a pointer that
slides past the rim while dragging should keep picking the colour it was
heading for, not stop dead.
*/
func (w *Wheel) at(p fyne.Position) {
	centre, radius := w.circle()
	if radius <= 0 {
		return
	}
	dx := float64(p.X) - float64(centre.X)
	dy := float64(p.Y) - float64(centre.Y)

	distance := math.Hypot(dx, dy)
	w.sat = clamp01(distance / radius)
	w.hue = math.Mod(math.Atan2(dy, dx)*180/math.Pi+360, 360)

	w.Refresh()
	w.pick()
}

func (w *Wheel) pick() {
	if w.OnPick != nil {
		w.OnPick(w.Colour())
	}
}

/*
circle is the disc as it is drawn: centred in the widget, square, as large as
the smaller side allows.

One definition, used by the drawing, the picking and the marker. Three copies
of this arithmetic is how the pointer and the picture came to disagree.
*/
func (w *Wheel) circle() (fyne.Position, float64) {
	size := w.Size()
	side := min32(size.Width, size.Height)
	return fyne.NewPos(size.Width/2, size.Height/2), float64(side) / 2
}

// Refresh redraws the disc at the current brightness and moves the marker.
func (w *Wheel) Refresh() {
	if w.disc != nil {
		w.disc.Image = w.image(wheelPixels)
		w.disc.Refresh()
	}
	w.place()
	w.BaseWidget.Refresh()
}

// Resize keeps the marker where it belongs when the window changes. The
// renderer lays the disc out; this is only the marker.
func (w *Wheel) Resize(size fyne.Size) {
	w.BaseWidget.Resize(size)
	w.place()
}

const markerSize = 10

func (w *Wheel) place() {
	if w.marker == nil {
		return
	}
	centre, radius := w.circle()
	angle := w.hue * math.Pi / 180
	x := float64(centre.X) + math.Cos(angle)*w.sat*radius
	y := float64(centre.Y) + math.Sin(angle)*w.sat*radius

	w.marker.Resize(fyne.NewSize(markerSize, markerSize))
	w.marker.Move(fyne.NewPos(float32(x)-markerSize/2, float32(y)-markerSize/2))
	w.marker.Refresh()
}

// image draws the disc: hue by angle, saturation by distance, at the current
// brightness. Outside the circle is transparent, so the widget is round.
func (w *Wheel) image(size int) image.Image {
	out := image.NewNRGBA(image.Rect(0, 0, size, size))
	radius := float64(size) / 2

	for y := range size {
		for x := range size {
			dx, dy := float64(x)-radius, float64(y)-radius
			distance := math.Hypot(dx, dy)
			if distance > radius {
				continue
			}
			hue := math.Mod(math.Atan2(dy, dx)*180/math.Pi+360, 360)
			r, g, b := fromHSV(hue, distance/radius, w.value)
			out.SetNRGBA(x, y, color.NRGBA{R: r, G: g, B: b, A: 255})
		}
	}
	return out
}

// fromHSV is the usual conversion, in the ranges this widget uses: hue in
// degrees, the other two from zero to one.
func fromHSV(hue, sat, value float64) (r, g, b uint8) {
	c := value * sat
	x := c * (1 - math.Abs(math.Mod(hue/60, 2)-1))
	m := value - c

	var rf, gf, bf float64
	switch {
	case hue < 60:
		rf, gf, bf = c, x, 0
	case hue < 120:
		rf, gf, bf = x, c, 0
	case hue < 180:
		rf, gf, bf = 0, c, x
	case hue < 240:
		rf, gf, bf = 0, x, c
	case hue < 300:
		rf, gf, bf = x, 0, c
	default:
		rf, gf, bf = c, 0, x
	}
	return byteOf(rf + m), byteOf(gf + m), byteOf(bf + m)
}

// toHSV is the reverse, for opening the wheel on a colour that already exists.
func toHSV(r, g, b uint8) (hue, sat, value float64) {
	rf, gf, bf := float64(r)/255, float64(g)/255, float64(b)/255
	maximum := math.Max(rf, math.Max(gf, bf))
	minimum := math.Min(rf, math.Min(gf, bf))
	span := maximum - minimum

	switch {
	case span == 0:
		hue = 0
	case maximum == rf:
		hue = math.Mod((gf-bf)/span*60+360, 360)
	case maximum == gf:
		hue = (bf-rf)/span*60 + 120
	default:
		hue = (rf-gf)/span*60 + 240
	}
	if maximum > 0 {
		sat = span / maximum
	}
	return hue, sat, maximum
}

func byteOf(v float64) uint8 { return uint8(math.Round(clamp01(v) * 255)) }

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }

func min32(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}
