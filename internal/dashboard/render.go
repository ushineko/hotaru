package dashboard

import (
	"bytes"
	"crypto/sha256"
	"image"
	"image/color"
	"image/gif"
	"math"
	"time"

	"github.com/ushineko/hotaru/internal/readings"
)

/*
Reading is what the screen shows.

Every number can be absent, and an absent one draws a placeholder: the panel
is decorative, and a render fault must not disturb telemetry, reconciliation
or lighting.

An alias rather than a type of this package's own: what the machine can say is
the readings package's subject, and the dashboard is one of the things that
asks.
*/
type Reading = readings.Reading

// Frame is a rendered dashboard, ready to send.
type Frame struct {
	// GIF is the encoded image. A GIF, not a still picture: the firmware
	// reverts a static image within seconds and loops a GIF indefinitely.
	GIF []byte
	// Content identifies what the frame says, ignoring the tick indicator.
	// Two readings that look the same have the same Content.
	Content [32]byte
	// Taken is when it was rendered.
	Taken time.Time
}

/*
Render draws a dashboard, and says what it says.

`behind` is the picture for a Picture background, already the panel's size
because the library converts on the way in. Nil falls back to the theme's
plain colour, which is what a picture somebody deleted has to cost: the
background, and not the frame.

The tick is drawn last and deliberately excluded from Content. A screen that
has stopped being written is otherwise indistinguishable from an idle machine
-- coolant, pump and fan hold steady for hours -- and most of the time spent
proving this worked went on not knowing whether a frame had landed. But an
indicator that changed every render would also defeat the gate that stops
hotaru writing when nothing has changed, so it advances only with accepted
pushes and is not part of what "changed" means.
*/
func Render(d Dashboard, r Reading, tick int, behind image.Image) Frame {
	theme := ThemeOf(d.Theme)
	paint := newPaint(d, theme, behind)

	switch d.Arrangement {
	case Grid:
		drawGrid(paint, d, r)
	case Stacked:
		drawStacked(paint, d, r)
	case Big:
		drawBig(paint, d, r)
	default:
		drawRing(paint, d, r)
	}
	caption(paint, d)

	img := paint.img
	content := sha256.Sum256(img.Pix)
	drawTick(img, tick, theme)

	var buf bytes.Buffer
	_ = gif.EncodeAll(&buf, &gif.GIF{
		Image: []*image.Paletted{img}, Delay: []int{100}, LoopCount: 0,
		Config: image.Config{ColorModel: img.Palette, Width: Size, Height: Size},
	})
	return Frame{GIF: buf.Bytes(), Content: content, Taken: time.Now()}
}

/*
headline draws the big number, and the ring around it where there is one.

The ring grades the headline reading between 20 and 70 degrees, which is a
coolant's range. A headline that is not a temperature still gets a ring -- it
is a gauge, and a gauge with nothing in it looks broken -- but it is graded on
its own scale rather than on the coolant's thresholds.
*/
func headline(p *paint, d Dashboard, r Reading, at headlineAt, ring bool) {
	label, unit := d.Headline.Words()
	value, known := r.Value(d.Headline.Source)
	colour := gradeOf(d.Headline.Source, value, known, p.theme)

	if ring {
		drawRings(p, d, r)
	}

	p.label(label, 0, at.labelY, Size, 36, 22)
	p.value(r.Text(d.Headline.Source), 0, at.valueY, Size, at.valueH, at.size,
		colour, graded(d.Headline.Source, value, known))
	p.label(unit, 0, at.unitY, Size, 38, 27)
}

/*
headlineAt is where an arrangement puts the headline.

Coordinates rather than an offset from the label, because the ring
arrangement's are spec 013's exact numbers -- arrived at by looking at the
panel in a case -- and deriving them from each other would move one of them
by two pixels for the sake of tidiness.
*/
type headlineAt struct {
	labelY, valueY, valueH, unitY int
	size                          float64
}

// drawRing is spec 013's screen: the ring, the headline inside it, and three
// columns along the bottom.
func drawRing(p *paint, d Dashboard, r Reading) {
	headline(p, d, r, headlineAt{labelY: 132, valueY: 170, valueH: 158, unitY: 330, size: 118}, true)

	slots := fit(d, Ring)
	width := (Size - 2*metricInset) / metricSlots
	for i, slot := range slots {
		x := metricInset + i*width
		column(p, slot, r, x, 416, width)
	}
}

/*
column draws one of the bottom readings.

The value font is smaller at three columns than it was at two, because a
four-digit pump reading at the old size overflows its width and collides with
its neighbour. The inset clears the ring, which at this height curves inward
far enough to have been drawn through the word "RPM".
*/
func column(p *paint, slot Slot, r Reading, x, y, width int) {
	label, unit := slot.Words()
	value, known := r.Value(slot.Source)
	p.label(label, x, y, width, 28, 16)
	p.value(r.Text(slot.Source), x, y+30, width, 66, 34,
		gradeOf(slot.Source, value, known, p.theme), graded(slot.Source, value, known))
	p.label(unit, x, y+100, width, 26, 14)
}

