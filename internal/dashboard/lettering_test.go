package dashboard

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"math/rand/v2"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/readings"
)

/*
lettered is a real dashboard on a flat background: the shipped coolant one,
which has a headline, three columns and labels on all of them.

An empty Dashboard draws almost no text -- a slot with no source has no words
-- so a test about lettering built on one would assert things about a picture
with nothing written on it.
*/
func lettered() Dashboard {
	d := Shipped()[0]
	d.Background = Background{Kind: Plain}
	return d
}

// drawn decodes a rendered frame, so a test can look at the pixels rather
// than at a hash of them: what a colour setting does is visible or it does
// nothing.
func drawn(t *testing.T, d Dashboard, r Reading) image.Image {
	t.Helper()

	frame := Render(d, r, 0, nil, nil)
	require.NotEmpty(t, frame.GIF, "the frame did not encode")

	first, err := gif.Decode(bytes.NewReader(frame.GIF))
	require.NoError(t, err)
	return first
}

// counted is how many pixels of a colour a picture has.
func counted(picture image.Image, want color.RGBA) int {
	n := 0
	bounds := picture.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := picture.At(x, y).RGBA()
			if uint8(r>>8) == want.R && uint8(g>>8) == want.G && uint8(b>>8) == want.B {
				n++
			}
		}
	}
	return n
}

func TestTheLetteringIsDrawnTheWayItIsAsked(t *testing.T) {
	/*
		Size, face and edge all reach the panel. Asserted as "the picture
		changed" rather than pixel by pixel, because what a 150% label looks
		like is a judgement and that it is not the 100% one is a fact.
	*/
	plain := lettered()
	was := Render(plain, reading(), 0, nil, nil).Content

	bigger := plain
	bigger.Lettering.Labels.Size = 150
	require.NotEqual(t, was, Render(bigger, reading(), 0, nil, nil).Content,
		"a bigger label drew the same picture")

	mono := plain
	mono.Lettering.Font = "mono"
	require.NotEqual(t, was, Render(mono, reading(), 0, nil, nil).Content,
		"another face drew the same picture")

	// An unknown face draws in the default rather than failing, because a
	// dashboard written by a later version should still light up.
	later := plain
	later.Lettering.Font = "a face from 2027"
	require.Equal(t, was, Render(later, reading(), 0, nil, nil).Content)
}

func TestASizeOutsideWhatFitsIsHeldToIt(t *testing.T) {
	// Below the lower bound the text is unreadable at arm's length, which is
	// the job; above the upper one it leaves the space the arrangement gave
	// it and lands on its neighbour.
	require.InDelta(t, 1.0, Text{}.Scale(), 0.001, "unset must mean unchanged")
	require.InDelta(t, float64(MinSize)/100, Text{Size: 10}.Scale(), 0.001)
	require.InDelta(t, float64(MaxSize)/100, Text{Size: 400}.Scale(), 0.001)
}

func TestAChosenColourIsTheColourDrawn(t *testing.T) {
	/*
		A paletted image draws in the nearest colour it has, so a colour that
		is not in the palette is a colour somebody asked for and did not get.
		This is the test that says it is in there and on the screen.
	*/
	magenta := color.RGBA{R: 0xff, G: 0x00, B: 0xff, A: 255}

	d := lettered()
	d.Lettering.Labels.Colour = "#ff00ff"

	require.Positive(t, counted(drawn(t, d, reading()), magenta),
		"the labels were drawn in some other colour")

	// And something that is not a colour leaves the theme's, rather than
	// drawing black on black.
	d.Lettering.Labels.Colour = "rather blue"
	require.Zero(t, counted(drawn(t, d, reading()), magenta))
	require.Positive(t, counted(drawn(t, d, reading()), ThemeOf("").Muted))
}

func TestAGradedReadingKeepsTheColourThatMeansSomething(t *testing.T) {
	/*
		The coolant's green, amber and red are the alert thresholds. A screen
		showing calm while a notification says critical is worse than either
		alone -- so a dashboard's own colour applies to the readings that
		carry no grade, and the one that means something keeps meaning it.
	*/
	hot := reading()
	hot.Set(readings.Coolant, 65)

	d := lettered()
	d.Lettering.Values.Colour = "#00ff88"

	picture := drawn(t, d, hot)
	require.Positive(t, counted(picture, colCrit),
		"a critical coolant was painted over with the dashboard's own colour")
	require.Positive(t, counted(picture, color.RGBA{R: 0x00, G: 0xff, B: 0x88, A: 255}),
		"the readings that carry no grade did not take the colour")
}

func TestTheOutlineIsWhatTheDashboardAsksFor(t *testing.T) {
	/*
		Automatic is what the panel has always drawn: two pixels over a
		picture, none over the theme's own colours. "None" is a different
		answer from "not set", which is why the field is a pointer.
	*/
	none, three := 0, 3

	d := lettered()
	require.Zero(t, counted(drawn(t, d, reading()), colOutline),
		"text over a flat colour was outlined without being asked")

	d.Lettering.Labels.Outline = &three
	require.Positive(t, counted(drawn(t, d, reading()), colOutline))

	// And the defaults, which are what most dashboards keep.
	require.Equal(t, DefaultOutline, Text{}.Edge(true))
	require.Zero(t, Text{}.Edge(false))
	require.Zero(t, Text{Outline: &none}.Edge(true), "none must mean none, over a picture too")
}

func TestAPictureWithManyColoursStillEncodes(t *testing.T) {
	/*
		A GIF holds 256 colours. The palette was the interface's 33 plus up
		to 236 of the picture's, and the renderer discards the encoder's
		error -- so a photograph with enough distinct colours came back as a
		**zero-byte frame**: the panel showed nothing and nothing said why.

		It took a busy photograph to reach, which is why it survived until a
		dashboard could add colours of its own.
	*/
	noise := image.NewRGBA(image.Rect(0, 0, Size, Size))
	r := rand.New(rand.NewPCG(1, 2))
	for y := range Size {
		for x := range Size {
			noise.SetRGBA(x, y, color.RGBA{
				R: uint8(r.IntN(256)), G: uint8(r.IntN(256)), B: uint8(r.IntN(256)), A: 255})
		}
	}

	d := lettered()
	d.Background = Background{Kind: Picture, Picture: "noise"}
	d.Lettering.Labels.Colour = "#ff00ff"
	d.Lettering.Values.Colour = "#00ff88"

	frame := Render(d, reading(), 0, noise, nil)
	require.NotEmpty(t, frame.GIF, "the frame did not encode")
	require.LessOrEqual(t, len(palette(ThemeOf(""), fromPicture(noise), d.Lettering.chosen()...)),
		MaxColours, "the palette is over what a GIF holds")
}
