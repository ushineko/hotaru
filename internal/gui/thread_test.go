package gui_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

/*
Nothing that draws is called from a worker.

Fyne is single-threaded for anything that touches the screen, and the work
started by Perform runs in a goroutine. Calling Flash or Invalidate there
prints a warning and then, a minute later, panics inside the text shaper with
an index out of range -- which is what happened on somebody's desk while they
were using the window.

A warning that only appears in a log nobody is reading is not a guard. This is:
it reads the package and fails on the shape rather than on the symptom.
*/
func TestNothingThatDrawsRunsInAWorker(t *testing.T) {
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)

	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		source, err := os.ReadFile(name)
		require.NoError(t, err)

		parsed, err := parser.ParseFile(token.NewFileSet(), name, source, 0)
		require.NoError(t, err)

		ast.Inspect(parsed, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || !isPerform(call) {
				return true
			}
			for _, arg := range call.Args {
				body, ok := arg.(*ast.FuncLit)
				if !ok {
					continue
				}
				if drawn := draws(body.Body, false); drawn != "" {
					t.Errorf("%s: %s is called inside Perform, which runs off the UI thread; "+
						"hand it back with onScreen", name, drawn)
				}
			}
			return true
		})
	}
}

// isPerform reports whether a call is one of the shell's operation starters.
func isPerform(call *ast.CallExpr) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	return ok && (selector.Sel.Name == "Perform" || selector.Sel.Name == "PerformCancellable")
}

/*
draws finds a call that touches the screen, ignoring anything already handed
back to the UI thread.

`wrapped` says the walk is inside onScreen or fyne.Do, where these calls are
exactly right.
*/
func draws(node ast.Node, wrapped bool) string {
	var found string
	ast.Inspect(node, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if handsBack(call) {
			// Everything inside is on the UI thread, so stop here.
			return false
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || wrapped || found != "" {
			return true
		}
		switch selector.Sel.Name {
		case "Flash", "Invalidate", "OK", "Report", "Select", "RedrawStatus":
			found = selector.Sel.Name
		}
		return true
	})
	return found
}

// handsBack reports a call that moves work to the UI thread.
func handsBack(call *ast.CallExpr) bool {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return fun.Name == "onScreen"
	case *ast.SelectorExpr:
		return fun.Sel.Name == "Do" || fun.Sel.Name == "DoAndWait"
	}
	return false
}
