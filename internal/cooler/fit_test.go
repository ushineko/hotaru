package cooler

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"testing"

	"github.com/stretchr/testify/require"
)

// animation is a GIF of a given size, with two frames so the loop over frames
// is exercised.
func animation(t *testing.T, width, height int) []byte {
	t.Helper()

	palette := color.Palette{color.RGBA{0, 0, 0, 255}, color.RGBA{255, 0, 0, 255}}
	frames := make([]*image.Paletted, 2)
	for i := range frames {
		frame := image.NewPaletted(image.Rect(0, 0, width, height), palette)
		for y := range height {
			for x := range width {
				if (x+y+i)%2 == 0 {
					frame.SetColorIndex(x, y, 1)
				}
			}
		}
		frames[i] = frame
	}

	var out bytes.Buffer
	require.NoError(t, gif.EncodeAll(&out, &gif.GIF{
		Image: frames, Delay: []int{10, 10}, LoopCount: 0,
		Config: image.Config{ColorModel: palette, Width: width, Height: height},
	}))
	return out.Bytes()
}

func TestAnImageOfTheWrongSizeIsMadeToFit(t *testing.T) {
	/*
		The panel says no by showing nothing: an image that is not 640x640
		transfers successfully, switches buckets successfully, and leaves the
		screen blank. Five of this desk's nine animations are 480x480 and
		every one of them was silently invisible.
	*/
	fitted, err := fit(animation(t, 480, 480), PanelSize)
	require.NoError(t, err)

	width, height, ok := dimensions(fitted)
	require.True(t, ok)
	require.Equal(t, PanelSize, width)
	require.Equal(t, PanelSize, height)

	decoded, err := gif.DecodeAll(bytes.NewReader(fitted))
	require.NoError(t, err)
	require.Len(t, decoded.Image, 2, "a frame was lost in the scaling")
	require.Equal(t, PanelSize, decoded.Image[0].Bounds().Dx())
}

func TestAnImageThatAlreadyFitsIsNotTouched(t *testing.T) {
	/*
		Byte for byte, and it matters: the largest of these animations is
		nearly twenty megabytes, and decoding it into a hundred frames to
		encode them again would cost more than the transfer does.
	*/
	original := animation(t, PanelSize, PanelSize)

	fitted, err := fit(original, PanelSize)
	require.NoError(t, err)
	require.Equal(t, original, fitted)
}

func TestScalingKeepsEachFramesOwnPalette(t *testing.T) {
	// Scaled in palette space, so nothing is re-quantised and nothing grows.
	// A frame that gained colours would be a frame that had been decoded and
	// guessed at.
	fitted, err := fit(animation(t, 480, 480), PanelSize)
	require.NoError(t, err)

	decoded, err := gif.DecodeAll(bytes.NewReader(fitted))
	require.NoError(t, err)
	require.Len(t, decoded.Image[0].Palette, 2)
}

func TestSomethingThatIsNotAnImageSaysSo(t *testing.T) {
	_, err := fit([]byte("not a gif at all"), PanelSize)
	require.Error(t, err)
}

func TestTheHeaderIsReadWithoutDecoding(t *testing.T) {
	// The cheap check that keeps the common case free.
	width, height, ok := dimensions(animation(t, 480, 480))
	require.True(t, ok)
	require.Equal(t, 480, width)
	require.Equal(t, 480, height)

	_, _, ok = dimensions([]byte("GIF"))
	require.False(t, ok)
}
