package service_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/scenes"
	"github.com/ushineko/hotaru/internal/service"
)

/*
A scene becomes lights one way, and an effect cannot be lost on the way.

On a canvas (spec 076) the effect is one hotaru draws, Breathing, because a
preview does not write a device's own effects (spec 071 R5.2).

The bug behind this: an editor changing a draft wrote it through the plain
colour route, which has no effects in it at all, so every keystroke put the
keyboard into Direct while a saved scene put it into the effect. One scene,
two routes, and only one of them knew what an effect was. See spec 054.
*/

// reactive is a scene whose keyboard runs an effect of its own colour, which
// is the case every path here has to carry.
func reactive(name string) scenes.Scene {
	return scenes.Scene{
		Name:        name,
		Assignments: []scenes.Assignment{{Target: "Keychron", Colour: "blue"}},
		Effects: map[string]scenes.Effect{
			"Keychron": {Mode: "Breathing", Colour: "#00ff00"},
		},
	}
}

// effectOn is the mode a device is drawn in and that mode's colour, as the
// service lists them.
func effectOn(t *testing.T, svc *service.Service, device string) (string, string) {
	t.Helper()
	view := viewOf(t, svc, device).Device
	mode, ok := view.Mode(view.ActiveMode)
	require.True(t, ok, "%s lists no mode %q", device, view.ActiveMode)
	return view.ActiveMode, mode.Colour.String()
}

func TestChangingADraftKeepsItsEffect(t *testing.T) {
	/*
		The whole bug in one test. The draft goes up with the effect, the
		editor changes something, and the keyboard is still running the
		effect in the effect's own colour -- rather than sitting in Direct
		because the second write went a different way.
	*/
	svc, _, _ := litDesk(t)

	up, err := svc.Preview(context.Background(), reactive(""), "editor", false)
	require.NoError(t, err)
	require.NotNil(t, up.Lease)

	changed := reactive("")
	changed.Assignments = []scenes.Assignment{{Target: "Keychron", Colour: "red"}}
	_, err = svc.Redraft(context.Background(), up.Lease.Token, changed)
	require.NoError(t, err)

	mode, tint := effectOn(t, svc, "Keychron K4 HE")
	require.Equal(t, "Breathing", mode, "changing a draft put the keyboard somewhere else")
	require.Equal(t, "#00ff00", tint, "the effect's colour went with it")
}

func TestADraftAndASavedSceneWriteTheSameThing(t *testing.T) {
	/*
		The rule, asserted rather than intended: a scene is a scene, and what
		reaches the hardware cannot depend on whether it arrived from an
		editor or from a list. Written as a comparison because that is the
		property -- the two paths agreeing -- rather than a repeat of the
		modes one of them happens to write today.
	*/
	applied, _, _ := litDesk(t, reactive("evening"))
	_, err := applied.ApplyScene(context.Background(), "evening")
	require.NoError(t, err)

	drafted, _, _ := litDesk(t)
	_, err = drafted.Preview(context.Background(), reactive(""), "editor", false)
	require.NoError(t, err)

	wantMode, wantColour := effectOn(t, applied, "Keychron K4 HE")
	gotMode, gotColour := effectOn(t, drafted, "Keychron K4 HE")
	require.Equal(t, []string{wantMode, wantColour}, []string{gotMode, gotColour},
		"an edited draft and a saved scene reach the keyboard differently")
}

func TestADraftThatGrowsADeviceTakesItIntoTheLease(t *testing.T) {
	// An editor that adds a device to a scene means that device, and a lease
	// that did not follow would leave the reconciler free to correct
	// something the window is showing somebody.
	svc, _, _ := litDesk(t)

	one := scenes.Scene{Assignments: []scenes.Assignment{{Target: "Keychron", Colour: "blue"}}}
	up, err := svc.Preview(context.Background(), one, "editor", false)
	require.NoError(t, err)

	both := scenes.Scene{Assignments: []scenes.Assignment{
		{Target: "Keychron", Colour: "blue"},
		{Target: "ASUS", Colour: "red"},
	}}
	out, err := svc.Redraft(context.Background(), up.Lease.Token, both)
	require.NoError(t, err)
	require.Contains(t, out.Lease.Devices, "ASUS ROG MAXIMUS Z790 HERO")
	require.Equal(t, up.Lease.Token, out.Lease.Token, "and it is the same lease")
}

func TestRedraftingWithoutALeaseIsRefused(t *testing.T) {
	// A token nobody holds is an editor whose preview lapsed, and writing
	// the draft anyway would put colours on hardware nothing is holding.
	svc, _, _ := litDesk(t)
	_, err := svc.Redraft(context.Background(), "nosuchtoken", reactive(""))
	require.Error(t, err)
}

func TestADraftCannotTakeADeviceSomebodyElseIsShowing(t *testing.T) {
	// One device, one preview -- whether the second claim arrives as a new
	// lease or as a draft that grew into one.
	svc, _, _ := litDesk(t)

	mine, err := svc.Preview(context.Background(),
		scenes.Scene{Assignments: []scenes.Assignment{{Target: "Keychron", Colour: "blue"}}},
		"editor", false)
	require.NoError(t, err)
	_, err = svc.Preview(context.Background(),
		scenes.Scene{Assignments: []scenes.Assignment{{Target: "ASUS", Colour: "red"}}},
		"somebody else", false)
	require.NoError(t, err)

	grown := scenes.Scene{Assignments: []scenes.Assignment{
		{Target: "Keychron", Colour: "blue"},
		{Target: "ASUS", Colour: "green"},
	}}
	_, err = svc.Redraft(context.Background(), mine.Lease.Token, grown)
	require.ErrorContains(t, err, "somebody else")
}
