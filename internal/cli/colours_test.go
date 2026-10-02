package cli_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/config"
	"github.com/ushineko/hotaru/internal/devices"
	"github.com/ushineko/hotaru/internal/openrgb"
)

// card is a device whose Breathing takes one or two colours, as OpenRGB
// reported a graphics card's doing on the machines spec 061 measured.
func card() devices.Device {
	return devices.Device{
		Name:     "Test Graphics Card",
		LEDCount: 2,
		Modes: []devices.Mode{
			{Name: "Direct", PerLED: true},
			{Name: "Breathing", ModeColour: true, ColoursMin: 1, ColoursMax: 2},
		},
		Zones:      []devices.Zone{{Name: "Card", First: 0, Count: 2}},
		ActiveMode: "Direct",
	}
}

func TestAnEffectCanBeGivenSeveralColours(t *testing.T) {
	socket := serving(t, &config.Config{}, openrgb.NewFake(board(), card()))

	_, err := run(t, socket, "scene", "write", "evening", "graphics=white",
		"--effect", "graphics=Breathing",
		"--effect-colour", "graphics=#ff0000,#0000ff")
	require.NoError(t, err)

	said, err := run(t, socket, "scene", "show", "evening")
	require.NoError(t, err)
	require.Contains(t, said, "Breathing in #ff0000, #0000ff")
}

func TestAPictureGivesItsColoursFromTheCommandLine(t *testing.T) {
	socket := serving(t, &config.Config{}, openrgb.NewFake(board(), card()))

	picture := image.NewRGBA(image.Rect(0, 0, 40, 40))
	for y := range 40 {
		for x := range 40 {
			c := color.RGBA{R: 255, A: 255}
			if x >= 30 {
				c = color.RGBA{B: 255, A: 255}
			}
			picture.Set(x, y, c)
		}
	}
	var encoded bytes.Buffer
	require.NoError(t, png.Encode(&encoded, picture))
	path := filepath.Join(t.TempDir(), "sunset.png")
	require.NoError(t, os.WriteFile(path, encoded.Bytes(), 0o600))
	_, err := run(t, socket, "image", "add", "sunset", path)
	require.NoError(t, err)

	said, err := run(t, socket, "image", "colours", "sunset", "--count", "2")
	require.NoError(t, err)
	lines := strings.Fields(said)
	require.Len(t, lines, 2, said)

	// And a scene made from it gives the effect the same two.
	_, err = run(t, socket, "image", "scene", "sunset", "themed", "--effect", "graphics=Breathing")
	require.NoError(t, err)
	shown, err := run(t, socket, "scene", "show", "themed")
	require.NoError(t, err)
	require.Contains(t, shown, "Breathing in "+lines[0]+", "+lines[1]+" (from the picture)")
}
