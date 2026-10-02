package render_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/render"
	"github.com/ushineko/sanshoku/lighting"
)

/*
Spec 061 R2's table, a renderer at a time: none, one and several colours of
its own. Every renderer is a pure function, so each case is a frame at a
chosen instant and nothing waits.
*/

var (
	red   = colour.Colour{R: 255}
	blue  = colour.Colour{B: 255}
	green = colour.Colour{G: 255}
)

func rgb(px lighting.Pixel) colour.Colour { return colour.Colour{R: px.R, G: px.G, B: px.B} }

func TestEachRendererSaysHowManyColoursItTakes(t *testing.T) {
	want := map[string]int{
		render.Static: 0, render.Breathing: 4, render.Spectrum: 8, render.Wave: 8, render.Off: 0,
	}
	for _, r := range render.Renderers() {
		require.Equal(t, want[r.Name], r.Colours, "%s takes the wrong number of colours", r.Name)
	}
}

func TestStaticIgnoresColoursOfItsOwnWhereThereIsAFrame(t *testing.T) {
	frame := []colour.Colour{green, green}
	px := draw(t, render.Static, 0, lights(2), render.Params{Colours: frame, Palette: []colour.Colour{red, blue}})
	require.Equal(t, green, rgb(px[0]))
	require.Equal(t, green, rgb(px[1]))
}

func TestBreathingWithNoColoursBreathesTheFrame(t *testing.T) {
	// The default speed's period is 4.5 s, so full is at 2.25 s.
	frame := []colour.Colour{red, blue}
	px := draw(t, render.Breathing, 2250*time.Millisecond, lights(2), render.Params{Colours: frame})
	require.Equal(t, red, rgb(px[0]))
	require.Equal(t, blue, rgb(px[1]))
}

func TestBreathingWithOneColourBreathesItAtEveryBreath(t *testing.T) {
	p := render.Params{Colours: []colour.Colour{green}, Palette: []colour.Colour{red}}
	for _, breath := range []time.Duration{0, 1, 2, 7} {
		px := draw(t, render.Breathing, breath*4500*time.Millisecond+2250*time.Millisecond, lights(1), p)
		require.Equal(t, red, rgb(px[0]), "breath %d was not the effect's colour", breath)
	}
}

func TestBreathingWithSeveralColoursTakesOneBreathEach(t *testing.T) {
	const period = 4500 * time.Millisecond
	p := render.Params{Palette: []colour.Colour{red, blue, green}}
	for breath, want := range []colour.Colour{red, blue, green, red} {
		at := time.Duration(breath)*period + period/2
		px := draw(t, render.Breathing, at, lights(2), p)
		require.Equal(t, want, rgb(px[0]), "breath %d at full", breath)
		dark := draw(t, render.Breathing, time.Duration(breath)*period, lights(2), p)
		require.Equal(t, colour.Black, rgb(dark[0]), "breath %d does not start dark", breath)
	}
}

func TestBreathingDrawsNoMoreThanFourColours(t *testing.T) {
	const period = 4500 * time.Millisecond
	five := []colour.Colour{red, blue, green, {R: 255, G: 255, B: 255}, {R: 1, G: 2, B: 3}}
	px := draw(t, render.Breathing, 4*period+period/2, lights(1), render.Params{Palette: five})
	require.Equal(t, red, rgb(px[0]), "the fifth breath was not the first colour again")
}

func TestSpectrumWithNoColoursGoesRoundTheWheel(t *testing.T) {
	// A third of the default 16.5 s period is green on the wheel.
	px := draw(t, render.Spectrum, 5500*time.Millisecond, lights(1), render.Params{})
	require.Equal(t, green, rgb(px[0]))
}

func TestSpectrumWithOneColourHoldsIt(t *testing.T) {
	for _, at := range []time.Duration{0, 3 * time.Second, 11 * time.Second} {
		px := draw(t, render.Spectrum, at, lights(2), render.Params{Palette: []colour.Colour{blue}})
		require.Equal(t, blue, rgb(px[0]))
	}
}

func TestSpectrumWithSeveralColoursGoesRoundThemInOrder(t *testing.T) {
	// Two colours at the default 16.5 s period: red at 0, blue at half,
	// halfway between at a quarter, and red again at the period.
	const period = 16500 * time.Millisecond
	p := render.Params{Palette: []colour.Colour{red, blue}}
	at := func(d time.Duration) colour.Colour { return rgb(draw(t, render.Spectrum, d, lights(3), p)[2]) }
	require.Equal(t, red, at(0))
	require.Equal(t, colour.Colour{R: 128, B: 128}, at(period/4))
	require.Equal(t, blue, at(period/2))
	require.Equal(t, red, at(period))
}

func TestTheWaveWithNoColoursIsTheRainbow(t *testing.T) {
	px := draw(t, render.Wave, 0, lights(3), render.Params{})
	require.Equal(t, red, rgb(px[0]))
	require.Equal(t, green, rgb(px[1]))
	require.Equal(t, blue, rgb(px[2]))
}

func TestTheWaveWithOneColourIsThatColourEverywhere(t *testing.T) {
	for _, px := range draw(t, render.Wave, 300*time.Millisecond, lights(5), render.Params{Palette: []colour.Colour{green}}) {
		require.Equal(t, green, rgb(px))
	}
}

func TestTheWaveWithSeveralColoursIsAMovingGradientThatWraps(t *testing.T) {
	// Four lights and two colours: red, half way, blue, half way back.
	p := render.Params{Palette: []colour.Colour{red, blue}, Speed: intp(render.FastestSpeed)}
	px := draw(t, render.Wave, 0, lights(4), p)
	require.Equal(t, []colour.Colour{red, {R: 128, B: 128}, blue, {R: 128, B: 128}},
		[]colour.Colour{rgb(px[0]), rgb(px[1]), rgb(px[2]), rgb(px[3])})

	// One step later (25 ms at full speed) every light shows its
	// predecessor's colour, and the first takes the last's.
	after := draw(t, render.Wave, 25*time.Millisecond, lights(4), p)
	for i := range px {
		require.Equal(t, rgb(px[(i+3)%4]), rgb(after[i]), "light %d did not move", i)
	}
}

func TestOffIgnoresColours(t *testing.T) {
	for _, px := range draw(t, render.Off, 0, lights(2), render.Params{Palette: []colour.Colour{red}}) {
		require.Equal(t, colour.Black, rgb(px))
	}
}
