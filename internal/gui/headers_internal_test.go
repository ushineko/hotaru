package gui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hotaru/internal/api"
)

func at(n int) *int { return &n }

/*
A device's parts are its zones, or the fans the device counts or the rules
divide a zone into, each with its name; a rename sends the old name away and
the new one in, and a cleared name removes it (spec 078 R5).
*/
func TestPartsAndTheirNames(t *testing.T) {
	device := api.Device{
		Name: "ASUS ROG MAXIMUS Z790 HERO",
		Zones: []api.Zone{
			{Name: "Aura Mainboard", Count: 1},
			{Name: "Addressable RGB Header 2", Count: 16, Max: 120},
			{Name: "Addressable RGB Header 3", Count: 20, Max: 120},
		},
		Named: []api.NamedPart{
			{Name: "top-front", RuleSegment: api.RuleSegment{Zone: "Addressable RGB Header 2", First: at(0), Last: at(7)}},
			{Name: "bottom-front", RuleSegment: api.RuleSegment{Zone: "Addressable RGB Header 2", First: at(8), Last: at(15)}},
			{Name: "rear-fan", RuleSegment: api.RuleSegment{Zone: "Addressable RGB Header 3"}},
		},
	}
	all, names := parts(device)
	require.Len(t, all, 4)
	assert.Equal(t, []string{"", "top-front", "bottom-front", "rear-fan"}, names)
	assert.Equal(t, "ASUS ROG MAXIMUS Z790 HERO/Addressable RGB Header 2[8:15]", all[2].target(device.Name))
	assert.True(t, all[3].whole)

	changes := nameChanges(all, names, []string{"Mobo", "top-front", "", "Back Fan"})
	assert.Equal(t, &api.RuleSegment{Zone: "Aura Mainboard"}, changes["mobo"])
	assert.Contains(t, changes, "bottom-front")
	assert.Nil(t, changes["bottom-front"], "a cleared name was not removed")
	assert.Nil(t, changes["rear-fan"])
	assert.Equal(t, "Addressable RGB Header 3", changes["back-fan"].Zone)
	assert.NotContains(t, changes, "top-front", "an unchanged name was sent")

	assert.Equal(t, "2 fans", holding(device, device.Zones[1]))
	assert.Equal(t, "a strip", holding(device, device.Zones[2]))
	// The rear fan: a ring named whole, which saving as a strip undid.
	ring := device.Zones[2]
	ring.Shape = api.ShapeRing
	assert.Equal(t, "1 fan", holding(device, ring), "a fan named whole was read as a strip")
	assert.Len(t, settable(device), 2)

	marked := endMarked(device.Name, "Addressable RGB Header 3", 20)
	assert.Equal(t, "red", marked[len(marked)-1].Colour)
	assert.Equal(t, "ASUS ROG MAXIMUS Z790 HERO/Addressable RGB Header 3[19:19]", marked[len(marked)-1].Target)
}
