package api_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/api"
	"github.com/ushineko/hotaru/internal/config"
	"github.com/ushineko/hotaru/internal/testsock"
)

func TestAShellReachesTheServiceOverTheSocket(t *testing.T) {
	client := drawing(t, nil, auraBoard(), k4())

	health, err := client.Health(t.Context())
	require.NoError(t, err)
	require.Equal(t, "healthy", health.State)
	require.Equal(t, 2, health.Devices)
	require.Equal(t, 2, health.InScope)
	require.NotEmpty(t, health.Version, "which hotaru is answering")
}

func TestTheDeviceListingAnswersWhyNothingHappened(t *testing.T) {
	cfg := &config.Config{Scope: []string{"maximus"}}
	board := auraBoard()
	client := drawing(t, cfg, board, k4())
	_, err := client.Apply(t.Context(), api.ApplyRequest{Colour: "red"})
	require.NoError(t, err)
	shows(t, board.Device, 1, "#ff0000")

	list, err := client.Devices(t.Context())
	require.NoError(t, err)
	require.Len(t, list, 2)
	byName := map[string]api.Device{}
	for _, d := range list {
		byName[d.Name] = d
	}
	asus := byName["ASUS ROG MAXIMUS Z790 HERO"]

	require.True(t, asus.InScope)
	require.Equal(t, []string{"Static", "Breathing", "Spectrum", "Rainbow Wave", "Off"}, asus.Modes)
	require.Equal(t, "Static", asus.ActiveMode)
	require.Equal(t, 4, asus.LEDs)
	require.Len(t, asus.Colours, 4, "what it is showing now")

	require.False(t, byName["Keychron K4 HE"].InScope, "so a user can see why it will not change")
}

func TestApplyingAColourTakesWhatAPersonWouldType(t *testing.T) {
	board := auraBoard()
	client := drawing(t, nil, board)

	out, err := client.Apply(t.Context(), api.ApplyRequest{
		Assignments: []api.Assignment{{Target: "ASUS", Colour: "orange"}},
	})
	require.NoError(t, err)
	require.Equal(t, 1, out.Changed)
	require.True(t, out.Results[0].Applied)
	require.Equal(t, "Static", out.Results[0].Mode)
	shows(t, board.Device, 1, "#ff5500")
}

func TestARequestThatChangedNothingSaysSo(t *testing.T) {
	cfg := &config.Config{Scope: []string{"nothing-here"}}
	client := drawing(t, cfg, auraBoard())

	out, err := client.Apply(t.Context(), api.ApplyRequest{
		Assignments: []api.Assignment{{Target: "ASUS", Colour: "blue"}},
	})
	require.NoError(t, err)
	require.Zero(t, out.Changed, "so a shell can exit non-zero without counting")
	require.Empty(t, out.Results)
}

func TestNonsenseIsRejectedBeforeAnythingIsWritten(t *testing.T) {
	board := auraBoard()
	client := drawing(t, nil, board)

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

	require.Zero(t, board.Frames(), "a rejected request wrote nothing")
}

func TestWithNoServiceRunningAShellSaysThatInOneLine(t *testing.T) {
	client := api.NewClient(filepath.Join(t.TempDir(), "absent.sock"))

	_, err := client.Health(t.Context())
	var down *api.NotRunning
	require.ErrorAs(t, err, &down)
	require.Contains(t, err.Error(), "hotaru is not running")
	require.Contains(t, err.Error(), "systemctl --user start hotaru")
}

func TestAStaleSocketIsClearedButALiveOneIsNot(t *testing.T) {
	socket := testsock.Path(t)

	first, err := api.Listen(t.Context(), socket)
	require.NoError(t, err)

	// Something is listening: taking the socket would silently break it.
	_, err = api.Listen(t.Context(), socket)
	require.ErrorContains(t, err, "already running")

	// Nothing is listening, but the file is still there — a killed process.
	// Refusing to start would cost a user their lighting over a path they have
	// never heard of.
	require.NoError(t, first.Close())
	require.NoError(t, os.WriteFile(socket, nil, 0o600))
	second, err := api.Listen(t.Context(), socket)
	require.NoError(t, err)
	require.NoError(t, second.Close())
}

