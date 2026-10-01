package render_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/render"
	"github.com/ushineko/sanshoku/lighting"
)

// lights is a canvas of n lights with made-up ids, so nothing here depends on
// any one device's layout.
func lights(n int) []lighting.Key {
	out := make([]lighting.Key, n)
	for i := range out {
		out[i] = lighting.Key{ID: byte(10 + i), Name: fmt.Sprintf("light %d", i)}
	}
	return out
}

func draw(t *testing.T, name string, at time.Duration, keys []lighting.Key, p render.Params) []lighting.Pixel {
	t.Helper()
	r, ok := render.Lookup(name)
	require.True(t, ok, "no renderer called %q", name)
	px := r.Draw(at, keys, p)
	require.Len(t, px, len(keys), "%s did not draw every light", name)
	for i, key := range keys {
		require.Equal(t, key.ID, px[i].ID, "%s drew light %d out of order", name, i)
	}
	return px
}

func intp(v int) *int { return &v }

func TestStaticIsTheSameFrameAtEveryInstant(t *testing.T) {
	// The property R3.2 depends on: an effect that does not move is drawn
	// once and held by the device, so its frame must not depend on t.
	keys := lights(5)
	p := render.Params{Colours: []colour.Colour{
		colour.MustParse("red"), colour.MustParse("blue"), colour.MustParse("#102030"),
		colour.MustParse("white"), colour.Black,
	}}
	first := draw(t, render.Static, 0, keys, p)
	for _, at := range []time.Duration{time.Millisecond, time.Second, time.Hour} {
		require.Equal(t, first, draw(t, render.Static, at, keys, p), "Static moved at %v", at)
	}
	require.Equal(t, lighting.Pixel{ID: 11, B: 255}, first[1], "Static did not show the frame's own colours")

	r, _ := render.Lookup(render.Static)
	require.False(t, r.Animated, "Static is marked as moving, so it would be streamed")
}

func TestStaticWithNoFrameShowsTheEffectsColour(t *testing.T) {
	green := colour.MustParse("lime")
	px := draw(t, render.Static, 0, lights(3), render.Params{Colour: &green})
	for _, p := range px {
		require.Equal(t, uint8(255), p.G)
	}
}

func TestBreathingIsDarkAtTheStartAndFullAtHalfItsPeriod(t *testing.T) {
	// At the default speed the period is 4.5 s: 8 s slowest, 1 s fastest.
	keys := lights(4)
	purple := colour.MustParse("#8000ff")
	p := render.Params{Colour: &purple}

	for _, px := range draw(t, render.Breathing, 0, keys, p) {
		require.Equal(t, lighting.Pixel{ID: px.ID}, px, "Breathing does not start dark")
	}
	for _, px := range draw(t, render.Breathing, 2250*time.Millisecond, keys, p) {
		require.Equal(t, [3]uint8{0x80, 0, 0xff}, [3]uint8{px.R, px.G, px.B}, "Breathing is not at full at half its period")
	}
	for _, px := range draw(t, render.Breathing, 4500*time.Millisecond, keys, p) {
		require.Equal(t, [3]uint8{}, [3]uint8{px.R, px.G, px.B}, "Breathing is not dark again at its period")
	}

	// Faster is a shorter period: at full speed, half a period is 500 ms.
	p.Speed = intp(render.FastestSpeed)
	px := draw(t, render.Breathing, 500*time.Millisecond, keys, p)
	require.Equal(t, uint8(0xff), px[0].B, "the speed did not change the period")
}

func TestSpectrumIsOneHueOnEveryLight(t *testing.T) {
	keys := lights(6)
	for _, at := range []time.Duration{0, 3 * time.Second, 7 * time.Second} {
		px := draw(t, render.Spectrum, at, keys, render.Params{})
		for _, p := range px[1:] {
			require.Equal(t, [3]uint8{px[0].R, px[0].G, px[0].B}, [3]uint8{p.R, p.G, p.B},
				"Spectrum showed two colours at %v", at)
		}
	}
	require.NotEqual(t,
		draw(t, render.Spectrum, 0, keys, render.Params{})[0],
		draw(t, render.Spectrum, 5*time.Second, keys, render.Params{})[0],
		"Spectrum did not move")
}

func TestTheWaveMovesOneLightPerStep(t *testing.T) {
	/*
		At full speed the wave moves 40 lights a second, so a step is 25 ms,
		and after one step every light shows what the light before it
		showed. The first light takes the last one's colour, because the
		wave wraps round.
	*/
	keys := lights(12)
	p := render.Params{Speed: intp(render.FastestSpeed)}
	const step = 25 * time.Millisecond

	for _, start := range []time.Duration{0, 5 * step, 41 * step} {
		before := draw(t, render.Wave, start, keys, p)
		after := draw(t, render.Wave, start+step, keys, p)
		for i := range keys {
			was := before[(i-1+len(keys))%len(keys)]
			require.Equal(t, [3]uint8{was.R, was.G, was.B}, [3]uint8{after[i].R, after[i].G, after[i].B},
				"light %d did not take its neighbour's colour after one step from %v", i, start)
		}
	}

	// One rainbow across the canvas: the first light is red at t=0, and no
	// two neighbours share a colour.
	px := draw(t, render.Wave, 0, keys, p)
	require.Equal(t, lighting.Pixel{ID: keys[0].ID, R: 255}, px[0])
	for i := 1; i < len(px); i++ {
		require.NotEqual(t, [3]uint8{px[i-1].R, px[i-1].G, px[i-1].B}, [3]uint8{px[i].R, px[i].G, px[i].B})
	}
}

func TestBrightnessScalesEveryChannel(t *testing.T) {
	keys := lights(3)
	c := colour.Colour{R: 200, G: 100, B: 50}
	full := render.Params{Colours: []colour.Colour{c, c, c}}
	half := full
	half.Brightness = intp(50)
	none := full
	none.Brightness = intp(0)

	for _, name := range []string{render.Static, render.Spectrum, render.Wave} {
		bright := draw(t, name, 1300*time.Millisecond, keys, full)
		dim := draw(t, name, 1300*time.Millisecond, keys, half)
		dark := draw(t, name, 1300*time.Millisecond, keys, none)
		for i := range keys {
			for _, pair := range [][2]uint8{{bright[i].R, dim[i].R}, {bright[i].G, dim[i].G}, {bright[i].B, dim[i].B}} {
				require.InDelta(t, float64(pair[0])/2, float64(pair[1]), 0.5,
					"%s at 50%% brightness is not half of full", name)
			}
			require.Equal(t, lighting.Pixel{ID: keys[i].ID}, dark[i], "%s at 0%% brightness is not dark", name)
		}
	}
}

func TestOffIsBlackAndHeld(t *testing.T) {
	r, ok := render.Lookup("off")
	require.True(t, ok)
	require.True(t, r.Blank)
	require.False(t, r.Animated)
	for _, px := range r.Draw(time.Minute, lights(3), render.Params{Colours: []colour.Colour{colour.MustParse("red")}}) {
		require.Equal(t, [3]uint8{}, [3]uint8{px.R, px.G, px.B})
	}
}

func TestTheNamesAreOpenRGBs(t *testing.T) {
	// A scene that says Breathing means the same thing on every device, so
	// the names are the ones OpenRGB gives the firmware modes they imitate.
	var names []string
	for _, r := range render.Renderers() {
		names = append(names, r.Name)
	}
	require.Equal(t, []string{"Static", "Breathing", "Spectrum", "Rainbow Wave", "Off"}, names)
}
