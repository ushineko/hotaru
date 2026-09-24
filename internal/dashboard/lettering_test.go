package dashboard

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"math"
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

	frame := Render(d, r, 0, nil, Trails{})
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
	was := Render(plain, reading(), 0, nil, Trails{}).Content

	bigger := plain
	bigger.Lettering.Labels.Size = 150
	require.NotEqual(t, was, Render(bigger, reading(), 0, nil, Trails{}).Content,
		"a bigger label drew the same picture")

	mono := plain
	mono.Lettering.Font = "mono"
	require.NotEqual(t, was, Render(mono, reading(), 0, nil, Trails{}).Content,
		"another face drew the same picture")

	// An unknown face draws in the default rather than failing, because a
	// dashboard written by a later version should still light up.
	later := plain
	later.Lettering.Font = "a face from 2027"
	require.Equal(t, was, Render(later, reading(), 0, nil, Trails{}).Content)
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

	frame := Render(d, reading(), 0, noise, Trails{})
	require.NotEmpty(t, frame.GIF, "the frame did not encode")
	require.LessOrEqual(t, len(palette(ThemeOf(""), fromPicture(noise), d.Lettering.chosen()...)),
		MaxColours, "the palette is over what a GIF holds")
}

func TestTheBigNumberHasASizeOfItsOwn(t *testing.T) {
	/*
		One size for every number scaled the arrangement's proportions
		together, which is what spec 037 chose: the headline is three times
		its label because somebody looked at a panel in a case. This is for
		the desk that wants the relationship changed -- a smaller headline so
		the rows under it can be read (#144).
	*/
	d := Shipped()[1]
	d.Arrangement = Stacked
	d.Background = Background{Kind: Plain}
	d.Headline = Slot{Source: readings.CPULoad, Label: "CPU"}

	was := decode(t, Render(d, reading(), 0, nil, Trails{}))
	head := inked(was, 98, 214)
	rows := inked(was, 296, 340)

	d.Lettering.Headline.Size = 60
	now := decode(t, Render(d, reading(), 0, nil, Trails{}))
	smaller := inked(now, 98, 214)
	require.Less(t, smaller[len(smaller)-1]-smaller[0], head[len(head)-1]-head[0],
		"the big number did not shrink")

	// And the rows are exactly where they were: that is the whole point.
	after := inked(now, 296, 340)
	require.Equal(t, rows, after, "sizing the big number moved the readings under it")
}

func TestTheBigNumberFallsBackToTheReadings(t *testing.T) {
	// Unset is what every dashboard saved before this says, so it must draw
	// what it drew: the readings' own size, colour and edge.
	var letters Lettering
	letters.Values = Text{Size: 120, Colour: "#ff00ff"}
	require.Equal(t, letters.Values, letters.Big(), "an unset headline is not the readings")

	// Per field, because somebody who set a size has said nothing about a
	// colour, and the colour they chose for the readings is still theirs.
	letters.Headline = Text{Size: 60}
	require.Equal(t, 60, letters.Big().Size)
	require.Equal(t, "#ff00ff", letters.Big().Colour, "the colour was lost with the size")
}

func TestNothingIsDrawnWhereThePanelCannotShowIt(t *testing.T) {
	/*
		**The frame is square and the panel is not.** A 640x640 GIF is
		displayed through a round bezel, so a band high up the panel is
		narrower than one across its middle: at y=98, where a stacked
		headline's digits start, the circle allows 461 pixels where the
		rectangular inset allows 576.

		`48% · 100°C` put 127 inked pixels outside the circle and `48% · 38°C`
		put none, which is why this was a bug that came and went with the
		temperature (#144).
	*/
	var r Reading
	r.Set(readings.CPULoad, 48)
	r.Set(readings.CPUTemp, 100)
	r.Set(readings.PumpRPM, 9999)
	r.Set(readings.FanRPM, 9999)

	for _, arrangement := range []string{Ring, Grid, Stacked, Big} {
		for _, headline := range []Slot{
			{Source: readings.CPULoad, Second: readings.CPUTemp, Label: "CPU"},
			{Source: readings.PumpRPM, Second: readings.FanRPM, Label: "PUMP"},
		} {
			d := Shipped()[1]
			d.Arrangement = arrangement
			d.Units = true
			d.Background = Background{Kind: Plain} // so every inked pixel is text
			d.Headline = headline

			require.Zero(t, outsideTheCircle(decode(t, Render(d, r, 0, nil, Trails{}))),
				"%s drew %v where the bezel covers it", arrangement, headline.Words(true))
		}
	}
}

// outsideTheCircle counts the inked pixels a round panel cannot show: the
// ones beyond the circle inscribed in the frame.
func outsideTheCircle(img *image.Paletted) int {
	const centre, radius = float64(Size)/2 - 0.5, float64(Size) / 2

	background := img.Pix[0]
	n := 0
	for y := range Size {
		for x := range Size {
			if img.Pix[y*img.Stride+x] == background {
				continue
			}
			if math.Hypot(float64(x)-centre, float64(y)-centre) > radius {
				n++
			}
		}
	}
	return n
}
