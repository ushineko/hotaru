package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"go/build"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/api"
	"github.com/ushineko/hotaru/internal/cli"
	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/config"
	"github.com/ushineko/hotaru/internal/devices"
	"github.com/ushineko/hotaru/internal/images"
	"github.com/ushineko/hotaru/internal/openrgb"
	"github.com/ushineko/hotaru/internal/scenes"
	"github.com/ushineko/hotaru/internal/service"
	"github.com/ushineko/hotaru/internal/state"
)

/*
The guarantee, asserted rather than described.

"The service is the only writer" is worth more as a fact about the import graph
than as a sentence in a document: if this package cannot reach a device
package, no amount of future carelessness can add a direct write to a CLI
command without the test saying so.
*/
func TestTheClientCommandsCannotReachADevice(t *testing.T) {
	pkg, err := build.ImportDir(".", 0)
	require.NoError(t, err)

	// The package's own imports, and those of any in-package test. This file
	// is an external test package and does import the service, because setting
	// up a fake one is what a test does -- but nothing a command can call at
	// run time may reach a device.

	forbidden := []string{
		"internal/openrgb", // the hardware
		"internal/service", // the thing that drives it
		"internal/devices", // and the knowledge of how
	}
	/*
		internal/desktop is deliberately not on that list. The rule is about
		devices: no command may write a light. Editing the desktop's own
		shortcut file is not a device write, and it happens here precisely
		because the service must not be able to do it -- its unit gives it
		write access to its own two directories and nothing else.
	*/
	for _, imported := range append(pkg.Imports, pkg.TestImports...) {
		for _, banned := range forbidden {
			require.NotContains(t, imported, banned,
				"a client command imported %s; the CLI asks the service, it does not do the work", imported)
		}
	}
}

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

// run executes a command against a running service and returns its output.
func run(t *testing.T, socket string, args ...string) (string, error) {
	t.Helper()

	root := &cobra.Command{Use: "hotaru", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().String("socket", "", "")
	root.AddCommand(cli.Commands()...)
	require.NoError(t, root.PersistentFlags().Set("socket", socket))

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)

	err := root.ExecuteContext(t.Context())
	return out.String(), err
}

// serving starts a service over a socket and returns its path.
func serving(t *testing.T, cfg *config.Config, server openrgb.Client) string {
	t.Helper()

	dir := t.TempDir()

	// Never the developer's own configuration. A test that reads ~/.config
	// passes or fails on whatever happens to be on the machine it runs on,
	// which is the opposite of a test.
	if os.Getenv("XDG_CONFIG_HOME") == "" {
		t.Setenv("XDG_CONFIG_HOME", dir)
	}

	socket := filepath.Join(dir, "s")
	listener, err := api.Listen(t.Context(), socket)
	require.NoError(t, err)

	// A recorder, as the daemon gives it: without one the service drives
	// hardware and remembers nothing, so there is nothing to restore.
	desired, err := state.Open(filepath.Join(dir, "state.yml"))
	require.NoError(t, err)
	svc := service.New(cfg, server, "127.0.0.1:6742")
	svc.SetRecorder(desired)
	// Where its rules live, as the daemon tells it at startup. Without this a
	// reload is a no-op, and the wizard's "in use now" would be a lie.
	if rules, err := config.RulesPath(); err == nil {
		svc.SetRulesPath(rules)
	}
	// And a picture library and somewhere to keep scenes, as the daemon also
	// gives it: under the test's own directory, so nothing this writes is
	// kept on the machine running it.
	if library, err := images.Open(filepath.Join(dir, "images")); err == nil {
		svc.SetImages(library)
	}
	if saved, err := scenes.Open(filepath.Join(dir, "scenes.yml")); err == nil {
		svc.SetScenes(saved)
	}

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- api.Serve(ctx, listener, svc) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatal("the service did not stop")
		}
	})
	return socket
}

func TestListShowsWhatIsThereAndWhetherItIsDriven(t *testing.T) {
	socket := serving(t, &config.Config{Scope: []string{"maximus"}}, openrgb.NewFake(board()))

	out, err := run(t, socket, "light", "list")
	require.NoError(t, err)
	require.Contains(t, out, "ASUS ROG MAXIMUS Z790 HERO")
	require.Contains(t, out, "Rainbow Wave")
	require.Contains(t, out, "yes", "in scope")
}

func TestSetTakesOneColourForEverything(t *testing.T) {
	server := openrgb.NewFake(board())
	socket := serving(t, nil, server)

	out, err := run(t, socket, "light", "set", "red")
	require.NoError(t, err)
	require.Contains(t, out, "ASUS ROG MAXIMUS Z790 HERO: Static")

	showing, _ := server.Showing("ASUS ROG MAXIMUS Z790 HERO")
	require.Equal(t, "#ff0000", showing.Colours[0].String())
}

