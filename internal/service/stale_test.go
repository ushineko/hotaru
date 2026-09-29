package service_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/openrgb"
	"github.com/ushineko/hotaru/internal/service"
)

/*
TestADeletedHandleIsNotHealthy is spec 058 AC2.

The state this is about is invisible from everything health used to look at.
The server answers, the device count is right, the scope is right, and the
hardware is not being driven -- so a listing that looks perfect is exactly the
listing a stale handle produces.
*/
func TestADeletedHandleIsNotHealthy(t *testing.T) {
	svc := service.New(nil, openrgb.NewFake(board(), keyboard()), "")
	svc.SetEnvironment(machine{holding: true})

	got := svc.Health(t.Context())
	require.Equal(t, service.StateStale, got.State,
		"a server holding a removed descriptor reported as healthy")
	require.False(t, got.OK())
	require.Equal(t, 2, got.InScope, "the devices are all still there, which is the point")
	require.Contains(t, got.Detail, "detects hardware once",
		"the reason a replug breaks it is not obvious and has to be said")
}

/*
TestTheStaleStateNamesTheDeviceThatMoved is spec 058 AC3.

The kernel and OpenRGB do not call the hardware the same thing. A keyboard the
server lists as "Keychron K4 HE" is "Keychron Keychron K4 HE" to the kernel,
and a state that could not bridge that would be reduced to saying "something".
*/
func TestTheStaleStateNamesTheDeviceThatMoved(t *testing.T) {
	svc := service.New(nil, openrgb.NewFake(board(), keyboard()), "")
	svc.SetEnvironment(machine{holding: true, moved: []string{"Keychron Keychron K4 HE"}})

	got := svc.Health(t.Context())
	require.Equal(t, service.StateStale, got.State)
	require.Contains(t, got.Detail, "Keychron K4 HE", "the device that moved was not named")
	require.NotContains(t, got.Detail, "ASUS", "a device that did not move was named")
}

/*
TestHardwareThatIsNotLightingIsNeverNamed guards the other half of AC3.

Most of the HID on a machine is a webcam, a headset or a power supply, and the
server holds descriptors for none of it. Naming those would turn one true
sentence into a list nobody can act on.
*/
func TestHardwareThatIsNotLightingIsNeverNamed(t *testing.T) {
	svc := service.New(nil, openrgb.NewFake(board(), keyboard()), "")
	svc.SetEnvironment(machine{holding: true, moved: []string{
		"Elgato Elgato Facecam", "CORSAIR HX1000i Power Supply", "SteelSeries Arctis Nova Pro Wireless",
	}})

	got := svc.Health(t.Context())
	require.Equal(t, service.StateStale, got.State, "the removed descriptor is still the fact")
	require.NotContains(t, got.Detail, "Facecam")
	require.NotContains(t, got.Detail, "Power Supply")
	require.Contains(t, got.Detail, "a device that is no longer there",
		"with nothing nameable it should still say what happened")
}

/*
TestWhatItCannotSeeItDoesNotClaim is spec 058 AC4.

A server running as root, one started by hand, and a machine without systemd
all answer "not known". None of them is evidence of a stale handle, and a
health check that guessed would be worse than one that does not look.
*/
func TestWhatItCannotSeeItDoesNotClaim(t *testing.T) {
	t.Run("a machine that cannot be asked", func(t *testing.T) {
		svc := service.New(nil, openrgb.NewFake(board(), keyboard()), "")
		svc.SetEnvironment(machine{holding: false})

		got := svc.Health(t.Context())
		require.Equal(t, service.StateHealthy, got.State)
		require.True(t, got.OK())
	})

	t.Run("no environment at all", func(t *testing.T) {
		got := service.New(nil, openrgb.NewFake(board(), keyboard()), "").Health(t.Context())
		require.Equal(t, service.StateHealthy, got.State)
	})

	t.Run("orphans without a removed descriptor are not a fault", func(t *testing.T) {
		// Every machine has HID the server does not drive. On its own that is
		// an ordinary machine, not a stale handle.
		svc := service.New(nil, openrgb.NewFake(board(), keyboard()), "")
		svc.SetEnvironment(machine{holding: false, moved: []string{"Keychron Keychron K4 HE"}})

		got := svc.Health(t.Context())
		require.Equal(t, service.StateHealthy, got.State,
			"a device with no descriptor was called stale without a removed one")
	})
}

/*
TestRescanIsOfferedEverywhereHealthIs is spec 058 AC8, the health half.

The state is useless without the way out. A person looking at "stale" has a
listing that looks entirely normal, so the sentence has to carry the command --
and one command, whether or not this particular server is theirs to restart,
because working that out first is not a step worth making somebody take.
*/
func TestRescanIsOfferedEverywhereHealthIs(t *testing.T) {
	svc := service.New(nil, openrgb.NewFake(board(), keyboard()), "")
	svc.SetEnvironment(machine{holding: true, moved: []string{"Keychron Keychron K4 HE"}})

	got := svc.Health(t.Context())
	require.Equal(t, service.StateStale, got.State)
	require.NotEmpty(t, got.Remedies)
	require.Contains(t, got.Remedies[0], "hotaru light rescan",
		"the state was reported without the command that fixes it")
	require.Contains(t, got.Remedies[0], "takes a few seconds",
		"a bounce that looks instant reads as a hang when it is not")
}