func TestOneColourReachesEverythingInScope(t *testing.T) {
	// The common case, said the way a person says it: no listing first, no
	// enumeration of devices the client had to fetch.
	board, keys := auraBoard(), k4()
	client := drawing(t, nil, board, keys)

	out, err := client.Apply(t.Context(), api.ApplyRequest{Colour: "teal"})
	require.NoError(t, err)
	require.Equal(t, 2, out.Changed)

	shows(t, board.Device, 1, "#00ff80")
	shows(t, keys.Device, 1, "#00ff80")
}

func TestAColourAndAnAssignmentIsEverythingExceptOneThing(t *testing.T) {
	board := auraBoard()
	client := drawing(t, nil, board)

	out, err := client.Apply(t.Context(), api.ApplyRequest{
		Colour:      "blue",
		Assignments: []api.Assignment{{Target: "ASUS/Addressable 1[0:0]", Colour: "red"}},
	})
	require.NoError(t, err)
	require.Equal(t, 1, out.Changed)

	shows(t, board.Device, 1, "#ff0000") // the exception
	shows(t, board.Device, 2, "#0000ff") // and everything else
}

func TestASocketPathTooLongForTheKernelSaysThat(t *testing.T) {
	// The kernel answers "invalid argument", which sends someone to inspect
	// their permissions and their directory before their path length.
	long := filepath.Join(t.TempDir(), strings.Repeat("a", 120), "s")
	_, err := api.Listen(t.Context(), long)
	require.ErrorContains(t, err, "the socket path is")
	require.ErrorContains(t, err, "limit is")
}

func TestTheListingSaysWhichModesShowOneColourAndWhichTakeASpeed(t *testing.T) {
	/*
		A client drawing an editor has to know before it draws: a colour per
		light is a control on a device in Static and a promise the hardware
		will not keep on the same device in a reactive mode. Asked of the mode
		rather than of the device, exactly as Dimmable is, because a keyboard
		is both.
	*/
	client := drawing(t, nil, auraBoard(), k4())

	found, err := client.Devices(t.Context())
	require.NoError(t, err)

	var keys api.Device
	for _, device := range found {
		if device.Name == "Keychron K4 HE" {
			keys = device
		}
	}
	require.Contains(t, keys.OneColour, "Solid Reactive")
	require.NotContains(t, keys.OneColour, "Static", "Static takes a colour per light and must not be listed")

	pace, ok := keys.Paced["Breathing"]
	require.True(t, ok, "the mode's speed range was not published")
	require.Equal(t, api.Speed{Slowest: 0, Fastest: 100, Now: 50}, pace)
	require.NotContains(t, keys.Paced, "Static")
}

func TestTheListingSaysHowManyColoursEachModeTakes(t *testing.T) {
	// Spec 061 R1.3: one picker per slot, the first Least always. A mode
	// whose colour is the frame takes none of its own.
	client := drawing(t, nil, auraBoard(), k4())

	found, err := client.Devices(t.Context())
	require.NoError(t, err)
	byName := map[string]api.Device{}
	for _, device := range found {
		byName[device.Name] = device
	}

	require.Equal(t, map[string]api.ColourSlots{
		"Breathing":    {Least: 0, Most: 4},
		"Spectrum":     {Least: 0, Most: 8},
		"Rainbow Wave": {Least: 0, Most: 8},
	}, byName["ASUS ROG MAXIMUS Z790 HERO"].Coloured)
	// A firmware effect that carries one colour takes one.
	require.Equal(t, api.ColourSlots{Least: 1, Most: 1}, byName["Keychron K4 HE"].Coloured["Solid Reactive"])
}
