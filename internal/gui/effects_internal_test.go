package gui

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/hotaru/internal/api"
)

// A listing handed to a canvas device is not offered a mode: hotaru writes
// the canvas, so a mode chosen for the listing would do nothing (spec 060).
func TestAHandedListingIsNotOfferedAMode(t *testing.T) {
	modes := []string{"Direct", "Onboard"}
	got := withModes([]api.Device{
		{Name: "board (openrgb)", InScope: true, Modes: modes, HandedTo: "board"},
		{Name: "board", InScope: true, Modes: modes},
	})
	require.Len(t, got, 1)
	require.Equal(t, "board", got[0].Name)
}
