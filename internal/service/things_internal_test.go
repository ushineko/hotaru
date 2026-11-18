package service

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ushineko/hotaru/internal/config"
	"github.com/ushineko/hotaru/internal/devices"
	"github.com/ushineko/hotaru/internal/render"
)

/*
A device's things: split as the device counts them, else as the rules'
segments tile a zone, else a zone whole; a ring where the device or a rule
says so (spec 077 R3.4).
*/
func TestThingsComeFromPartsThenSegmentsThenZones(t *testing.T) {
	zones := []devices.Zone{
		{Name: "Aura Mainboard", Shape: devices.ShapeSingle, First: 0, Count: 1},
		{Name: "Header 2", Shape: devices.ShapeLine, First: 1, Count: 16},
		{Name: "Header 3", Shape: devices.ShapeLine, First: 17, Count: 20},
		{Name: "Header 4", Shape: devices.ShapeLine, First: 37, Count: 0},
		{Name: "Fan", Shape: devices.ShapeRing, First: 37, Count: 24, Parts: []int{8, 8, 8}},
	}
	rule := devices.Rule{
		Segments: map[string]config.Segment{
			"side-top":    {Zone: "Header 2", LEDs: &config.LEDs{First: 0, Last: 7}},
			"side-bottom": {Zone: "Header 2", LEDs: &config.LEDs{First: 8, Last: 15}},
			"rear-half":   {Zone: "Header 3", LEDs: &config.LEDs{First: 0, Last: 9}}, // does not tile
		},
		Shapes: map[string]string{"Header 2": "ring", "Header 3": "ring"},
	}
	things := thingsOf(zones, rule)
	sizes := func(from []render.Thing) (out []int) {
		for _, th := range from {
			out = append(out, len(th.Lights))
		}
		return out
	}
	assert.Equal(t, []int{1, 8, 8, 20, 8, 8, 8}, sizes(things))
	assert.Equal(t, []int{1, 2, 3, 4, 5, 6, 7, 8}, things[1].Lights)
	assert.Equal(t, []int{9, 10, 11, 12, 13, 14, 15, 16}, things[2].Lights)
	assert.False(t, things[0].Ring)
	assert.True(t, things[1].Ring && things[3].Ring && things[4].Ring)
	assert.Equal(t, things[1].Zone, things[2].Zone, "the two side fans are one zone's chain")
	assert.NotEqual(t, things[2].Zone, things[3].Zone)
}
