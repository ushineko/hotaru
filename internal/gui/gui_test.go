package gui_test

import (
	"context"
	"encoding/json"
	"fmt"
	"go/build"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
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

	/*
		Inside the Create group as well as beside it.

		Pictures, Screen and Scenes are three parts of one entry now, and a
		test that had to press a tab to reach one would be a test about
		tabs.
	*/
	for _, s := range sh.Sections() {
		if s.Title() == section {
			return fynetest.Text(s.Build(sh))
		}
		if group, ok := s.(*gui.Create); ok && group.Show(section) {
			return fynetest.Text(group.Build(sh))
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
	require.Equal(t, []string{"Service", "System", "Create", "Appearance", "About"}, titles,
		"the navigation changed shape")
}

func TestAServiceThatIsNotRunningSaysWhatToType(t *testing.T) {
	/*
		The ordinary first-run case, not a failure. A window that opens onto a
		red error because the thing it talks to has not been started yet has
		misread its own situation.
	*/
	absent := api.NewClient(filepath.Join(t.TempDir(), "nothing.sock"))

	for _, section := range []string{"Service", "System", "Scenes", "Pictures", "Screen"} {
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

func TestTheCoolingNumbersAreOnTheSystemPage(t *testing.T) {
	// Cooling was a section of its own and was too small to be one: four
	// readings about a device the same page already lists.
	said := screen(t, service(t, healthy()), "System")

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

	said := screen(t, service(t, routes), "System")
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

	/*
		Nor does the System section, which draws the coolant.

		It is the section the cooling card moved into, and a Changed that
		watched the coolant would rebuild every device row twice a minute.
		The card is refilled by Tick instead.
	*/
	require.False(t, (&gui.SystemSection{}).Changed(before, after),
		"the device list rebuilt because the pump changed speed")
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

func TestALightChangingRebuildsTheDeviceList(t *testing.T) {
	before := gui.Snapshot{Devices: []api.Device{{Name: "Keychron", Colours: []string{"#ff0000"}}}}
	after := gui.Snapshot{Devices: []api.Device{{Name: "Keychron", Colours: []string{"#00ff00"}}}}

	require.True(t, (&gui.SystemSection{}).Changed(before, after))
}

func TestTheCoolingCardFollowsTheMachineWithoutARebuild(t *testing.T) {
	/*
		The other half of a narrow Changed. Cooling was a section of its own
		and was too small to be one -- four readings about a device the same
		page already lists -- so it is a card in System now, and the numbers
		arrive through Tick rather than by rebuilding sixty device rows.
	*/
	a := test.NewApp()
	t.Cleanup(a.Quit)

	app := gui.New(service(t, healthy()))
	opts := app.Options("s")
	opts.SettingsPath = filepath.Join(t.TempDir(), "gui.yml")
	sh := shell.Headless(a, opts)
	app.Refresh(context.Background())

	section := &gui.SystemSection{}
	gui.OpenSystem(section, app)
	built := section.Build(sh)

	section.Tick(gui.Snapshot{
		Devices: []api.Device{{Name: "Keychron"}},
		Cooling: api.Cooling{Device: "NZXT Kraken", Coolant: 41.2, PumpRPM: 2608},
		Status:  api.Status{Scene: "evening", Showing: "dashboard: quiet"},
	})

	said := fynetest.Text(built)
	require.Contains(t, said, "41.2", "the coolant did not reach the card")
	require.Contains(t, said, "evening", "the applied scene is not shown")
	require.Contains(t, said, "dashboard: quiet", "what the screen shows is not shown")
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

	/*
		Given a size, and drawn.

		The scene's lines are a widget.List now -- it builds the rows that are
		on screen and recycles them, which is what stopped a 225-line scene
		costing 1,575 measured objects on every layout (spec 026). A list
		with no size has no rows, so a test that reads what it says has to
		give it one.
	*/
	window := test.NewWindow(built)
	t.Cleanup(window.Close)
	window.Resize(fyne.NewSize(900, 700))

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
		if group, ok := s.(*gui.Create); ok && group.Show(section) {
			return group.Build(sh)
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

func TestTheAboutSectionIsTheReadme(t *testing.T) {
	/*
		Embedded rather than restated. A program that describes itself twice
		has one description somebody maintains and another they forget, and
		the one in the window is the one that goes stale.
	*/
	tree := built(t, service(t, healthy()), "About")
	text := fynetest.Text(tree)

	require.Contains(t, text, "github.com/ushineko/hotaru", "the project link is missing")
	require.Contains(t, text, "hotaru")

	/*
		And one scrollbar for the section.

		The document pane is not scrollable itself: it follows the shell's
		content scroller, because a document that scrolls inside a page that
		also scrolls takes the wheel and stops the page.
	*/
	require.False(t, fynetest.ScrollableIn(tree),
		"the document brought a scroller of its own")
}

func TestShowingADashboardAsksTheService(t *testing.T) {
	/*
		Reported: "Show it" does nothing. The route works from the terminal,
		so the question is whether the window asks at all.
	*/
	a := test.NewApp()
	t.Cleanup(a.Quit)

	routes := healthy()
	routes["GET /"+api.Version+"/dashboards"] = api.DashboardsResponse{
		Active: "coolant",
		Dashboards: []api.Dashboard{
			{Name: "coolant", Arrangement: "ring", Shipped: true},
			{Name: "mine", Arrangement: "big"},
		},
		Arrangements: []api.Arrangement{{Name: "ring", Slots: 3, Rings: 2}},
		Themes:       []string{"midnight"},
	}
	routes["POST /"+api.Version+"/dashboards/mine/use"] = api.Dashboard{Name: "mine"}

	client, asked := watching(t, routes)
	app := gui.New(client)
	opts := app.Options("s")
	opts.SettingsPath = filepath.Join(t.TempDir(), "gui.yml")
	sh := shell.Headless(a, opts)

	window := test.NewWindow(widget.NewLabel("behind"))
	t.Cleanup(window.Close)
	sh.Window = window
	app.Refresh(context.Background())

	section := &gui.DashboardsSection{}
	gui.OpenDashboards(section, app)
	built := section.Build(sh)

	/*
		The second one. The first belongs to the dashboard already showing,
		which is disabled on purpose -- and a test that tapped it would prove
		nothing while looking as though it had.
	*/
	var shows []*widget.Button
	fynetest.Walk(built, func(o fyne.CanvasObject) bool {
		if b, ok := o.(*widget.Button); ok && b.Text == "Show it" {
			shows = append(shows, b)
		}
		return false
	})
	require.Len(t, shows, 2, "one Show it per dashboard")
	require.True(t, shows[0].Disabled(), "the dashboard already showing offers to show itself")
	require.False(t, shows[1].Disabled())
	test.Tap(shows[1])

	want := "POST /" + api.Version + "/dashboards/mine/use"
	require.Eventually(t, func() bool {
		select {
		case route := <-asked:
			return route == want
		default:
			return false
		}
	}, 3*time.Second, 10*time.Millisecond, "tapping Show it never reached the service")
}

func TestABindingIsChangedFromTheRowThatShowsIt(t *testing.T) {
	/*
		The key was already the first thing on a scene's line and the one
		thing on that line the window could not change. Moving it is two
		calls: the service binds a key to a scene and does not know a scene
		should have at most one, so the old key is unbound and the new one
		bound.
	*/
	a := test.NewApp()
	t.Cleanup(a.Quit)

	routes := healthy()
	routes["GET /"+api.Version+"/keys"] = api.KeysResponse{
		Bindings: []api.Binding{
			{Key: "Ctrl+Alt+Num+1", Scene: "red"},
			{Key: "Ctrl+Alt+Num+2", Scene: "evening"},
		},
		Reserved: []string{"Ctrl+Alt+Shift+Num+1"},
	}
	routes["POST /"+api.Version+"/keys/bind"] = struct{}{}

	client, asked := watching(t, routes)
	app := gui.New(client)
	opts := app.Options("s")
	opts.SettingsPath = filepath.Join(t.TempDir(), "gui.yml")
	sh := shell.Headless(a, opts)

	window := test.NewWindow(widget.NewLabel("behind"))
	t.Cleanup(window.Close)
	sh.Window = window
	app.Refresh(context.Background())

	/*
		The chooser offers every key, free or not, unshifted first.

		A key already pointing at another scene is a legitimate thing to
		choose -- rearranging a bank means moving keys between scenes -- so a
		chooser that hid the taken ones would hide the rearrangement.
	*/
	bank := gui.KeyBank(api.KeysResponse{
		Bindings: []api.Binding{
			{Key: "Ctrl+Alt+Num+2", Scene: "evening"},
			{Key: "Ctrl+Alt+Shift+Num+1", Scene: "mine"},
			{Key: "Ctrl+Alt+Num+1", Scene: "red"},
		},
		Reserved: []string{"Ctrl+Alt+Shift+Num+2", "Ctrl+Alt+Shift+Num+1"},
	})
	require.Equal(t, []string{
		"Ctrl+Alt+Num+1", "Ctrl+Alt+Num+2",
		"Ctrl+Alt+Shift+Num+1", "Ctrl+Alt+Shift+Num+2",
	}, bank, "a key was offered twice or in the wrong order")

	// And moving one is an unbind and a bind, in that order.
	section := &gui.ScenesSection{}
	gui.OpenScenes(section, app)
	section.Rebind(sh, "evening", "Ctrl+Alt+Shift+Num+1", "Ctrl+Alt+Num+2")

	want := "POST /" + api.Version + "/keys/bind"
	seen := 0
	require.Eventually(t, func() bool {
		select {
		case route := <-asked:
			if route == want {
				seen++
			}
		default:
		}
		return seen == 2
	}, 3*time.Second, 10*time.Millisecond, "moving a shortcut made %d calls, not two", seen)
}

func TestNothingThatIgnoresTheMachineIsRebuiltByIt(t *testing.T) {
	/*
		The poll rebuilds whatever is on screen when something it watches
		moves, and a section that says nothing about what it watches is
		rebuilt whenever anything does -- which on a machine with a running
		pump is every two seconds. A rebuild takes the page back to the top
		under whoever is reading it.

		Reported twice: once for the README, once for the appearance. Both
		are sections about the program rather than about the machine, and
		this is the list of them.
	*/
	a := test.NewApp()
	t.Cleanup(a.Quit)

	app := gui.New(service(t, healthy()))
	opts := app.Options("/run/nowhere/hotaru.sock")
	opts.SettingsPath = filepath.Join(t.TempDir(), "gui.yml")
	sh := shell.Headless(a, opts)

	moved := gui.Snapshot{Cooling: api.Cooling{PumpRPM: 2400}}
	for _, name := range []string{"Appearance", "About"} {
		var found gui.Watcher
		for _, section := range sh.Sections() {
			if section.Title() != name {
				continue
			}
			watcher, ok := section.(gui.Watcher)
			require.True(t, ok, "%s does not say what it watches", name)
			found = watcher
		}
		require.NotNil(t, found, "no section called %q", name)
		require.False(t, found.Changed(gui.Snapshot{}, moved),
			"%s is rebuilt when the pump moves", name)
	}
}

func TestARowsButtonsSitBesideIt(t *testing.T) {
	/*
		They were pushed to the far edge of the window, so a wide window left
		a hand's width of nothing between the end of a line and the button
		belonging to it. With twenty rows on screen the eye has to track
		across that gap and back for every one, and which button belongs to
		which line stops being obvious exactly when there are enough lines
		for it to matter.
	*/
	test.NewTempApp(t)

	/*
		Measured against the shape it replaces, because the fault was the
		border's right slot rather than anything about the button: a row
		built as `Border(nil, nil, left, actions, middle)` pins the actions
		to the window's edge however wide the window is.
	*/
	const wide = 1600

	beside := widget.NewLabel("a scene")
	near := widget.NewButton("Apply", func() {})
	put(t, gui.ListRow([]fyne.CanvasObject{beside}, near), wide)

	away := widget.NewLabel("a scene")
	far := widget.NewButton("Apply", func() {})
	put(t, container.NewBorder(nil, nil, away, far, widget.NewLabel("")), wide)

	closeBy := near.Position().X - (beside.Position().X + beside.Size().Width)
	pinned := far.Position().X - (away.Position().X + away.Size().Width)

	require.Less(t, closeBy, pinned/4,
		"the button is %.0f from the end of the row against %.0f pinned to the edge",
		closeBy, pinned)
}

// put lays an object out at a width, which is what makes positions real.
func put(t *testing.T, o fyne.CanvasObject, width float32) {
	t.Helper()
	window := test.NewWindow(o)
	t.Cleanup(window.Close)
	window.Resize(fyne.NewSize(width, 200))
}

func TestEveryRowsButtonsAreInTheSamePlace(t *testing.T) {
	/*
		A shipped scene cannot be deleted, so its row carries two buttons
		where the rows around it carry three -- and the key that starts the
		line is "6" on one row and "Shift+1" on the next. Both are widths,
		and anything packed after a width that changes lands somewhere else
		on every line. The result is a column of buttons that is not a
		column, which is the fault this row shape exists to fix.
	*/
	a := test.NewApp()
	t.Cleanup(a.Quit)

	// One key held with Shift and one without, which is the pair of widths
	// that took the lines out of line.
	routes := healthy()
	routes["GET /"+api.Version+"/keys"] = api.KeysResponse{Bindings: []api.Binding{
		{Key: "Ctrl+Alt+Num+1", Scene: "red"},
		{Key: "Ctrl+Alt+Shift+Num+1", Scene: "evening"},
	}}

	app := gui.New(service(t, routes))
	opts := app.Options("s")
	opts.SettingsPath = filepath.Join(t.TempDir(), "gui.yml")
	sh := shell.Headless(a, opts)
	app.Refresh(context.Background())

	section := &gui.ScenesSection{}
	gui.OpenScenes(section, app)
	built := section.Build(sh)
	window := test.NewWindow(built)
	t.Cleanup(window.Close)
	window.Resize(fyne.NewSize(1200, 700))

	var at []float32
	fynetest.Walk(built, func(o fyne.CanvasObject) bool {
		if button, ok := o.(*widget.Button); ok && button.Text == "Apply" {
			at = append(at, fyne.CurrentApp().Driver().AbsolutePositionForObject(o).X)
		}
		return false
	})
	require.Len(t, at, 2, "the list did not draw both scenes")
	require.Equal(t, at[0], at[1],
		"one row's Apply is at %.0f and the other's at %.0f", at[0], at[1])
}

// counting is a fake service that says how many times each route was asked
// for, which is the thing a test about round trips has to see.
func counting(t *testing.T, routes map[string]any) (*api.Client, func(string) int) {
	t.Helper()

	var mu sync.Mutex
	times := map[string]int{}

	socket := filepath.Join(t.TempDir(), "s")
	listener, err := api.Listen(t.Context(), socket)
	require.NoError(t, err)

	mux := http.NewServeMux()
	for path, body := range routes {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			times[r.URL.Path]++
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(body))
		})
	}
	server := &http.Server{Handler: mux, ReadHeaderTimeout: api.ReadHeaderTimeout}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })

	return api.NewClient(socket), func(path string) int {
		mu.Lock()
		defer mu.Unlock()
		return times[path]
	}
}

// screenful is a fixture with enough to choose between that asking per option
// is visible in the count.
func screenful() map[string]any {
	routes := healthy()

	pictures := make([]api.Image, 0, 12)
	for i := range 12 {
		name := fmt.Sprintf("picture%d", i)
		pictures = append(pictures, api.Image{Name: name, Path: "/tmp/" + name + ".gif"})
	}
	routes["GET /"+api.Version+"/images"] = api.ImagesResponse{Images: pictures}
	routes["GET /"+api.Version+"/dashboards"] = api.DashboardsResponse{
		Dashboards: []api.Dashboard{{Name: "cooling"}, {Name: "load"}},
	}
	return routes
}

func TestTheScreenChooserAsksTheServiceOnceNotOncePerOption(t *testing.T) {
	/*
		It fetched the pictures and the dashboards to build the list, and then
		called both routes again for every line in it to find the one
		thumbnail that line needed. Fourteen options meant thirty round trips
		over the socket on the thread drawing the window, which is the pause
		this is about -- and it grew with the number of pictures somebody had
		kept.
	*/
	a := test.NewApp()
	t.Cleanup(a.Quit)

	client, times := counting(t, screenful())
	app := gui.New(client)
	opts := app.Options("s")
	opts.SettingsPath = filepath.Join(t.TempDir(), "gui.yml")
	sh := shell.Headless(a, opts)
	app.Refresh(context.Background())

	window := test.NewWindow(widget.NewLabel("behind"))
	t.Cleanup(window.Close)
	window.Resize(fyne.NewSize(900, 700))
	sh.Window = window

	section := &gui.ScenesSection{}
	gui.OpenEditor(section, app, gui.NewDraft())
	gui.ChooseScreen(section, sh)

	images := "/" + api.Version + "/images"
	require.LessOrEqual(t, times(images), 1,
		"the chooser asked for the pictures %d times for 12 of them", times(images))
}

func TestTheScreenChoosersPicturesLineUpWithItsOptions(t *testing.T) {
	/*
		Fyne's radio group draws text and nothing else, so a thumbnail can
		only sit beside its option -- and beside means the column is laid out
		at the group's own pitch. As a VBox it was not: a VBox puts padding
		between its children and the group puts none between its options, so
		the pictures drifted a few pixels further out of line with every row
		until they were beside the wrong names.
	*/
	a := test.NewApp()
	t.Cleanup(a.Quit)

	client, _ := counting(t, screenful())
	app := gui.New(client)
	opts := app.Options("s")
	opts.SettingsPath = filepath.Join(t.TempDir(), "gui.yml")
	sh := shell.Headless(a, opts)
	app.Refresh(context.Background())

	window := test.NewWindow(widget.NewLabel("behind"))
	t.Cleanup(window.Close)
	window.Resize(fyne.NewSize(900, 900))
	sh.Window = window

	section := &gui.ScenesSection{}
	gui.OpenEditor(section, app, gui.NewDraft())
	gui.ChooseScreen(section, sh)

	shown := window.Canvas().Overlays().Top()
	require.NotNil(t, shown, "the chooser did not open")

	group := fynetest.Find[*widget.RadioGroup](shown)
	require.NotNil(t, group, "no list of options")

	var column *fyne.Container
	fynetest.WalkRendered(shown, func(o fyne.CanvasObject) bool {
		box, ok := o.(*fyne.Container)
		if !ok {
			return false
		}
		if _, ok := box.Layout.(gui.Beside); ok {
			column = box
			return true
		}
		return false
	})
	require.NotNil(t, column, "the thumbnails are not laid out beside the options")

	require.Len(t, column.Objects, len(group.Options),
		"%d pictures for %d options", len(column.Objects), len(group.Options))

	pitch := group.MinSize().Height / float32(len(group.Options))
	require.InDelta(t, pitch, column.Layout.(gui.Beside).Pitch, 0.01,
		"the pictures are stacked at %.2f and the options at %.2f",
		column.Layout.(gui.Beside).Pitch, pitch)
}

func TestTheTabStripSaysThePartsNameAndTheBodyDoesNot(t *testing.T) {
	/*
		The three parts of Create were sections of their own and each drew
		its own heading. Inside a tab strip that is the same word twice, a
		hand's width apart, and the second one is the one that is not a
		control.
	*/
	a := test.NewApp()
	t.Cleanup(a.Quit)

	// A fixture with pictures and dashboards in it, so every part draws its
	// list rather than the note it shows when the service will not answer.
	app := gui.New(service(t, screenful()))
	opts := app.Options("s")
	opts.SettingsPath = filepath.Join(t.TempDir(), "gui.yml")
	sh := shell.Headless(a, opts)
	app.Refresh(context.Background())

	create := gui.NewCreate(app)
	for _, part := range create.Parts() {
		require.True(t, create.Show(part.Title()), "no part called %q", part.Title())

		said := 0
		fynetest.WalkRendered(create.Build(sh), func(o fyne.CanvasObject) bool {
			if text, ok := o.(*canvas.Text); ok && text.Text == part.Title() {
				said++
			}
			return false
		})
		require.Equal(t, 1, said,
			"%q is on screen %d times: the tab and the heading say the same thing",
			part.Title(), said)
	}
}
