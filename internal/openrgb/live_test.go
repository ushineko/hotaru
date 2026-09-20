package openrgb_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/openrgb"
)

/*
The one test that is not a fake.

The integration boundary here is a wire protocol, and a fake cannot fail the way
a real server can: a field that moved between protocol versions, a device whose
zone sizes do not add up to its LED count, a name with a character nobody
expected. So this runs against a live server when there is one and skips when
there is not — which is every CI machine and every PKGBUILD check().

**It writes nothing.** A test that set a colour would change the lighting of
whoever ran it, which is the same discourtesy as a fresh install stamping over
someone's configuration. Everything here is a read.
*/
func TestALiveServerReportsHardwareThatMakesSense(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()

	conn, err := openrgb.Dial(ctx, openrgb.DefaultAddress)
	if err != nil {
		t.Skipf("no OpenRGB server on %s: %v", openrgb.DefaultAddress, err)
	}
	defer func() { require.NoError(t, conn.Close()) }()

	require.NotZero(t, conn.ProtocolVersion(),
		"a version was agreed, which is what health reports")

	list, err := conn.Devices(ctx)
	require.NoError(t, err)
	t.Logf("protocol %d, %d devices", conn.ProtocolVersion(), len(list))

	for _, device := range list {
		t.Run(device.Name, func(t *testing.T) {
			require.NotEmpty(t, device.Name)
			require.NotContains(t, device.Name, "\x00",
				"the wire's C string terminator does not belong in a name")
			for _, mode := range device.ModeNames() {
				require.NotContains(t, mode, "\x00")
			}
			require.Equal(t, device.LEDCount, len(device.Colours),
				"a device reports one colour per LED, which is what a frame has to match")

			// Zones are contiguous runs the protocol gives sizes for and
			// leaves offsets to be counted. If that assumption is wrong on
			// real hardware, a segment addresses the wrong lights.
			next := 0
			for _, zone := range device.Zones {
				require.Equal(t, next, zone.First, "zone %q starts where the previous ended", zone.Name)
				next += zone.Count
			}
			if len(device.Zones) > 0 {
				require.LessOrEqual(t, next, device.LEDCount,
					"zones fit inside the device")
			}

			if device.ActiveMode != "" {
				require.Contains(t, device.ModeNames(), device.ActiveMode,
					"the active mode is one of the modes it listed")
			}
			/*
				Which modes carry their own colour, and what colour each
				holds.

				Read-only, and worth reporting: a mode hotaru sets without
				also setting its colour displays whatever the vendor left
				there, which is how a request for purple lit three radiator
				fans red. Seeing the flag and the stored colour on real
				hardware is what says the fix has something true to act on.
				See spec 009.
			*/
			for _, mode := range device.Modes {
				if !mode.ModeColour {
					continue
				}
				t.Logf("  mode %q carries its own colour, holding %s", mode.Name, mode.Colour)
				require.False(t, mode.PerLED && mode.Name == "",
					"a mode with no name cannot be chosen")
			}

			t.Logf("%d LEDs, %d zones, modes: %v (active %q)",
				device.LEDCount, len(device.Zones), device.ModeNames(), device.ActiveMode)
		})
	}
}
