package dashboard

import (
	"image"
	"image/color"
	"testing"

	"github.com/ushineko/hotaru/internal/motion"
	"github.com/ushineko/hotaru/internal/readings"
)

/*
What a frame of a moving dashboard costs, piece by piece (spec 081 R8): the
pieces a frame is made of, so the cheap ones can be done once and the dear
ones measured.

	go test ./internal/dashboard -run - -bench Piece -benchmem
*/
func BenchmarkPiece(b *testing.B) {
	d := Shipped()[0]
	r := reading()
	photo := image.NewRGBA(image.Rect(0, 0, Size, Size))
	for i := range photo.Pix {
		photo.Pix[i] = uint8(i * 7)
	}
	pal := make(color.Palette, 256)
	for i := range pal {
		pal[i] = color.RGBA{uint8(i), uint8(255 - i), uint8(i / 2), 255}
	}
	frame := image.NewPaletted(image.Rect(0, 0, Size, Size), pal)
	for i := range frame.Pix {
		frame.Pix[i] = uint8(i)
	}
	picture := d
	picture.Background = Background{Kind: Picture, Picture: "p"}
	trails := Trails{Below: level(40), Points: 60}
	l := RenderLayers(d, r, nil, trails)
	before := RingsLayer(d, r)
	r2 := reading()
	r2.Set(readings.Coolant, 60)

	for _, c := range []struct {
		name string
		run  func()
	}{
		{"rings", func() { RingsLayer(d, r) }},
		{"trails", func() { TrailsLayer(d, r, trails, false) }},
		{"text", func() { TextLayer(d, r, false) }},
		{"background-starfield", func() { BackgroundLayer(d, nil) }},
		{"background-photo", func() { BackgroundLayer(picture, photo) }},
		{"background-paletted-frame", func() { BackgroundLayer(picture, frame) }},
		{"compose-3", func() { Compose(l.Background, l.Rings, l.Trails, l.Text) }},
		{"glow", func() { motion.Glow(RingsLayer(d, r2), before, 0.5) }},
	} {
		b.Run(c.name, func(b *testing.B) {
			for range b.N {
				c.run()
			}
		})
	}
}
