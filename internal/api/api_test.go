package api_test

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/api"
	"github.com/ushineko/hotaru/internal/config"
	"github.com/ushineko/hotaru/internal/devices"
	"github.com/ushineko/hotaru/internal/openrgb"
	"github.com/ushineko/hotaru/internal/service"
)

func board() devices.Device {
	return devices.Device{
		Name:     "ASUS ROG MAXIMUS Z790 HERO",
		LEDCount: 4,
		Modes: []devices.Mode{
			{Name: "Static", PerLED: true},
			{Name: "Direct", PerLED: true},
			{Name: "Rainbow Wave"},
		},
		Zones:      []devices.Zone{{Name: "Addressable 1", First: 0, Count: 4}},
		ActiveMode: "Rainbow Wave",
	}
}

func keyboard() devices.Device {
	return devices.Device{
		Name:       "Keychron K4 HE",
		LEDCount:   2,
		Modes:      []devices.Mode{{Name: "Direct", PerLED: true}},
		Zones:      []devices.Zone{{Name: "Keyboard", First: 0, Count: 2}},
		ActiveMode: "Direct",
	}
}

// running is the service over a real socket, as a shell meets it.
func running(t *testing.T, cfg *config.Config, server openrgb.Client) *api.Client {
	t.Helper()

	// A short path: a Unix socket's address has a low length limit, and
	// t.TempDir() under a long test name can exceed it.
	socket := filepath.Join(t.TempDir(), "s")
	listener, err := api.Listen(socket)
	require.NoError(t, err)

	svc := service.New(cfg, server, "127.0.0.1:6742")
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- api.Serve(ctx, listener, svc) }()

	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(3 * time.Second):
			t.Fatal("the service did not stop")
		}
	})
	return api.NewClient(socket)
}

func TestAShellReachesTheServiceOverTheSocket(t *testing.T) {
	client := running(t, nil, openrgb.NewFake(board(), keyboard()))

	health, err := client.Health(t.Context())
	require.NoError(t, err)
	require.Equal(t, "healthy", health.State)
	require.Equal(t, 2, health.Devices)
	require.Equal(t, 2, health.InScope)
	require.NotEmpty(t, health.Version, "which hotaru is answering")
}

func TestTheDeviceListingAnswersWhyNothingHappened(t *testing.T) {
	cfg := &config.Config{Scope: []string{"maximus"}}
	client := running(t, cfg, openrgb.NewFake(board(), keyboard()))

	list, err := client.Devices(t.Context())
	require.NoError(t, err)
	require.Len(t, list, 2)

	require.True(t, list[0].InScope)
	require.Equal(t, []string{"Static", "Direct", "Rainbow Wave"}, list[0].Modes)
	require.Equal(t, "Rainbow Wave", list[0].ActiveMode)
	require.Equal(t, 4, list[0].LEDs)
	require.Len(t, list[0].Colours, 4, "what it is showing now")

	require.False(t, list[1].InScope, "so a user can see why it will not change")
}

func TestApplyingAColourTakesWhatAPersonWouldType(t *testing.T) {
	server := openrgb.NewFake(board())
	client := running(t, nil, server)

	out, err := client.Apply(t.Context(), api.ApplyRequest{
		Assignments: []api.Assignment{{Target: "ASUS", Colour: "orange"}},
	})
	require.NoError(t, err)
	require.Equal(t, 1, out.Changed)
	require.True(t, out.Results[0].Applied)
	require.Equal(t, "Static", out.Results[0].Mode)

	showing, _ := server.Showing("ASUS ROG MAXIMUS Z790 HERO")
	require.Equal(t, "#ff5500", showing.Colours[0].String())
}

func TestTheFallThroughIsVisibleInTheResponse(t *testing.T) {
	// What the service discovered has to reach the person: two attempts, the
	// first accepted and not honoured.
	server := openrgb.NewFake(board())
	server.Lies["ASUS ROG MAXIMUS Z790 HERO"] = "Static"
	client := running(t, nil, server)

	out, err := client.Apply(t.Context(), api.ApplyRequest{
		Assignments: []api.Assignment{{Target: "ASUS", Colour: "blue"}},
	})
	require.NoError(t, err)
	require.Equal(t, "Direct", out.Results[0].Mode)
	require.Len(t, out.Results[0].Attempts, 2)
	require.True(t, out.Results[0].Attempts[0].Accepted)
	require.Equal(t, "Rainbow Wave", out.Results[0].Attempts[0].Active,
		"the server took the write and the device stayed where it was")
}

func TestARequestThatChangedNothingSaysSo(t *testing.T) {
	cfg := &config.Config{Scope: []string{"nothing-here"}}
	client := running(t, cfg, openrgb.NewFake(board()))

	out, err := client.Apply(t.Context(), api.ApplyRequest{
		Assignments: []api.Assignment{{Target: "ASUS", Colour: "blue"}},
	})
	require.NoError(t, err)
	require.Zero(t, out.Changed, "so a shell can exit non-zero without counting")
	require.Empty(t, out.Results)
}

