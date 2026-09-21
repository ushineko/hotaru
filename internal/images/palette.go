package images

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"math"
	"os"
)

/*
Reading a picture as lighting.

A scene built by hand is a colour per zone, chosen one at a time, and somebody
who wants their machine to match a wallpaper has to do that six times and
compromise. The picture already knows what its colours are.

Two things follow, and the second is the interesting one. Sampling *across* the
picture rather than averaging it keeps the difference between a red storm and a
blue sky: an average of those two is mud, and a machine lit with mud is a
machine that looks broken. And a run of lights is a run across the picture, so
a twenty-four light ring carries the same left-to-right sweep the image has.
*/

/*
Scan samples a picture into n colours, left to right.

Each one is the mean of a vertical slice, which is what an ambient light does
with a screen and for the same reason: a slice is what that part of the picture
looks like from across the room.
*/
func Scan(picture image.Image, n int) []color.NRGBA {
	if n <= 0 {
		return nil
	}
	bounds := picture.Bounds()
	out := make([]color.NRGBA, 0, n)

	for i := range n {
		from := bounds.Min.X + bounds.Dx()*i/n
		to := bounds.Min.X + bounds.Dx()*(i+1)/n
		if to <= from {
			to = from + 1
		}
		out = append(out, mean(picture, image.Rect(from, bounds.Min.Y, to, bounds.Max.Y)))
	}
	return out
}

/*
mean is the colour a region reads as, weighted by how much colour each pixel
has.

A plain average is the wrong instrument for a photograph. Pluto is a rust
planet on black space, and half of every vertical slice through it is space --
averaged, the rust comes out as a grey-brown nobody would call "the colour of
that picture". Weighting each pixel by its chroma asks the region what colour
it *has* rather than what it averages to, so an unlit background contributes
nothing and the subject wins.

A region with no colour in it at all weighs nothing, and falls back to the
plain average, which for grey is grey.
*/
func mean(picture image.Image, region image.Rectangle) color.NRGBA {
	var rs, gs, bs, weights float64
	var flatR, flatG, flatB, count float64
	step := max(1, region.Dy()/64) // enough samples for an average, few enough to be free

	for y := region.Min.Y; y < region.Max.Y; y += step {
		for x := region.Min.X; x < region.Max.X; x++ {
			r, g, b, _ := picture.At(x, y).RGBA()
			fr, fg, fb := float64(r>>8), float64(g>>8), float64(b>>8)
			flatR, flatG, flatB, count = flatR+fr, flatG+fg, flatB+fb, count+1

			chroma := (math.Max(fr, math.Max(fg, fb)) - math.Min(fr, math.Min(fg, fb))) / 255
			rs, gs, bs = rs+fr*chroma, gs+fg*chroma, bs+fb*chroma
			weights += chroma
		}
	}
	switch {
	case weights > 0:
		return color.NRGBA{
			R: uint8(rs / weights), G: uint8(gs / weights), B: uint8(bs / weights), A: 255,
		}
	case count > 0:
		return color.NRGBA{
			R: uint8(flatR / count), G: uint8(flatG / count), B: uint8(flatB / count), A: 255,
		}
	default:
		return color.NRGBA{A: 255}
	}
}

/*
Lit raises a colour to something a light can show.

A photograph is mostly midtones and shadow, and an LED given a shadow is an LED
that is off. What somebody means by "match this picture" is the picture's
*colours*, at the brightness a light works at -- so the hue and the relative
saturation are kept and the value is lifted to the floor.

Grey stays grey rather than becoming an arbitrary hue: a picture with no colour
in a region should light that region white, not invent one.
*/
func Lit(c color.NRGBA, floor float64) color.NRGBA {
	r, g, b := float64(c.R)/255, float64(c.G)/255, float64(c.B)/255
	value := math.Max(r, math.Max(g, b))
	if value >= floor || value == 0 {
		if value == 0 {
			// Pure black lights nothing. A dim white is the honest reading of
			// "this part of the picture has no colour to give".
			level := uint8(floor * 255)
			return color.NRGBA{R: level, G: level, B: level, A: 255}
		}
		return c
	}

	lift := floor / value
	return color.NRGBA{
		R: uint8(math.Min(255, r*lift*255)),
		G: uint8(math.Min(255, g*lift*255)),
		B: uint8(math.Min(255, b*lift*255)),
		A: 255,
	}
}

// Brightness is the floor a scanned colour is lifted to. Chosen by looking at
// a dark wallpaper on a lit machine: below this the room shows nothing.
const Brightness = 0.55

