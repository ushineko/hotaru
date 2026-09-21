package gui_test

import (
	"context"
	"encoding/json"
	"go/build"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/require"
	"github.com/ushineko/fynedesygn/fynetest"
	"github.com/ushineko/fynedesygn/shell"
	"github.com/ushineko/hotaru/internal/api"
	"github.com/ushineko/hotaru/internal/gui"
)

/*
The guarantee, asserted rather than described.

The same rule the CLI carries: this program cannot reach a device, because it
imports nothing that can. A window is the place where "just write it directly,
it is only a preview" is most tempting, and a test over the import graph is
worth more than a sentence in a document saying not to.
*/
func TestTheWindowCannotReachADevice(t *testing.T) {
	pkg, err := build.ImportDir(".", 0)
	require.NoError(t, err)

	forbidden := []string{
		"internal/openrgb", // the hardware
		"internal/service", // the thing that drives it
		"internal/devices", // and the knowledge of how
		"internal/cooler",  // including the one over hidraw
		"internal/desktop", // and the keys
	}
	for _, imported := range pkg.Imports {
		for _, banned := range forbidden {
			require.NotContains(t, imported, banned,
				"the window imported %s; it asks the service, it does not do the work", imported)
		}
	}
}

// service is a fake hotaru over a real socket, answering what the window asks.
// Not a fake of the hardware: what is tested here is the window's reading of
// an answer, and the answers themselves are the API's own contract.
func service(t *testing.T, routes map[string]any) *api.Client {
	t.Helper()

	socket := filepath.Join(t.TempDir(), "s")
	listener, err := api.Listen(t.Context(), socket)
	require.NoError(t, err)

	mux := http.NewServeMux()
	for path, body := range routes {
		mux.HandleFunc(path, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(body))
		})
	}
	server := &http.Server{Handler: mux, ReadHeaderTimeout: api.ReadHeaderTimeout}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })

	return api.NewClient(socket)
}

func healthy() map[string]any {
	return map[string]any{
		"GET /" + api.Version + "/health": api.Health{
			State: "healthy", Version: "test", Address: "127.0.0.1:6742",
			Devices: 2, InScope: 1,
		},
		"GET /" + api.Version + "/devices": api.DevicesResponse{Devices: []api.Device{
			{
				Name: "NZXT Kraken", LEDs: 4, ActiveMode: "Direct", InScope: true,
				Zones:    []api.Zone{{Name: "Ring", First: 0, Count: 2}, {Name: "Fans", First: 2, Count: 2}},
				Colours:  []string{"#ff0000", "#ff0000", "#0000ff", "#0000ff"},
				Reassert: "1m0s",
			},
			{Name: "Corsair MM700", LEDs: 3, ActiveMode: "Direct", InScope: false},
		}},
		"GET /" + api.Version + "/cooling": api.Cooling{
			Device: "NZXT Kraken", Coolant: 37.5, PumpRPM: 2608, FanRPM: 1190,
		},
		"GET /" + api.Version + "/keys": api.KeysResponse{},
	}
}

// screen builds a section headless and returns everything it says.
func screen(t *testing.T, client *api.Client, section string) string {
	t.Helper()

	a := test.NewApp()
	t.Cleanup(a.Quit)

	app := gui.New(client)
	opts := app.Options("/run/nowhere/hotaru.sock")
	opts.SettingsPath = filepath.Join(t.TempDir(), "gui.yml")

	sh := shell.Headless(a, opts)
	app.Refresh(context.Background())

	for _, s := range sh.Sections() {
		if s.Title() == section {
			return fynetest.Text(s.Build(sh))
		}
	}
	t.Fatalf("no section called %q", section)
	return ""
}

func TestTheWindowHasItsThreeSections(t *testing.T) {
	a := test.NewApp()
	t.Cleanup(a.Quit)

	sh := shell.Headless(a, gui.New(service(t, healthy())).Options("s"))

	var titles []string
	for _, s := range sh.Sections() {
		titles = append(titles, s.Title())
	}
	require.Equal(t, []string{"Service", "System", "Cooling"}, titles)
}

