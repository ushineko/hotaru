package images_test

import (
	"image"
	"image/color"
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
