package gui_test

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/stretchr/testify/require"
	"github.com/ushineko/fynedesygn/fynetest"
	"github.com/ushineko/fynedesygn/shell"

	"github.com/ushineko/hotaru/internal/api"
	"github.com/ushineko/hotaru/internal/gui"
	"github.com/ushineko/hotaru/internal/testsock"
)

/*
A device's card says what it follows and changes it (spec 077 R3.2), and a
follower offers a match of its pace (R3.9), each through the service.
*/

// systemPage builds the System section headless, for a test that presses
// something on it.
func systemPage(t *testing.T, client *api.Client) fyne.CanvasObject {
	t.Helper()
	a := test.NewApp()
	t.Cleanup(a.Quit)
	app := gui.New(client)
	opts := app.Options("/run/nowhere/hotaru.sock")
	opts.SettingsPath = filepath.Join(t.TempDir(), "gui.yml")
	sh := shell.Headless(a, opts)
	app.Refresh(context.Background())
	for _, s := range sh.Sections() {
		if s.Title() == "System" {
			return s.Build(sh)
		}
	}
	t.Fatal("no System section")
	return nil
}

// following is the healthy machine with the Aura following the Kraken.
func following() map[string]any {
	routes := healthy()
	devices := routes["GET /"+api.Version+"/devices"].(api.DevicesResponse)
	devices.Devices[0].Stored = []string{"Breathing", "Tai Chi"}
	devices.Devices[0].Modes = append(devices.Devices[0].Modes, "Tai Chi")
	devices.Devices = append(devices.Devices, api.Device{
		Name: "ASUS ROG MAXIMUS Z790 HERO", LEDs: 4, ActiveMode: "Static", InScope: true,
		Modes: []string{"Static", "Breathing", "Tai Chi"}, Follows: "kraken",
		Pace: map[string]float64{"breathing": 2.16},
	})
	routes["GET /"+api.Version+"/devices"] = devices
	routes["POST /"+api.Version+"/rules/device"] = api.RuleResponse{RulesFile: "hotaru.yml", Match: "maximus"}
	return routes
}

func TestEveryCardSaysWhatItFollows(t *testing.T) {
	said := screen(t, service(t, following()), "System")
	require.Contains(t, said, "Follows")
	require.Contains(t, said, "No one", "the Kraken follows nothing and does not say so")
	require.Contains(t, said, "Match pace", "a follower has no match")
}

func TestOnlyAFollowerOffersAMatch(t *testing.T) {
	said := screen(t, service(t, healthy()), "System")
	require.Contains(t, said, "Follows")
	require.NotContains(t, said, "Match pace")
}

// Choosing a leader reaches the service's rules route.
func TestChoosingALeaderWritesTheRule(t *testing.T) {
	client, asked := watching(t, following())
	page := systemPage(t, client)
	choice := fynetest.FindSelect(page)
	require.NotNil(t, choice)
	require.Equal(t, "No one", choice.Selected, "the first card is the Kraken's")
	drain(asked)

	choice.SetSelected("ASUS ROG MAXIMUS Z790 HERO")
	require.Eventually(t, func() bool {
		for {
			select {
			case route := <-asked:
				if route == "POST /"+api.Version+"/rules/device" {
					return true
				}
			default:
				return false
			}
		}
	}, 5*time.Second, 10*time.Millisecond)
}

// drain empties what the page asked while it was built.
func drain(asked chan string) {
	for {
		select {
		case <-asked:
		default:
			return
		}
	}
}

/*
An effect chosen in the scene editor is shown on the hardware as it is
chosen, as a colour is in the picker, and Cancel puts the draft back: an
effect that only appeared once the scene was saved and applied again seemed
to do nothing (the live run).
*/
func TestAnEffectIsShownAsItIsChosen(t *testing.T) {
	seen := &watched{}
	socket := testsock.Path(t)
	listener, err := api.Listen(t.Context(), socket)
	require.NoError(t, err)
	mux := http.NewServeMux()
	for path, body := range healthy() {
		mux.HandleFunc(path, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(body)
		})
	}
	mux.HandleFunc("POST /"+api.Version+"/preview", func(w http.ResponseWriter, r *http.Request) {
		seen.note("preview", r)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(api.SceneResponse{Preview: &api.Preview{Token: "a-token", Holder: "hotaru-gui"}})
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: api.ReadHeaderTimeout}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })

	a := test.NewApp()
	t.Cleanup(a.Quit)
	app := gui.New(api.NewClient(socket))
	opts := app.Options("s")
	opts.SettingsPath = filepath.Join(t.TempDir(), "gui.yml")
	sh := shell.Headless(a, opts)
	app.Refresh(context.Background())
	section := &gui.ScenesSection{}
	draft := gui.NewDraft()
	gui.OpenEditor(section, app, draft)
	window := test.NewWindow(section.Build(sh))
	t.Cleanup(window.Close)
	window.Resize(fyne.NewSize(1200, 900))
	sh.Window = window

	gui.ChooseEffects(section, sh, app.Machine())
	shown := window.Canvas().Overlays().Top()
	require.NotNil(t, shown)
	var modes *widget.Select
	fynetest.WalkRendered(shown, func(o fyne.CanvasObject) bool {
		if choose, ok := o.(*widget.Select); ok && slices.Contains(choose.Options, "Rainbow Wave") {
			modes = choose
			return true
		}
		return false
	})
	require.NotNil(t, modes)

	modes.SetSelected("Rainbow Wave")
	require.Eventually(t, func() bool {
		seen.mu.Lock()
		defer seen.mu.Unlock()
		for _, c := range seen.calls {
			if c.draft.Scene.Effects["NZXT Kraken"].Mode == "Rainbow Wave" {
				return true
			}
		}
		return false
	}, 5*time.Second, 10*time.Millisecond, "the effect was not shown as it was chosen")

	cancel := fynetest.FindButton(shown, "Cancel")
	require.NotNil(t, cancel)
	test.Tap(cancel)
	require.Empty(t, draft.Scene("evening").Effects, "Cancel kept the effect")
}

// A device with headers has Headers… on its card, and every in-scope device
// has Names… (spec 078 R5).
func TestCardsOfferHeadersAndNames(t *testing.T) {
	routes := healthy()
	devices := routes["GET /"+api.Version+"/devices"].(api.DevicesResponse)
	devices.Devices = append(devices.Devices, api.Device{
		Name: "ASUS ROG MAXIMUS Z790 HERO", LEDs: 21, ActiveMode: "Static", InScope: true, Modes: []string{"Static"},
		Zones: []api.Zone{{Name: "Aura Mainboard", Count: 1}, {Name: "Addressable RGB Header 3", First: 1, Count: 20, Max: 120}},
	})
	routes["GET /"+api.Version+"/devices"] = devices
	said := screen(t, service(t, routes), "System")
	require.Equal(t, 1, strings.Count(said, "Headers…"), "only the board has headers")
	require.Equal(t, 2, strings.Count(said, "Names…"), "every in-scope device has names")
}
