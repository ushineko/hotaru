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
	"github.com/ushineko/hotaru/internal/canvas/canvastest"
	"github.com/ushineko/hotaru/internal/config"
)

// card is a graphics card of two lights, drawn as a canvas, whose Breathing
// takes several colours as hotaru's own renderer does.
func card() *canvastest.Board {
	return canvastest.NewBoard("Test Graphics Card", canvastest.Zone("Card", 2))
}

func TestAnEffectCanBeGivenSeveralColours(t *testing.T) {
	socket := drawing(t, &config.Config{}, auraBoard(), card())

	_, err := run(t, socket, "scene", "write", "evening", "graphics=white",
		"--effect", "graphics=Breathing",
		"--effect-colour", "graphics=#ff0000,#0000ff")
	require.NoError(t, err)

	said, err := run(t, socket, "scene", "show", "evening")
	require.NoError(t, err)
	require.Contains(t, said, "Breathing in #ff0000, #0000ff")
}

func TestAPictureGivesItsColoursFromTheCommandLine(t *testing.T) {
	socket := drawing(t, &config.Config{}, auraBoard(), card())

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

	// As many as the canvas's Breathing takes, which is four.
	said, err := run(t, socket, "image", "colours", "sunset", "--count", "4")
	require.NoError(t, err)
	lines := strings.Fields(said)
	require.GreaterOrEqual(t, len(lines), 2, said)

	// And a scene made from it gives the effect the same ones.
	_, err = run(t, socket, "image", "scene", "sunset", "themed", "--effect", "graphics=Breathing")
	require.NoError(t, err)
	shown, err := run(t, socket, "scene", "show", "themed")
	require.NoError(t, err)
	require.Contains(t, shown, "Breathing in "+strings.Join(lines, ", ")+" (from the picture)")
}
