package images

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/gif"
)

/*
Slideshows, and what the program this replaces learned making them.

peripheral-battery-monitor built these for the same panel and its crossfades
came out "very chunky" -- five steps per transition. The cause was not the
memory budget but two encoding choices that spent most of it, and both are
easy to make again:

 1. **One palette for the whole reel, not one per frame.** A per-frame adaptive
    palette means consecutive frames share no colour table, so nothing
    delta-encodes -- which is exactly what a crossfade is. That change alone
    took a reel from 20.4 MB to 14.2 MB.
 2. **Per-frame durations.** A held photograph is *one* frame carrying the whole
    hold; only the fade steps carry the fade's own short delay. Giving every
    frame the same duration means a smoother fade can only be bought by making
    it slower.

Together those took that reel from five crossfade steps to twenty-four at the
same transition length. See the monitor's spec 035; this is its conclusion,
carried over rather than rediscovered.
*/

// Budget is how much of the panel's memory a slideshow may take. The device
// holds about 24 MB and a reel is the one thing that can fill it.
const Budget = 23 * 1000 * 1024

// Hold is how long each picture stays up, and Fade how long the crossfade
// between two of them takes. Both in hundredths of a second, which is what a
// GIF counts in.
const (
	Hold = 400
	Fade = 84
)

// MostSteps is the smoothest crossfade attempted. The count steps down from
// here until the reel fits, because a three-picture reel has room for more
// than an eight-picture one and a fixed number wastes one or overruns the
// other.
const MostSteps = 28

/*
Slideshow turns several pictures into one animation.

Each picture is held, then crossfaded into the next, and the last fades back
into the first so a loop has no jump in it.
*/
func Slideshow(sources [][]byte) (converted []byte, frames int, err error) {
	if len(sources) == 0 {
		return nil, 0, fmt.Errorf("a slideshow needs at least one picture")
	}
	if len(sources) == 1 {
		return Convert(sources[0])
	}

	photos := make([]*image.RGBA, 0, len(sources))
	for i, source := range sources {
		decoded, _, err := image.Decode(bytes.NewReader(source))
		if err != nil {
			return nil, 0, fmt.Errorf("picture %d is not an image this can read: %w", i+1, err)
		}
		photos = append(photos, square(decoded))
	}

	/*
		One palette, built from every photograph.

		The whole reason a crossfade encodes at all: frames that share a colour
		table delta-encode against each other, and frames that do not are each
		a fresh image.
	*/
	shared := chosenFrom(photos)
	through := newMapper(shared)

	for steps := MostSteps; steps >= 1; steps /= 2 {
		reel, count := build(photos, through, steps)
		encoded, err := encodeFrames(reel.frames, reel.delays, shared)
		if err != nil {
			return nil, 0, err
		}
		if len(encoded) <= Budget || steps == 1 {
			return encoded, count, nil
		}
	}
	return nil, 0, fmt.Errorf("that many pictures will not fit on the panel")
}

// reel is the frames of a slideshow and how long each is shown.
type reel struct {
	frames []*image.Paletted
	delays []int
}

// build lays out the reel: each photograph held, then faded into the next.
func build(photos []*image.RGBA, through *mapper, steps int) (reel, int) {
	var out reel
	for i, photo := range photos {
		out.frames = append(out.frames, toPalette(photo, through))
		out.delays = append(out.delays, Hold)

		next := photos[(i+1)%len(photos)]
		for step := 1; step <= steps; step++ {
			mixed := blend(photo, next, float64(step)/float64(steps+1))
			out.frames = append(out.frames, toPalette(mixed, through))
			out.delays = append(out.delays, max(1, Fade/steps))
		}
	}
	return out, len(out.frames)
}

// blend mixes two pictures, which is what a crossfade is made of.
func blend(from, to *image.RGBA, amount float64) *image.RGBA {
	out := image.NewRGBA(from.Bounds())
	for i := 0; i < len(out.Pix); i++ {
		out.Pix[i] = uint8(float64(from.Pix[i])*(1-amount) + float64(to.Pix[i])*amount)
	}
	return out
}

/*
mapper turns pixels into palette indices through a lookup table.

Go's paletted draw compares every pixel against every colour in the palette:
four hundred thousand pixels times two hundred and fifty-six colours, per
frame, and a reel is eighty frames. That is thirteen seconds for three
photographs, measured, and a person dropping eight would have waited minutes
wondering whether it had hung.

A table over the same coarse cube the palette was chosen from answers each
pixel with one index lookup. The table costs thirty-two thousand comparisons
once.
*/
type mapper struct {
	palette color.Palette
	// index is the nearest palette entry for each cell of a 32x32x32 cube.
	// -1 means not worked out yet.
	index []int16
}

const cube = 256 / step // 32 levels per channel

func newMapper(shared color.Palette) *mapper {
	m := &mapper{palette: shared, index: make([]int16, cube*cube*cube)}
	for i := range m.index {
		m.index[i] = -1
	}
	return m
}

// at is the palette index for a colour, worked out once per cell.
func (m *mapper) at(r, g, b uint8) uint8 {
	cell := int(r/step)*cube*cube + int(g/step)*cube + int(b/step)
	if known := m.index[cell]; known >= 0 {
		return uint8(known) //nolint:gosec // an index into a 256-colour palette
	}
	found := m.palette.Index(color.RGBA{R: r, G: g, B: b, A: 255})
	m.index[cell] = int16(found) //nolint:gosec // as above
	return uint8(found)          //nolint:gosec // as above
}

/*
toPalette draws a picture through the reel's shared colour table.

Not dithered: dithering breaks delta-encoding between frames, because the noise
pattern moves even where the picture does not -- which is the thing a crossfade
depends on.
*/
func toPalette(source *image.RGBA, m *mapper) *image.Paletted {
	out := image.NewPaletted(image.Rect(0, 0, Panel, Panel), m.palette)
	for i := range out.Pix {
		at := i * 4
		out.Pix[i] = m.at(source.Pix[at], source.Pix[at+1], source.Pix[at+2])
	}
	return out
}

// chosenFrom picks one palette for several pictures, by popularity across all
// of them.
func chosenFrom(photos []*image.RGBA) color.Palette {
	counts := map[color.RGBA]int{}
	for _, photo := range photos {
		tally(photo, counts)
	}
	return popular(counts)
}

func encodeFrames(frames []*image.Paletted, delays []int, shared color.Palette) ([]byte, error) {
	var out bytes.Buffer
	err := gif.EncodeAll(&out, &gif.GIF{
		Image: frames, Delay: delays, LoopCount: 0,
		Config: image.Config{ColorModel: shared, Width: Panel, Height: Panel},
	})
	if err != nil {
		return nil, fmt.Errorf("write the slideshow: %w", err)
	}
	return out.Bytes(), nil
}
