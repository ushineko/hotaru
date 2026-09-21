package cooler_test

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/cooler"
)

/*
The tests that are not fakes.

Everything this package knows about the cooler was learned from the cooler.
The protocol is somebody else's firmware, so a model of it proves nothing about
the hardware -- the testing policy's rule, and spec 012's account of the day it
cost. These are the acceptance criteria that exercise the real device, written
down so they can be re-run rather than retyped.

They skip where there is no cooler, which is every CI machine and every
PKGBUILD check().

**Reading is free; writing is not.** A test that puts a picture on somebody's
screen changes their machine, so the screen tests are opt-in through
HOTARU_LIVE_SCREEN and put the firmware readout back when they finish.
*/
func live(t *testing.T) *cooler.Cooler {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	c, err := cooler.Open(ctx)
	if errors.Is(err, cooler.ErrNoCooler) {
		t.Skip("no supported liquid cooler on this machine")
	}
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestALiveCoolerReportsNumbersThatMakeSense(t *testing.T) {
	c := live(t)
	t.Logf("found %s at %s", c.Device().Name, c.Device().HID)

	status, err := c.Status(t.Context())
	require.NoError(t, err)
	t.Logf("coolant %.1f C, pump %d rpm (%d%%), fan %d rpm (%d%%)",
		status.Coolant, status.PumpRPM, status.PumpDuty, status.FanRPM, status.FanDuty)

	// Ranges rather than values: this is a running machine, not a fixture.
	require.Greater(t, status.Coolant, 10.0, "a coolant temperature below the room is a parse error")
	require.Less(t, status.Coolant, 90.0, "a coolant temperature this high is a fault or a parse error")
	require.Positive(t, status.PumpRPM, "a stopped pump, or the wrong bytes")
	require.LessOrEqual(t, status.PumpDuty, 100)
	require.LessOrEqual(t, status.FanDuty, 100)
	require.False(t, status.Taken.IsZero())
}

func TestALiveCoolerAgreesWithLiquidctl(t *testing.T) {
	/*
		The check that matters, and the only one that can catch a byte offset
		read off the wrong driver: two independent implementations of the same
		protocol, asked in the same minute.

		Skipped where liquidctl is not installed -- it is no longer a
		dependency, which is the point of this package.
	*/
	c := live(t)

	path, err := exec.LookPath("liquidctl")
	if err != nil {
		t.Skip("liquidctl is not installed, so there is nothing to compare against")
	}

	mine, err := c.Status(t.Context())
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "--match", "kraken", "status").Output()
	require.NoError(t, err)

	theirs := string(out)
	t.Logf("liquidctl says:\n%s", theirs)
	require.Contains(t, theirs, formatted(mine.PumpRPM),
		"hotaru and liquidctl disagree about the pump speed")
	require.Contains(t, theirs, formatted(mine.FanRPM),
		"hotaru and liquidctl disagree about the fan speed")
}

func formatted(rpm int) string {
	return strings.TrimSpace(strings.Join(strings.Fields(itoa(rpm)), ""))
}

func TestALiveCoolerSurvivesBeingLeftAlone(t *testing.T) {
	/*
		The bug that got past everything else: this cooler broadcasts about
		once a second, the kernel queues those per open handle, and a reader
		that does not clear them finds a dozen stale reports and concludes the
		device is silent.

		It only shows up after idleness, so a loop of calls never sees it --
		which is exactly why forty consecutive calls passed either side of the
		failure somebody reported. Six seconds is enough to queue more than a
		single read will look at.
	*/
	if testing.Short() {
		t.Skip("this one waits, deliberately")
	}
	c := live(t)

	_, err := c.Status(t.Context())
	require.NoError(t, err)

	time.Sleep(6 * time.Second)

	_, err = c.Status(t.Context())
	require.NoError(t, err, "a reading failed after the handle sat idle; the queue was not cleared")
}

func TestALiveScreenTakesAnImageAndGivesItBack(t *testing.T) {
	/*
		Opt-in, because it changes what somebody's machine is showing. It puts
		the firmware readout back whatever happens, including on failure: a
		test that leaves a checkerboard on a cooler is a test nobody runs
		twice.
	*/
	if os.Getenv("HOTARU_LIVE_SCREEN") == "" {
		t.Skip("set HOTARU_LIVE_SCREEN=1 to let this draw on the cooler")
	}
	c := live(t)

	screen, err := c.Screen()
	require.NoError(t, err)
	defer func() { _ = screen.Close() }()
	defer func() { _ = screen.Liquid(t.Context()) }()

	require.NoError(t, screen.Image(t.Context(), testCard()),
		"a still image did not reach the panel")

	// And the control channel still answers while the panel is claimed, which
	// is the coexistence the whole design rests on.
	_, err = c.Status(t.Context())
	require.NoError(t, err, "claiming the panel broke the control channel")

	require.NoError(t, screen.Liquid(t.Context()), "the screen could not be given back")
}

// testCard is a picture that is obviously not a coolant readout, so somebody
// watching can tell the test from the firmware.
func testCard() []byte {
	pal := color.Palette{
		color.RGBA{10, 10, 20, 255},
		color.RGBA{240, 200, 60, 255},
		color.RGBA{60, 180, 220, 255},
	}
	img := image.NewPaletted(image.Rect(0, 0, 640, 640), pal)
	for y := range 640 {
		for x := range 640 {
			i := uint8(1)
			if (x/80+y/80)%2 == 0 {
				i = 2
			}
			if y < 60 || y > 580 {
				i = 0
			}
			img.SetColorIndex(x, y, i)
		}
	}
	var buf bytes.Buffer
	_ = gif.EncodeAll(&buf, &gif.GIF{
		Image: []*image.Paletted{img}, Delay: []int{10}, LoopCount: 0,
		Config: image.Config{ColorModel: pal, Width: 640, Height: 640},
	})
	return buf.Bytes()
}

// itoa avoids pulling strconv in for one call in a log line.
func itoa(n int) string { return strconv.Itoa(n) }
