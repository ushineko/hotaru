package render

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ushineko/sanshoku/lighting"
)

// Named keys sit where a standard layout puts them, so a wave crosses the
// board rather than the alphabet: Q and A are neighbours, and Escape is left
// of Backspace, whatever order the device lists them in.
func TestNamedKeysSitOnTheStandardLayout(t *testing.T) {
	keys := []lighting.Key{{ID: 1, Name: "A"}, {ID: 2, Name: "Q"}, {ID: 3, Name: "Backspace"}, {ID: 4, Name: "Escape"}}
	at := positions(keys)
	assert.InDelta(t, at[1], at[0], 0.02, "Q and A are a quarter key apart")
	assert.Less(t, at[3], at[2], "Escape is left of Backspace")
}

// Lights with no name the layout knows keep their place in the device's
// order, so an unnamed canvas draws as it always did.
func TestUnnamedLightsKeepTheDevicesOrder(t *testing.T) {
	keys := []lighting.Key{{ID: 1}, {ID: 2, Name: "zone 2"}, {ID: 3}, {ID: 4}}
	assert.Equal(t, []float64{0, 0.25, 0.5, 0.75}, positions(keys))
}
