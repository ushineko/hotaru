package images_test

import (
	"image"
	"image/color"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/images"
)

// blocks is a picture of vertical bands, each colour as wide as asked.
func blocks(bands ...band) image.Image {
	width := 0
	for _, b := range bands {
		width += b.width
	}
	picture := image.NewRGBA(image.Rect(0, 0, width, 50))
	x := 0
	for _, b := range bands {
		for end := x + b.width; x < end; x++ {
			for y := range 50 {
				picture.Set(x, y, b.colour)
			}
		}
	}
	return picture
}

type band struct {
	colour color.RGBA
	width  int
}

var (
	red    = color.RGBA{R: 255, A: 255}
	orange = color.RGBA{R: 255, G: 100, A: 255}
	blue   = color.RGBA{B: 255, A: 255}
	green  = color.RGBA{G: 255, A: 255}
)

func nrgba(c color.RGBA) color.NRGBA { return color.NRGBA{R: c.R, G: c.G, B: c.B, A: 255} }

func TestAPictureOfBlocksGivesItsColoursInCoverageOrder(t *testing.T) {
	picture := blocks(band{blue, 20}, band{red, 50}, band{green, 30})
	require.Equal(t,
		[]color.NRGBA{nrgba(red), nrgba(green), nrgba(blue)},
		images.Palette(picture, 4, 1))
}

func TestNoMoreColoursThanAskedFor(t *testing.T) {
	picture := blocks(band{blue, 20}, band{red, 50}, band{green, 30})
	require.Equal(t, []color.NRGBA{nrgba(red), nrgba(green)}, images.Palette(picture, 2, 1))
}

func TestDistanceMergesNearColours(t *testing.T) {
	// Red and an orange 100 apart: two colours at a distance of 1, one at 2,
	// where the red's coverage counts towards the orange's.
	picture := blocks(band{orange, 35}, band{blue, 40}, band{red, 25})
	require.Equal(t,
		[]color.NRGBA{nrgba(blue), nrgba(orange), nrgba(red)},
		images.Palette(picture, 4, 1), "the orange and red were merged at a distance of 1")

	require.Equal(t,
		[]color.NRGBA{nrgba(orange), nrgba(blue)},
		images.Palette(picture, 4, 2), "red and orange together cover more than blue")
}

func TestColourCountsForMoreThanBackground(t *testing.T) {
	// A small red subject on a large black ground: the subject is the colour.
	picture := blocks(band{color.RGBA{A: 255}, 90}, band{red, 10})
	require.Equal(t, []color.NRGBA{nrgba(red)}, images.Palette(picture, 4, 1))
}

func TestAPictureWithNoColourGivesItsGreys(t *testing.T) {
	picture := blocks(band{color.RGBA{R: 200, G: 200, B: 200, A: 255}, 10})
	got := images.Palette(picture, 4, 1)
	require.Len(t, got, 1)
	require.Equal(t, got[0].R, got[0].B, "a grey picture gave a colour")
}

func TestPickingIsTheSameEveryTime(t *testing.T) {
	picture := blocks(band{blue, 25}, band{red, 25}, band{green, 25}, band{orange, 25})
	first := images.Palette(picture, 4, 1)
	for range 5 {
		require.Equal(t, first, images.Palette(picture, 4, 1))
	}
}
