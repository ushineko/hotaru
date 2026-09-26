package gui_test

import (
	"context"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/require"
	"github.com/ushineko/fynedesygn/shell"
	"github.com/ushineko/hotaru/internal/gui"
)

/*
A screen's row fits the window it is drawn in.

The list scrolls up and down and not sideways, so a row wider than the
section is a row with its last button off the edge -- and the last button is
the one that deletes things. Spec 053 put two more on the row, which is what
makes this worth asserting rather than looking at.
*/
func TestAScreenRowFitsTheWindowAtItsDefaultSize(t *testing.T) {
	a := test.NewApp()
	t.Cleanup(a.Quit)

	app := gui.New(service(t, screenful()))
	opts := app.Options("s")
	opts.SettingsPath = filepath.Join(t.TempDir(), "gui.yml")
	sh := shell.Headless(a, opts)
	app.Refresh(context.Background())

	section := &gui.DashboardsSection{}
	gui.OpenDashboards(section, app)
	built := section.Build(sh)

	window := test.NewWindow(built)
	t.Cleanup(window.Close)
	window.Resize(fyne.NewSize(shell.DefaultWidth, shell.DefaultHeight))

	/*
		What the section actually gets: the window less the list of sections
		beside it, which is a fixed fraction of the width.

		Measured against that rather than against the window, because a test
		that built the section on its own and resized it to the whole window
		would pass with the delete button under the nav's shadow.
	*/
	content := shell.DefaultWidth * (1 - float32(shell.NavOffset))
	t.Logf("a row needs %.0f of the %.0f the section is given",
		built.MinSize().Width, content)
	require.LessOrEqual(t, built.MinSize().Width, content,
		"a screen's row does not fit the section it is drawn in")
}
