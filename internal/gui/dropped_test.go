package gui_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/stretchr/testify/require"
	"github.com/ushineko/fynedesygn/fynetest"
	"github.com/ushineko/fynedesygn/shell"
	"github.com/ushineko/hotaru/internal/api"
	"github.com/ushineko/hotaru/internal/gui"
	"github.com/ushineko/hotaru/internal/images"
)

// asked is one request the fake service took, with its body.
type asked struct {
	route string
	body  []byte
}

/*
recording is a fake service that keeps every request it is sent, bodies
included, and answers the writes a drop leads to.

A drop is a chain -- import, choose, save, make -- and the end of it is what
reaches the service. A test that stopped at what the window drew would pass a
chain that saved nothing.
*/
func recording(t *testing.T, routes map[string]any) (*api.Client, func() []asked) {
	t.Helper()

	var mu sync.Mutex
	var got []asked

	socket := filepath.Join(t.TempDir(), "s")
	listener, err := api.Listen(t.Context(), socket)
	require.NoError(t, err)

	mux := http.NewServeMux()
	keep := func(r *http.Request) []byte {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		got = append(got, asked{route: r.Method + " " + r.URL.Path, body: body})
		mu.Unlock()
		return body
	}
	for path, body := range routes {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			keep(r)
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(body))
		})
	}
	mux.HandleFunc("PUT /"+api.Version+"/dashboards/{name}", func(w http.ResponseWriter, r *http.Request) {
		keep(r)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /"+api.Version+"/images/{name}/scene", func(w http.ResponseWriter, r *http.Request) {
		var in api.SceneFromImageRequest
		require.NoError(t, json.Unmarshal(keep(r), &in))
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(api.Scene{Name: in.Scene, Screen: in.Screen}))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		keep(r)
		http.NotFound(w, r)
	})

	server := &http.Server{Handler: mux, ReadHeaderTimeout: api.ReadHeaderTimeout}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })

	return api.NewClient(socket), func() []asked {
		mu.Lock()
		defer mu.Unlock()
		return append([]asked{}, got...)
	}
}

// sent is every request to a route, in order.
func sent(all []asked, route string) []asked {
	var out []asked
	for _, one := range all {
		if one.route == route {
			out = append(out, one)
		}
	}
	return out
}

// sentPrefix is every request whose route starts with this.
func sentPrefix(all []asked, prefix string) []asked {
	var out []asked
	for _, one := range all {
		if strings.HasPrefix(one.route, prefix) {
			out = append(out, one)
		}
	}
	return out
}

/*
alreadyKept is a library that holds the file a test drops, and the screens to
start from: two saved ones, and one named after the picture so the new
screen's name has to step aside.
*/
func alreadyKept(t *testing.T) (map[string]any, string) {
	t.Helper()
	source := []byte("a wallpaper, as somebody has it on disk")
	path := filepath.Join(t.TempDir(), "wall.jpg")
	require.NoError(t, os.WriteFile(path, source, 0o600))

	routes := healthy()
	routes["GET /"+api.Version+"/images"] = api.ImagesResponse{Images: []api.Image{
		{Name: "wall", Path: "/tmp/wall.gif", Source: images.SourceHash(source)},
	}}
	routes["GET /"+api.Version+"/dashboards"] = api.DashboardsResponse{
		Active: "coolant",
		Dashboards: []api.Dashboard{
			{Name: "coolant", Shipped: true, Arrangement: "ring", Rings: []string{"coolant"}},
			{
				Name: "cooling", Shipped: true, Arrangement: "stacked",
				Theme: "ice", Background: api.DashboardBackground{Kind: "starfield", Dim: 30},
			},
			{Name: "wall", Arrangement: "grid"},
		},
	}
	return routes, path
}

// dropWindow is the window as it runs, on a part of Create.
func dropWindow(t *testing.T, client *api.Client, part string) (*gui.App, *shell.Shell, fyne.Window, *gui.Create) {
	t.Helper()
	a := test.NewApp()
	t.Cleanup(a.Quit)

	app := gui.New(client)
	opts := app.Options("s")
	opts.SettingsPath = filepath.Join(t.TempDir(), "gui.yml")
	sh := shell.Headless(a, opts)
	opts.OnCreate(sh)
	app.Refresh(context.Background())

	window := test.NewWindow(widget.NewLabel("behind"))
	t.Cleanup(window.Close)
	window.Resize(fyne.NewSize(1000, 800))
	sh.Window = window

	var group *gui.Create
	for _, section := range sh.Sections() {
		if found, ok := section.(*gui.Create); ok {
			group = found
		}
	}
	require.NotNil(t, group)
	require.True(t, group.Show(part))
	sh.Select("Create")
	return app, sh, window, group
}

