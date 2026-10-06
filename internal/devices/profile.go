package devices

import (
	"math"

	"github.com/ushineko/hotaru/internal/colour"
)

/*
Profile corrects the colours written to a device whose lights wash out
(spec 066).

Saturation and Value are how far toward 100% each is pushed, from 0 (as
written) to 1 (always 100%). Red, Green and Blue are a gamma per channel,
applied after them; 0 is unset, which is 1. The zero Profile changes nothing.

Measured on one desk: to look like the NZXT fans' #80aad1, the RTX 4090's
LEDs needed #003eff. Saturation and value were far apart, and the hue was 16°
off because the card's green shows its middle levels too bright. The LEDs
show a smaller range of colour, and a scene should still say the colour that
was meant.
*/
type Profile struct {
	Saturation float64
	Value      float64

	Red, Green, Blue float64
}

// Zero is a profile that changes nothing.
func (p Profile) Zero() bool {
	return p.Saturation == 0 && p.Value == 0 && flat(p.Red) && flat(p.Green) && flat(p.Blue)
}

// flat is a curve that leaves its channel alone.
func flat(gamma float64) bool { return gamma == 0 || gamma == 1 }

/*
Apply is the colour to write so the device shows c.

Worked in HSV without computing a hue. Every channel x of a colour is
V·(1 − S·t), where t = (max − x)/(max − min) is the channel's place between
the brightest and the dimmest, and t is what fixes the hue. Keeping each t and
changing S and V keeps the hue exactly.

Black stays black, and saturation and value never give a grey a hue: a light
asked to be dark is not lit. A grey's value still moves. The curve comes last
and keeps off and full as they are, so black, white and the pure colours pass
it unchanged; a curve on one channel does tint a grey, which is what it is
for.
*/
func (p Profile) Apply(c colour.Colour) colour.Colour {
	if p.Zero() {
		return c
	}
	if c == colour.Black {
		return c
	}
	r, g, b := float64(c.R)/255, float64(c.G)/255, float64(c.B)/255
	hi, lo := max(r, g, b), min(r, g, b)
	s := (hi - lo) / hi
	v := hi + (1-hi)*p.Value
	if s > 0 {
		s += (1 - s) * p.Saturation
	}
	channel := func(x float64) float64 {
		t := 0.0
		if hi > lo {
			t = (hi - x) / (hi - lo)
		}
		return v * (1 - s*t)
	}
	return colour.Colour{R: curved(channel(r), p.Red), G: curved(channel(g), p.Green), B: curved(channel(b), p.Blue)}
}

// curved is one channel, from 0 to 1, through its gamma, as a byte.
func curved(x, gamma float64) uint8 {
	if !flat(gamma) {
		x = math.Pow(x, gamma)
	}
	return uint8(math.Round(x * 255))
}

// Colours is Apply over a list, into a new one.
func (p Profile) Colours(in []colour.Colour) []colour.Colour {
	if in == nil {
		return nil
	}
	out := make([]colour.Colour, len(in))
	for i, c := range in {
		out[i] = p.Apply(c)
	}
	return out
}
