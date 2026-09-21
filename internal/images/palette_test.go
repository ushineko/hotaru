package images_test

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/images"
)

// halves is a picture with a red left and a blue right, which is the case
// averaging destroys.
func halves() image.Image {
	picture := image.NewRGBA(image.Rect(0, 0, 100, 40))
	for y := range 40 {
		for x := range 100 {
			c := color.RGBA{R: 255, A: 255}
			if x >= 50 {
				c = color.RGBA{B: 255, A: 255}
			}
			picture.Set(x, y, c)
		}
	}
	return picture
}

func TestScanningKeepsWhatAveragingDestroys(t *testing.T) {
	/*
		An average of a red storm on a blue sky is mud, and a machine lit with
		mud looks broken rather than themed. Sampling across the picture keeps
		the difference, which is the whole idea.
	*/
	got := images.Scan(halves(), 4)
	require.Len(t, got, 4)

	require.Greater(t, got[0].R, uint8(200), "the left of the picture is red")
	require.Less(t, got[0].B, uint8(50))
	require.Greater(t, got[3].B, uint8(200), "the right of the picture is blue")
	require.Less(t, got[3].R, uint8(50))
}

func TestOneSampleIsTheWholePicture(t *testing.T) {
	// A single-light zone -- a logo -- gets the picture's average, which is
	// the only honest answer when there is one light to say it with.
	got := images.Scan(halves(), 1)
	require.Len(t, got, 1)
	require.InDelta(t, 127, got[0].R, 12)
	require.InDelta(t, 127, got[0].B, 12)
}

func TestAShadowIsLiftedToSomethingALightCanShow(t *testing.T) {
	/*
		A photograph is mostly midtones and shadow, and an LED given a shadow
		is an LED that is off. What somebody means by "match this picture" is
		its colours at the brightness a light works at.
	*/
	dark := color.NRGBA{R: 40, G: 10, B: 10, A: 255}
	lit := images.Lit(dark, images.Brightness)

	require.Greater(t, lit.R, dark.R)
	require.InDelta(t, float64(dark.G)/float64(dark.R), float64(lit.G)/float64(lit.R), 0.05,
		"lifting a shadow changed its colour")
}

func TestABrightColourIsLeftAlone(t *testing.T) {
	bright := color.NRGBA{R: 255, G: 80, B: 0, A: 255}
	require.Equal(t, bright, images.Lit(bright, images.Brightness))
}

func TestBlackLightsAWhiteRatherThanInventingAHue(t *testing.T) {
	// A picture with no colour in a region should light that region white.
	// The alternative is a hue nobody chose, from arithmetic on zero.
	lit := images.Lit(color.NRGBA{A: 255}, images.Brightness)
	require.Equal(t, lit.R, lit.G)
	require.Equal(t, lit.G, lit.B)
	require.Positive(t, lit.R)
}

// planet is a photograph's shape: a saturated subject on a dark background,
// which is what a plain average turns to mud.
func planet() image.Image {
	picture := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := range 100 {
		for x := range 100 {
			c := color.RGBA{R: 8, G: 8, B: 20, A: 255} // space
			if (x-50)*(x-50)+(y-50)*(y-50) < 30*30 {
				c = color.RGBA{R: 200, G: 70, B: 30, A: 255} // rust
			}
			picture.Set(x, y, c)
		}
	}
	return picture
}

func TestASubjectOnADarkBackgroundKeepsItsColour(t *testing.T) {
	/*
		Half of every slice through a planet is empty space, and averaging
		that in reads the rust as a grey-brown. Weighting by how much colour
		each pixel carries asks the slice what colour it has.
	*/
	got := images.Scan(planet(), 4)
	require.Len(t, got, 4)

	middle := got[1]
	require.Greater(t, middle.R, uint8(150), "the planet is rust, not grey-brown")
	require.Greater(t, float64(middle.R)/float64(middle.B), 3.0)
}

func TestSeparationLeavesColoursAloneAtOne(t *testing.T) {
	// The knob's resting position: as measured.
	in := []color.NRGBA{{R: 200, G: 40, B: 40, A: 255}, {R: 40, G: 200, B: 40, A: 255}}
	require.Equal(t, in, images.Separate(in, 1, images.Brightness))
}

func TestSeparationPushesHuesApart(t *testing.T) {
	/*
		What a person picks and what an LED shows are not the same thing: a
		strip's colour is filtered through a diffuser, a case window and
		whatever else is lit in the room, and colours that differ clearly on
		a screen can arrive as one wash.
	*/
	in := []color.NRGBA{{R: 200, G: 120, B: 60, A: 255}, {R: 200, G: 160, B: 60, A: 255}}
	out := images.Separate(in, 2.5, images.Brightness)

	require.Greater(t, hueGap(out[0], out[1]), hueGap(in[0], in[1]),
		"the hues came out no further apart than they went in")
}

func TestSeparationDoesNotPutALightOut(t *testing.T) {
	/*
		Trading the difference between two lights for the disappearance of
		one is not a trade. An LED given a shadow is an LED that is off, and
		that is the panel's lesson as much as the case's.
	*/
	in := []color.NRGBA{{R: 20, G: 20, B: 30, A: 255}, {R: 220, G: 220, B: 230, A: 255}}
	out := images.Separate(in, 3, images.Brightness)

	for i, c := range out {
		value := max(max(c.R, c.G), c.B)
		require.GreaterOrEqual(t, float64(value)/255, images.Brightness-0.01,
			"colour %d was separated into the dark", i)
	}
}

func TestSeparationKeepsGreyGrey(t *testing.T) {
	// A run with no colour in it has nothing to push apart, and inventing a
	// hue from arithmetic on zero is what the sampler already refuses to do.
	in := []color.NRGBA{{R: 128, G: 128, B: 128, A: 255}, {R: 160, G: 160, B: 160, A: 255}}
	for _, c := range images.Separate(in, 3, images.Brightness) {
		require.Equal(t, c.R, c.G)
		require.Equal(t, c.G, c.B)
	}
}

func TestHuesEitherSideOfZeroAreClose(t *testing.T) {
	/*
		Hues are angles: the average of red at 350 degrees and red at 10 is
		red, not cyan. A mean taken without that gives every colour in the
		run a deviation of about 180 degrees, and separation sends them all
		to the far side of the wheel.
	*/
	in := []color.NRGBA{{R: 255, G: 0, B: 30, A: 255}, {R: 255, G: 30, B: 0, A: 255}}
	out := images.Separate(in, 2, images.Brightness)

	for i, c := range out {
		require.Greater(t, c.R, c.G, "colour %d stopped being red", i)
		require.Greater(t, c.R, c.B, "colour %d stopped being red", i)
	}
}

// hueGap is how far apart two colours are on the wheel, in degrees.
func hueGap(a, b color.NRGBA) float64 {
	return math.Abs(images.Turn(images.Hue(a) - images.Hue(b)))
}