// chooseLayoutIn picks an option in the layout chooser and confirms it.
func chooseLayoutIn(t *testing.T, window fyne.Window, label string) []string {
	t.Helper()
	shown := window.Canvas().Overlays().Top()
	require.NotNil(t, shown, "the layout chooser did not open")
	chooser := fynetest.Find[*widget.RadioGroup](shown)
	require.NotNil(t, chooser, "the layout chooser has no options")
	options := append([]string{}, chooser.Options...)
	chooser.SetSelected(label)
	use := fynetest.FindButton(shown, "Use it")
	require.NotNil(t, use)
	test.Tap(use)
	return options
}

func TestADroppedFileTheLibraryHoldsIsNotKeptAgain(t *testing.T) {
	// Spec 062 R3: the same wallpaper dragged in twice is one picture.
	routes, path := alreadyKept(t)
	client, requests := recording(t, routes)
	app, sh, _, _ := dropWindow(t, client, "Pictures")

	gui.Drop(app, sh, []fyne.URI{storage.NewFileURI(path)})
	gui.Library(app).Settle()

	require.Empty(t, sent(requests(), "POST /"+api.Version+"/images/preview"),
		"a file the library holds was converted again")
	require.Empty(t, sentPrefix(requests(), "PUT /"+api.Version+"/images/"),
		"a file the library holds was stored again")
}

func TestAPictureDroppedOnScreenOpensTheEditorWithItBehind(t *testing.T) {
	// Spec 062 R5: a saved screen's layout, with the picture behind it, in
	// the editor and not yet saved.
	routes, path := alreadyKept(t)
	client, requests := recording(t, routes)
	app, sh, window, group := dropWindow(t, client, "Screen")

	gui.Drop(app, sh, []fyne.URI{storage.NewFileURI(path)})
	gui.Library(app).Settle()

	require.Equal(t, "Create", sh.Current().Title())
	require.Equal(t, "Screen", group.Showing(), "a drop on Screen moved the window")

	options := chooseLayoutIn(t, window, gui.LayoutLabel(api.Dashboard{Name: "cooling", Arrangement: "stacked"}))
	require.NotContains(t, options, gui.PictureAlone, "a screen was offered no screen at all")

	var screen *gui.DashboardsSection
	for _, part := range group.Parts() {
		if found, ok := part.(*gui.DashboardsSection); ok {
			screen = found
		}
	}
	require.NotNil(t, screen)
	require.True(t, screen.Busy(), "choosing a layout did not open the editor")

	draft := gui.DraftDashboard(screen)
	require.Equal(t, "stacked", draft.Arrangement, "the layout chosen is not the draft's")
	require.Equal(t, "ice", draft.Theme, "the copy left the template's theme behind")
	require.False(t, draft.Shipped, "the copy of a shipped screen is still marked shipped")
	require.Equal(t, api.DashboardBackground{Kind: "picture", Picture: "wall", Dim: 30}, draft.Background)

	said := fynetest.Text(screen.Build(sh))
	require.Contains(t, said, "wall-2", "the name offered is one a screen already has")

	require.Empty(t, sentPrefix(requests(), "PUT /"+api.Version+"/dashboards/"),
		"a screen was saved before anybody pressed Save")
}

func TestAPictureDroppedOnScenesMakesAScreenAndAScene(t *testing.T) {
	// Spec 062 R6: the chooser, then Make a scene, then a screen and a scene
	// that shows it.
	routes, path := alreadyKept(t)
	client, requests := recording(t, routes)
	app, sh, window, group := dropWindow(t, client, "Scenes")

	gui.Drop(app, sh, []fyne.URI{storage.NewFileURI(path)})
	gui.Library(app).Settle()
	require.Equal(t, "Scenes", group.Showing(), "a drop on Scenes moved the window")

	options := chooseLayoutIn(t, window, gui.LayoutLabel(api.Dashboard{Name: "coolant", Arrangement: "ring"}))
	require.Equal(t, gui.PictureAlone, options[0], "the picture alone is not the first option")

	// Make a scene is up, named after the picture.
	shown := window.Canvas().Overlays().Top()
	require.NotNil(t, shown, "Make a scene did not open")
	require.Contains(t, fynetest.Text(shown), "Make a scene from wall")
	require.Empty(t, sentPrefix(requests(), "PUT /"+api.Version+"/dashboards/"),
		"a screen was saved before Make it")

	makeIt := fynetest.FindButton(shown, "Make it")
	require.NotNil(t, makeIt)
	test.Tap(makeIt)

	scene := "POST /" + api.Version + "/images/wall/scene"
	require.Eventually(t, func() bool { return len(sent(requests(), scene)) > 0 },
		3*time.Second, 20*time.Millisecond, "no scene was made")

	saved := sent(requests(), "PUT /"+api.Version+"/dashboards/wall-2")
	require.Len(t, saved, 1, "the screen was not saved under an unused name")
	var board api.Dashboard
	require.NoError(t, json.Unmarshal(saved[0].body, &board))
	require.Equal(t, "ring", board.Arrangement)
	require.Equal(t, api.DashboardBackground{Kind: "picture", Picture: "wall"}, board.Background)

	var made api.SceneFromImageRequest
	require.NoError(t, json.Unmarshal(sent(requests(), scene)[0].body, &made))
	require.Equal(t, "wall", made.Scene)
	require.Equal(t, api.ScreenDashboardPrefix+"wall-2", made.Screen,
		"the scene does not show the screen made for it")
}

