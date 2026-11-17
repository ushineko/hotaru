package render_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/render"
)

/*
The Kraken's effects, drawn by hotaru (spec 077 R3.3), each checked against
what E1 recorded of the cooler: the order, the direction and the layout. The
look itself is matched by eye on the desk.
*/

// cooler is the Kraken's layout: a ring of 24 in zone 0, and three fans of 8
// chained in zone 1, each clockwise from its top.
func cooler() []render.Thing {
	span := func(from, n int) []int {
		out := make([]int, n)
		for i := range out {
			out[i] = from + i
		}
		return out
	}
	return []render.Thing{
		{Lights: span(0, 24), Zone: 0, Ring: true},
		{Lights: span(24, 8), Zone: 1, Ring: true},
		{Lights: span(32, 8), Zone: 1, Ring: true},
		{Lights: span(40, 8), Zone: 1, Ring: true},
	}
}

func speed(n int) *int { return &n }

// The Kraken's own speed gives the renderer the same pace: CAM's speed is the
// middle of the range for most, and the ends are the ends.
func TestTheKrakensSpeedIsTheSamePace(t *testing.T) {
	fast, ok := render.SpeedFor(render.SpectrumWave, 80)
	require.True(t, ok)
	assert.Equal(t, render.FastestSpeed, fast)
	slow, _ := render.SpeedFor(render.SpectrumWave, 350)
	assert.Equal(t, render.SlowestSpeed, slow)
	cam, _ := render.SpeedFor(render.SpectrumWave, 250)
	assert.InDelta(t, 37, cam, 1, "250 is about a third of the way from 350 to 80")
	_, ok = render.SpeedFor("Disco", 10)
	assert.False(t, ok)
	breath, ok := render.SpeedFor(render.Breathing, 20)
	require.True(t, ok, "the Kraken's Breathing is hotaru's")
	assert.Positive(t, breath)
}

// Fading shows each colour in turn on every light, crossfading between them.
func TestFadingGoesThroughItsColoursInOrder(t *testing.T) {
	p := render.Params{Palette: []colour.Colour{red, green, blue}, Speed: speed(render.FastestSpeed)}
	step, _ := time.ParseDuration("520ms") // Fading's fastest step, 10 × 52 ms
	for i, want := range []colour.Colour{red, green, blue, red} {
		px := draw(t, render.Fading, time.Duration(i)*step, lights(4), p)
		for _, x := range px {
			assert.Equal(t, want, rgb(x), "colour %d", i)
		}
	}
	half := draw(t, render.Fading, step/2, lights(1), p)
	assert.Equal(t, colour.Colour{R: 128, G: 128}, rgb(half[0]), "a crossfade, not a cut")
}

// Spectrum Wave is the whole wheel round each thing, turning clockwise, each
// fan alike.
func TestSpectrumWaveTurnsClockwiseOnEveryThing(t *testing.T) {
	p := render.Params{Things: cooler(), Speed: speed(render.FastestSpeed)}
	turn := 80 * 20 * time.Millisecond
	now := draw(t, render.SpectrumWave, 0, lights(48), p)
	later := draw(t, render.SpectrumWave, turn/24, lights(48), p)
	assert.Equal(t, rgb(now[0]), rgb(later[1]), "the colour at the top moved one light clockwise")
	assert.Equal(t, rgb(now[24]), rgb(now[32]), "the fans are alike")
	assert.Equal(t, rgb(now[24]), rgb(now[40]))
}

// Covering Marquee runs along a zone's fans as one chain: the first fan is
// covered before the second begins.
func TestCoveringMarqueeRunsAlongTheChain(t *testing.T) {
	p := render.Params{Things: cooler(), Palette: []colour.Colour{red, blue}}
	sweep := 150 * 17 * time.Millisecond                               // CAM's speed, the middle of the range
	px := draw(t, render.CoveringMarquee, sweep*32/100, lights(48), p) // fan 1 covered, fan 2 not reached
	for i := 24; i < 32; i++ {
		assert.Equal(t, red, rgb(px[i]), "fan 1 light %d not yet covered", i-24)
	}
	for i := 32; i < 48; i++ {
		assert.Equal(t, blue, rgb(px[i]), "fans 2 and 3 covered too soon")
	}
	back := draw(t, render.CoveringMarquee, sweep+sweep/100, lights(48), p)
	assert.Equal(t, blue, rgb(back[24]), "the other colour sweeps back")
}