// drawGrid is the headline over four readings in two rows of two.
func drawGrid(p *paint, d Dashboard, r Reading) {
	headline(p, d, r, headlineAt{labelY: 76, valueY: 114, valueH: 116, unitY: 232, size: 88}, true)

	slots := fit(d, Grid)
	width := (Size - 2*gridInset) / 2
	for i, slot := range slots {
		x := gridInset + (i%2)*width
		y := gridTop + (i/2)*columnHeight
		column(p, slot, r, x, y, width)
	}
}

// drawStacked is four rows under the headline, with no ring: numbers rather
// than an instrument.
func drawStacked(p *paint, d Dashboard, r Reading) {
	headline(p, d, r, headlineAt{labelY: 60, valueY: 98, valueH: 116, unitY: 216, size: 88}, false)

	slots := fit(d, Stacked)
	for i, slot := range slots {
		y := stackTop + i*rowHeight
		label, unit := slot.Words()
		value, known := r.Value(slot.Source)
		p.label(label, rowInset, y, 160, 48, 22)
		p.value(r.Text(slot.Source), rowInset+160, y, 200, 48, 34,
			gradeOf(slot.Source, value, known, p.theme), graded(slot.Source, value, known))
		p.label(unit, rowInset+370, y, 100, 48, 18)
	}
}

// drawBig is the headline alone, as large as the panel will take.
func drawBig(p *paint, d Dashboard, r Reading) {
	headline(p, d, r, headlineAt{labelY: 170, valueY: 210, valueH: 220, unitY: 434, size: 170}, true)
}

// caption is the author's own line, drawn low enough to clear the readings
// and high enough to clear the tick. Empty draws nothing.
func caption(p *paint, d Dashboard) {
	if d.Caption == "" {
		return
	}
	p.label(d.Caption, 0, captionY, Size, 36, 20)
}

/*
fit is the slots an arrangement has room for.

Extra ones are dropped. A dashboard saved against an arrangement with four
slots and then switched to one with three must lose a reading rather than draw
two numbers in the same place -- and keeping the fourth in the file means
switching back brings it home.
*/
func fit(d Dashboard, arrangement string) []Slot {
	room := Slots(arrangement)
	if len(d.Slots) <= room {
		return d.Slots
	}
	return d.Slots[:room]
}

/*
gradeOf is the colour a reading is drawn in.

Only the coolant and a stopped pump carry a grade. A chip boosting to 100 C is
a chip doing its job and reddening it would cry wolf on every compile; a pump
reading zero is the opposite -- the most alarming number this screen can show,
whatever else is calm.
*/
/*
graded says whether a reading's colour carries meaning rather than style.

The coolant's green, amber and red are the alert thresholds, and a pump at
zero is the most alarming thing this screen can say. Those are the colours a
dashboard's own choice must not paint over; everything else is the theme's
accent, which is decoration and may be changed.
*/
func graded(source readings.Source, value float64, known bool) bool {
	switch source {
	case readings.Coolant:
		return true
	case readings.PumpRPM, readings.PumpDuty:
		return known && value == 0
	}
	return false
}

func gradeOf(source readings.Source, value float64, known bool, theme Theme) color.RGBA {
	switch source {
	case readings.Coolant:
		return coolantColour(value, known, theme)
	case readings.PumpRPM, readings.PumpDuty:
		if known && value == 0 {
			return colCrit
		}
	}
	return theme.Accent
}

/*
fraction is how far round the ring a reading sits.

The coolant's scale is 20 to 70 degrees, which is spec 013's. A percentage is
already a fraction. Everything else is graded against a range wide enough to
move: a gauge pinned at either end says nothing.
*/
func fraction(source readings.Source, value float64) float64 {
	span := func(low, high float64) float64 {
		return math.Max(0, math.Min(1, (value-low)/(high-low)))
	}
	switch source {
	case readings.Coolant:
		return span(20, 70)
	case readings.CPULoad, readings.GPULoad, readings.PumpDuty, readings.FanDuty:
		return span(0, 100)
	case readings.CPUTemp, readings.GPUTemp:
		return span(20, 100)
	case readings.PumpRPM:
		return span(0, 3000)
	case readings.FanRPM:
		return span(0, 2000)
	}
	return span(0, 100)
}

// drawTick marks that the screen was written, which nothing else on it does.
func drawTick(img *image.Paletted, tick int, theme Theme) {
	for i := range 12 {
		c := theme.Muted
		if i == ((tick%12)+12)%12 {
			c = theme.Bright
		}
		for y := 600; y < 608; y++ {
			for x := 236 + i*14; x < 236+i*14+9; x++ {
				img.Set(x, y, c)
			}
		}
	}
}
