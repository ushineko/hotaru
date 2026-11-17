package gui

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ushineko/hotaru/internal/api"
)

// The leader is found by any part of its name; the match offers the leader's
// own effects the follower draws, and not one the follower runs itself.
func TestTheLeaderAndItsEffects(t *testing.T) {
	kraken := api.Device{Name: "NZXT Kraken Elite V2", Stored: []string{"Breathing", "Tai Chi", "Candle"}}
	card := api.Device{Name: "MSI GeForce RTX 4090", Follows: "kraken",
		Modes: []string{"Breathing", "Tai Chi"}, Stored: []string{"Breathing"},
		Pace: map[string]float64{"tai chi": 1.4}}
	leader, ok := leaderNamed([]api.Device{card, kraken}, card)
	assert.True(t, ok)
	assert.Equal(t, kraken.Name, leader.Name)
	assert.Equal(t, []string{"Tai Chi"}, pacedEffects(card, leader))
	assert.InDelta(t, 1.4, paceOf(card, "Tai Chi"), 0.001)
	assert.InDelta(t, 1.0, paceOf(card, "Candle"), 0.001)

	req := matchRequest(card.Name, kraken.Name, "Tai Chi", 1.4)
	assert.True(t, req.Preview, "a match is recorded")
	assert.True(t, req.Restart, "the two do not start together")
	assert.Equal(t, map[string]float64{card.Name: 1.4}, req.Pace)
	assert.ElementsMatch(t, []string{card.Name, kraken.Name}, req.Devices)

	_, ok = leaderNamed([]api.Device{kraken}, api.Device{Name: "Corsair MM700"})
	assert.False(t, ok)
}
