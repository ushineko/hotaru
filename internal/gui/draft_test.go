package gui_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/api"
	"github.com/ushineko/hotaru/internal/gui"
)

func TestADraftHoldsColoursUntilItIsAsked(t *testing.T) {
	// The point of staging: a colour picker that wrote as it moved would be
	// sixty writes a second and sixty recorded intentions.
	draft := gui.NewDraft()
	require.True(t, draft.Empty())

	draft.Set("kraken/ring", "red")
	colour, has := draft.Colour("kraken/ring")
	require.True(t, has)
	require.Equal(t, "red", colour)
	require.False(t, draft.Empty())
}

func TestSettingATargetTwiceIsACorrection(t *testing.T) {
	// Not a second assignment: somebody changing their mind about a fan means
	// the fan, not the fan twice.
	draft := gui.NewDraft()
	draft.Set("kraken/ring", "red")
	draft.Set("kraken/ring", "blue")

	require.Len(t, draft.Scene("x").Assignments, 1)
	require.Equal(t, "blue", draft.Scene("x").Assignments[0].Colour)
}

func TestAnEmptyColourTakesTheTargetBackOut(t *testing.T) {
	draft := gui.NewDraft()
	draft.Set("kraken/ring", "red")
	draft.Set("kraken/ring", "")

	require.True(t, draft.Empty())
}

func TestEditingASceneKeepsWhatTheEditorDoesNotEdit(t *testing.T) {
	/*
		The whole risk of an editor that understands part of a format. This
		one edits colours; a scene also carries an effect per device and what
		the screen shows, and saving must not quietly drop either.
	*/
	original := api.Scene{
		Name:        "evening",
		Assignments: []api.SceneAssignment{{Target: "kraken", Colour: "#201040"}},
		Effects:     map[string]api.Effect{"keychron": {Mode: "Solid Splash"}},
		Screen:      "dashboard",
	}

	draft := gui.DraftFrom(original)
	draft.Set("kraken", "#400000")
	saved := draft.Scene("evening")

	require.Equal(t, "Solid Splash", saved.Effects["keychron"].Mode, "the effect was dropped")
	require.Equal(t, "dashboard", saved.Screen, "the screen state was dropped")
	require.Len(t, saved.Assignments, 1)
	require.Equal(t, "#400000", saved.Assignments[0].Colour)
}

func TestEditingASceneOffersItsOwnName(t *testing.T) {
	require.Equal(t, "evening", gui.DraftFrom(api.Scene{Name: "evening"}).From)
	require.Empty(t, gui.NewDraft().From)
}

func TestADraftsTargetsAreStable(t *testing.T) {
	// A listing that reshuffles itself between rebuilds is a listing nobody
	// can click on, and the window rebuilds on every poll. The order is the
	// order they were set in, which is the order the scene applies them.
	draft := gui.NewDraft()
	for _, target := range []string{"mousepad", "kraken/ring", "keychron"} {
		draft.Set(target, "red")
	}
	require.Equal(t, []string{"mousepad", "kraken/ring", "keychron"}, draft.Targets())
	require.Equal(t, draft.Targets(), draft.Targets())
}

// targets is a scene's assignments as "target colour" lines, in order.
func targets(scene api.Scene) []string {
	out := make([]string, 0, len(scene.Assignments))
	for _, a := range scene.Assignments {
		out = append(out, a.Target+" "+a.Colour)
	}
	return out
}

/*
Where two lines overlap, the one set last shows (#183).

The service applies a scene's lines in order. The draft wrote them sorted,
so which colour won depended on the alphabet, not on what somebody did last.
*/
func TestTheNewestColourShowsWhereTwoOverlap(t *testing.T) {
	draft := gui.NewDraft()
	draft.Set("kraken/ring[3]", "blue")
	draft.Set("kraken/ring[0:5]", "red")
	require.Equal(t, []string{"kraken/ring[3] blue", "kraken/ring[0:5] red"}, targets(draft.Scene("x")))

	// Setting a line again is a correction, and it is now the newest.
	draft.Set("kraken/ring[3]", "green")
	require.Equal(t, []string{"kraken/ring[0:5] red", "kraken/ring[3] green"}, targets(draft.Scene("x")))
}

// A scene opened for editing keeps the order it was saved in, which is what
// it means.
func TestAScenesOwnOrderIsKept(t *testing.T) {
	draft := gui.DraftFrom(api.Scene{Name: "x", Assignments: []api.SceneAssignment{
		{Target: "mat/Logo[0]", Colour: "red"},
		{Target: "mat", Colour: "blue"},
	}})
	require.Equal(t, []string{"mat/Logo[0] red", "mat blue"}, targets(draft.Scene("x")))
}