func TestNonsenseIsRejectedBeforeAnythingIsWritten(t *testing.T) {
	server := openrgb.NewFake(board())
	client := running(t, nil, server)

	_, err := client.Apply(t.Context(), api.ApplyRequest{
		Assignments: []api.Assignment{{Target: "ASUS", Colour: "burgundy"}},
	})
	require.ErrorContains(t, err, "burgundy")

	_, err = client.Apply(t.Context(), api.ApplyRequest{
		Assignments: []api.Assignment{{Target: "ASUS/ring[9:1]", Colour: "red"}},
	})
	require.ErrorContains(t, err, "ends before it starts")

	_, err = client.Apply(t.Context(), api.ApplyRequest{})
	require.ErrorContains(t, err, "nothing to apply")

	require.Empty(t, server.Writes, "a rejected request wrote nothing")
}

func TestAnUnreachableOpenRGBIsTheMachineNotTheRequest(t *testing.T) {
	// A nil client is "no server", which is an ordinary state on a machine
	// where OpenRGB has not been started yet.
	client := running(t, nil, nil)

	_, err := client.Devices(t.Context())
	require.ErrorContains(t, err, "no OpenRGB server")
	require.ErrorContains(t, err, "start it")

	// Health still answers: a shell can ask what is wrong even when nothing
	// else works, which is the point of it being a separate route.
	health, err := client.Health(t.Context())
	require.NoError(t, err)
	require.Equal(t, "unreachable", health.State)
}

func TestWithNoServiceRunningAShellSaysThatInOneLine(t *testing.T) {
	client := api.NewClient(filepath.Join(t.TempDir(), "absent.sock"))

	_, err := client.Health(t.Context())
	var down *api.NotRunning
	require.ErrorAs(t, err, &down)
	require.Contains(t, err.Error(), "hotaru is not running")
	require.Contains(t, err.Error(), "systemctl --user start hotaru")
}

func TestTheSocketIsThisUsersAlone(t *testing.T) {
	// The socket's permissions are the whole authentication story: there is no
	// port and no token, and this is what stands in for both.
	socket := filepath.Join(t.TempDir(), "s")
	listener, err := api.Listen(socket)
	require.NoError(t, err)
	defer func() { _ = listener.Close() }()

	require.Equal(t, socket, listener.Addr().(*net.UnixAddr).Name)

	stat, err := os.Stat(socket)
	require.NoError(t, err)
	require.Equal(t, api.SocketMode, stat.Mode().Perm())
}

func TestAStaleSocketIsClearedButALiveOneIsNot(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "s")

	first, err := api.Listen(socket)
	require.NoError(t, err)

	// Something is listening: taking the socket would silently break it.
	_, err = api.Listen(socket)
	require.ErrorContains(t, err, "already running")

	// Nothing is listening, but the file is still there — a killed process.
	// Refusing to start would cost a user their lighting over a path they have
	// never heard of.
	require.NoError(t, first.Close())
	require.NoError(t, os.WriteFile(socket, nil, 0o600))
	second, err := api.Listen(socket)
	require.NoError(t, err)
	require.NoError(t, second.Close())
}

func TestOneColourReachesEverythingInScope(t *testing.T) {
	// The common case, said the way a person says it: no listing first, no
	// enumeration of devices the client had to fetch.
	server := openrgb.NewFake(board(), keyboard())
	client := running(t, nil, server)

	out, err := client.Apply(t.Context(), api.ApplyRequest{Colour: "teal"})
	require.NoError(t, err)
	require.Equal(t, 2, out.Changed)

	for _, name := range []string{"ASUS ROG MAXIMUS Z790 HERO", "Keychron K4 HE"} {
		showing, ok := server.Showing(name)
		require.True(t, ok)
		require.Equal(t, "#00ff80", showing.Colours[0].String(), name)
	}
}

func TestAColourAndAnAssignmentIsEverythingExceptOneThing(t *testing.T) {
	server := openrgb.NewFake(board())
	client := running(t, nil, server)

	out, err := client.Apply(t.Context(), api.ApplyRequest{
		Colour:      "blue",
		Assignments: []api.Assignment{{Target: "ASUS/Addressable 1[0:0]", Colour: "red"}},
	})
	require.NoError(t, err)
	require.Equal(t, 1, out.Changed)

	showing, _ := server.Showing("ASUS ROG MAXIMUS Z790 HERO")
	require.Equal(t, "#ff0000", showing.Colours[0].String(), "the exception")
	require.Equal(t, "#0000ff", showing.Colours[1].String(), "and everything else")
}

func TestASocketPathTooLongForTheKernelSaysThat(t *testing.T) {
	// The kernel answers "invalid argument", which sends someone to inspect
	// their permissions and their directory before their path length.
	long := filepath.Join(t.TempDir(), strings.Repeat("a", 120), "s")
	_, err := api.Listen(long)
	require.ErrorContains(t, err, "the socket path is")
	require.ErrorContains(t, err, "limit is")
}
