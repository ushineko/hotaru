package dashboard

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"math/rand"
	"strconv"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/gomonobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/gofont/gosmallcaps"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// ptToPx converts the Python's Qt point sizes, which were chosen by looking at
// the panel, into the pixels this rasteriser wants.
const ptToPx = 4.0 / 3.0

/*
drawing guards everything in this file that is shared between renders.

Until the preview route existed, one goroutine rendered: the push loop, for
the life of the service. Now a window asks for a frame on every change while
that loop is still drawing, and the two met in the font cache -- an
opentype.Face is not safe for concurrent use, and two goroutines shaping text
through one of them panicked inside sfnt with an index out of range.

One lock rather than one per map, held across the drawing and not only across
the lookup, because the unsafe part is the Face itself and not the map that
found it. A frame takes about 8 ms; serialising them costs nothing anybody
can see.
*/
var drawing sync.Mutex

// faces are the sized fonts, kept because building one parses the TTF. The
// caller holds `drawing`.
var faces = map[string]font.Face{}

func face(pt float64, bold bool, family string) font.Face {
	key, source := faceSource(family, bold)
	key += strconv.Itoa(int(pt))
	if f, ok := faces[key]; ok {
		return f
	}
	parsed, err := opentype.Parse(source)
	if err != nil {
		panic(err) // the fonts are compiled in; a failure here is a broken build
	}
	f, err := opentype.NewFace(parsed, &opentype.FaceOptions{
		Size: pt * ptToPx, DPI: 72, Hinting: font.HintingFull,
	})
	if err != nil {
		panic(err)
	}
	faces[key] = f
	return f
}

/*
faceSource is the TTF a family and weight draw from, and a key for the cache.

An unknown family draws in the default rather than failing: a dashboard
written by a later version of hotaru, or edited by hand, should still light
up. The faces are compiled in, because a panel drawn in a font somebody
installed would draw differently on the next machine.
*/
func faceSource(family string, bold bool) (key string, source []byte) {
	switch family {
	case "mono":
		if bold {
			return "mb", gomonobold.TTF
		}
		return "m", gomono.TTF
	case "smallcaps":
		// One face, so a bold smallcaps value is drawn in the same weight as
		// its label. Smallcaps is a shape rather than a weight, and the
		// family has no bold.
		return "sc", gosmallcaps.TTF
	}
	if bold {
		return "b", gobold.TTF
	}
	return "r", goregular.TTF
}

/*
Align is where a string sits in the box it is given, horizontally. Vertically
it is always centred: that is the reason this helper exists at all, because
text is top-aligned by default and a large glyph overruns its box into the
band beneath it.
*/
type Align int

// The alignments. Centre is what every arrangement drew before spec 041, and
// is still what a column under its own label wants.
const (
	Centre Align = iota
	Left
	Right
)

// centred draws text in a rect, centred vertically and placed horizontally by
// the alignment.
func centred(dst draw.Image, s string, x, y, w, h int, pt float64, bold bool,
	family string, c color.Color, align Align,
) {
	drawing.Lock()
	defer drawing.Unlock()

	f := face(pt, bold, family)
	advance := font.MeasureString(f, s).Round()
	metrics := f.Metrics()
	(&font.Drawer{
		Dst: dst, Src: image.NewUniform(c), Face: f,
		Dot: fixed.P(x+offset(w, advance, align),
			y+(h+metrics.Ascent.Round()-metrics.Descent.Round())/2),
	}).DrawString(s)
}

// offset is how far into a box of w a string of advance starts.
func offset(w, advance int, align Align) int {
	switch align {
	case Left:
		return 0
	case Right:
		return w - advance
	}
	return (w - advance) / 2
}

// nebulae are placed off-centre and kept dim: the middle of the panel carries
// the headline number and stays the darkest part of the image.
var nebulae = []struct {
	cx, cy, r float64
	c         color.RGBA
}{
	{0.20, 0.22, 0.46, color.RGBA{96, 60, 190, 255}},
	{0.82, 0.30, 0.40, color.RGBA{30, 120, 170, 255}},
	{0.68, 0.84, 0.44, color.RGBA{150, 45, 120, 255}},
	{0.30, 0.78, 0.34, color.RGBA{40, 90, 160, 255}},
}