// Alternating is blocks of four, stepping clockwise by half a block.
func TestAlternatingStepsHalfABlock(t *testing.T) {
	p := render.Params{Palette: []colour.Colour{red, blue}, Speed: speed(render.FastestSpeed)}
	before := draw(t, render.Alternating, 0, lights(24), p)
	for i, want := range []colour.Colour{red, red, red, red, blue, blue, blue, blue} {
		assert.Equal(t, want, rgb(before[i]))
	}
	step := 500 * 700 * time.Microsecond
	after := draw(t, render.Alternating, step, lights(24), p)
	assert.Equal(t, rgb(before[0]), rgb(after[2]), "the pattern moved two lights clockwise")
}

// Pulse rises quickly, falls smoothly, and is dark for a moment between.
func TestPulseRisesFastAndRestsDark(t *testing.T) {
	p := render.Params{Palette: []colour.Colour{red}, Speed: speed(render.SlowestSpeed)}
	period := 25 * 74 * time.Millisecond
	rise := draw(t, render.Pulse, period*10/100, lights(1), p)
	assert.Equal(t, red, rgb(rise[0]), "full after a tenth of the period")
	gap := draw(t, render.Pulse, period*98/100, lights(1), p)
	assert.Equal(t, colour.Black, rgb(gap[0]), "no moment dark between pulses")
}

// On fans, Tai Chi is one colour covering each fan in turn, staggered fan to
// fan; on the ring, two halves.
func TestTaiChiIsStaggeredOnTheFans(t *testing.T) {
	p := render.Params{Things: cooler(), Palette: []colour.Colour{red, blue}}
	px := draw(t, render.TaiChi, time.Second, lights(48), p)
	assert.NotEqual(t, rgb(px[28]), rgb(px[36]), "the fans are in step, not staggered")
	assert.NotEqual(t, rgb(px[0]), rgb(px[12]), "the ring is not two halves")
}

// Loading goes dark before it starts again.
func TestLoadingIsDarkBeforeItStartsAgain(t *testing.T) {
	p := render.Params{Palette: []colour.Colour{red}}
	lap := 10 * 600 * time.Millisecond
	end := draw(t, render.Loading, lap-lap/200, lights(24), p)
	for i, x := range end {
		assert.Equal(t, colour.Black, rgb(x), "light %d lit at the end of the lap", i)
	}
	mid := draw(t, render.Loading, lap/2, lights(24), p)
	lit := 0
	for _, x := range mid {
		if rgb(x) != colour.Black {
			lit++
		}
	}
	assert.Positive(t, lit, "nothing lit halfway")
}

// With no things given, every effect draws on the whole canvas as one strip.
func TestEveryKrakenEffectDrawsWithNoLayout(t *testing.T) {
	for _, name := range []string{
		render.Fading, render.SpectrumWave, render.CoveringMarquee, render.Alternating, render.Pulse,
		render.Candle, render.StarryNight, render.RainbowFlow, render.SuperRainbow, render.RainbowPulse,
		render.TaiChi, render.WaterCooler, render.Loading,
	} {
		draw(t, name, 1234*time.Millisecond, lights(5), render.Params{})
	}
}

// A device's pace trim lengthens or shortens a copy's period (spec 077 R3.9).
func TestAPaceTrimChangesThePeriod(t *testing.T) {
	plain := render.Params{Palette: []colour.Colour{red, blue}}
	slow := render.Params{Palette: []colour.Colour{red, blue}, Pace: 2}
	step := 40 * 52 * time.Millisecond // Fading at CAM's speed
	assert.Equal(t, blue, rgb(draw(t, render.Fading, step, lights(1), plain)[0]))
	assert.Equal(t, colour.Colour{R: 128, B: 128}, rgb(draw(t, render.Fading, step, lights(1), slow)[0]),
		"twice the period is halfway at one step")
	breathe := render.Params{Palette: []colour.Colour{red}, Speed: speed(render.FastestSpeed), Pace: 2}
	assert.Equal(t, red, rgb(draw(t, render.Breathing, time.Second, lights(1), breathe)[0]),
		"hotaru's own effects take the trim too: a 1 s breath trimmed to 2 s is full at 1 s")
}
