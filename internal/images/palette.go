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
