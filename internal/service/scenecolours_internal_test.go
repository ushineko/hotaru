package service

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/devices"
)

/*
A picture's frame is many near shades: an effect takes the colours that
differ, not the first lights' shades, in the order the lights show them.
*/
func TestSceneColoursAreTheOnesThatDiffer(t *testing.T) {
	teal1, teal2, teal3 := colour.MustParse("#5c8c80"), colour.MustParse("#6f8c6e"), colour.MustParse("#548c88")
	magenta := colour.MustParse("#c02090")
	frame := devices.Frame{Colours: []colour.Colour{
		teal1, teal2, teal3, teal1, teal1, colour.Black, magenta, teal2, teal1,
	}}
	assert.Equal(t, []colour.Colour{teal1, magenta}, sceneColours(frame, 8),
		"near shades are one colour, and the different one is kept")
	assert.Equal(t, []colour.Colour{teal1}, sceneColours(frame, 1))

	red, blue := colour.MustParse("red"), colour.MustParse("blue")
	two := devices.Frame{Colours: []colour.Colour{blue, red, red, red}}
	assert.Equal(t, []colour.Colour{blue, red}, sceneColours(two, 2), "not in the order the lights show them")
}
