package gui_test

import (
	"context"
	"encoding/json"
	"go/build"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
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
		"GET /" + api.Version + "/scenes": api.ScenesResponse{Scenes: []api.Scene{
			{Name: "red", Colour: "red", Screen: "dashboard", Shipped: true},
			{Name: "evening", Assignments: []api.SceneAssignment{
				{Target: "Keychron", Colour: "#201040"}}},
		}},
	}
}

/*
watching is a fake service that says which routes were asked for.

For the paths whose whole point is that something reached the service: a drop
handler that quietly did nothing would pass any test that only looked at what
the window drew.
*/
func watching(t *testing.T, routes map[string]any) (*api.Client, chan string) {
	t.Helper()

	asked := make(chan string, 8)
	socket := filepath.Join(t.TempDir(), "s")
	listener, err := api.Listen(t.Context(), socket)
	require.NoError(t, err)

	mux := http.NewServeMux()
	for path, body := range routes {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			select {
			case asked <- r.Method + " " + r.URL.Path:
			default:
			}
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(body))
		})
	}
	server := &http.Server{Handler: mux, ReadHeaderTimeout: api.ReadHeaderTimeout}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })

	return api.NewClient(socket), asked
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

func TestTheWindowHasItsSections(t *testing.T) {
	a := test.NewApp()
	t.Cleanup(a.Quit)

	sh := shell.Headless(a, gui.New(service(t, healthy())).Options("s"))

	var titles []string
	for _, s := range sh.Sections() {
		titles = append(titles, s.Title())
	}
	require.Equal(t, []string{"Service", "System", "Scenes", "Pictures", "Cooling"}, titles)
}

