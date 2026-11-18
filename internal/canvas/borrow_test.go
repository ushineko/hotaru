package canvas_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/hotaru/internal/canvas"
	"github.com/ushineko/hotaru/internal/canvas/canvastest"
)

// The cooler's lights are drawn on the cooler's own handle, which the cooler
// closes: nothing the canvas does closes it (spec 073 R1.2).
func TestABorrowedCanvasNeverClosesTheDevice(t *testing.T) {
	fake := canvastest.New(6)
	device, ok := canvas.Borrow(fake)
	require.True(t, ok)
	require.True(t, device.Borrowed())

	require.NoError(t, device.Release(t.Context()))
	require.Equal(t, 1, fake.Released())
	require.False(t, fake.Closed(), "releasing closed the cooler's handle")
	select {
	case <-device.Gone():
		t.Fatal("releasing the cooler's lights lost the device; it does not reboot")
	default:
	}

	require.NoError(t, device.Close())
	require.False(t, fake.Closed(), "closing the canvas closed the cooler's handle")

	device.Lose()
	require.False(t, fake.Closed(), "losing the canvas closed the cooler's handle")
	<-device.Gone()
}

// A cooler whose accessory list has not answered has no keys yet: it is not
// a canvas until it has (R1.3).
func TestACoolerWithNoKeysIsNotBorrowedYet(t *testing.T) {
	_, ok := canvas.Borrow(canvastest.New(0))
	require.False(t, ok)
}

// The Apex's release is unchanged: it reboots, so the handle goes (R2.2).
func TestAnOwnedCanvasIsStillLostOnRelease(t *testing.T) {
	fake := canvastest.New(6)
	device, ok := canvas.New(fake)
	require.True(t, ok)
	require.False(t, device.Borrowed())

	require.NoError(t, device.Release(t.Context()))
	<-device.Gone()
	require.True(t, fake.Closed())
}