func TestAPictureAloneMakesASceneAndNoScreen(t *testing.T) {
	routes, path := alreadyKept(t)
	client, requests := recording(t, routes)
	app, sh, window, _ := dropWindow(t, client, "Scenes")

	gui.Drop(app, sh, []fyne.URI{storage.NewFileURI(path)})
	gui.Library(app).Settle()
	chooseLayoutIn(t, window, gui.PictureAlone)

	makeIt := fynetest.FindButton(window.Canvas().Overlays().Top(), "Make it")
	require.NotNil(t, makeIt)
	test.Tap(makeIt)

	scene := "POST /" + api.Version + "/images/wall/scene"
	require.Eventually(t, func() bool { return len(sent(requests(), scene)) > 0 },
		3*time.Second, 20*time.Millisecond, "no scene was made")
	var made api.SceneFromImageRequest
	require.NoError(t, json.Unmarshal(sent(requests(), scene)[0].body, &made))
	require.Empty(t, made.Screen, "the picture alone asked for a screen")
	require.Empty(t, sentPrefix(requests(), "PUT /"+api.Version+"/dashboards/"))
}

func TestADropOnAnOpenScreenEditorGoesToPictures(t *testing.T) {
	// Spec 062 R4.2: an edit in progress is not interrupted by a chooser.
	routes, path := alreadyKept(t)
	client, _ := recording(t, routes)
	app, sh, _, group := dropWindow(t, client, "Screen")

	for _, part := range group.Parts() {
		if found, ok := part.(*gui.DashboardsSection); ok {
			gui.EditDashboard(found, api.Dashboard{Name: "draft", Arrangement: "ring"})
		}
	}

	gui.Drop(app, sh, []fyne.URI{storage.NewFileURI(path)})
	gui.Library(app).Settle()
	require.Equal(t, "Pictures", group.Showing())
}

func TestANewPictureIsKeptBeforeTheLayoutIsAsked(t *testing.T) {
	// Spec 062 R3.1 and R5.1: a file the library does not hold is converted,
	// named and kept as Pictures keeps one, and the chooser follows.
	routes, _ := alreadyKept(t)
	routes["POST /"+api.Version+"/images/preview"] = api.ConvertedImage{Frames: 1}
	routes["PUT /"+api.Version+"/images/aurora"] = api.Image{Name: "aurora", Path: "/tmp/aurora.gif"}
	client, requests := recording(t, routes)
	app, sh, window, _ := dropWindow(t, client, "Screen")

	path := filepath.Join(t.TempDir(), "aurora.jpg")
	require.NoError(t, os.WriteFile(path, []byte("a file nobody has kept"), 0o600))
	gui.Drop(app, sh, []fyne.URI{storage.NewFileURI(path)})
	gui.Library(app).Settle()

	shown := window.Canvas().Overlays().Top()
	require.NotNil(t, shown, "the conversion was not shown")
	require.Contains(t, fynetest.Text(shown), "This is what the panel will show")
	keep := fynetest.FindButton(shown, "Keep it")
	require.NotNil(t, keep)
	test.Tap(keep)

	require.Eventually(t, func() bool {
		top := window.Canvas().Overlays().Top()
		return top != nil && strings.Contains(fynetest.Text(top), "A layout for aurora")
	}, 3*time.Second, 20*time.Millisecond, "the layout chooser did not follow the keep")
	require.Len(t, sent(requests(), "PUT /"+api.Version+"/images/aurora"), 1)
}