func TestAServiceThatIsNotRunningSaysWhatToType(t *testing.T) {
	/*
		The ordinary first-run case, not a failure. A window that opens onto a
		red error because the thing it talks to has not been started yet has
		misread its own situation.
	*/
	absent := api.NewClient(filepath.Join(t.TempDir(), "nothing.sock"))

	for _, section := range []string{"Service", "System", "Scenes", "Pictures", "Cooling"} {
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

/*
affixed are the controls that start, commit or cancel work.

fynedesygn's rule, and the reason is sharper than "keep them visible": Fyne
hands a wheel event to the innermost scroller under the pointer and does not
pass it on, so a control below a long list is not inconvenient, it is
unreachable. Somebody opened this window with eighteen scenes in it and asked
where the buttons were.

The list lives here and is walked, rather than being a habit each section is
trusted to keep.
*/
var affixed = map[string][]string{
	"Scenes": {"New scene"},
}

func TestTheControlsThatDoSomethingDoNotScrollAway(t *testing.T) {
	client := service(t, healthy())

	a := test.NewApp()
	t.Cleanup(a.Quit)

	app := gui.New(client)
	opts := app.Options("s")
	opts.SettingsPath = filepath.Join(t.TempDir(), "gui.yml")
	sh := shell.Headless(a, opts)
	app.Refresh(context.Background())

	for _, section := range sh.Sections() {
		wanted, ok := affixed[section.Title()]
		if !ok {
			continue
		}
		built := section.Build(sh)

		/*
			Present first, then affixed.

			Asserting only "not in the scrolled set" passes for a control that
			is not drawn at all, which is how this test passed while the
			section it was checking was rendering an error page.
		*/
		drawn := fynetest.Text(built)
		scrolled := fynetest.ScrolledButtons(built)
		for _, control := range wanted {
			require.Contains(t, drawn, control,
				"%s does not draw %q at all", section.Title(), control)
			require.NotContains(t, scrolled, control,
				"%s's %q scrolls with the content it acts on", section.Title(), control)
		}
	}
}

func TestTheEditorsOwnControlsDoNotScrollAway(t *testing.T) {
	// The editor draws every device under its buttons, which is a scroll on
	// any ordinary window: Show it, Save and Close are the three things that
	// must not go looking for the user.
	a := test.NewApp()
	t.Cleanup(a.Quit)

	app := gui.New(service(t, healthy()))
	opts := app.Options("s")
	opts.SettingsPath = filepath.Join(t.TempDir(), "gui.yml")
	sh := shell.Headless(a, opts)
	app.Refresh(context.Background())

	editor := &gui.ScenesSection{}
	gui.OpenEditor(editor, app, gui.NewDraft())

	built := editor.Build(sh)
	drawn := fynetest.Text(built)
	scrolled := fynetest.ScrolledButtons(built)
	for _, control := range []string{"Show it on the hardware", "Save", "Close"} {
		require.Contains(t, drawn, control, "the editor does not draw %q at all", control)
		require.NotContains(t, scrolled, control, "%q scrolls away in the editor", control)
	}
}

func TestScenesAreOrderedByTheKeysTheySitOn(t *testing.T) {
	/*
		The bank has been on this numpad for two years, so that is the order
		somebody recognises. Alphabetically, "blue" comes above "cats" and the
		keyboard's own order appears nowhere -- a list to read rather than one
		to recognise.
	*/
	routes := healthy()
	routes["GET /"+api.Version+"/scenes"] = api.ScenesResponse{Scenes: []api.Scene{
		{Name: "aardvark"},
		{Name: "blue", Colour: "blue"},
		{Name: "corgis"},
		{Name: "red", Colour: "red"},
	}}
	routes["GET /"+api.Version+"/keys"] = api.KeysResponse{Bindings: []api.Binding{
		{Key: "Ctrl+Alt+Shift+Num+1", Scene: "corgis"},
		{Key: "Ctrl+Alt+Num+3", Scene: "blue"},
		{Key: "Ctrl+Alt+Num+1", Scene: "red"},
	}}

	said := screen(t, service(t, routes), "Scenes")

	// red (Num1), blue (Num3), corgis (Shift+Num1), then the unbound one.
	require.Less(t, strings.Index(said, "red"), strings.Index(said, "blue"))
	require.Less(t, strings.Index(said, "blue"), strings.Index(said, "corgis"))
	require.Less(t, strings.Index(said, "corgis"), strings.Index(said, "aardvark"))
}

func TestAScenesKeyIsShownBesideIt(t *testing.T) {
	routes := healthy()
	routes["GET /"+api.Version+"/scenes"] = api.ScenesResponse{Scenes: []api.Scene{{Name: "red"}}}
	routes["GET /"+api.Version+"/keys"] = api.KeysResponse{
		Bindings: []api.Binding{{Key: "Ctrl+Alt+Num+1", Scene: "red"}},
	}

	require.Contains(t, screen(t, service(t, routes), "Scenes"), "1")
}

func TestAnIdleMachineRedrawsNothing(t *testing.T) {
	/*
		The reason a list jumped under the pointer: the window rebuilt its
		section every two seconds whether or not anything had moved, and a
		rebuilt list is a new list.
	*/
	first := gui.Snapshot{
		Health:  api.Health{State: "healthy", Devices: 6, InScope: 6},
		Devices: []api.Device{{Name: "Keychron", Colours: []string{"#ff0000"}, InScope: true}},
		Cooling: api.Cooling{Coolant: 37.5, PumpRPM: 2608},
		At:      time.Now(),
	}
	second := first
	second.At = time.Now().Add(time.Minute) // a later reading of the same machine

	require.True(t, first.Same(second), "a poll that found nothing new counted as a change")
}

func TestAColourChangeIsAChange(t *testing.T) {
	first := gui.Snapshot{Devices: []api.Device{{Name: "Keychron", Colours: []string{"#ff0000"}}}}
	second := gui.Snapshot{Devices: []api.Device{{Name: "Keychron", Colours: []string{"#00ff00"}}}}

	require.False(t, first.Same(second))
}

func TestTheEditorIsLeftAloneWhileItIsOpen(t *testing.T) {
	// A draft, a selection and an open picker are the user's work in
	// progress. None of them survive a rebuild, and none of them should be
	// interrupted by the machine's own state moving.
	section := &gui.ScenesSection{}
	require.False(t, section.Busy())

	gui.OpenEditor(section, gui.New(service(t, healthy())), gui.NewDraft())
	require.True(t, section.Busy())
}

func TestTheCoolerMovingDoesNotRebuildTheSceneList(t *testing.T) {
	/*
		Why the list jumped. A pump reading and a fan reading never sit still,
		so "has anything changed?" was yes on every poll, and a list of
		eighteen scenes was rebuilt under the pointer every two seconds.

		The question belongs to the section: Scenes draws scenes and keys.
	*/
	before := gui.Snapshot{
		Scenes:  []api.Scene{{Name: "red"}},
		Cooling: api.Cooling{Coolant: 37.5, PumpRPM: 2608, FanRPM: 1190},
	}
	after := before
	after.Cooling = api.Cooling{Coolant: 37.6, PumpRPM: 2611, FanRPM: 1194}

	scenes := &gui.ScenesSection{}
	require.False(t, scenes.Changed(before, after),
		"the scene list rebuilt because the pump changed speed")

	cooling := &gui.CoolingSection{}
	require.True(t, cooling.Changed(before, after),
		"the cooling section did not notice its own numbers moving")
}

func TestASceneAppearingDoesRebuildTheList(t *testing.T) {
	before := gui.Snapshot{Scenes: []api.Scene{{Name: "red"}}}
	after := gui.Snapshot{Scenes: []api.Scene{{Name: "red"}, {Name: "evening"}}}

	require.True(t, (&gui.ScenesSection{}).Changed(before, after))
}

func TestABindingChangingRebuildsTheList(t *testing.T) {
	// The order depends on the keys, so a rebind reorders the list.
	before := gui.Snapshot{Keys: api.KeysResponse{
		Bindings: []api.Binding{{Key: "Ctrl+Alt+Num+1", Scene: "red"}}}}
	after := gui.Snapshot{Keys: api.KeysResponse{
		Bindings: []api.Binding{{Key: "Ctrl+Alt+Num+1", Scene: "blue"}}}}

	require.True(t, (&gui.ScenesSection{}).Changed(before, after))
}

func TestALightChangingDoesNotRebuildTheCoolingSection(t *testing.T) {
	before := gui.Snapshot{Devices: []api.Device{{Name: "Keychron", Colours: []string{"#ff0000"}}}}
	after := gui.Snapshot{Devices: []api.Device{{Name: "Keychron", Colours: []string{"#00ff00"}}}}

	require.False(t, (&gui.CoolingSection{}).Changed(before, after))
	require.True(t, (&gui.SystemSection{}).Changed(before, after))
}

func TestASceneLineIsEditedWhereItIsShown(t *testing.T) {
	/*
		"This scene" listed the draft's entries exactly where somebody would
		reach to change one, and was a summary: changing an entry meant
		scrolling down to find the device it named and starting again. What
		looks like the thing you click has to be the thing you click.
	*/
	a := test.NewApp()
	t.Cleanup(a.Quit)

	app := gui.New(service(t, healthy()))
	opts := app.Options("s")
	opts.SettingsPath = filepath.Join(t.TempDir(), "gui.yml")
	sh := shell.Headless(a, opts)
	app.Refresh(context.Background())

	draft := gui.NewDraft()
	draft.Set("NZXT Kraken", "blue")
	draft.SetEverything("red")

	section := &gui.ScenesSection{}
	gui.OpenEditor(section, app, draft)
	built := section.Build(sh)

	said := fynetest.Text(built)
	require.Contains(t, said, "NZXT Kraken")
	require.Contains(t, said, "Everything in scope")
	require.Contains(t, said, "Change", "the scene's own lines offer no way to change them")
}

func TestASingleLightIsAddressable(t *testing.T) {
	/*
		Showing somebody twenty-four blocks and letting them address only the
		zone is an interface making a promise it does not keep. One light is
		`kraken/ring[12:12]` -- exactly the typing this window exists to
		replace.
	*/
	a := test.NewApp()
	t.Cleanup(a.Quit)

	app := gui.New(service(t, healthy()))
	opts := app.Options("s")
	opts.SettingsPath = filepath.Join(t.TempDir(), "gui.yml")
	sh := shell.Headless(a, opts)
	app.Refresh(context.Background())

	section := &gui.ScenesSection{}
	gui.OpenEditor(section, app, gui.NewDraft())
	built := section.Build(sh)

	// The Kraken in the fixture has two zones of two lights each, so every
	// block is one light and every one of them carries its own address.
	tips := fynetest.Tips(built)
	require.Contains(t, tips, "Ring, light 0")
	require.Contains(t, tips, "Ring, light 1")
	require.Contains(t, tips, "Fans, light 0")
}

func TestABigZoneIsDrawnAsRunsRatherThanASmear(t *testing.T) {
	// A hundred keys at one block each is something nobody can aim at, and a
	// run is still an address hotaru understands.
	routes := healthy()
	devices := routes["GET /"+api.Version+"/devices"].(api.DevicesResponse)
	devices.Devices = append(devices.Devices, api.Device{
		Name: "Keychron K4 HE", LEDs: 100, InScope: true,
		Zones: []api.Zone{{Name: "Keyboard", First: 0, Count: 100}},
	})
	routes["GET /"+api.Version+"/devices"] = devices

	a := test.NewApp()
	t.Cleanup(a.Quit)

	app := gui.New(service(t, routes))
	opts := app.Options("s")
	opts.SettingsPath = filepath.Join(t.TempDir(), "gui.yml")
	sh := shell.Headless(a, opts)
	app.Refresh(context.Background())

	section := &gui.ScenesSection{}
	gui.OpenEditor(section, app, gui.NewDraft())

	tips := fynetest.Tips(section.Build(sh))
	require.Contains(t, tips, "Keyboard, lights 0 to 4")
	require.NotContains(t, tips, "Keyboard, light 7",
		"a hundred lights were drawn one block each")
}

func TestTheEditorKeepsItsPlaceWhenSomethingIsClicked(t *testing.T) {
	/*
		Clicking a light rebuilds the section, which is how the selection
		shows. A scroller built fresh on every rebuild starts at the top, so
		every click threw the picture back to the beginning -- twenty-four
		blocks down, a control that cannot be used twice.
	*/
	a := test.NewApp()
	t.Cleanup(a.Quit)

	app := gui.New(service(t, healthy()))
	opts := app.Options("s")
	opts.SettingsPath = filepath.Join(t.TempDir(), "gui.yml")
	sh := shell.Headless(a, opts)
	app.Refresh(context.Background())

	section := &gui.ScenesSection{}
	gui.OpenEditor(section, app, gui.NewDraft())

	built := section.Build(sh)
	built.Resize(fyne.NewSize(900, 300))

	first := picture(built)
	require.NotNil(t, first)
	first.ScrollToOffset(fyne.NewPos(0, 120))
	require.Positive(t, first.Offset.Y, "the picture does not scroll at all")
	was := first.Offset.Y

	// What a click does: the section is rebuilt so the selection can show.
	again := section.Build(sh)
	again.Resize(fyne.NewSize(900, 300))

	scroller := picture(again)
	require.Same(t, first, scroller, "the editor built a new scroller and lost its place")
	require.Equal(t, was, scroller.Offset.Y)
}

func TestTheListAndTheEditorDoNotShareAPlace(t *testing.T) {
	// Different material: an offset from a list of eighteen scenes means
	// nothing in a picture of six devices, and carrying it over reads as the
	// window losing its place rather than keeping it.
	a := test.NewApp()
	t.Cleanup(a.Quit)

	app := gui.New(service(t, healthy()))
	opts := app.Options("s")
	opts.SettingsPath = filepath.Join(t.TempDir(), "gui.yml")
	sh := shell.Headless(a, opts)
	app.Refresh(context.Background())

	section := &gui.ScenesSection{}
	gui.OpenList(section, app)
	list := fynetest.Find[*container.Scroll](section.Build(sh))
	list.Offset = fyne.NewPos(0, 300)

	gui.OpenEditor(section, app, gui.NewDraft())
	editor := fynetest.Find[*container.Scroll](section.Build(sh))
	require.Zero(t, editor.Offset.Y, "the editor opened where the list had been scrolled to")
}

func TestARebuildIsNotAnArrival(t *testing.T) {
	/*
		The trap that made the preview flash and the picture jump, in two
		different methods. Detach runs before *every* rebuild, so anything
		reset there is reset on every click; Arrive runs on navigation only.
	*/
	a := test.NewApp()
	t.Cleanup(a.Quit)

	app := gui.New(service(t, healthy()))
	opts := app.Options("s")
	opts.SettingsPath = filepath.Join(t.TempDir(), "gui.yml")
	sh := shell.Headless(a, opts)
	app.Refresh(context.Background())

	section := &gui.ScenesSection{}
	gui.OpenEditor(section, app, gui.NewDraft())

	built := section.Build(sh)
	built.Resize(fyne.NewSize(900, 300))
	first := picture(built)
	first.ScrollToOffset(fyne.NewPos(0, 120))
	was := first.Offset.Y
	require.Positive(t, was)

	// A click: the shell detaches, then builds again.
	section.Detach()
	again := section.Build(sh)
	again.Resize(fyne.NewSize(900, 300))
	require.Equal(t, was, picture(again).Offset.Y,
		"a rebuild threw the picture back to the top")

	// Navigating away and back: a fresh start is what somebody expects.
	section.Arrive()
	arrived := section.Build(sh)
	arrived.Resize(fyne.NewSize(900, 300))
	require.Zero(t, picture(arrived).Offset.Y)
}

func TestSeveralLightsAreColouredInOneGo(t *testing.T) {
	/*
		"These four lights" is one intention, and four trips through a colour
		picker is not. What the scene ends up saying is the merged form --
		one run, which is what somebody would have typed.
	*/
	a := test.NewApp()
	t.Cleanup(a.Quit)

	app := gui.New(service(t, healthy()))
	opts := app.Options("s")
	opts.SettingsPath = filepath.Join(t.TempDir(), "gui.yml")
	sh := shell.Headless(a, opts)
	app.Refresh(context.Background())

	draft := gui.NewDraft()
	section := &gui.ScenesSection{}
	gui.OpenEditor(section, app, draft)
	gui.Select(section,
		gui.Lights("NZXT Kraken", "Ring", 0, 0),
		gui.Lights("NZXT Kraken", "Ring", 1, 1))

	gui.SetColour(section, "#00ff88")

	scene := draft.Scene("x")
	require.Len(t, scene.Assignments, 1, "two adjacent lights became two lines")
	require.Equal(t, "NZXT Kraken/Ring[0:1]", scene.Assignments[0].Target)
	require.Equal(t, "#00ff88", scene.Assignments[0].Colour)

	// And the card says what is about to be coloured.
	require.Contains(t, fynetest.Text(section.Build(sh)), "2 selected")
}

func TestASceneLineSelectsWhatItNames(t *testing.T) {
	// Editing one line is a selection of one, whatever was picked in the
	// picture: the colour has to land where it was asked to.
	draft := gui.NewDraft()
	draft.Set("NZXT Kraken/Ring[4:6]", "red")

	section := &gui.ScenesSection{}
	gui.OpenEditor(section, gui.New(service(t, healthy())), draft)
	gui.Select(section, gui.Lights("NZXT Kraken", "Ring", 0, 0))

	gui.EditLine(section, "NZXT Kraken/Ring[4:6]")
	gui.SetColour(section, "blue")

	// One line, recoloured. The light selected in the picture was set aside
	// by opening the line, so nothing landed on it.
	require.Len(t, draft.Scene("x").Assignments, 1)
	colour, _ := draft.Colour("NZXT Kraken/Ring[4:6]")
	require.Equal(t, "blue", colour)
	stray, _ := draft.Colour("NZXT Kraken/Ring[0]")
	require.Empty(t, stray, "the colour landed on what was selected in the picture")
}

func TestSelectingALightDoesNotMoveThePicture(t *testing.T) {
	/*
		Clicking a light is how the picker appears, and the picker appearing
		pushed the picture down under the pointer that had just clicked it.
		Nothing transient may reflow the interface -- least of all the
		interface it appears in front of.

		The picture's own scroller is the measure: its position in the section
		does not depend on what is selected.
	*/
	a := test.NewApp()
	t.Cleanup(a.Quit)

	app := gui.New(service(t, healthy()))
	opts := app.Options("s")
	opts.SettingsPath = filepath.Join(t.TempDir(), "gui.yml")
	sh := shell.Headless(a, opts)
	app.Refresh(context.Background())

	section := &gui.ScenesSection{}
	gui.OpenEditor(section, app, gui.NewDraft())

	nothing := section.Build(sh)
	nothing.Resize(fyne.NewSize(900, 500))
	before := picture(nothing)

	gui.Select(section, gui.Lights("NZXT Kraken", "Ring", 0, 0))
	selected := section.Build(sh)
	selected.Resize(fyne.NewSize(900, 500))
	after := picture(selected)

	require.Equal(t, before.Position(), after.Position(), "selecting a light moved the picture")
	require.Equal(t, before.Size(), after.Size(), "selecting a light resized the picture")
}

// picture is the editor's right-hand scroller: the machine. The left pane has
// one too, so the last is the one that matters.
func picture(o fyne.CanvasObject) *container.Scroll {
	var found *container.Scroll
	// visit returns true to *stop*, so this keeps going and remembers the
	// last one it saw.
	fynetest.Walk(o, func(child fyne.CanvasObject) bool {
		if scroll, ok := child.(*container.Scroll); ok {
			found = scroll
		}
		return false
	})
	return found
}

func TestAStoredPictureCanBeAScenesScreen(t *testing.T) {
	/*
		A scene carries a screen state already -- the dashboard, the firmware's
		readout, or a picture -- and the editor had no way to set it, so
		editing a scene that showed a GIF and saving it kept the GIF by luck.
	*/
	routes := healthy()
	routes["GET /"+api.Version+"/images"] = api.ImagesResponse{Images: []api.Image{
		{Name: "wallpaper", Path: "/home/somebody/.local/share/hotaru/images/wallpaper.gif"},
	}}

	a := test.NewApp()
	t.Cleanup(a.Quit)

	app := gui.New(service(t, routes))
	opts := app.Options("s")
	opts.SettingsPath = filepath.Join(t.TempDir(), "gui.yml")
	sh := shell.Headless(a, opts)
	app.Refresh(context.Background())

	section := &gui.ScenesSection{}
	gui.OpenEditor(section, app, gui.NewDraft())

	said := fynetest.Text(section.Build(sh))
	require.Contains(t, said, "The screen")
	require.Contains(t, said, "leave it alone",
		"the option that changes nothing was not offered")
}

func TestASceneKeepsAScreenTheEditorDidNotTouch(t *testing.T) {
	// The round-trip risk, in the one field that names a file somebody chose.
	draft := gui.DraftFrom(api.Scene{Name: "evening", Screen: "/pictures/rain.gif"})
	require.Equal(t, "/pictures/rain.gif", draft.Screen())

	draft.Set("kraken", "red")
	require.Equal(t, "/pictures/rain.gif", draft.Scene("evening").Screen)

	draft.SetScreen(api.ScreenDashboard)
	require.Equal(t, api.ScreenDashboard, draft.Scene("evening").Screen)
}

func TestThePicturesSectionListsWhatIsStored(t *testing.T) {
	routes := healthy()
	routes["GET /"+api.Version+"/images"] = api.ImagesResponse{Images: []api.Image{
		{Name: "wallpaper", Path: "/tmp/wallpaper.gif", Bytes: 198 * 1024},
		{Name: "corgis", Path: "/tmp/corgis.gif", Bytes: 19 * 1024 * 1024, Frames: 120},
	}}

	said := screen(t, service(t, routes), "Pictures")
	require.Contains(t, said, "wallpaper")
	require.Contains(t, said, "198 KB")
	require.Contains(t, said, "120 frames", "an animation was not marked as one")
}

func TestADialogIsShownBeforeItIsResized(t *testing.T) {
	/*
		A FileDialog builds its window in Show and its Resize dereferences
		that window, so sizing one before showing it is a nil pointer -- a
		crash rather than a small dialog. It went in front of somebody before
		this test existed.
	*/
	a := test.NewApp()
	t.Cleanup(a.Quit)

	window := test.NewWindow(widget.NewLabel("behind"))
	t.Cleanup(window.Close)
	window.Resize(fyne.NewSize(1200, 800))

	app := gui.New(service(t, healthy()))
	opts := app.Options("s")
	opts.SettingsPath = filepath.Join(t.TempDir(), "gui.yml")
	sh := shell.Headless(a, opts)
	sh.Window = window

	require.NotPanics(t, func() {
		gui.Roomy(dialog.NewFileOpen(func(fyne.URIReadCloser, error) {}, window), sh)
	})
	require.NotPanics(t, func() {
		gui.Roomy(dialog.NewCustom("t", "Done", widget.NewLabel("x"), window), sh)
	})
}

func TestADroppedFileIsConverted(t *testing.T) {
	/*
		A person dragging a wallpaper at a program is not aiming at a
		rectangle, so the whole window takes a drop and the navigation follows
		it to the Pictures section.
	*/
	a := test.NewApp()
	t.Cleanup(a.Quit)

	routes := healthy()
	routes["POST /"+api.Version+"/images/preview"] = api.ConvertedImage{Frames: 1}

	client, asked := watching(t, routes)
	app := gui.New(client)
	opts := app.Options("s")
	opts.SettingsPath = filepath.Join(t.TempDir(), "gui.yml")
	sh := shell.Headless(a, opts)

	// A window for the preview dialog to open in. Headless has none, and the
	// end of this path is a dialog.
	window := test.NewWindow(widget.NewLabel("behind"))
	t.Cleanup(window.Close)
	sh.Window = window
	app.Refresh(context.Background())

	picture := filepath.Join(t.TempDir(), "wallpaper.jpg")
	require.NoError(t, os.WriteFile(picture, []byte("not really a jpeg"), 0o600))

	section := &gui.PicturesSection{}
	gui.OpenPictures(section, app)
	section.Dropped(sh, []fyne.URI{storage.NewFileURI(picture)})

	// The window polls, so its own reads are in this channel too; what
	// matters is that the conversion is among them.
	want := "POST /" + api.Version + "/images/preview"
	require.Eventually(t, func() bool {
		select {
		case route := <-asked:
			return route == want
		default:
			return false
		}
	}, 3*time.Second, 10*time.Millisecond, "a dropped file never reached the converter")
}

func TestADroppedFileThatCannotBeReadSaysSo(t *testing.T) {
	// A directory of holiday photographs contains one thing that is not a
	// photograph, and the name is how anybody finds out which.
	a := test.NewApp()
	t.Cleanup(a.Quit)

	app := gui.New(service(t, healthy()))
	opts := app.Options("s")
	opts.SettingsPath = filepath.Join(t.TempDir(), "gui.yml")
	sh := shell.Headless(a, opts)

	section := &gui.PicturesSection{}
	gui.OpenPictures(section, app)

	require.NotPanics(t, func() {
		section.Dropped(sh, []fyne.URI{storage.NewFileURI("/nothing/here.jpg")})
	})
}

func TestEveryFileInADroppedStackArrives(t *testing.T) {
	/*
		Dropping four photographs processed the first and discarded the other
		three without a word. The ones that say no by doing nothing are the
		expensive ones, so the whole stack queues and the program asks what it
		is before touching any of it.
	*/
	a := test.NewApp()
	t.Cleanup(a.Quit)

	client, asked := watching(t, healthy())
	app := gui.New(client)
	opts := app.Options("s")
	opts.SettingsPath = filepath.Join(t.TempDir(), "gui.yml")
	sh := shell.Headless(a, opts)

	window := test.NewWindow(widget.NewLabel("behind"))
	t.Cleanup(window.Close)
	sh.Window = window

	dir := t.TempDir()
	var uris []fyne.URI
	for _, name := range []string{"one.jpg", "two.jpg", "three.jpg", "four.jpg"} {
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, []byte("not really a jpeg"), 0o600))
		uris = append(uris, storage.NewFileURI(path))
	}

	section := &gui.PicturesSection{}
	gui.OpenPictures(section, app)
	section.Dropped(sh, uris)

	require.Equal(t, len(uris), section.Waiting(), "part of the stack was dropped on the floor")

	// And nothing was converted yet: a stack is a question, and the question
	// comes before the work.
	converted := "POST /" + api.Version + "/images/preview"
	select {
	case route := <-asked:
		require.NotEqual(t, converted, route, "a stack was converted before it was asked about")
	default:
	}
}

