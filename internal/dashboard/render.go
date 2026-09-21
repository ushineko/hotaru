package dashboard

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"image"
	"image/gif"
	"math"
	"strconv"
	"time"
)

/*
Reading is what the screen shows.

Every field can be absent, and an absent one draws a placeholder: the panel is
decorative, and a render fault must not disturb telemetry, reconciliation or
lighting.
*/
type Reading struct {
	Coolant   float64
	CoolantOK bool
	CPU       int
	CPUOK     bool
	GPU       int
	GPUOK     bool
	PumpRPM   int
	PumpOK    bool
}

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

const placeholder = "--"

func temperature(v float64, ok bool) string {
	if !ok {
		return placeholder
	}
	return fmt.Sprintf("%.1f", v)
}

func whole(v int, ok bool) string {
	if !ok {
		return placeholder
	}
	return strconv.Itoa(v)
}

/*
Render draws a reading, and says what it says.

The tick is drawn last and deliberately excluded from Content. A screen that
has stopped being written is otherwise indistinguishable from an idle machine
-- coolant, pump and fan hold steady for hours -- and most of the time spent
proving this worked went on not knowing whether a frame had landed. But an
indicator that changed every render would also defeat the gate that stops
hotaru writing when nothing has changed, so it advances only with accepted
pushes and is not part of what "changed" means.
*/
func Render(r Reading, tick int) Frame {
	img := image.NewPaletted(image.Rect(0, 0, Size, Size), palette())
	copy(img.Pix, background().Pix)

	colour := coolantColour(r.Coolant, r.CoolantOK)
	centre, radius := float64(Size)/2, float64(Size-2*margin)/2

	// A single arc carrying the headline's severity, legible across a room
	// without reading the number.
	arc(img, centre, centre, radius, ringWidth, 0, 2*math.Pi, colEdge)
	if r.CoolantOK {
		fraction := math.Max(0, math.Min(1, (r.Coolant-20)/50))
		arc(img, centre, centre, radius, ringWidth, math.Pi/2, -2*math.Pi*fraction, colour)
	}

	centred(img, "COOLANT", 0, 132, Size, 36, 22, false, colMuted)
	centred(img, temperature(r.Coolant, r.CoolantOK), 0, 170, Size, 158, 118, true, colour)
	centred(img, "°C", 0, 330, Size, 38, 27, false, colMuted)

	/*
		CPU is never colour-graded: a chip boosting to 100 C is normal and
		reddening it would cry wolf on every compile. A pump reading zero is
		the opposite -- the most alarming number this screen can show.
	*/
	pumpColour := colAccent
	if r.PumpOK && r.PumpRPM == 0 {
		pumpColour = colCrit
	}
	metric(img, 0, "CPU", whole(r.CPU, r.CPUOK), "°C", colAccent)
	metric(img, 1, "GPU", whole(r.GPU, r.GPUOK), "°C", colAccent)
	metric(img, 2, "PUMP", whole(r.PumpRPM, r.PumpOK), "RPM", pumpColour)

	content := sha256.Sum256(img.Pix)
	drawTick(img, tick)

	var buf bytes.Buffer
	_ = gif.EncodeAll(&buf, &gif.GIF{
		Image: []*image.Paletted{img}, Delay: []int{100}, LoopCount: 0,
		Config: image.Config{ColorModel: img.Palette, Width: Size, Height: Size},
	})
	return Frame{GIF: buf.Bytes(), Content: content, Taken: time.Now()}
}

/*
metric draws one of the bottom columns.

The value font is smaller at three columns than it was at two, because a
four-digit pump reading at the old size overflows its width and collides with
its neighbour. The inset clears the ring, which at this height curves inward
far enough to have been drawn through the word "RPM".
*/
func metric(img *image.Paletted, slot int, label, value, unit string, c interface{ RGBA() (r, g, b, a uint32) }) {
	width := (Size - 2*metricInset) / metricSlots
	x := metricInset + slot*width
	centred(img, label, x, 416, width, 28, 16, false, colMuted)
	centred(img, value, x, 446, width, 66, 34, true, c)
	centred(img, unit, x, 516, width, 26, 14, false, colMuted)
}

// drawTick marks that the screen was written, which nothing else on it does.
func drawTick(img *image.Paletted, tick int) {
	for i := range 12 {
		c := colMuted
		if i == ((tick%12)+12)%12 {
			c = colBright
		}
		for y := 600; y < 608; y++ {
			for x := 236 + i*14; x < 236+i*14+9; x++ {
				img.Set(x, y, c)
			}
		}
	}
}
