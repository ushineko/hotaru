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
		Effects:     map[string]string{"keychron": "Solid Splash"},
		Screen:      "dashboard",
	}

	draft := gui.DraftFrom(original)
	draft.Set("kraken", "#400000")
	saved := draft.Scene("evening")

	require.Equal(t, "Solid Splash", saved.Effects["keychron"], "the effect was dropped")
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
	// can click on, and the window rebuilds on every poll.
	draft := gui.NewDraft()
	for _, target := range []string{"mousepad", "kraken/ring", "keychron"} {
		draft.Set(target, "red")
	}
	require.Equal(t, []string{"keychron", "kraken/ring", "mousepad"}, draft.Targets())
	require.Equal(t, draft.Targets(), draft.Targets())
}

func TestAnOffSceneIsNotEmpty(t *testing.T) {
	// It says something -- turn the lights off -- and an editor that called
	// it empty would refuse to save it.
	draft := gui.DraftFrom(api.Scene{Name: "off", Off: true})
	require.False(t, draft.Empty())
	require.True(t, draft.Scene("off").Off)
}
