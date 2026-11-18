package service

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/devices"
	"github.com/ushineko/hotaru/internal/render"
)

/*
A picture's frame is many near shades: an effect takes the most different of
them across its range, not the first lights' shades, in the order the lights
show them, and only colours nobody could tell apart are one.
*/
func TestSceneColoursAreTheOnesThatDiffer(t *testing.T) {
	teal1, teal2, teal3 := colour.MustParse("#5c8c80"), colour.MustParse("#6f8c6e"), colour.MustParse("#548c88")
	magenta := colour.MustParse("#c02090")
	same := colour.MustParse("#5d8c81") // teal1, a step away
	frame := devices.Frame{Colours: []colour.Colour{
		teal1, teal2, teal3, same, teal1, teal1, colour.Black, magenta, teal2, teal1,
	}}
	// Far apart already, so not stretched; in a smooth order, each next the
	// nearest, so a wave through them does not snap back and forth.
	assert.Equal(t, []colour.Colour{teal1, teal3, teal2, magenta}, sceneColours(frame, 8),
		"the shades are kept apart, and a colour a step away is the same one")
	assert.Equal(t, []colour.Colour{teal1, magenta}, sceneColours(frame, 2),
		"two are the main one and the furthest from it")
	assert.Equal(t, []colour.Colour{teal1}, sceneColours(frame, 1))

	red, blue := colour.MustParse("red"), colour.MustParse("blue")
	two := devices.Frame{Colours: []colour.Colour{blue, red, red, red}}
	assert.Equal(t, []colour.Colour{blue, red}, sceneColours(two, 2), "not in the order the lights show them")
}

/*
A subtle scene's colours are pushed apart until an effect through them is
seen, each in its own direction: the teal stays the bluer, the olive the
redder (the live run's space-2, chosen by the maintainer).
*/
func TestSubtleColoursAreStretchedApart(t *testing.T) {
	teal, olive := colour.MustParse("#4c8c88"), colour.MustParse("#788c60")
	frame := devices.Frame{Colours: []colour.Colour{teal, teal, olive}}
	got := sceneColours(frame, 8)
	assert.Len(t, got, 2)
	assert.GreaterOrEqual(t, apart(got[0], got[1]), float64(visibleSpread)-2, "still too close to see")
	assert.Greater(t, got[0].B, got[1].B, "the teal is no longer the bluer")
	assert.Greater(t, got[1].R, got[0].R, "the olive is no longer the redder")
	assert.Equal(t, uint8(0x8c), got[0].G, "a channel they share moved")
}

// Every one of the Kraken's fourteen effects has a best fit on the RTX 4090,
// and each is one of the card's own effects (spec 077 R3.6, E4).
func TestTheCardsTableMapsEachOfTheFourteen(t *testing.T) {
	card := []string{"Breathing", "Color Cycle", "Rainbow Wave", "Magic", "Fade In", "Flowing",
		"Patrolling", "Double Flashing"}
	kraken := []string{render.Fading, render.SpectrumWave, render.CoveringMarquee, render.Alternating,
		render.Pulse, render.Breathing, render.Candle, render.StarryNight, render.RainbowFlow,
		render.SuperRainbow, render.RainbowPulse, render.TaiChi, render.WaterCooler, render.Loading}
	assert.Len(t, kraken, 14)
	for _, effect := range kraken {
		fits := bestFits[effect]
		if assert.NotEmpty(t, fits, "%s has no best fit", effect) {
			assert.Contains(t, card, fits[0], "%s's best fit is not the card's", effect)
		}
	}
}
