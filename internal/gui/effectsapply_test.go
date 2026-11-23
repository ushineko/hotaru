package gui_test

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"slices"
	"sync"
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
The Effects dialog changes the draft, Preview is a look, and Apply saves the
scene and puts it on the machine (spec 087).
*/

// effectCalls is every route a fake service was effectCalls, in order.
type effectCalls struct {
	mu     sync.Mutex
	routes []string
	saved  []api.Scene
}

func (a *effectCalls) note(route string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.routes = append(a.routes, route)
}

func (a *effectCalls) seen() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.routes)
}

// writes are the routes that change something, which is what this dialog is
// about: reads come from every refresh.
func (a *effectCalls) writes() []string {
	var out []string
	for _, r := range a.seen() {
		if r[:4] != "GET " {
			out = append(out, r)
		}
	}
	return out
}

// effectsService is the healthy machine with showing as the scene last
// applied, answering a save, an apply and a preview.
func effectsService(t *testing.T, showing string) (*api.Client, *effectCalls) {
	t.Helper()
	got := &effectCalls{}
	socket := testsock.Path(t)
	listener, err := api.Listen(t.Context(), socket)
	require.NoError(t, err)

	reply := func(w http.ResponseWriter, body any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}
	mux := http.NewServeMux()
	routes := healthy()
	routes["GET /"+api.Version+"/status"] = api.Status{Version: "test", Scene: showing}
	for path, body := range routes {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			got.note(r.Method + " " + r.URL.Path)
			reply(w, body)
		})
	}
	mux.HandleFunc("PUT /"+api.Version+"/scenes/{name}", func(w http.ResponseWriter, r *http.Request) {
		var scene api.Scene
		_ = json.NewDecoder(r.Body).Decode(&scene)
		got.mu.Lock()
		got.saved = append(got.saved, scene)
		got.mu.Unlock()
		got.note(r.Method + " " + r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /"+api.Version+"/scenes/{name}/apply", func(w http.ResponseWriter, r *http.Request) {
		got.note(r.Method + " " + r.URL.Path)
		reply(w, api.SceneResponse{Scene: r.PathValue("name")})
	})
	mux.HandleFunc("POST /"+api.Version+"/preview", func(w http.ResponseWriter, r *http.Request) {
		got.note(r.Method + " " + r.URL.Path)
		reply(w, api.SceneResponse{Preview: &api.Preview{Token: "a-token", Holder: "hotaru-gui"}})
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: api.ReadHeaderTimeout}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	return api.NewClient(socket), got
}

// effectsOpen is the editor on draft with the Effects dialog open, and the
// dialog.
func effectsOpen(t *testing.T, client *api.Client, draft *gui.Draft) (*shell.Shell, *gui.App, fyne.Window) {
	t.Helper()
	a := test.NewApp()
	t.Cleanup(a.Quit)
	app := gui.New(client)
	opts := app.Options("s")
	opts.SettingsPath = filepath.Join(t.TempDir(), "gui.yml")
	sh := shell.Headless(a, opts)
	app.Refresh(context.Background())

	section := &gui.ScenesSection{}
	gui.OpenEditor(section, app, draft)
	window := test.NewWindow(section.Build(sh))
	t.Cleanup(window.Close)
	window.Resize(fyne.NewSize(1200, 900))
	sh.Window = window

	gui.ChooseEffects(section, sh, app.Machine())
	require.NotNil(t, window.Canvas().Overlays().Top(), "the chooser did not open")
	return sh, app, window
}

// pickRainbow chooses Rainbow Wave for the Kraken in the open dialog.
func pickRainbow(t *testing.T, window fyne.Window) {
	t.Helper()
	var modes *widget.Select
	fynetest.WalkRendered(window.Canvas().Overlays().Top(), func(o fyne.CanvasObject) bool {
		if choose, ok := o.(*widget.Select); ok && slices.Contains(choose.Options, "Rainbow Wave") {
			modes = choose
			return true
		}
		return false
	})
	require.NotNil(t, modes)
	modes.SetSelected("Rainbow Wave")
}

// press taps the button labelled text on the topmost overlay.
func press(t *testing.T, window fyne.Window, text string) {
	t.Helper()
	button := fynetest.FindButton(window.Canvas().Overlays().Top(), text)
	require.NotNil(t, button, "no %q on the topmost dialog", text)
	test.Tap(button)
}

// eventually waits for the routes a fake service was effectCalls to match want.
func eventually(t *testing.T, got *effectCalls, want []string, why string) {
	t.Helper()
	require.Eventually(t, func() bool { return slices.Equal(got.writes(), want) },
		5*time.Second, 10*time.Millisecond, "%s: effectCalls %v", why, got.writes())
}

func TestAPickChangesTheDraftAndNothingElse(t *testing.T) {
	client, got := effectsService(t, "")
	draft := gui.DraftFrom(api.Scene{Name: "evening"})
	_, _, window := effectsOpen(t, client, draft)

	pickRainbow(t, window)
	require.Equal(t, "Rainbow Wave", draft.Effect("NZXT Kraken").Mode, "the pick did not reach the draft")
	// A pick starts nothing in the background, so nothing can arrive later.
	require.Empty(t, got.writes(), "a pick went to the service")
}

func TestCancelPutsTheEffectsBack(t *testing.T) {
	client, got := effectsService(t, "")
	draft := gui.DraftFrom(api.Scene{Name: "evening",
		Effects: map[string]api.Effect{"NZXT Kraken": {Mode: "Breathing"}}})
	_, _, window := effectsOpen(t, client, draft)

	pickRainbow(t, window)
	press(t, window, "Cancel")
	require.Equal(t, "Breathing", draft.Effect("NZXT Kraken").Mode, "Cancel kept the pick")
	require.Empty(t, got.writes(), "Cancel sent something")
}

func TestPreviewIsALookThatEnds(t *testing.T) {
	client, got := effectsService(t, "")
	draft := gui.DraftFrom(api.Scene{Name: "evening"})
	_, app, window := effectsOpen(t, client, draft)

	pickRainbow(t, window)
	press(t, window, "Preview")
	eventually(t, got, []string{"POST /" + api.Version + "/preview"}, "Preview did not show the draft")
	require.True(t, app.Previewing())

	// The look is over the Effects dialog; dismissing it puts the lights back.
	press(t, window, "Done")
	require.False(t, app.Previewing(), "dismissing the look kept the preview")
	require.NotNil(t, fynetest.FindButton(window.Canvas().Overlays().Top(), "Apply"),
		"the Effects dialog closed with the look")
	require.Equal(t, []string{"POST /" + api.Version + "/preview"}, got.writes(), "the look saved something")
}

func TestApplySavesAndAppliesASavedScene(t *testing.T) {
	client, got := effectsService(t, "morning")
	draft := gui.DraftFrom(api.Scene{Name: "evening"})
	sh, _, window := effectsOpen(t, client, draft)

	pickRainbow(t, window)
	press(t, window, "Apply")
	eventually(t, got, []string{
		"PUT /" + api.Version + "/scenes/evening",
		"POST /" + api.Version + "/scenes/evening/apply",
	}, "Apply did not save and then apply")

	got.mu.Lock()
	require.Equal(t, "Rainbow Wave", got.saved[0].Effects["NZXT Kraken"].Mode, "the pick was not saved")
	got.mu.Unlock()
	// The hand-backs drawn first, or a banner would still be queued.
	gui.Settle()
	require.Empty(t, sh.FlashText(), "Apply left a banner to dismiss")
}

func TestApplyDoesNotApplyTheShowingSceneTwice(t *testing.T) {
	// Saving the scene the machine shows lights it again in the service.
	client, got := effectsService(t, "evening")
	draft := gui.DraftFrom(api.Scene{Name: "evening"})
	_, _, window := effectsOpen(t, client, draft)

	pickRainbow(t, window)
	press(t, window, "Apply")
	eventually(t, got, []string{"PUT /" + api.Version + "/scenes/evening"}, "Apply did not save")
	require.Never(t, func() bool { return len(got.writes()) > 1 }, 200*time.Millisecond, 10*time.Millisecond,
		"the showing scene was applied a second time")
}

func TestApplyAsksANewSceneForItsName(t *testing.T) {
	client, got := effectsService(t, "")
	draft := gui.NewDraft()
	_, _, window := effectsOpen(t, client, draft)

	pickRainbow(t, window)
	press(t, window, "Apply")

	var entry *widget.Entry
	fynetest.WalkRendered(window.Canvas().Overlays().Top(), func(o fyne.CanvasObject) bool {
		if e, ok := o.(*widget.Entry); ok {
			entry = e
			return true
		}
		return false
	})
	require.NotNil(t, entry, "a new scene was not effectCalls for a name")
	require.Empty(t, got.writes(), "something was saved before it had a name")

	entry.SetText("night")
	press(t, window, "Apply")
	eventually(t, got, []string{
		"PUT /" + api.Version + "/scenes/night",
		"POST /" + api.Version + "/scenes/night/apply",
	}, "Apply did not save and apply under the name given")
	gui.Settle()
	require.Equal(t, "night", draft.From, "the editor does not know the scene is saved")
}