// skies are the starfields drawn so far, one per theme. Cached for the reason
// the comment below gives, and keyed because a theme changes the sky.
var skies = map[string]*image.Paletted{}

/*
background is the starfield, drawn and quantised once.

Once for two reasons. A field regenerated per frame would shimmer between
updates, which on a screen that only redraws when something changes reads as a
fault rather than decoration -- so the Python caches it, with a fixed seed so
the sky is the same after every restart. And re-quantising four hundred
thousand pixels per frame cost 128 ms against 7.8 ms for copying a quantised
one, which is the difference between a renderer that could run forever and one
that could not.
*/
func background(theme Theme) *image.Paletted {
	drawing.Lock()
	sky, drawn := skies[theme.Name]
	drawing.Unlock()
	if drawn {
		return sky
	}
	rgba := image.NewRGBA(image.Rect(0, 0, Size, Size))
	draw.Draw(rgba, rgba.Bounds(), image.NewUniform(theme.BG), image.Point{}, draw.Src)

	for i, n := range nebulae {
		if len(theme.Sky) > i {
			n.c = theme.Sky[i]
		}
		cx, cy, r := n.cx*Size, n.cy*Size, n.r*Size
		for y := range Size {
			for x := range Size {
				d := math.Hypot(float64(x)-cx, float64(y)-cy)
				if d > r {
					continue
				}
				f := (1 - d/r)
				f = f * f * 0.28 // dim: decoration, not content
				o := rgba.RGBAAt(x, y)
				rgba.SetRGBA(x, y, color.RGBA{
					R: clamp(float64(o.R) + float64(n.c.R)*f),
					G: clamp(float64(o.G) + float64(n.c.G)*f),
					B: clamp(float64(o.B) + float64(n.c.B)*f),
					A: 255,
				})
			}
		}
	}

	/*
		Stars as small blocks, and fewer than the Python's 420.

		Single scattered pixels are the worst possible input to LZW: each one
		breaks a run that would otherwise compress away. The Python did not
		care because it was pushing to a screen over a path that tolerated it;
		this panel will not display a frame that is too large to settle in the
		time between updates.
	*/
	rng := rand.New(rand.NewSource(0x5EED)) //nolint:gosec // a fixed sky, not a secret
	for range 150 {
		x, y := rng.Intn(Size-2), rng.Intn(Size-2)
		for dy := range 2 {
			for dx := range 2 {
				rgba.SetRGBA(x+dx, y+dy, color.RGBA{200, 205, 230, 255})
			}
		}
	}

	sky = image.NewPaletted(rgba.Bounds(), palette(theme, nil))
	draw.Draw(sky, sky.Bounds(), rgba, image.Point{}, draw.Src)

	drawing.Lock()
	defer drawing.Unlock()
	if already, drawn := skies[theme.Name]; drawn {
		// Two renders raced to build the same sky. Either is correct -- it is
		// drawn from a fixed seed -- and keeping the first means the cached
		// pointer never changes under a caller holding it.
		return already
	}
	skies[theme.Name] = sky
	return sky
}

// arc strokes a circle segment, clockwise from twelve o'clock.
func arc(dst draw.Image, cx, cy, radius, width, from, sweep float64, c color.Color) {
	steps := int(math.Abs(sweep) * radius / 0.5)
	if steps < 2 {
		return
	}
	for i := range steps {
		a := from + sweep*float64(i)/float64(steps-1)
		for w := -width / 2; w <= width/2; w += 0.5 {
			r := radius + w
			x, y := int(cx+r*math.Cos(a)), int(cy-r*math.Sin(a))
			if x >= 0 && y >= 0 && x < Size && y < Size {
				dst.Set(x, y, c)
			}
		}
	}
}

/*
minValuePt is the smallest a value is shrunk to.

Below it the digits stop being readable at arm's length, which is the whole
job of the panel -- so a value that still does not fit overruns instead.
Overrunning visibly is better than being illegible quietly.
*/
const minValuePt = 16
