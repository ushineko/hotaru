/*
Package render draws the effects hotaru shows on a device that has none of its
own.

Most lit devices carry their effects in firmware, and hotaru switches them
into one through OpenRGB. A canvas device does not: its vendor's software
draws every effect on the host and streams the result, one frame at a time,
and the device holds whatever it was sent last (sanshoku spec 010). On such a
device an effect is hotaru's to draw, and this package is where it is drawn.

An effect here is a pure function of time and the device's lights. There is
no I/O, no goroutine and no clock in this package: the animator owns the
clock and the canvas, and a test asks for a frame at any instant it likes
without waiting for it.

Nothing here names a product. A canvas is a list of lights in the device's
order, and every effect is written against that and nothing else.
*/
package render

import (
	"math"
	"strings"
	"time"

	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/sanshoku/lighting"
)

/*
Params are what a scene says about the effect, in the terms a scene already
has: the colours its assignments composed, the effect's own colour and its
speed, and the device rule's brightness.
*/
type Params struct {
	// Colours are one per light, in the canvas's order: the frame a scene's
	// assignments composed. Static shows them.
	Colours []colour.Colour

	// Colour is the effect's own colour, for an effect that shows one. Nil
	// falls back to the frame, as a firmware mode with a colour of its own
	// falls back to the colour most of the scene is.
	Colour *colour.Colour

	// Speed is 0 to 100, slowest to fastest. Nil is DefaultSpeed.
	Speed *int

	// Brightness is 0 to 100, a multiply on every channel as it is in
	// SteelSeries GG. Nil is full brightness.
	Brightness *int
}

/*
Effect is one effect: the frame it shows at t, measured from when it started.

Every light in keys has a pixel in the result, in the same order, so a light
an effect has no opinion about is drawn black rather than left showing an
earlier effect's colour.
*/
type Effect func(t time.Duration, keys []lighting.Key, p Params) []lighting.Pixel

/*
Renderer is an effect with what the rest of hotaru needs to know about it.

The flags are the ones a firmware mode carries, so a canvas device can offer
its renderers as its modes and every shell, scene and editor reads them the
way it reads a keyboard's own.
*/
type Renderer struct {
	// Name matches OpenRGB's where the two mean the same, so a scene that
	// says Breathing means the same thing on every device.
	Name string
	Draw Effect

	// Animated is an effect whose frame changes with t. One that does not is
	// drawn once, and the device holds it.
	Animated bool
	// PerLight is an effect that shows the frame's own colours.
	PerLight bool
	// OneColour is an effect that shows one colour of its own.
	OneColour bool
	// Paced is an effect that takes a speed.
	Paced bool
	// Blank is the effect that turns the lights off.
	Blank bool
}

// The speed range every paced effect takes. A percentage, because these are
// hotaru's own effects and there is no device unit to stay faithful to.
const (
	SlowestSpeed = 0
	FastestSpeed = 100
	DefaultSpeed = 50
)

/*
The periods each paced effect runs at, at the slowest and the fastest speed.

Chosen by eye against what OpenRGB's own effects and SteelSeries GG show at
their defaults, and not measured: an effect's pace is a matter of taste, and
the speed is how a person adjusts it.
*/
const (
	breathingSlowest = 8 * time.Second
	breathingFastest = time.Second

	spectrumSlowest = 30 * time.Second
	spectrumFastest = 3 * time.Second

	// The wave's speed is in lights a second, because "moves one light per
	// step" is the property a person sees and the one a test can check.
	waveSlowest = 2.0
	waveFastest = 40.0
)

// Off, Static, Breathing, Spectrum and Wave are the renderers' names.
const (
	Off       = "Off"
	Static    = "Static"
	Breathing = "Breathing"
	Spectrum  = "Spectrum"
	Wave      = "Rainbow Wave"
)

/*
Renderers is every effect hotaru draws, in the order a picker offers them.

A deliberate short list (spec 060 R2.2). Effects that react to key presses
are not here: they read the keyboard's input, which is a question of
permissions and privacy and not of drawing.

Off is listed because "off" is something a scene asks of every device, and a
canvas device turns its lights off by being sent a black frame.
*/
func Renderers() []Renderer {
	return []Renderer{
		{Name: Static, Draw: static, PerLight: true},
		{Name: Breathing, Draw: breathing, Animated: true, OneColour: true, Paced: true},
		{Name: Spectrum, Draw: spectrum, Animated: true, Paced: true},
		{Name: Wave, Draw: wave, Animated: true, Paced: true},
		{Name: Off, Draw: blank, Blank: true},
	}
}

// Lookup is a renderer by name, matched case-insensitively as a mode's name
// is everywhere else in hotaru.
func Lookup(name string) (Renderer, bool) {
	for _, r := range Renderers() {
		if strings.EqualFold(r.Name, name) {
			return r, true
		}
	}
	return Renderer{}, false
}