func TestAServiceThatIsNotRunningSaysWhatToType(t *testing.T) {
	/*
		The ordinary first-run case, not a failure. A window that opens onto a
		red error because the thing it talks to has not been started yet has
		misread its own situation.
	*/
	absent := api.NewClient(filepath.Join(t.TempDir(), "nothing.sock"))

	for _, section := range []string{"Service", "System", "Cooling"} {
		said := screen(t, absent, section)
		require.Contains(t, said, "systemctl --user start hotaru",
			"%s did not say how to start the service", section)
	}
}

func TestTheServiceSectionShowsHealthAndItsRemedies(t *testing.T) {
	routes := healthy()
	routes["GET /"+api.Version+"/health"] = api.Health{
		State: "none-in-scope", Detail: "6 devices, none in scope",
		Remedies: []string{"Widen the scope in hotaru.yml"}, Devices: 6,
	}

	said := screen(t, service(t, routes), "Service")
	require.Contains(t, said, "none-in-scope")
	require.Contains(t, said, "6 devices, none in scope")
	require.Contains(t, said, "Widen the scope in hotaru.yml")
	require.Contains(t, said, "Put the lights back")
}

func TestTheSystemViewDrawsEveryDevice(t *testing.T) {
	said := screen(t, service(t, healthy()), "System")
	require.Contains(t, said, "This machine")

	require.Contains(t, said, "NZXT Kraken")
	require.Contains(t, said, "Ring")
	require.Contains(t, said, "Fans")
	require.Contains(t, said, "Direct")
}

func TestADeviceOutOfScopeIsDrawnRatherThanOmitted(t *testing.T) {
	// A device hotaru is not driving is a fact somebody is looking for, and
	// leaving it out answers the question with silence.
	said := screen(t, service(t, healthy()), "System")

	require.Contains(t, said, "Corsair MM700")
	require.Contains(t, said, "Out of scope")
}

func TestADeviceThatForgetsSaysSo(t *testing.T) {
	// The difference between "hotaru keeps changing this" and "this keeps
	// forgetting", which is otherwise invisible.
	require.Contains(t, screen(t, service(t, healthy()), "System"), "re-sent every 1m0s")
}

func TestADeviceUnderAPreviewIsMarked(t *testing.T) {
	routes := healthy()
	devices := routes["GET /"+api.Version+"/devices"].(api.DevicesResponse)
	devices.Devices[0].Preview = &api.Preview{Token: "abc", Scene: "evening", Holder: "hotaru-gui"}
	routes["GET /"+api.Version+"/devices"] = devices

	said := screen(t, service(t, routes), "System")
	require.Contains(t, said, "Showing a draft held by hotaru-gui")
}

func TestTheCoolingSectionShowsTheNumbers(t *testing.T) {
	said := screen(t, service(t, healthy()), "Cooling")

	require.Contains(t, said, "37.5 °C")
	require.Contains(t, said, "2608 rpm")
	require.Contains(t, said, "1190 rpm")
}

func TestAMachineWithNoCoolerIsNotAFailure(t *testing.T) {
	// Most machines are that machine, and showing it as an error would put a
	// red mark on every ordinary desktop.
	routes := healthy()
	routes["GET /"+api.Version+"/cooling"] = api.Cooling{
		Absent: true, Detail: "no supported liquid cooler",
	}

	said := screen(t, service(t, routes), "Cooling")
	require.Contains(t, said, "No cooler")
	require.Contains(t, said, "no supported liquid cooler")
	require.NotContains(t, strings.ToLower(said), "error")
}

func TestTheWindowKeepsItsViewStateSomewhereOfItsOwn(t *testing.T) {
	/*
		One writer per file. The service owns the rules, the scenes and the
		desired state; this program owns geometry, a scheme and a section --
		and the editor is exactly where somebody would be tempted to blur it.
	*/
	opts := gui.New(service(t, healthy())).Options("s")
	require.True(t, strings.HasSuffix(opts.SettingsPath, "gui.yml"),
		"the window's settings went somewhere unexpected: %s", opts.SettingsPath)
	require.NotContains(t, opts.SettingsPath, "hotaru.yml")
	require.NotContains(t, opts.SettingsPath, "scenes.yml")
}
