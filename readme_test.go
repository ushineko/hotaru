package hotaru_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/fynedesygn/mermaid"
	"github.com/ushineko/hotaru"
)

/*
The window shows the README, so the README's diagrams are part of the program.

A mermaid fence with no rendered image does not fail a build and does not fail
a launch: it draws as its own source with a caption saying it was not
rendered. This is the check that catches an edited diagram before somebody
sees the apology in the About pane.
*/
func TestEveryDiagramInTheRepositoryIsRendered(t *testing.T) {
	missing, stale, err := mermaid.Check(".", "diagrams")
	require.NoError(t, err)

	for _, m := range missing {
		t.Errorf("%s:%d has no image; run make generate", m.File, m.Line)
	}
	for _, s := range stale {
		t.Errorf("%s has no source; run make generate -- prune", s)
	}
}

func TestTheEmbeddedReadmeIsTheReadme(t *testing.T) {
	// Embedded rather than restated: a program that describes itself twice
	// keeps one description and forgets the other.
	require.Contains(t, hotaru.README, "# hotaru")
	require.Contains(t, hotaru.README, "```mermaid")
}
