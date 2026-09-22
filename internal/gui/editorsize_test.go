package gui_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/require"
	"github.com/ushineko/fynedesygn/shell"
	"github.com/ushineko/hotaru/internal/api"
	"github.com/ushineko/hotaru/internal/gui"
)

func TestTheEditorFitsAWindow(t *testing.T) {
	/*
		The editor's controls are fixed -- they do not scroll away, which is
		the point of them -- so anything added to them comes out of the space
		the scene's own lines have. Five devices with a mode chooser and a
		line saying what each is doing now was three hundred pixels of it,
		and on a default window the card fell off the bottom: it was there,
		it was built, and nobody could see it.

		Hence a card that says what is set and a dialog that sets it, like
		the screen chooser beside it.
	*/
	a := test.NewApp()
	t.Cleanup(a.Quit)

	// This desk: six devices, five of them with modes.
	routes := healthy()
	devices := api.DevicesResponse{}
	for _, d := range []struct {
		name  string
		modes int
	}{{"MSI GeForce RTX 4090", 18}, {"NZXT Kraken 2024 ELITE", 14},
		{"ASUS ROG MAXIMUS Z790 HERO", 9}, {"G502 X PLUS", 5},
		{"Corsair MM700", 1}, {"Keychron K4 HE", 23}} {
		modes := make([]string, 0, d.modes)
		for i := range d.modes {
			modes = append(modes, fmt.Sprintf("Mode %d", i))
		}
		devices.Devices = append(devices.Devices, api.Device{
			Name: d.name, LEDs: 4, InScope: true, ActiveMode: "Direct", Modes: modes,
			Zones: []api.Zone{{Name: "Z", First: 0, Count: 4}},
		})
	}
	routes["GET /"+api.Version+"/devices"] = devices

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
	window.Resize(fyne.NewSize(1180, 760)) // the shell's default size

	// The shell's default window, less its header, status bar and the
	// section list: what a section actually gets.
	const room = 640
	require.Less(t, built.MinSize().Height, float32(room),
		"the editor needs %.0f pixels of a %d-pixel window before anything scrolls",
		built.MinSize().Height, room)
}