func TestSetTakesExceptionsAfterTheColour(t *testing.T) {
	server := openrgb.NewFake(board())
	socket := serving(t, nil, server)

	_, err := run(t, socket, "light", "set", "blue", "ASUS/Addressable 1[0:0]=red")
	require.NoError(t, err)

	showing, _ := server.Showing("ASUS ROG MAXIMUS Z790 HERO")
	require.Equal(t, "#ff0000", showing.Colours[0].String(), "the exception")
	require.Equal(t, "#0000ff", showing.Colours[1].String(), "and the rest")
}

func TestWhatTheDeviceDidInsteadIsPrintedNotHidden(t *testing.T) {
	server := openrgb.NewFake(board())
	server.Lies["ASUS ROG MAXIMUS Z790 HERO"] = "Static"
	socket := serving(t, nil, server)

	out, err := run(t, socket, "light", "set", "red")
	require.NoError(t, err)
	require.Contains(t, out, "tried Static first; the device stayed in Rainbow Wave")
	require.Contains(t, out, ": Direct")
}

func TestASceneThatLitNothingExitsNonZero(t *testing.T) {
	socket := serving(t, &config.Config{Scope: []string{"absent"}}, openrgb.NewFake(board()))

	_, err := run(t, socket, "light", "set", "red")
	require.Error(t, err, "a scene that lit nothing has not worked")
	require.True(t, cli.Silent(err), "and the device lines above already said so")
}

func TestHealthExplainsAndExitsNonZeroWhenItIsNotHealthy(t *testing.T) {
	socket := serving(t, nil, nil)

	out, err := run(t, socket, "light", "health")
	require.Error(t, err)
	require.True(t, cli.Silent(err))
	require.Contains(t, out, "unreachable")
	require.Contains(t, out, "Start it")
}

func TestJSONIsAStableShapeForScripts(t *testing.T) {
	socket := serving(t, nil, openrgb.NewFake(board()))

	out, err := run(t, socket, "light", "list", "--json")
	require.NoError(t, err)

	var body []api.Device
	require.NoError(t, json.Unmarshal([]byte(out), &body))
	require.Len(t, body, 1)
	require.Equal(t, 4, body[0].LEDs)
	require.True(t, body[0].InScope)

	out, err = run(t, socket, "light", "set", "red", "--json")
	require.NoError(t, err)
	var applied api.ApplyResponse
	require.NoError(t, json.Unmarshal([]byte(out), &applied))
	require.Equal(t, 1, applied.Changed)
}

func TestWithNoServiceEveryCommandSaysSoInOneLine(t *testing.T) {
	absent := filepath.Join(t.TempDir(), "absent.sock")

	for _, args := range [][]string{
		{"light", "list"},
		{"light", "set", "red"},
		{"light", "off"},
		{"light", "health"},
	} {
		out, err := run(t, absent, args...)
		require.Error(t, err, strings.Join(args, " "))
		require.Contains(t, err.Error(), "hotaru is not running")
		require.NotContains(t, out, "panic")
	}
}

func TestTwoColoursWithoutTargetsIsAUsageErrorNotAGuess(t *testing.T) {
	socket := serving(t, nil, openrgb.NewFake(board()))

	_, err := run(t, socket, "light", "set", "red", "blue")
	require.ErrorContains(t, err, "one colour for everything")
}

func TestStatusSaysWhatIsRunningAndWhatItRemembers(t *testing.T) {
	server := openrgb.NewFake(board())
	socket := serving(t, nil, server)

	out, err := run(t, socket, "status")
	require.NoError(t, err)
	require.Contains(t, out, "OpenRGB")
	require.Contains(t, out, "remembers nothing yet, so it restores nothing",
		"the inert state said plainly rather than left as an absence")

	_, err = run(t, socket, "light", "set", "red")
	require.NoError(t, err)

	out, err = run(t, socket, "status")
	require.NoError(t, err)
	require.Contains(t, out, "remembers ASUS ROG MAXIMUS Z790 HERO")
}

func TestReconcileFromTheCommandLinePutsThingsBack(t *testing.T) {
	server := openrgb.NewFake(board())
	socket := serving(t, nil, server)

	out, err := run(t, socket, "reconcile")
	require.NoError(t, err)
	require.Contains(t, out, "nothing to put back")

	_, err = run(t, socket, "light", "set", "teal")
	require.NoError(t, err)
	require.NoError(t, server.SetFrame(t.Context(), "ASUS ROG MAXIMUS Z790 HERO", devices.Frame{
		Device: "ASUS ROG MAXIMUS Z790 HERO", Colours: make([]colour.Colour, 4),
	}))

	out, err = run(t, socket, "reconcile")
	require.NoError(t, err)
	require.Contains(t, out, "restored 1 devices")

	showing, _ := server.Showing("ASUS ROG MAXIMUS Z790 HERO")
	require.Equal(t, colour.MustParse("teal"), showing.Colours[0])
}

