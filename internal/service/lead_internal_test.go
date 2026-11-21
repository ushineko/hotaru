package service

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ushineko/hotaru/internal/config"
	"github.com/ushineko/hotaru/internal/devices"
	"github.com/ushineko/hotaru/internal/scenes"
)

/*
Spec 084. A device whose rule leads is followed by every device whose rule
names no leader of its own; a device with a follow of its own keeps it; the
leader follows nothing, and a device a canvas draws follows nothing.
*/
func TestEveryDeviceWithoutALeaderFollowsTheOneThatLeads(t *testing.T) {
	found := []devices.Device{
		{Name: "Corsair Dominator Platinum RGB DDR5 (0x18)"},
		{Name: "Corsair Dominator Platinum RGB DDR5 (0x19)"},
		{Name: "EVGA GeForce RTX 3080 FTW3 Ultra LHR"},
		{Name: "Razer Mouse Dock Pro"},
		{Name: "SteelSeries Apex Pro TKL Gen 3 Wireless", HandedTo: "SteelSeries Apex Pro TKL Wireless Gen 3"},
	}
	cfg := &config.Config{Devices: []config.DeviceRule{
		{Match: "0x18", Lead: true},
		{Match: "razer", Follow: "evga"},
	}}
	assert.Empty(t, followOf(cfg, found, found[0].Name), "the leader follows")
	assert.Equal(t, found[0].Name, followOf(cfg, found, found[1].Name))
	assert.Equal(t, found[0].Name, followOf(cfg, found, found[2].Name))
	assert.Equal(t, "evga", followOf(cfg, found, found[3].Name), "a device's own follow lost to the lead")

	none := &config.Config{}
	assert.Empty(t, followOf(none, found, found[1].Name), "with no leader, nothing follows")

	first := leadersFirst(cfg, found)
	assert.Equal(t, found[0].Name, first[0].Name, "the leader is not written first")
}

// Spec 084. A request that gives several devices in scope an effect starts
// them together; one that gives one device an effect does not need to.
func TestSeveralEffectsAreCounted(t *testing.T) {
	found := []devices.Device{{Name: "stick (0x18)"}, {Name: "stick (0x19)"}, {Name: "card"}}
	cfg := &config.Config{}
	breathing := scenes.Effect{Mode: "Breathing"}
	one := Request{Effects: map[string]scenes.Effect{"0x18": breathing}}
	assert.Equal(t, 1, givenEffects(one, found, cfg))
	two := Request{Effects: map[string]scenes.Effect{"0x18": breathing, "0x19": breathing}}
	assert.Equal(t, 2, givenEffects(two, found, cfg))
	two.Devices = []string{"0x18"}
	assert.Equal(t, 1, givenEffects(two, found, cfg), "a device the request does not name was counted")
}