/*
The whole device replaces what was set inside it (#183).

A scene made from a picture gives a mouse mat one line per light. Colouring
the whole mat afterwards was sorted ahead of those lines and painted over by
them, so "the whole device" seemed to do nothing.
*/
func TestTheWholeDeviceReplacesWhatWasSetInsideIt(t *testing.T) {
	draft := gui.DraftFrom(api.Scene{Name: "x", Assignments: []api.SceneAssignment{
		{Target: "Corsair MM700/Left[0]", Colour: "#925841"},
		{Target: "Corsair MM700/Right[0]", Colour: "#925841"},
		{Target: "Corsair MM700/mouse-top", Colour: "#925841"},
		{Target: "Corsair MM7000/Left[0]", Colour: "red"}, // another device, a longer name
		{Target: "G502/Mouse LEDs[5]", Colour: "red"},
	}})

	draft.Set("Corsair MM700", "#0060ff")

	require.Equal(t, []string{
		"Corsair MM7000/Left[0] red",
		"G502/Mouse LEDs[5] red",
		"Corsair MM700 #0060ff",
	}, targets(draft.Scene("x")))
}

// A whole zone replaces its own lights, and nothing else on the device.
func TestAWholeZoneReplacesItsLightsOnly(t *testing.T) {
	draft := gui.NewDraft()
	draft.Set("kraken/ring[0:3]", "red")
	draft.Set("kraken/logo[0]", "red")
	draft.Set("kraken/ringside[0]", "red")

	draft.Set("kraken/ring", "blue")

	require.Equal(t, []string{
		"kraken/logo[0] red", "kraken/ringside[0] red", "kraken/ring blue",
	}, targets(draft.Scene("x")))
}

// The whole scene's colour is the base under the lines, not a line, so it
// keeps every exception: "everything blue except the top fan".
func TestTheWholeScenesColourKeepsItsExceptions(t *testing.T) {
	draft := gui.NewDraft()
	draft.Set("fans/top", "red")
	draft.SetEverything("blue")
	require.Equal(t, []string{"fans/top red"}, targets(draft.Scene("x")))
}

// A cancelled wheel puts back what a whole-device colour took out.
func TestRestoringAClonePutsBackWhatWasReplaced(t *testing.T) {
	draft := gui.NewDraft()
	draft.Set("mat/Left[0]", "red")
	draft.Set("mat/Logo[0]", "green")
	saved := draft.Clone()

	draft.Set("mat", "blue")
	draft.Restore(saved)

	require.Equal(t, []string{"mat/Left[0] red", "mat/Logo[0] green"}, targets(draft.Scene("x")))
}

/*
Each light is drawn in the line that covers it (#183).

The editor looked up a light's own target string, so a colour for the whole
device, the whole zone, or a light written `[5]` left the light drawn in the
hardware's colour -- as if nothing had been set.
*/
func TestEachLightIsInTheNewestLineThatCoversIt(t *testing.T) {
	draft := gui.NewDraft()
	for _, at := range []struct{ target, colour string }{
		{"mat", "device"},
		{"mat/Left", "zone"},
		{"mat/Left[5]", "five"},
		{"mat/Left[1,7:9]", "list"},
	} {
		draft.Set(at.target, at.colour)
	}

	for _, want := range []struct {
		zone   string
		light  int
		colour string
	}{
		{"Left", 0, "zone"},
		{"Left", 5, "five"},
		{"Left", 1, "list"},
		{"Left", 8, "list"},
		{"Logo", 0, "device"},
	} {
		got, has := draft.Light("mat", want.zone, want.light)
		require.True(t, has, "%s %d", want.zone, want.light)
		require.Equal(t, want.colour, got, "%s %d", want.zone, want.light)
	}

	_, has := draft.Light("keyboard", "Keys", 0)
	require.False(t, has, "a device the draft says nothing about took a colour")

	draft.SetEverything("white")
	got, _ := draft.Light("keyboard", "Keys", 0)
	require.Equal(t, "white", got, "the whole scene's colour reaches every light")
}

func TestAnOffSceneIsNotEmpty(t *testing.T) {
	// It says something -- turn the lights off -- and an editor that called
	// it empty would refuse to save it.
	draft := gui.DraftFrom(api.Scene{Name: "off", Off: true})
	require.False(t, draft.Empty())
	require.True(t, draft.Scene("off").Off)
}
