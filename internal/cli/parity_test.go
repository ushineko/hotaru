package cli_test

import (
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/api"
	"github.com/ushineko/hotaru/internal/openrgb"
	"github.com/ushineko/hotaru/internal/service"
	"github.com/ushineko/hotaru/internal/state"
)

/*
Everything the service can do is reachable from the CLI.

The parity rule, asserted rather than intended. It is symmetrical on purpose:
the GUI will have buttons and the temptation is to add a capability there and
mean to get to the terminal later, which leaves a headless machine unable to do
something the program plainly does.

The evidence is what the commands actually ask for. A recording proxy sits in
front of the service, every command is run, and the routes they touched are
compared against the routes that exist.
*/
func TestEveryRouteTheServiceServesIsReachableFromTheCommandLine(t *testing.T) {
	seen := &recorder{}
	socket := recording(t, seen)

	for _, args := range [][]string{
		{"light", "list"},
		{"light", "set", "red"},
		{"light", "off"},
		{"light", "health"},
		{"light", "probe"},
		{"status"},
		{"cooling"},
		{"screen", "readout"},
		{"scene", "list"},
		{"scene", "set", "parity", "kraken=red"},
		{"scene", "save", "parity"},
		{"scene", "apply", "parity"},
		{"scene", "delete", "parity"},
		{"preview"},
		{"preview", "renew", "nosuchtoken"},
		{"preview", "release", "nosuchtoken"},
		{"reconcile"},
		{"reload"},
	} {
		// Errors are fine here: a command that ran and reported something
		// still reached its route, which is what this is measuring.
		_, _ = run(t, socket, args...)
	}

	missing := seen.missing(api.Routes())
	require.Empty(t, missing,
		"the service serves routes no command reaches: %s", strings.Join(missing, ", "))
}

type recorder struct {
	mu    sync.Mutex
	paths map[string]bool
}

func (r *recorder) note(method, path string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.paths == nil {
		r.paths = map[string]bool{}
	}
	r.paths[method+" "+path] = true
}

func (r *recorder) missing(routes []string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, route := range routes {
		if !r.reached(route) {
			out = append(out, route)
		}
	}
	sort.Strings(out)
	return out
}

// reached matches a route against what was asked for, allowing for the
// wildcard segments in a pattern: "PUT /v1/scenes/{name}" is reached by a
// request to "PUT /v1/scenes/evening".
func (r *recorder) reached(route string) bool {
	if r.paths[route] {
		return true
	}
	want := strings.Split(route, "/")
	for asked := range r.paths {
		got := strings.Split(asked, "/")
		if len(got) != len(want) {
			continue
		}
		match := true
		for i := range want {
			if strings.HasPrefix(want[i], "{") && strings.HasSuffix(want[i], "}") {
				continue
			}
			if want[i] != got[i] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// recording serves the API behind a handler that notes what was asked for.
func recording(t *testing.T, seen *recorder) string {
	t.Helper()

	dir := t.TempDir()
	socket := filepath.Join(dir, "s")
	listener, err := api.Listen(t.Context(), socket)
	require.NoError(t, err)

	desired, err := state.Open(filepath.Join(dir, "state.yml"))
	require.NoError(t, err)
	svc := service.New(nil, openrgb.NewFake(board()), "127.0.0.1:6742")
	svc.SetRecorder(desired)

	inner := api.Handler(svc)
	watched := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.note(r.Method, r.URL.Path)
		inner.ServeHTTP(w, r)
	})

	server := &http.Server{Handler: watched, ReadHeaderTimeout: api.ReadHeaderTimeout}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = server.Serve(listener)
	}()
	t.Cleanup(func() {
		_ = server.Close()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatal("the recording service did not stop")
		}
	})
	return socket
}
