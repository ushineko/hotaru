package cooler_test

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/cooler"
	"github.com/ushineko/hotaru/internal/cooler/coolertest"
	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/screen"
)

// oneFrame is the smallest GIF there is: one pixel, one frame.
func oneFrame(t *testing.T) []byte {
	t.Helper()
	frame := image.NewPaletted(image.Rect(0, 0, 1, 1), color.Palette{color.Black, color.White})
	var buf bytes.Buffer
	require.NoError(t, gif.EncodeAll(&buf, &gif.GIF{Image: []*image.Paletted{frame}, Delay: []int{0}}))
	return buf.Bytes()
}

func TestTheReadingPassesThrough(t *testing.T) {
	dev := coolertest.New()
	c := cooler.New(dev)

	got, err := c.Status(t.Context())
	require.NoError(t, err)
	require.InDelta(t, 37.5, got.Coolant, 0.05)
	require.Equal(t, 2608, got.PumpRPM)
	require.Equal(t, 1190, got.FanRPM)
	require.False(t, got.Taken.IsZero(), "a reading with no time on it cannot be judged stale")

	// And a failure is the device's, unchanged: the API names it.
	dev.Err = errors.New("no 7501 reply")
	_, err = c.Status(t.Context())
	require.ErrorContains(t, err, "no 7501 reply")
}

func TestTheDeviceIsReportedFromItsIdentityAndPanel(t *testing.T) {
	d := cooler.New(coolertest.NewPanel()).Device()
	require.Equal(t, "fake cooler", d.Name)
	require.Equal(t, uint16(0x3012), d.Product)
	require.Equal(t, "/dev/hidraw-fake", d.HID)
	require.Equal(t, "640x640 LCD", d.Screen)
}

func TestShowDecodesTheGIFAndDraws(t *testing.T) {
	dev := coolertest.NewPanel()
	c := cooler.New(dev)

	require.NoError(t, c.Show(t.Context(), oneFrame(t)))
	require.Equal(t, 1, dev.Images())

	// Bytes that are not a GIF are refused here, before the panel is asked.
	require.Error(t, c.Show(t.Context(), []byte("not a gif")))
	require.Equal(t, 1, dev.Images())

	require.Equal(t, 2*time.Second, c.Floor(8*1024), "the floor is the panel's")
}

func TestNoPanelIsNoScreen(t *testing.T) {
	/*
		A cooler without a panel is a machine without one: the scene still
		applies and the lights still change. Every caller already treats
		ErrNoScreen that way, so that is what this has to say.
	*/
	c := cooler.New(coolertest.New())

	require.ErrorIs(t, c.Show(t.Context(), oneFrame(t)), cooler.ErrNoScreen)
	require.ErrorIs(t, c.Readout(t.Context()), cooler.ErrNoScreen)
	require.ErrorIs(t, c.Appearance(t.Context(), 50, 0), cooler.ErrNoScreen)
	desc, err := c.Panel()
	require.Empty(t, desc)
	require.ErrorIs(t, err, cooler.ErrNoScreen)
	require.Empty(t, c.Device().Screen)
	require.Zero(t, c.Floor(8*1024))
}

func TestAPanelThatWillNotBeClaimedIsNoScreen(t *testing.T) {
	// Another program holds the interface. The driver says ErrNoPanel; the
	// service has always heard ErrNoScreen, and the window shows why once.
	dev := coolertest.NewPanel()
	dev.Refuse = fmt.Errorf("%w: %w: claim interface 0: device busy", screen.ErrNoPanel, sanshoku.ErrAbsent)
	c := cooler.New(dev)

	desc, wrong := c.Panel()
	require.Equal(t, "640x640 LCD", desc)
	require.NoError(t, wrong, "nothing has tried the panel yet")

	require.ErrorIs(t, c.Show(t.Context(), oneFrame(t)), cooler.ErrNoScreen)
	_, wrong = c.Panel()
	require.ErrorIs(t, wrong, cooler.ErrNoScreen)
	require.ErrorContains(t, wrong, "device busy")
}

func TestTheSentinelsAreSanshokus(t *testing.T) {
	// So that a caller asking the library's question gets the same answer.
	require.ErrorIs(t, cooler.ErrNoCooler, sanshoku.ErrAbsent)
	require.ErrorIs(t, cooler.ErrNoScreen, screen.ErrNoPanel)
	require.Equal(t, "no supported liquid cooler", cooler.ErrNoCooler.Error())
}

func TestAGoneDeviceClosesTheAdapter(t *testing.T) {
	/*
		An unplugged cooler. The adapter closes the device and says so, once,
		and the daemon looks for it again; before spec 059 the service held
		the dead handle until it was restarted.
	*/
	dev := coolertest.New()
	dev.GoneOnce = true
	c := cooler.New(dev)

	select {
	case <-c.Gone():
		t.Fatal("a cooler that has not been read was reported gone")
	default:
	}

	_, err := c.Status(t.Context())
	require.ErrorIs(t, err, sanshoku.ErrGone)
	require.True(t, dev.Closed(), "the device was not closed")
	select {
	case <-c.Gone():
	default:
		t.Fatal("the adapter did not say the device had gone")
	}

	// Gone is said once; a second failure does not close a closed channel.
	dev.GoneOnce = true
	_, err = c.Status(t.Context())
	require.ErrorIs(t, err, sanshoku.ErrGone)
}
