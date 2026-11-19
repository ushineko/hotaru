package dashboard

import (
	"crypto/sha256"
	"image"
	"image/color"
	"image/draw"
	"time"

	"github.com/ushineko/hotaru/internal/readings"
)

/*
Layers is a dashboard drawn as four images (spec 081): the background, opaque,
and the rings, the trails and the text, each on a transparent canvas.

A frame is the four laid in that order. Render draws every frame that way, so
a still frame and a frame in the middle of a transition are made by the same
code, and an effect can move one layer and leave the others as they are.
*/
type Layers struct {
	Background, Rings, Trails, Text *image.RGBA

	theme Theme
	pal   color.Palette
}

// RenderLayers draws a dashboard as its layers.
func RenderLayers(d Dashboard, r Reading, behind image.Image, trails Trails) Layers {
	theme := ThemeOf(d.Theme)
	base := newPaint(d, theme, behind)
	return Layers{
		Background: base.img,
		Rings:      RingsLayer(d, r),
		Trails:     TrailsLayer(d, r, trails, base.picture),
		Text:       TextLayer(d, r, base.picture),
		theme:      theme,
		pal:        base.pal,
	}
}

/*
Frame lays the layers into one picture: the frame Render returns.

Content is taken before the tick is drawn, as Render always did, so the push
gate still compares what the frame says and not whether it was sent.
*/
func (l Layers) Frame(tick int) Frame {
	img := Compose(l.Background, l.Rings, l.Trails, l.Text)
	content := sha256.Sum256(img.Pix)
	drawTick(img, tick, l.theme)
	return Frame{Image: img, Content: content, Taken: time.Now(), pal: l.pal}
}

// Compose lays transparent layers over a copy of an opaque background, in
// order. Nil layers are skipped.
func Compose(background *image.RGBA, layers ...*image.RGBA) *image.RGBA {
	out := image.NewRGBA(background.Bounds())
	copy(out.Pix, background.Pix)
	for _, l := range layers {
		if l != nil {
			draw.Draw(out, out.Bounds(), l, image.Point{}, draw.Over)
		}
	}
	return out
}

// layerPaint is a transparent canvas with a dashboard's theme and lettering.
// picture says the background is a photograph, which outlines text and
// trails.
func layerPaint(d Dashboard, picture bool) *paint {
	return &paint{
		img:     image.NewRGBA(image.Rect(0, 0, Size, Size)),
		theme:   ThemeOf(d.Theme),
		picture: picture,
		letters: d.Lettering,
	}
}

// TextLayer is everything a dashboard writes: the headline, its label, the
// slots and the caption, without the rings or the trails.
func TextLayer(d Dashboard, r Reading, picture bool) *image.RGBA {
	p := layerPaint(d, picture)
	p.noRings = true
	switch d.Arrangement {
	case Grid:
		drawGrid(p, d, r)
	case Stacked:
		drawStacked(p, d, r)
	case Big:
		drawBig(p, d, r)
	default:
		drawRing(p, d, r)
	}
	caption(p, d)
	return p.img
}

// RingsLayer is the rings alone, or nothing for an arrangement without them.
func RingsLayer(d Dashboard, r Reading) *image.RGBA {
	p := layerPaint(d, false)
	if Rings(d.Arrangement) > 0 {
		drawRings(p, d, r)
	}
	return p.img
}

// TrailsLayer is the trail band alone, so it can drift while the text does
// something else.
func TrailsLayer(d Dashboard, r Reading, t Trails, picture bool) *image.RGBA {
	p := layerPaint(d, picture)
	p.drawTrails(d, r, t)
	return p.img
}

/*
BackgroundLayer is the background alone, opaque, and whether it is a
photograph: for an animated picture, one per frame of it. Theme is the
dashboard's, for drawing the tick over a frame composed by hand.
*/
func BackgroundLayer(d Dashboard, behind image.Image) (img *image.RGBA, picture bool) {
	base := newPaint(d, ThemeOf(d.Theme), behind)
	return base.img, base.picture
}

// TrailDomain is the range a trace is drawn against, for blending it while
// the trace drifts (spec 081 R4.2).
func TrailDomain(points Series, source readings.Source) (lo, hi float64) {
	return domain(points, source)
}
