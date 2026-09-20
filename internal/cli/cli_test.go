package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"go/build"
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
	"github.com/ushineko/hotaru/internal/openrgb"
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
	socket := filepath.Join(dir, "s")
	listener, err := api.Listen(socket)
	require.NoError(t, err)

	// A recorder, as the daemon gives it: without one the service drives
	// hardware and remembers nothing, so there is nothing to restore.
	desired, err := state.Open(filepath.Join(dir, "state.yml"))
	require.NoError(t, err)
	svc := service.New(cfg, server, "127.0.0.1:6742")
	svc.SetRecorder(desired)

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

func TestReloadReportsWhatIsWrongWithTheRulesFile(t *testing.T) {
	socket := serving(t, nil, openrgb.NewFake(board()))

	out, err := run(t, socket, "reload")
	require.NoError(t, err)
	require.Contains(t, out, "no rules file", "and that is a supported way to run")
}
