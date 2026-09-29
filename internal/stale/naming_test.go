package stale

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// What the server calls the hardware on this machine.
var listed = []string{
	"ASUS ROG MAXIMUS Z790 HERO", "Corsair MM700", "G502 X PLUS",
	"Keychron K4 HE", "MSI GeForce RTX 4090 Suprim Liquid X",
	"NZXT Kraken 2024 ELITE Series RGB",
}

/*
TestTheStaleStateNamesTheDeviceThatMoved is spec 058 AC3.

The kernel and OpenRGB do not call the hardware the same thing. A keyboard the
server lists as "Keychron K4 HE" is "Keychron Keychron K4 HE" to the kernel,
and a state that could not bridge that would be reduced to saying "something".
*/
func TestTheStaleStateNamesTheDeviceThatMoved(t *testing.T) {
	require.Equal(t, []string{"Keychron K4 HE"},
		named([]string{"Keychron Keychron K4 HE"}, listed))

	// The mousepad doubles its vendor too, and was checked on the machine.
	require.Equal(t, []string{"Corsair MM700"},
		named([]string{"Corsair CORSAIR MM700 RGB Gaming Mousepad"}, listed))
}

/*
TestHardwareThatIsNotLightingIsNeverNamed guards the other half.

Most of the HID on a machine is a webcam, a headset or a power supply, and the
server holds descriptors for none of it. Every one of these was in the orphan
list on the development machine while nothing at all was wrong, so naming them
would turn one true sentence into a list nobody can act on.
*/
func TestHardwareThatIsNotLightingIsNeverNamed(t *testing.T) {
	require.Empty(t, named([]string{
		"SteelSeries Arctis Nova Pro Wireless", "Generic USB Audio",
		"Elgato Elgato Facecam", "CORSAIR HX1000i Power Supply", "STC STC USB Keyboard",
	}, listed))
}

// A device the kernel could not name still gets the sentence: something moved,
// and that is true whether or not it left a name behind when it went.
func TestWithNothingNameableItStillSaysWhatHappened(t *testing.T) {
	require.Contains(t, detail(nil), "a device that is no longer there")
	require.Contains(t, detail(nil), "detects hardware once")
	require.Contains(t, detail([]string{"Keychron K4 HE"}), "Keychron K4 HE")
}
