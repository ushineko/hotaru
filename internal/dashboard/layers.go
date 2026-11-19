package dashboard

import (
	"image"

	"github.com/ushineko/hotaru/internal/readings"
)

/*
Layers is a dashboard drawn as three images, for animating the parts
separately (spec 081 PROTOTYPE: the shape of this is not settled).

Background is what Render draws first. Text is everything else except the
rings, and Rings is the rings alone, both on transparent canvases, so an
effect can move them and lay them over the background.
*/
type Layers struct {
	Background, Text, Rings *image.RGBA
}

// RenderLayers draws d as Layers.
func RenderLayers(d Dashboard, r Reading, behind image.Image, trails Trails) Layers {
	theme := ThemeOf(d.Theme)
	base := newPaint(d, theme, behind)

	text := *base
	text.img = image.NewRGBA(base.img.Bounds())
	text.noRings = true
	switch d.Arrangement {
	case Grid:
		drawGrid(&text, d, r)
	case Stacked:
		drawStacked(&text, d, r)
	case Big:
		drawBig(&text, d, r)
	default:
		drawRing(&text, d, r)
	}
	text.drawTrails(d, r, trails)
	caption(&text, d)

	return Layers{Background: base.img, Text: text.img, Rings: RingsLayer(d, r)}
}

// RingsLayer is the rings alone on a transparent canvas, or an empty canvas
// for an arrangement without them.
func RingsLayer(d Dashboard, r Reading) *image.RGBA {
	theme := ThemeOf(d.Theme)
	p := &paint{img: image.NewRGBA(image.Rect(0, 0, Size, Size)), theme: theme, letters: d.Lettering}
	if Rings(d.Arrangement) > 0 {
		drawRings(p, d, r)
	}
	return p.img
}

// TrailDomain is the range a trace is drawn against (spec 081 PROTOTYPE, for
// blending it while the trace drifts).
func TrailDomain(points Series, source readings.Source) (lo, hi float64) {
	return domain(points, source)
}

// TrailsLayer is the trail band alone on a transparent canvas (spec 081
// PROTOTYPE), so it can drift while the text does something else.
func TrailsLayer(d Dashboard, r Reading, t Trails) *image.RGBA {
	base := newPaint(d, ThemeOf(d.Theme), nil)
	p := *base
	p.img = image.NewRGBA(base.img.Bounds())
	p.drawTrails(d, r, t)
	return p.img
}