func TestProbeReportsWhatTookAndWhatDidNot(t *testing.T) {
	server := openrgb.NewFake(board())
	server.Lies["ASUS ROG MAXIMUS Z790 HERO"] = "Static"
	socket := serving(t, nil, server)

	out, err := run(t, socket, "light", "probe")
	require.NoError(t, err)
	require.Contains(t, out, "Static")
	require.Contains(t, out, "accepted, and did not take")
	require.Contains(t, out, "suggested rule:")
	require.Contains(t, out, "solid_modes: [direct, static]")
	require.Contains(t, out, "zone Addressable 1", "where segment naming starts")
}

func TestReloadOnAMachineWithNoRulesFileIsFine(t *testing.T) {
	// Running with no configuration at all is the supported case, not an
	// error: every device present is driven and nothing is corrected.
	socket := serving(t, nil, openrgb.NewFake(board()))

	out, err := run(t, socket, "reload")
	require.NoError(t, err)
	require.Contains(t, out, "hotaru.yml", "it says which file it looked at")
	require.NotContains(t, out, "does not decode")
}

func TestAPreviewedColourIsWrittenButNotRemembered(t *testing.T) {
	/*
		Diagnosing lighting means setting a colour, looking at the case, and
		setting another. Without this flag each of those became the state the
		machine restores, and the reconciler put the previous one back while
		somebody was still looking -- which reads as the hardware misbehaving,
		and did, for most of an evening. See #18.
	*/
	server := openrgb.NewFake(board())
	socket := serving(t, nil, server)

	_, err := run(t, socket, "light", "set", "red", "--preview")
	require.NoError(t, err)

	showing, ok := server.Showing("ASUS ROG MAXIMUS Z790 HERO")
	require.True(t, ok)
	require.Equal(t, colour.MustParse("red"), showing.Colours[0],
		"a previewed colour still has to reach the hardware")

	// And nothing to put back: a preview is not what the machine should
	// return to.
	out, err := run(t, socket, "reconcile")
	require.NoError(t, err)
	require.Contains(t, out, "nothing to put back",
		"a previewed colour was remembered as the state to restore")
}

func TestAColourSetWithoutPreviewIsRemembered(t *testing.T) {
	// The other half: the flag has to be the difference, not the test setup.
	server := openrgb.NewFake(board())
	socket := serving(t, nil, server)

	_, err := run(t, socket, "light", "set", "red")
	require.NoError(t, err)

	out, err := run(t, socket, "reconcile")
	require.NoError(t, err)
	require.Contains(t, out, "restored 1")
}

func TestAdoptTakesAScenesOwnPictureIntoTheLibrary(t *testing.T) {
	/*
		A scene written before hotaru kept pictures names a file wherever it
		happened to be. It works until the file moves -- and the window
		cannot offer it, because the chooser lists what hotaru keeps rather
		than every GIF on the machine, so a scene made by hand could not be
		edited in the same place as one made in the window.
	*/
	socket := serving(t, &config.Config{}, openrgb.NewFake(board()))

	elsewhere := filepath.Join(t.TempDir(), "rain.gif")
	require.NoError(t, os.WriteFile(elsewhere, gif(t), 0o600))

	_, err := run(t, socket, "scene", "write", "storm", "--colour", "blue", "--screen", elsewhere)
	require.NoError(t, err)

	said, err := run(t, socket, "scene", "adopt")
	require.NoError(t, err)
	require.Contains(t, said, "storm")
	require.Contains(t, said, `"rain"`, "it did not say what the picture is called now")

	// The scene points at the library's copy, and the original is untouched.
	listed, err := run(t, socket, "scene", "list")
	require.NoError(t, err)
	require.NotContains(t, listed, elsewhere, "the scene still points outside the library")
	require.FileExists(t, elsewhere, "it moved somebody's file instead of copying it")

	kept, err := run(t, socket, "image", "list")
	require.NoError(t, err)
	require.Contains(t, kept, "rain")

	// And running it again adopts nothing: the scene is already inside.
	again, err := run(t, socket, "scene", "adopt")
	require.NoError(t, err)
	require.Contains(t, again, "already points at a picture hotaru keeps")
}

// gif is the smallest picture the library will take.
func gif(t *testing.T) []byte {
	t.Helper()
	var out bytes.Buffer
	require.NoError(t, png.Encode(&out, image.NewRGBA(image.Rect(0, 0, 8, 8))))
	return out.Bytes()
}
