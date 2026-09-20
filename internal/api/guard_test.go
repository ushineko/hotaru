package api_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/api"
	"github.com/ushineko/hotaru/internal/openrgb"
	"github.com/ushineko/hotaru/internal/service"
)

/*
The service listens on a Unix socket and opens no port.

Asserted against the source rather than the runtime, because the promise is
about what the program can do and not about what one test run happened to see.
The socket's permissions are the whole authentication story: a port would need
an authentication scheme invented for a program that changes the colour of
lights, and this is what stops one appearing by accident.
*/
func TestNothingInTheModuleOpensANetworkListener(t *testing.T) {
	root := filepath.Join("..", "..")
	fset := token.NewFileSet()

	require.NoError(t, filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}

		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			// Any listen whose network argument is a TCP or UDP literal.
			for _, arg := range call.Args {
				literal, ok := arg.(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					continue
				}
				switch strings.Trim(literal.Value, `"`) {
				case "tcp", "tcp4", "tcp6", "udp":
					if name := called(call); strings.Contains(name, "Listen") {
						t.Errorf("%s opens a %s listener", fset.Position(call.Pos()), literal.Value)
					}
				}
			}
			if name := called(call); name == "http.ListenAndServe" || name == "http.ListenAndServeTLS" {
				t.Errorf("%s serves on a network address", fset.Position(call.Pos()))
			}
			return true
		})
		return nil
	}))
}

func called(call *ast.CallExpr) string {
	switch fn := call.Fun.(type) {
	case *ast.SelectorExpr:
		if ident, ok := fn.X.(*ast.Ident); ok {
			return ident.Name + "." + fn.Sel.Name
		}
		return fn.Sel.Name
	case *ast.Ident:
		return fn.Name
	}
	return ""
}

// Every route is reachable and answers, so a client of this package can rely
// on the list rather than on reading the handler.
func TestEveryRouteIsServed(t *testing.T) {
	client := running(t, nil, openrgb.NewFake(board()))
	require.NotEmpty(t, api.Routes())

	// Exercised through the client, which is the only way a shell reaches
	// them: a route with no client method is one no shell can use.
	_, err := client.Health(t.Context())
	require.NoError(t, err)
	_, err = client.Devices(t.Context())
	require.NoError(t, err)
	_, err = client.Status(t.Context())
	require.NoError(t, err)
	_, err = client.Apply(t.Context(), api.ApplyRequest{Colour: "red"})
	require.NoError(t, err)
	_, err = client.Probe(t.Context(), api.ProbeRequest{})
	require.NoError(t, err)
	_, err = client.Reconcile(t.Context())
	require.NoError(t, err)
	_, err = client.Reload(t.Context())
	require.NoError(t, err)
}

// An unknown path is a 404 rather than something worse, so a client that is
// newer than the service gets a clear answer.
func TestAnUnknownRouteIsPlainlyNotFound(t *testing.T) {
	server := httptest.NewServer(api.Handler(service.New(nil, openrgb.NewFake(board()), "")))
	defer server.Close()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/v1/nonsense", nil)
	require.NoError(t, err)
	resp, err := server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

/*
The packages that could one day be a public SDK are written as if they already
were.

No printing and no exiting below the command boundary. A library that writes to
stdout is unusable inside anything that formats its own output -- a GUI, a
daemon, another program importing it -- and one that exits takes its caller with
it. The rule costs nothing to keep and is expensive to retrofit, which is why it
is a test rather than a note.

Mutable package-level state is the third part of that rule and is not asserted
here: a var holding a default table is fine and a var holding a device's last
colour is not, and no amount of parsing tells them apart. That one stays a
review question.
*/
func TestTheCorePackagesNeitherPrintNorExit(t *testing.T) {
	core := []string{"colour", "config", "devices", "openrgb", "queue", "service", "state"}
	fset := token.NewFileSet()

	for _, pkg := range core {
		dir := filepath.Join("..", pkg)
		entries, err := os.ReadDir(dir)
		require.NoError(t, err)

		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") ||
				strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			file, err := parser.ParseFile(fset, path, nil, 0)
			require.NoError(t, err)

			ast.Inspect(file, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				switch name := called(call); name {
				case "fmt.Print", "fmt.Printf", "fmt.Println", "print", "println":
					t.Errorf("%s prints; a library hands its words back instead",
						fset.Position(call.Pos()))
				case "os.Exit", "log.Fatal", "log.Fatalf", "log.Fatalln":
					t.Errorf("%s exits; that decision belongs to the program, not the package",
						fset.Position(call.Pos()))
				}
				return true
			})
		}
	}
}
