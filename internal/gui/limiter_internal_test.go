package gui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/api"
)

/*
gate is a send the test drives: it says when it has started and waits to be
let go.

Every assertion about this code is an assertion about *ordering* -- what was
in flight when, and what was offered while it was. Sleeping and counting
afterwards tests the machine the test is running on.
*/
type gate struct {
	entered chan string
	release chan struct{}

	mu   sync.Mutex
	in   []time.Time
	out  []time.Time
	seen []string
}

func newGate() *gate {
	return &gate{entered: make(chan string, 8), release: make(chan struct{}, 8)}
}

func (g *gate) send(scene api.Scene) {
	g.mu.Lock()
	g.seen = append(g.seen, scene.Colour)
	g.in = append(g.in, time.Now())
	g.mu.Unlock()

	g.entered <- scene.Colour
	<-g.release

	g.mu.Lock()
	g.out = append(g.out, time.Now())
	g.mu.Unlock()
}

// colours is what reached the send, in order.
func (g *gate) colours() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]string(nil), g.seen...)
}

// started waits for a send to begin, and fails rather than hanging.
func (g *gate) started(t *testing.T) string {
	t.Helper()
	select {
	case colour := <-g.entered:
		return colour
	case <-time.After(2 * time.Second):
		t.Fatal("no send started")
		return ""
	}
}

// quiet fails if a send begins within the grace given.
func (g *gate) quiet(t *testing.T, grace time.Duration) {
	t.Helper()
	select {
	case colour := <-g.entered:
		t.Fatalf("a send began that should not have: %s", colour)
	case <-time.After(grace):
	}
}

func scene(colour string) api.Scene { return api.Scene{Colour: colour} }

func TestOnlyOneColourIsInFlightAndTheLastOneWins(t *testing.T) {
	/*
		The whole point. A drag produces colours far faster than a USB write
		takes, so the ones produced during a write must collapse rather than
		queue: nobody wants the second-to-last colour applied after the last.

		And the last one must survive the collapse. Dropping the intermediate
		steps is the design; dropping the final step is the bug this shape of
		code usually has, and it looks exactly like the picker being wrong.
	*/
	g := newGate()
	l := newLimiter(g.send)
	l.every = 0
	t.Cleanup(l.stop)

	l.offer(scene("#100000"))
	require.Equal(t, "#100000", g.started(t))

	// A pointer moving while the first write is out.
	for _, colour := range []string{"#200000", "#300000", "#400000"} {
		l.offer(scene(colour))
	}
	g.quiet(t, 50*time.Millisecond)

	g.release <- struct{}{}
	require.Equal(t, "#400000", g.started(t), "the last colour offered is not the one that went")
	g.release <- struct{}{}

	l.stop()
	require.Equal(t, []string{"#100000", "#400000"}, g.colours(),
		"three colours offered during one send produced more than one more send")
}

func TestTheGapIsMeasuredFromTheEndOfTheLastSend(t *testing.T) {
	/*
		The bug in the throttle this replaces: it took the time at the moment
		a send *started* and then made the send synchronously, so the gap was
		spent inside the call. Hardware slow enough to need the spacing never
		got any.
	*/
	const every = 60 * time.Millisecond

	g := newGate()
	l := newLimiter(g.send)
	l.every = every
	t.Cleanup(l.stop)

	l.offer(scene("#100000"))
	require.Equal(t, "#100000", g.started(t))

	l.offer(scene("#200000"))
	g.release <- struct{}{} // the first send returns

	require.Equal(t, "#200000", g.started(t))
	g.release <- struct{}{}
	l.stop()

	g.mu.Lock()
	defer g.mu.Unlock()
	require.GreaterOrEqual(t, g.in[1].Sub(g.out[0]), every,
		"the second send began less than a gap after the first one finished")
}

func TestNewLimitersSpaceSendsByLive(t *testing.T) {
	// The floor a limiter is built with, since every test above sets its own.
	require.Equal(t, live, newLimiter(func(api.Scene) {}).every)
}

func TestOneColourOfferedIsOneColourSent(t *testing.T) {
	// Nothing is held back waiting for a drag that is not coming.
	g := newGate()
	l := newLimiter(g.send)
	t.Cleanup(l.stop)

	l.offer(scene("#abcdef"))
	require.Equal(t, "#abcdef", g.started(t))
	g.release <- struct{}{}

	g.quiet(t, 2*live)
	require.Equal(t, []string{"#abcdef"}, g.colours())
}

func TestStoppingWaitsForTheSendInFlight(t *testing.T) {
	/*
		The race that moving off the UI thread creates. The editor releases the
		preview lease as the modal closes, and an apply still in flight would
		land after the release -- the lights left on a preview colour with
		nothing holding it and nothing due to put it back.
	*/
	g := newGate()
	l := newLimiter(g.send)
	l.every = 0

	l.offer(scene("#100000"))
	require.Equal(t, "#100000", g.started(t))
	l.offer(scene("#200000"))

	stopped := make(chan struct{})
	go func() { l.stop(); close(stopped) }()

	select {
	case <-stopped:
		t.Fatal("stop returned while a send was still in flight")
	case <-time.After(50 * time.Millisecond):
	}

	g.release <- struct{}{}
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("stop did not return once the send came back")
	}

	require.Equal(t, []string{"#100000"}, g.colours(),
		"a colour offered before stopping was sent after it")
}

func TestNothingIsSentAfterStopping(t *testing.T) {
	g := newGate()
	l := newLimiter(g.send)
	l.stop()

	l.offer(scene("#ff0000"))
	g.quiet(t, 2*live)
	require.Empty(t, g.colours())

	// And stopping twice is not a panic: the editor stops on dismissal and on
	// every detach after it.
	l.stop()
}

func TestARebuildCannotStopTheSending(t *testing.T) {
	/*
		The shell detaches a section before *every* rebuild, and this window
		rebuilds every two seconds. A sender kept in a field and stopped from
		Detach would therefore be dropped under a pointer that was still
		dragging -- which is the trap Detach's own comment already records
		being walked into once, with the preview.

		So the sender is a local that the modal owns, and the guard is that
		nothing in the section can reach it: Detach has no body to put it in.
		The same shape as the thread guard next door, for the same reason --
		a rule that only lives in a comment is a rule somebody will undo.
	*/
	parsed, err := parser.ParseFile(token.NewFileSet(), "scenes.go", nil, 0)
	require.NoError(t, err)

	for _, declaration := range parsed.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != "Detach" {
			continue
		}
		require.Empty(t, function.Body.List,
			"Detach gained a body; the shell calls it before every rebuild")
		return
	}
	t.Fatal("ScenesSection.Detach is gone; the rebuild trap it documents is not")
}

func TestTheSectionHoldsNoSender(t *testing.T) {
	// The other half: a limiter in a field is a limiter something will stop
	// at the wrong moment.
	parsed, err := parser.ParseFile(token.NewFileSet(), "scenes.go", nil, 0)
	require.NoError(t, err)

	ast.Inspect(parsed, func(node ast.Node) bool {
		spec, ok := node.(*ast.TypeSpec)
		if !ok || spec.Name.Name != "ScenesSection" {
			return true
		}
		structure, ok := spec.Type.(*ast.StructType)
		require.True(t, ok)
		for _, field := range structure.Fields.List {
			pointer, ok := field.Type.(*ast.StarExpr)
			if !ok {
				continue
			}
			name, ok := pointer.X.(*ast.Ident)
			require.False(t, ok && name.Name == "limiter",
				"the section holds a limiter; it belongs to the modal that opened it")
		}
		return false
	})
}
