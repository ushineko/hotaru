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
	value, known := r.Value(d.Headline.Source)
	colour := gradeOf(d.Headline.Source, value, known, p.theme)

	if ring {
		drawRings(p, d, r)
	}

	p.label(d.Headline.Words(d.Units), 0, at.labelY, Size, 36, 22)
	p.fields(d.Headline.Fields(r, d.Units), headlineInset, at.valueY, Size-2*headlineInset,
		at.valueH, at.size, colour, graded(d.Headline.Source, value, known), Centre)
}

/*
headlineAt is where an arrangement puts the headline.

Coordinates rather than an offset from the label, because the ring
arrangement's are spec 013's exact numbers -- arrived at by looking at the
panel in a case -- and deriving them from each other would move one of them
by two pixels for the sake of tidiness.

The unit line that used to sit below the value is gone (spec 041), and its
coordinate with it. The others are untouched: the number stays exactly where
spec 013 put it, and what is under it is now whatever the arrangement draws
there.
*/
type headlineAt struct {
	labelY, valueY, valueH int
	size                   float64
}

// drawRing is spec 013's screen: the ring, the headline inside it, and three
// columns along the bottom.
func drawRing(p *paint, d Dashboard, r Reading) {
	headline(p, d, r, headlineAt{labelY: 132, valueY: 170, valueH: 158, size: 118}, true)

	slots := fit(d, Ring)
	width := (Size - 2*metricInset) / metricSlots
	for i, slot := range slots {
		x := metricInset + i*width
		column(p, slot, r, d.Units, x, 416, width)
	}
}

/*
column draws one of the bottom readings.

The value font is smaller at three columns than it was at two, because a
four-digit pump reading at the old size overflows its width and collides with
its neighbour -- which is now also handled by the value shrinking to fit, but
the smaller size is what the panel was looked at with. The inset clears the
ring, which at this height curves inward far enough to have been drawn through
the word "RPM".

The unit line that used to sit under the value is gone (spec 041); the label
above carries it.
*/
func column(p *paint, slot Slot, r Reading, units bool, x, y, width int) {
	value, known := r.Value(slot.Source)
	p.label(slot.Words(units), x, y, width, 28, 16)
	p.fields(slot.Fields(r, units), x, y+30, width, 66, 34,
		gradeOf(slot.Source, value, known, p.theme), graded(slot.Source, value, known), Centre)
}

// drawGrid is the headline over four readings in two rows of two.
func drawGrid(p *paint, d Dashboard, r Reading) {
	headline(p, d, r, headlineAt{labelY: 76, valueY: 114, valueH: 116, size: 88}, true)

	slots := fit(d, Grid)
	width := (Size - 2*gridInset) / 2
	for i, slot := range slots {
		x := gridInset + (i%2)*width
		y := gridTop + (i/2)*columnHeight
		column(p, slot, r, d.Units, x, y, width)
	}
}

// drawStacked is four rows under the headline, with no ring: numbers rather
// than an instrument.
func drawStacked(p *paint, d Dashboard, r Reading) {
	headline(p, d, r, headlineAt{labelY: 60, valueY: 98, valueH: 116, size: 88}, false)

	/*
		The rows are a table, so they are set like one: the words hard left,
		the numbers hard right, and the space between them absorbing the
		difference.

		Centred in their bands is what they were, and it reads as ragged the
		moment somebody scans down the column -- "2709 / 90" is fifty pixels
		wider than "12 / 63", so each row started and ended somewhere
		different. Four rows of that is four left edges and four right ones.

		The value band runs from the label to the far inset. It used to stop
		at 200 pixels with the unit drawn beyond it; with the unit gone there
		is nothing to leave room for, and a pair needs every pixel of it --
		"1450 / 1450" is eleven characters where the band was sized for four.
	*/
	/*
		The rows are one table, so the column is measured once for all of
		them rather than each row settling its own.

		Two things go wrong when a row is left to itself. A label wider than
		its band overflowed both ways when it was centred and nobody noticed;
		justified it overflows one way, into the number -- "PUMP RPM / %" is
		208 pixels and left three of them before "2709 / 90". And a row that
		shrinks its own value to fit puts three point sizes down one column,
		which reads worse than the ragged edges this was meant to fix.

		So: the words column is as wide as the widest label, the numbers get
		what is left, and they are all drawn at the size the widest of them
		needs. One left edge, one right edge, one size.
	*/
	slots := fit(d, Stacked)
	words := make([]string, len(slots))
	rows := make([][]Field, len(slots))
	labelWidth := rowLabelWidth
	for i, slot := range slots {
		words[i] = slot.Words(d.Units)
		rows[i] = slot.Fields(r, d.Units)
		labelWidth = max(labelWidth, p.textWidth(words[i], 22, false, p.letters.Labels))
	}

	left := rowInset + labelWidth + rowGap
	band := Size - rowInset - left

	/*
		Every field is as wide as the widest of *that* field across every row,
		so the separators land in one column (spec 044).

		Per row, the pump's four digits and the processor's two put their dots
		in different places, and four rows of that is a table with a bend in
		it. One measurement for the column costs the short rows some empty
		space to the left of their numbers, which is what a table of numbers
		looks like.
	*/
	widths, total, size := p.column(rows, band, rowValuePt*p.letters.Values.Scale())

	/*
		The column sits against the right edge of the band, and every row
		starts at the same place inside it -- so the rows that have all the
		fields still end together, which is spec 041's right edge.

		A row with fewer fields ends earlier, and that is the honest cost: a
		screen mixing pairs and single values cannot have both a dot column
		and a right edge, and putting a lone number under the other rows'
		second numbers would be a lie about what it is.
	*/
	start := left + max(band-total, 0)

	for i, slot := range slots {
		y := stackTop + i*rowHeight
		reading, known := r.Value(slot.Source)
		p.labelAt(words[i], rowInset, y, labelWidth, 48, 22, Left)
		p.inColumn(rows[i], widths, start, y, 48, size,
			gradeOf(slot.Source, reading, known, p.theme),
			graded(slot.Source, reading, known))
	}
}

// drawBig is the headline alone, as large as the panel will take.
func drawBig(p *paint, d Dashboard, r Reading) {
	headline(p, d, r, headlineAt{labelY: 170, valueY: 210, valueH: 220, size: 170}, true)
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
