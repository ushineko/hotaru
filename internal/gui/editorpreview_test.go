package gui_test

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/api"
	"github.com/ushineko/hotaru/internal/gui"
)

/*
What the editor sends when somebody changes a draft.

**The bug this exists for.** Putting a draft up sent the scene, effects and
all. Changing it sent a hand-built request to the plain lighting route --
which has no effects field at all -- so the first change to any draft took the
keyboard out of its effect and put it in Direct, and only applying a saved
scene ever put it back. One scene, two ways of becoming lights, and the
difference was invisible from the window. See spec 054.

Asserted as what goes over the socket, because that is where the two paths
differed: both looked right from inside the window.
*/

// watched is a service that records the route and body of every call.
type watched struct {
	mu    sync.Mutex
	calls []call
}

type call struct {
	route string
	draft api.DraftRequest
}

func (w *watched) note(route string, r *http.Request) {
	var in api.DraftRequest
	_ = json.NewDecoder(r.Body).Decode(&in)

	w.mu.Lock()
	defer w.mu.Unlock()
	w.calls = append(w.calls, call{route: route, draft: in})
}

func (w *watched) routes() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]string, 0, len(w.calls))
	for _, one := range w.calls {
		out = append(out, one.route)
	}
	return out
}

// listening serves the two routes a preview can reach, and records which.
func listening(t *testing.T, seen *watched) *api.Client {
	t.Helper()

	socket := filepath.Join(t.TempDir(), "s")
	listener, err := api.Listen(t.Context(), socket)
	require.NoError(t, err)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /"+api.Version+"/preview", func(w http.ResponseWriter, r *http.Request) {
		seen.note("preview", r)
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(api.SceneResponse{
			Preview: &api.Preview{Token: "a-token", Holder: "hotaru-gui"},
		}))
	})
	mux.HandleFunc("POST /"+api.Version+"/lighting/apply", func(w http.ResponseWriter, r *http.Request) {
		seen.note("lighting/apply", r)
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(api.ApplyResponse{}))
	})

	server := &http.Server{Handler: mux, ReadHeaderTimeout: api.ReadHeaderTimeout}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })

	return api.NewClient(socket)
}

// drafted is a scene whose keyboard runs an effect of its own colour: the
// thing that was being dropped.
func drafted(colour string) api.Scene {
	return api.Scene{
		Name:        "draft",
		Assignments: []api.SceneAssignment{{Target: "Keychron", Colour: colour}},
		Effects: map[string]api.Effect{
			"Keychron": {Mode: "Solid Reactive", Colour: "#00ff00"},
		},
	}
}

func TestChangingADraftSendsTheSceneAndNotJustItsColours(t *testing.T) {
	seen := &watched{}
	app := gui.New(listening(t, seen))

	require.NoError(t, app.Preview(context.Background(), drafted("blue")))
	require.NoError(t, app.Preview(context.Background(), drafted("red")))
	t.Cleanup(app.EndPreview)

	require.Equal(t, []string{"preview", "preview"}, seen.routes(),
		"a change to a draft went somewhere a scene's effects cannot follow")

	seen.mu.Lock()
	defer seen.mu.Unlock()
	change := seen.calls[1]
	require.Equal(t, "a-token", change.draft.Token,
		"the change should name the lease rather than take a second one")
	require.Equal(t, drafted("red").Effects, change.draft.Scene.Effects,
		"the effect did not travel with the change")
}