func TestOneDroppedFileIsNotAQuestion(t *testing.T) {
	// A single picture is unambiguous, and asking about it would be the
	// program making somebody confirm what they already said.
	a := test.NewApp()
	t.Cleanup(a.Quit)

	routes := healthy()
	routes["POST /"+api.Version+"/images/preview"] = api.ConvertedImage{Frames: 1}
	client, asked := watching(t, routes)
	app := gui.New(client)
	opts := app.Options("s")
	opts.SettingsPath = filepath.Join(t.TempDir(), "gui.yml")
	sh := shell.Headless(a, opts)

	window := test.NewWindow(widget.NewLabel("behind"))
	t.Cleanup(window.Close)
	sh.Window = window

	picture := filepath.Join(t.TempDir(), "wallpaper.jpg")
	require.NoError(t, os.WriteFile(picture, []byte("not really a jpeg"), 0o600))

	section := &gui.PicturesSection{}
	gui.OpenPictures(section, app)
	section.Dropped(sh, []fyne.URI{storage.NewFileURI(picture)})

	require.Zero(t, section.Waiting(), "a single picture was queued behind a question")

	want := "POST /" + api.Version + "/images/preview"
	require.Eventually(t, func() bool {
		select {
		case route := <-asked:
			return route == want
		default:
			return false
		}
	}, 3*time.Second, 10*time.Millisecond, "the one picture went nowhere")
}

