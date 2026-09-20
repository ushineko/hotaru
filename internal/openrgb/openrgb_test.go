package openrgb_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/devices"
	"github.com/ushineko/hotaru/internal/openrgb"
)

func strip() devices.Device {
	return devices.Device{
		Name:     "Generic Strip",
		LEDCount: 4,
		Modes: []devices.Mode{
			{Name: "Static", PerLED: true},
			{Name: "Breathing"},
		},
		Zones:      []devices.Zone{{Name: "Strip", First: 0, Count: 4}},
		ActiveMode: "Breathing",
	}
}

func TestTheFakeShowsWhatWasWrittenToIt(t *testing.T) {
	server := openrgb.NewFake(strip())
	red := colour.MustParse("red")

	require.NoError(t, server.SetFrame(t.Context(), "Generic Strip", devices.Frame{
		Device:  "Generic Strip",
		Colours: []colour.Colour{red, red, red, red},
	}))

	showing, ok := server.Showing("Generic Strip")
	require.True(t, ok)
	require.Equal(t, red, showing.Colours[0])
	require.Len(t, server.Writes, 1, "and the write itself was recorded")
}

func TestAFrameThatIsTheWrongSizeIsRefusedRatherThanPartlyApplied(t *testing.T) {
	server := openrgb.NewFake(strip())
	err := server.SetFrame(t.Context(), "Generic Strip", devices.Frame{
		Device:  "Generic Strip",
		Colours: []colour.Colour{colour.MustParse("red")},
	})
	require.ErrorContains(t, err, "has 4 LEDs and the frame has 1")
}

func TestADeviceCanAcceptAModeAndNotHonourIt(t *testing.T) {
	// The ASUS board: Static is accepted, reports success, and leaves the
	// addressable headers dark. Nothing but a read-back notices, which is why
	// the fake can do it and why resolution offers a list of candidates.
	server := openrgb.NewFake(strip())
	server.Lies["Generic Strip"] = "Static"

	require.NoError(t, server.SetMode(t.Context(), "Generic Strip", "Static", nil))

	list, err := server.Devices(t.Context())
	require.NoError(t, err)
	require.Equal(t, "Breathing", list[0].ActiveMode, "the write landed nowhere")
	require.Len(t, server.Modes, 1, "though the server said yes")

	// A mode it does not lie about takes normally.
	require.NoError(t, server.SetMode(t.Context(), "Generic Strip", "Breathing", nil))
	list, err = server.Devices(t.Context())
	require.NoError(t, err)
	require.Equal(t, "Breathing", list[0].ActiveMode)
}

func TestAStoppedServerFailsEveryCallRatherThanSomeOfThem(t *testing.T) {
	server := openrgb.NewFake(strip())
	server.Unreachable = errors.New("connection refused")

	_, err := server.Devices(t.Context())
	require.Error(t, err)
	require.Error(t, server.SetMode(t.Context(), "Generic Strip", "Static", nil))
	require.Error(t, server.SetFrame(t.Context(), "Generic Strip", devices.Frame{}))
}

func TestADeviceThatArrivesLateIsSeenOnTheNextListing(t *testing.T) {
	// OpenRGB enumerates once at server start, so this is what a restart looks
	// like from hotaru's side, and what reconciliation waits for.
	server := openrgb.NewFake(strip())
	server.Add(devices.Device{Name: "Late Arrival", LEDCount: 2})

	list, err := server.Devices(t.Context())
	require.NoError(t, err)
	require.Len(t, list, 2)
}

func TestWhatIsNotThereIsNamedRatherThanIgnored(t *testing.T) {
	server := openrgb.NewFake(strip())
	require.ErrorContains(t, server.SetFrame(t.Context(), "Nothing", devices.Frame{}), `no device called "Nothing"`)
	require.ErrorContains(t, server.SetMode(t.Context(), "Generic Strip", "Rainbow", nil), `no mode called "Rainbow"`)
}

func TestDialingNothingSaysWhereItTried(t *testing.T) {
	// A refused connection is an ordinary result: the server may not be
	// running, and hotaru's job is to say so, naming the address.
	_, err := openrgb.Dial(t.Context(), "127.0.0.1:1")
	require.Error(t, err)
	require.ErrorContains(t, err, "127.0.0.1:1")
	require.ErrorContains(t, err, "OpenRGB server")
}

func TestDialingHonoursCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := openrgb.Dial(ctx, openrgb.DefaultAddress)
	require.Error(t, err)
}
