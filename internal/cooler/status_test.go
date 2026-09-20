package cooler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStatusIsReadFromTheReplyNotTheBroadcast(t *testing.T) {
	/*
		The device streams 0x75 0x02 reports unasked: eleven of them arrived
		in the twelve reads after one request on the development machine. A
		reader matching only the first byte parses a broadcast and calls it a
		reply -- and because the broadcast carries status too, it even looks
		right.

		The fake chatters for the same reason, so this is a test and not a
		hope.
	*/
	fake := NewFake()
	fake.Chatter = 11
	c := NewWithFake(fake)

	status, err := c.Status(context.Background())
	require.NoError(t, err)
	require.InDelta(t, 37.5, status.Coolant, 0.05)
	require.Equal(t, 2608, status.PumpRPM)
	require.Equal(t, 81, status.PumpDuty)
	require.Equal(t, 1190, status.FanRPM)
	require.Equal(t, 51, status.FanDuty)
	require.False(t, status.Taken.IsZero(), "a reading with no time on it cannot be judged stale")
}

func TestTheStatusRequestIsTheOneLiquidctlSends(t *testing.T) {
	// The protocol is read off liquidctl's driver, and this is the line that
	// says so: if the request changes, the comparison that verified the
	// offsets no longer holds.
	fake := NewFake()
	c := NewWithFake(fake)

	_, err := c.Status(context.Background())
	require.NoError(t, err)
	require.Len(t, fake.Told, 1)
	require.Equal(t, []byte{0x74, 0x01}, fake.Told[0])
}

func TestAFirmwareFaultIsNotReportedAsATemperature(t *testing.T) {
	/*
		liquidctl#172: a faulted controller reports 0xFFFF where a temperature
		belongs. Parsed, that is 255.5 degrees -- which would raise an alarm
		about a coolant temperature rather than about the cooler.
	*/
	fake := NewFake()
	fake.Faulty = true
	c := NewWithFake(fake)

	_, err := c.Status(context.Background())
	require.ErrorContains(t, err, "firmware fault")
}

func TestADeviceThatStopsAnsweringIsAnError(t *testing.T) {
	// Not a zero reading. A caller is about to read numbers out of this, and
	// a coolant temperature of zero is a plausible-looking lie.
	fake := NewFake()
	fake.Silent = true
	c := NewWithFake(fake)

	_, err := c.Status(context.Background())
	require.Error(t, err)
}

func TestAnAbandonedReadGivesUp(t *testing.T) {
	// A question is a place a program waits, and a service being shut down
	// must not wait for a device that has stopped talking.
	fake := NewFake()
	fake.Chatter = 1000
	c := NewWithFake(fake)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.Status(ctx)
	require.ErrorIs(t, err, context.Canceled)
}
