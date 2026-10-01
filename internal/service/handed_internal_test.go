package service

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/config"
	"github.com/ushineko/hotaru/internal/devices"
)

// OpenRGB's names and a canvas's names for the same keys compare equal.
func TestKeyNamesCompareAcrossTheTwoListings(t *testing.T) {
	for openrgb, canvas := range map[string]string{
		"Key: Escape":       "Escape",
		"Key: Page Up":      "Page Up",
		"Key: Up Arrow":     "Up",
		`Key: \ (ANSI)`:     `\`,
		"Key: Left Windows": "Left GUI",
		"Key: `":            "`",
	} {
		require.Equal(t, keyName(canvas), keyName(openrgb), "%q and %q", openrgb, canvas)
	}
}

// A light is carried by its name, not its number: the two listings number
// their lights differently. A whole-listing colour is a whole-canvas one,
// and a light the canvas has no name for is left out.
func TestAssignmentsAreCarriedByName(t *testing.T) {
	twin := &devices.Device{
		Name: "board (openrgb)", LEDCount: 4,
		LEDNames: []string{"Key: Escape", "Key: Logo", "Key: Up Arrow", "Key: A"},
		Zones:    []devices.Zone{{Name: "Keyboard", First: 0, Count: 4}},
	}
	canvas := &devices.Device{Name: "board", LEDCount: 3, LEDNames: []string{"A", "Up", "Escape"}}
	red, blue := colour.MustParse("red"), colour.MustParse("blue")
	got := carry(twin, canvas, devices.Rule{}, []devices.Assignment{
		{Target: devices.Target{Device: twin.Name, Part: "Keyboard", Picks: []config.LEDs{{First: 0, Last: 2}}}, Colour: red},
		{Target: devices.Target{Device: twin.Name}, Colour: blue},
	})

	require.Len(t, got, 2)
	require.Equal(t, canvas.Name, got[0].Target.Device)
	require.Equal(t, []config.LEDs{{First: 2, Last: 2}, {First: 1, Last: 1}}, got[0].Target.Picks,
		"Escape and Up are carried to where the canvas has them; Logo is not on it")
	require.Equal(t, red, got[0].Colour)
	require.Equal(t, devices.Target{Device: canvas.Name}, got[1].Target, "the whole listing is the whole canvas")
}
