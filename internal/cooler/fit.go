package cooler

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/gif"
)

/*
PanelSize is what this cooler's screen takes, in pixels square.

A constant under protest. Nothing in the protocol reports it, the panel is
640x640 on every Kraken Elite that has one, and a cooler with a different
screen would need this measured rather than assumed -- which is the same
caution the dashboard's floor carries.
*/
const PanelSize = 640

/*
fit makes an image the size the panel takes.

**A GIF of the wrong size displays as nothing at all.** Not an error, not a
refusal, not a garbled picture: the transfer succeeds, the bucket switch
succeeds, and the screen goes blank. Four of this desk's nine animations are
640x640 and play; the other five are 480x480 and were silently blank, which is
how the panel says "no" -- see spec 016.

The program this replaces never met the problem because Pillow resized every
frame on its way out. hotaru sends files, so hotaru has to do the same.

Scaled in palette space, by nearest neighbour. That is the cheap way and here
it is also the honest one: each frame keeps its own palette exactly, so nothing
is re-quantised, nothing grows, and a twenty-megabyte animation is not decoded
into a hundred full-colour frames to be encoded again. It is blocky under a
magnifying glass and indistinguishable at arm's length on a 640-pixel circle.
*/
func fit(data []byte, size int) ([]byte, error) {
	if width, height, ok := dimensions(data); ok && width == size && height == size {
		// Already right. The common case, and the one where decoding twenty
		// megabytes to learn nothing would be the whole cost of the write.
		return data, nil
	}

	decoded, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("read the image: %w", err)
	}
	if decoded.Config.Width == 0 || decoded.Config.Height == 0 {
		return nil, fmt.Errorf("the image reports no size")
	}

	for i, frame := range decoded.Image {
		decoded.Image[i] = stretch(frame, decoded.Config.Width, decoded.Config.Height, size)
	}
	decoded.Config.Width, decoded.Config.Height = size, size

	var out bytes.Buffer
	if err := gif.EncodeAll(&out, decoded); err != nil {
		return nil, fmt.Errorf("re-encode the image: %w", err)
	}
	return out.Bytes(), nil
}

/*
stretch scales one frame, keeping its palette and its place in the canvas.

A GIF's frames are often partial -- a rectangle somewhere inside the canvas
that changes -- so the frame's own bounds are scaled along with its pixels,
and the result sits where the original did.
*/
func stretch(frame *image.Paletted, canvasW, canvasH, size int) *image.Paletted {
	scaleX := float64(size) / float64(canvasW)
	scaleY := float64(size) / float64(canvasH)

	bounds := frame.Bounds()
	out := image.Rect(
		int(float64(bounds.Min.X)*scaleX), int(float64(bounds.Min.Y)*scaleY),
		int(float64(bounds.Max.X)*scaleX), int(float64(bounds.Max.Y)*scaleY),
	)
	if out.Empty() {
		out = image.Rect(bounds.Min.X, bounds.Min.Y, bounds.Min.X+1, bounds.Min.Y+1)
	}

	scaled := image.NewPaletted(out, frame.Palette)
	for y := out.Min.Y; y < out.Max.Y; y++ {
		source := bounds.Min.Y + int(float64(y-out.Min.Y)/scaleY)
		if source >= bounds.Max.Y {
			source = bounds.Max.Y - 1
		}
		for x := out.Min.X; x < out.Max.X; x++ {
			from := bounds.Min.X + int(float64(x-out.Min.X)/scaleX)
			if from >= bounds.Max.X {
				from = bounds.Max.X - 1
			}
			scaled.SetColorIndex(x, y, frame.ColorIndexAt(from, source))
		}
	}
	return scaled
}

/*
dimensions reads a GIF's logical screen size from its header.

Ten bytes, so that an image already the right size costs nothing to check. The
header is "GIF87a" or "GIF89a" followed by width and height as little-endian
sixteen-bit values.
*/
func dimensions(data []byte) (width, height int, ok bool) {
	if len(data) < 10 || !bytes.HasPrefix(data, []byte("GIF")) {
		return 0, 0, false
	}
	return int(binary.LittleEndian.Uint16(data[6:8])),
		int(binary.LittleEndian.Uint16(data[8:10])), true
}
