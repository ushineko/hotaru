package gui_test

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/stretchr/testify/require"
	"github.com/ushineko/fynedesygn/fynetest"
	"github.com/ushineko/fynedesygn/shell"
	"github.com/ushineko/hotaru/internal/api"
	"github.com/ushineko/hotaru/internal/gui"
)

// twoColour is a device whose Breathing takes one or two colours, as OpenRGB
// reported a graphics card's doing on the machines spec 061 measured.
func twoColour() api.Device {
	return api.Device{
		Name: "Test Graphics Card", LEDs: 3, ActiveMode: "Direct", InScope: true,
		Modes:     []string{"Direct", "Breathing"},
		OneColour: []string{"Breathing"},
		Coloured:  map[string]api.ColourSlots{"Breathing": {Least: 1, Most: 2}},
		Zones:     []api.Zone{{Name: "Card", First: 0, Count: 3}},
		Colours:   []string{"#ffffff", "#ffffff", "#ffffff"},
	}
}

func twoColourRoutes() map[string]any {
	routes := healthy()
	routes["GET /"+api.Version+"/devices"] = api.DevicesResponse{Devices: []api.Device{twoColour()}}
	return routes
}

// editing is the editor open on a draft, built in a window.
func editing(t *testing.T, draft *gui.Draft) (*gui.ScenesSection, *shell.Shell, fyne.Window) {
	t.Helper()
	a := test.NewApp()
	t.Cleanup(a.Quit)
	app := gui.New(service(t, twoColourRoutes()))
	opts := app.Options("s")
	opts.SettingsPath = filepath.Join(t.TempDir(), "gui.yml")
	sh := shell.Headless(a, opts)
	app.Refresh(context.Background())

	section := &gui.ScenesSection{}
	gui.OpenEditor(section, app, draft)
	window := test.NewWindow(section.Build(sh))
	t.Cleanup(window.Close)
	window.Resize(fyne.NewSize(1200, 900))
	sh.Window = window
	return section, sh, window
}

func TestAModeThatTakesTwoColoursOffersASecond(t *testing.T) {
	// R4.1: the slot it needs, and a control to add the one it allows.
	draft := gui.NewDraft()
	draft.SetEffect("Test Graphics Card", "Breathing")
	draft.SetEffectColours("Test Graphics Card", []string{"#ff0000"})
	section, sh, _ := editing(t, draft)

	built := section.Build(sh)
	said := fynetest.Text(built)
	require.Contains(t, said, "Breathing takes up to 2 colours.")
	require.Contains(t, said, "#ff0000")
	require.NotNil(t, buttonSaying(built, "Add a colour"), "nothing adds the second colour")
	require.NotContains(t, said, "Card · 3", "the zone is still offered for a mode that cannot show it")
}

func TestASecondColourCanBeRemovedAndTheFirstCleared(t *testing.T) {
	draft := gui.NewDraft()
	draft.SetEffect("Test Graphics Card", "Breathing")
	draft.SetEffectColours("Test Graphics Card", []string{"#ff0000", "#0000ff"})
	section, sh, _ := editing(t, draft)

	built := section.Build(sh)
	require.Nil(t, buttonSaying(built, "Add a colour"), "a third colour was offered to a mode that takes two")
	remove := buttonSaying(built, "Remove")
	require.NotNil(t, remove, "the colour past the one the mode needs cannot be removed")
	test.Tap(remove)
	require.Equal(t, []string{"#ff0000"}, draft.Scene("evening").Effects["Test Graphics Card"].Palette())

	clearing := buttonSaying(section.Build(sh), "Clear")
	require.NotNil(t, clearing)
	test.Tap(clearing)
	require.Empty(t, draft.Scene("evening").Effects["Test Graphics Card"].Palette())
}

func TestMakeASceneShowsThePicturesColoursBeforeMakingIt(t *testing.T) {
	// R5.3: chosen in the dialog, shown under the effect, and sent with the
	// scene marked as picked so a recolour picks them again.
	a := test.NewApp()
	t.Cleanup(a.Quit)
	sh := shell.Headless(a, shell.Options{SettingsPath: filepath.Join(t.TempDir(), "gui.yml")})
	window := test.NewWindow(widget.NewLabel(""))
	t.Cleanup(window.Close)
	window.Resize(fyne.NewSize(1200, 900))
	sh.Window = window

	var mu sync.Mutex
	var asked []int
	var made map[string]api.Effect
	gui.MakeScene(sh, "sunset", []api.Device{twoColour()},
		func(_ context.Context, name string, _ float64, effects map[string]api.Effect) (api.Scene, error) {
			mu.Lock()
			defer mu.Unlock()
			made = effects
			return api.Scene{Name: name}, nil
		},
		func(_ context.Context, count int, _ float64) ([]string, error) {
			mu.Lock()
			defer mu.Unlock()
			asked = append(asked, count)
			return []string{"#ff0000", "#0000ff"}[:count], nil
		})

	dialog := window.Canvas().Overlays().Top()
	require.NotNil(t, dialog, "the dialog did not open")
	var choose *widget.Select
	fynetest.WalkRendered(dialog, func(o fyne.CanvasObject) bool {
		if s, ok := o.(*widget.Select); ok {
			choose = s
			return true
		}
		return false
	})
	require.NotNil(t, choose, "the dialog offers no effect")
	choose.SetSelected("Breathing")

	require.Eventually(t, func() bool {
		return len(fynetest.Text(window.Canvas().Overlays().Top())) > 0 &&
			containsAll(fynetest.Text(window.Canvas().Overlays().Top()), "#ff0000", "#0000ff", "picked from the picture")
	}, soon, every, "the picked colours were not shown")
	mu.Lock()
	require.Equal(t, []int{2}, asked, "the dialog did not ask for as many colours as the mode takes")
	mu.Unlock()

	makeIt := buttonSaying(window.Canvas().Overlays().Top(), "Make it")
	require.NotNil(t, makeIt)
	test.Tap(makeIt)
	require.Eventually(t, func() bool { mu.Lock(); defer mu.Unlock(); return made != nil }, soon, every)
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, []string{"#ff0000", "#0000ff"}, made["Test Graphics Card"].Palette())
	require.Equal(t, api.ColoursFromPicture, made["Test Graphics Card"].ColoursFrom)
}

const (
	soon  = 2 * time.Second
	every = 5 * time.Millisecond
)

func containsAll(text string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(text, part) {
			return false
		}
	}
	return true
}
