package dashboard

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"testing"

	"github.com/stretchr/testify/require"
)

// colours counts the distinct colours in an RGBA picture.
func colours(img *image.RGBA) int {
	seen := map[[3]uint8]bool{}
	for i := 0; i < len(img.Pix); i += 4 {
		seen[[3]uint8{img.Pix[i], img.Pix[i+1], img.Pix[i+2]}] = true
	}
	return len(seen)
}

/*
The frame is full colour and the GIF is not (spec 080).

The starfield's nebulae are gradients, which the frame keeps and the GIF
brings down to its palette. A frame of 256 colours or fewer would mean the
canvas was still paletted somewhere.
*/
func TestTheFrameIsFullColourAndItsGIFIsNot(t *testing.T) {
	frame := shown(reading(), 0)
	require.Greater(t, colours(frame.Image), MaxColours, "the starfield was quantised before the frame")

	g, err := gif.DecodeAll(bytes.NewReader(frame.GIF()))
	require.NoError(t, err)
	require.Len(t, g.Image, 1)
	require.Equal(t, image.Rect(0, 0, Size, Size), g.Image[0].Bounds())
	require.LessOrEqual(t, len(g.Image[0].Palette), MaxColours)
}

// A photograph behind the numbers keeps its own colours in the frame: a
// smooth ramp through 236 palette entries is posterised, through RGBA it is
// not.
func TestAPictureBackgroundKeepsItsColours(t *testing.T) {
	photo := image.NewRGBA(image.Rect(0, 0, Size, Size))
	for y := range Size {
		for x := range Size {
			photo.SetRGBA(x, y, color.RGBA{uint8(x * 255 / Size), uint8(y * 255 / Size), 128, 255}) //nolint:gosec // below 256
		}
	}
	d := Shipped()[0]
	d.Background = Background{Kind: Picture}
	frame := Render(d, reading(), 0, photo, Trails{})

	require.Greater(t, colours(frame.Image), 4*MaxColours)
	g, err := gif.Decode(bytes.NewReader(frame.GIF()))
	require.NoError(t, err)
	require.Equal(t, image.Rect(0, 0, Size, Size), g.Bounds())
}

// Quantising is to the nearest palette colour, the same answer
// color.Palette.Index gives, so the cache changes the cost and not the
// picture.
func TestQuantiseIsTheNearestColour(t *testing.T) {
	pal := color.Palette{color.RGBA{0, 0, 0, 255}, color.RGBA{255, 255, 255, 255}, color.RGBA{200, 40, 40, 255}}
	src := image.NewRGBA(image.Rect(0, 0, 3, 1))
	src.SetRGBA(0, 0, color.RGBA{20, 10, 10, 255})
	src.SetRGBA(1, 0, color.RGBA{240, 230, 250, 255})
	src.SetRGBA(2, 0, color.RGBA{180, 60, 50, 255})

	out := quantise(src, pal)
	require.Equal(t, []uint8{0, 1, 2}, out.Pix)
}

/*
The GIF path re-quantises every frame now; spec 080 R2.2 gives it 30 ms. The
starfield, the worst case, is the shipped dashboard.

	go test ./internal/dashboard -run - -bench Quantise -benchmem
*/
func BenchmarkQuantiseTheShippedDashboard(b *testing.B) {
	frame := shown(reading(), 0)
	b.ResetTimer()
	for range b.N {
		_ = quantise(frame.Image, frame.pal())
	}
}

// Rendering without a GIF is what a panel that takes frames pays per cycle.
func BenchmarkRenderTheShippedDashboard(b *testing.B) {
	r := reading()
	for range b.N {
		_ = shown(r, 0)
	}
}