// static is the frame as composed, or the effect's colour where there is no
// frame to show.
func static(_ time.Duration, keys []lighting.Key, p Params) []lighting.Pixel {
	return paint(keys, p, p.base)
}

/*
breathing is the colour scaled by a sine: dark at t=0, full at half the period,
dark again at the period.

Starting dark is deliberate. A scene applied while the device shows something
else fades in from nothing, where starting at full would jump to the colour
and then fade.
*/
func breathing(t time.Duration, keys []lighting.Key, p Params) []lighting.Pixel {
	period := pace(p, breathingSlowest, breathingFastest)
	phase := 2 * math.Pi * float64(t%period) / float64(period)
	scale := (1 - math.Cos(phase)) / 2
	return paint(keys, p, func(i int) colour.Colour {
		c := p.base(i)
		if p.Colour != nil {
			c = *p.Colour
		}
		return scaled(c, scale)
	})
}

// spectrum is every light one hue, going round the colour wheel once a period.
func spectrum(t time.Duration, keys []lighting.Key, p Params) []lighting.Pixel {
	period := pace(p, spectrumSlowest, spectrumFastest)
	c := hue(360 * float64(t%period) / float64(period))
	return paint(keys, p, func(int) colour.Colour { return c })
}

/*
wave is one rainbow across the lights, moving towards the end of the list.

The hue is set by a light's place in the canvas's order (spec 060 R2.3). After
1/v seconds every light shows the colour its predecessor showed: that is what
moving one light per step means, and the shift is worked out in whole
nanoseconds so that a step lands on exactly one light and not a rounding error
beside it.
*/
func wave(t time.Duration, keys []lighting.Key, p Params) []lighting.Pixel {
	speed := waveSlowest + (waveFastest-waveSlowest)*float64(speedOf(p)-SlowestSpeed)/(FastestSpeed-SlowestSpeed)
	shift := float64(t.Nanoseconds()) * speed / float64(time.Second)
	width := float64(max(len(keys), 1))
	return paint(keys, p, func(i int) colour.Colour {
		at := math.Mod(float64(i)-shift, width)
		if at < 0 {
			at += width
		}
		return hue(360 * at / width)
	})
}

// blank is black on every light.
func blank(_ time.Duration, keys []lighting.Key, p Params) []lighting.Pixel {
	return paint(keys, p, func(int) colour.Colour { return colour.Black })
}

// paint is a pixel per light, coloured by at and dimmed by the brightness.
func paint(keys []lighting.Key, p Params, at func(i int) colour.Colour) []lighting.Pixel {
	level := 1.0
	if p.Brightness != nil {
		level = float64(min(max(*p.Brightness, 0), 100)) / 100
	}
	out := make([]lighting.Pixel, len(keys))
	for i, key := range keys {
		c := scaled(at(i), level)
		out[i] = lighting.Pixel{ID: key.ID, R: c.R, G: c.G, B: c.B}
	}
	return out
}

// base is the light's own colour in the frame, or the effect's colour where
// the frame does not reach that light.
func (p Params) base(i int) colour.Colour {
	if i < len(p.Colours) {
		return p.Colours[i]
	}
	if p.Colour != nil {
		return *p.Colour
	}
	return colour.Black
}

// speedOf is the speed asked for, inside the range.
func speedOf(p Params) int {
	if p.Speed == nil {
		return DefaultSpeed
	}
	return min(max(*p.Speed, SlowestSpeed), FastestSpeed)
}

// pace is a period between slowest and fastest, at the speed asked for.
func pace(p Params, slowest, fastest time.Duration) time.Duration {
	fraction := float64(speedOf(p)-SlowestSpeed) / (FastestSpeed - SlowestSpeed)
	return slowest - time.Duration(fraction*float64(slowest-fastest))
}

// scaled is a colour with every channel multiplied by f, rounded to nearest.
func scaled(c colour.Colour, f float64) colour.Colour {
	channel := func(v uint8) uint8 { return uint8(math.Round(float64(v) * f)) }
	return colour.Colour{R: channel(c.R), G: channel(c.G), B: channel(c.B)}
}

// hue is the fully saturated, full-value colour at h degrees round the wheel.
func hue(h float64) colour.Colour {
	h = math.Mod(h, 360)
	if h < 0 {
		h += 360
	}
	x := 1 - math.Abs(math.Mod(h/60, 2)-1)
	var r, g, b float64
	switch {
	case h < 60:
		r, g = 1, x
	case h < 120:
		r, g = x, 1
	case h < 180:
		g, b = 1, x
	case h < 240:
		g, b = x, 1
	case h < 300:
		r, b = x, 1
	default:
		r, b = 1, x
	}
	byte255 := func(v float64) uint8 { return uint8(math.Round(v * 255)) }
	return colour.Colour{R: byte255(r), G: byte255(g), B: byte255(b)}
}
