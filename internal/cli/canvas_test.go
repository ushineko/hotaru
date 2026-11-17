package cli_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/canvas/canvastest"
)

// canvasBoard is a canvas of three lights at a node, as the daemon finds one.
func canvasBoard() *canvastest.Device {
	fake := canvastest.New(3)
	fake.Name, fake.Path = "Test Canvas Board", "/dev/hidraw-test9"
	return fake
}

func TestTheListingSaysWhatACanvasIsDoing(t *testing.T) {
	socket := drawing(t, nil, canvasBoard())

	out, err := run(t, socket, "light", "list")
	require.NoError(t, err)
	require.Contains(t, out, "Static, Breathing, Spectrum, Rainbow Wave, Off", "the canvas does not offer its renderers")

	_, err = run(t, socket, "light", "set", "red")
	require.NoError(t, err)
	out, err = run(t, socket, "light", "list")
	require.NoError(t, err)
	require.Contains(t, out, "Static, holding", "a canvas holding its frame does not say so")

	out, _ = run(t, socket, "light", "health")
	require.Contains(t, out, "drawing on Test Canvas Board: Static, holding")
}

func TestReleaseSaysTheDeviceReenumerates(t *testing.T) {
	fake := canvasBoard()
	socket := drawing(t, nil, fake)

	out, err := run(t, socket, "light", "release", "canvas board")
	require.NoError(t, err)
	require.Contains(t, out, "re-enumerates")
	require.Equal(t, 1, fake.Released())

	_, err = run(t, socket, "light", "release", "nothing like it")
	require.Error(t, err)
}