// built returns a section's widget tree, for the tests that need more than
// the text it draws.
func built(t *testing.T, client *api.Client, section string) fyne.CanvasObject {
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
			return s.Build(sh)
		}
	}
	t.Fatalf("no section called %q", section)
	return nil
}

// deletes counts the delete buttons in a tree. An icon-only button says
// nothing that reading the text would find, and the rows sit inside a
// scroller, so this walks widgets as well as containers.
func deletes(t *testing.T, object fyne.CanvasObject) int {
	t.Helper()

	switch it := object.(type) {
	case *widget.Button:
		if it.Icon != nil && it.Icon.Name() == theme.DeleteIcon().Name() {
			return 1
		}
		return 0
	case *fyne.Container:
		var found int
		for _, child := range it.Objects {
			found += deletes(t, child)
		}
		return found
	case fyne.Widget:
		var found int
		for _, child := range test.TempWidgetRenderer(t, it).Objects() {
			found += deletes(t, child)
		}
		return found
	}
	return 0
}

func TestASceneCanBeDeletedFromTheWindow(t *testing.T) {
	/*
		The window had no way to remove a scene, and the parity test only runs
		one way: it proves the CLI reaches every route the service serves, not
		that the window does.

		Not on a shipped row, because the store takes that request and the
		scene is still there afterwards -- a button that does nothing is the
		silent no-op this project keeps paying for. Saving over a shipped
		scene is how it changes, and deleting that replacement is how hotaru's
		comes back.
	*/
	tree := built(t, service(t, healthy()), "Scenes")
	require.Equal(t, 1, deletes(t, tree),
		"healthy() has one shipped scene and one saved one, so one delete button")
}
