package gui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/require"
	"github.com/ushineko/fynedesygn/markdown"
	"github.com/ushineko/fynedesygn/mermaid"
	"github.com/ushineko/hotaru/internal/api"
)

/*
TestTheReadmesDiagramIsDrawnNotApologisedFor is the check nothing else makes.

A mermaid fence whose image is missing from the embedded set renders as its
own source with the caption "Diagram not rendered; run make generate". The
program builds, the window opens, the section draws -- and the architecture
diagram is a wall of text saying somebody forgot a step.
*/
func TestTheReadmesDiagramIsDrawnNotApologisedFor(t *testing.T) {
	test.NewTempApp(t)
	src, opts := document()

	var fences int
	for _, block := range markdown.Blocks(src) {
		lang, _, ok := markdown.CodeBlock(block)
		if !ok || lang != "mermaid" {
			continue
		}
		fences++

		drawn := markdown.RenderBlock(block, opts)
		require.IsType(t, &mermaid.Diagram{}, drawn,
			"the diagram has no embedded image; run make generate")
	}
	require.Positive(t, fences, "the README has no diagram, so this test proves nothing")
}

func TestTheDocumentIsTheReadmeItself(t *testing.T) {
	// Embedded, not restated: a program that describes itself twice keeps one
	// description and forgets the other.
	src, _ := document()
	require.True(t, strings.HasPrefix(src, "# hotaru"))
}

func TestTheReadmeIsNotRebuiltByThePump(t *testing.T) {
	/*
		The window rebuilds the section on screen when something it watches
		moves, and a section that says nothing about what it watches is
		rebuilt whenever anything does -- which on a machine with a running
		pump is every poll, two seconds apart.

		For a document that means a new pane under whoever is reading it,
		which loses their place. The README does not follow the machine, and
		this is it saying so.
	*/
	section := about("/run/nowhere/hotaru.sock")

	watcher, ok := section.(Watcher)
	require.True(t, ok, "the About section does not say what it watches")

	before := Snapshot{}
	after := Snapshot{Cooling: api.Cooling{PumpRPM: 2400}}
	require.False(t, watcher.Changed(before, after))
}
