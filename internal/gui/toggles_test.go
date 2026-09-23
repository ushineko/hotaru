package gui_test

import (
	"context"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/require"
	"github.com/ushineko/fynedesygn/fynetest"
	"github.com/ushineko/fynedesygn/shell"
	"github.com/ushineko/hotaru/internal/api"
	"github.com/ushineko/hotaru/internal/gui"
)

// keyboardRoutes is a hundred-key board in one zone, with two of its keys
// named and marked as switches -- the shape this was built for.
func keyboardRoutes(t *testing.T, toggles []string) map[string]any {
	t.Helper()
	routes := healthy()
	routes["GET /"+api.Version+"/devices"] = api.DevicesResponse{Devices: []api.Device{{
		Name: "Keychron K4 HE", LEDs: 100, InScope: true, ActiveMode: "Direct",
		Modes:    []string{"Direct"},
		Zones:    []api.Zone{{Name: "Keyboard", First: 0, Count: 100}},
		Segments: []string{"caps", "keyboard", "num"},
		Toggles:  toggles,
	}}}
	return routes
}

func editorFor(t *testing.T, routes map[string]any) (*gui.ScenesSection, fyne.CanvasObject) {
	t.Helper()
	a := test.NewApp()
	t.Cleanup(a.Quit)

	app := gui.New(service(t, routes))
	opts := app.Options("s")
	opts.SettingsPath = filepath.Join(t.TempDir(), "gui.yml")
	sh := shell.Headless(a, opts)
	app.Refresh(context.Background())

	section := &gui.ScenesSection{}
	gui.OpenEditor(section, app, gui.NewDraft())
	built := section.Build(sh)

	window := test.NewWindow(built)
	t.Cleanup(window.Close)
	window.Resize(fyne.NewSize(1180, 760))
	return section, built
}

func TestTheEditorOffersTheTogglesTheRulesNamed(t *testing.T) {
	/*
		Why they need a control of their own: a hundred-key zone is drawn as
		twenty-four blocks of four or five keys, so Caps Lock is a fifth of a
		block somebody would be guessing at. Named in the rules file, it is
		one click.
	*/
	_, built := editorFor(t, keyboardRoutes(t, []string{"caps", "num"}))

	said := fynetest.Text(built)
	require.Contains(t, said, "Toggles", "the editor does not say it has any")
	require.Contains(t, said, "caps")
	require.Contains(t, said, "num")
}

func TestADeviceWithNoTogglesGetsNoRow(t *testing.T) {
	// Which is every device until somebody's rules say otherwise: the row
	// must not appear as an empty heading on a board that has none.
	_, built := editorFor(t, keyboardRoutes(t, nil))
	require.NotContains(t, fynetest.Text(built), "Toggles")
}

func TestChoosingAToggleSetsItsOwnTargetAndNotARange(t *testing.T) {
	/*
		The scene must say `Keychron K4 HE/caps`, not the lights behind it.
		A range would have to be found again and re-typed the day somebody
		corrects which LED Caps Lock is; a name goes on meaning the key.
	*/
	section, _ := editorFor(t, keyboardRoutes(t, []string{"caps", "num"}))

	// What the editor does when somebody clicks the toggle and picks a
	// colour: select that spot, then set.
	gui.Choose(section, gui.NamedPart("Keychron K4 HE", "caps"))
	gui.SetColour(section, "#ff0000")

	require.Equal(t, "#ff0000", gui.DraftColour(section, "Keychron K4 HE/caps"),
		"the toggle was recorded as something other than its name")
}
