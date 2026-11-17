package canvas_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hotaru/internal/canvas"
	"github.com/ushineko/hotaru/internal/canvas/canvastest"
	"github.com/ushineko/sanshoku/lighting"
)

// A canvas with no keys is not one: a Logitech receiver with nothing lit
// behind it (spec 075 R1.2).
func TestAKeylessCanvasIsNotOne(t *testing.T) {
	_, ok := canvas.New(canvastest.New(0))
	assert.False(t, ok)
}

// A zoned canvas gives its zones, and a header's length changes its keys.
func TestAZonedCanvasIsResized(t *testing.T) {
	board := canvastest.NewBoard("fake board", canvastest.Zone("Board", 1), canvastest.Header("Header 1", 0, 500))
	device, ok := canvas.New(board)
	require.True(t, ok)
	assert.Len(t, device.Keys(), 1)
	assert.Equal(t, "Header 1", device.Zones()[1].Name)

	require.NoError(t, device.Resize("Header 1", 18))
	assert.Len(t, device.Keys(), 19, "the keys were not read again")

	require.ErrorIs(t, device.Resize("Board", 2), lighting.ErrNoZone)
	require.ErrorIs(t, device.Resize("Header 9", 2), lighting.ErrNoZone)
	require.Error(t, device.Resize("Header 1", 501))
	assert.Len(t, device.Keys(), 19)

	plain, ok := canvas.New(canvastest.New(6))
	require.True(t, ok)
	assert.Nil(t, plain.Zones())
	require.ErrorIs(t, plain.Resize("Header 1", 3), lighting.ErrNoZone)
}