// First is a stored picture's first frame, which is what a scene is built
// from: an animation's colours are its opening colours.
func First(path string) (image.Image, error) {
	body, err := os.ReadFile(path) //nolint:gosec // a path from the library
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	decoded, err := gif.Decode(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return decoded, nil
}

/*
Separate pushes a run of colours further apart.

Wanted because what a person picks and what an LED shows are not the same
thing. A strip's colour is filtered through a diffuser, a case window and
whatever else is lit in the room, and a set of colours that differ clearly on
a screen can arrive as one wash on the hardware -- more so on cheaper
controllers, where the dimmer channels have less to say.

So this is a knob for the eye rather than a correction with a right answer.
Each colour is moved away from the run's average: `distance` of 1 leaves them
as they were measured, 2 doubles every colour's difference from the mean.

**Hue and saturation move freely; value is held above the floor.** A
separation that darkened a light into invisibility would be trading the
difference between two lights for the disappearance of one, and the panel's
own lesson applies -- an LED given a shadow is an LED that is off.

The mean hue is a circular mean, because hues are angles: the average of red
at 350 degrees and red at 10 is red, not cyan.
*/
func Separate(colours []color.NRGBA, distance, floor float64) []color.NRGBA {
	if len(colours) == 0 || distance == 1 {
		return colours
	}

	hues, sats, vals := make([]float64, len(colours)), make([]float64, len(colours)), make([]float64, len(colours))
	var sinH, cosH, sumS, sumV float64
	for i, c := range colours {
		hues[i], sats[i], vals[i] = toHSV(c)
		radians := hues[i] * math.Pi / 180
		sinH, cosH = sinH+math.Sin(radians), cosH+math.Cos(radians)
		sumS, sumV = sumS+sats[i], sumV+vals[i]
	}

	n := float64(len(colours))
	meanH := math.Atan2(sinH/n, cosH/n) * 180 / math.Pi
	meanS, meanV := sumS/n, sumV/n

	out := make([]color.NRGBA, len(colours))
	for i := range colours {
		h := meanH + turn(hues[i]-meanH)*distance
		s := clamp01(meanS + (sats[i]-meanS)*distance)
		v := math.Max(floor, clamp01(meanV+(vals[i]-meanV)*distance))
		out[i] = fromHSV(h, s, v)
	}
	return out
}

// MostDistance is as far apart as this will push a run of colours. Beyond it
// every colour is at one end of its range and the run stops being the
// picture's.
const MostDistance = 3

// turn is an angle wrapped to the shortest way round, so a hue either side of
// zero is a small difference rather than a nearly complete circle.
func turn(degrees float64) float64 {
	for degrees > 180 {
		degrees -= 360
	}
	for degrees < -180 {
		degrees += 360
	}
	return degrees
}

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }

// toHSV is hue in degrees, saturation and value in 0..1.
func toHSV(c color.NRGBA) (h, s, v float64) {
	r, g, b := float64(c.R)/255, float64(c.G)/255, float64(c.B)/255
	high := math.Max(r, math.Max(g, b))
	low := math.Min(r, math.Min(g, b))
	chroma := high - low

	switch {
	case chroma == 0:
		h = 0
	case high == r:
		h = 60 * math.Mod((g-b)/chroma+6, 6)
	case high == g:
		h = 60 * ((b-r)/chroma + 2)
	default:
		h = 60 * ((r-g)/chroma + 4)
	}
	if high > 0 {
		s = chroma / high
	}
	return h, s, high
}

func fromHSV(h, s, v float64) color.NRGBA {
	h = math.Mod(math.Mod(h, 360)+360, 360)
	chroma := v * s
	x := chroma * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := v - chroma

	var r, g, b float64
	switch {
	case h < 60:
		r, g, b = chroma, x, 0
	case h < 120:
		r, g, b = x, chroma, 0
	case h < 180:
		r, g, b = 0, chroma, x
	case h < 240:
		r, g, b = 0, x, chroma
	case h < 300:
		r, g, b = x, 0, chroma
	default:
		r, g, b = chroma, 0, x
	}
	return color.NRGBA{
		R: uint8(math.Round((r + m) * 255)),
		G: uint8(math.Round((g + m) * 255)),
		B: uint8(math.Round((b + m) * 255)),
		A: 255,
	}
}

// Hue and Turn are the wheel, exported for tests: a test that measured hues
// with its own arithmetic would be testing that arithmetic.
func Hue(c color.NRGBA) float64 { h, _, _ := toHSV(c); return h }

// Turn wraps an angle to the shortest way round.
func Turn(degrees float64) float64 { return turn(degrees) }
