package service_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/openrgb"
	"github.com/ushineko/hotaru/internal/scenes"
)

/*
A scene becomes lights one way, and an effect cannot be lost on the way.

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
			"Keychron": {Mode: "Solid Reactive", Colour: "#00ff00"},
		},
	}
}

// modesFor is every mode written to a device, in order.
func modesFor(server *openrgb.Fake, device string) []openrgb.ModeWrite {
	var out []openrgb.ModeWrite
	for _, written := range server.Modes {
		if written.Device == device {
			out = append(out, written)
		}
	}
	return out
}

func TestChangingADraftKeepsItsEffect(t *testing.T) {
	/*
		The whole bug in one test. The draft goes up with the effect, the
		editor changes something, and the keyboard is still running the
		effect in the effect's own colour -- rather than sitting in Direct
		because the second write went a different way.
	*/
	svc, server := lit(t)

	up, err := svc.Preview(context.Background(), reactive(""), "editor", false)
	require.NoError(t, err)
	require.NotNil(t, up.Lease)

	server.Modes = nil // what the change writes, and nothing before it

	changed := reactive("")
	changed.Assignments = []scenes.Assignment{{Target: "Keychron", Colour: "red"}}
	_, err = svc.Redraft(context.Background(), up.Lease.Token, changed)
	require.NoError(t, err)

	written := modesFor(server, "Keychron K4 HE")
	require.NotEmpty(t, written, "the change wrote no mode at all")
	for _, one := range written {
		require.Equal(t, "Solid Reactive", one.Mode,
			"changing a draft put the keyboard somewhere else")
		require.NotNil(t, one.Colour, "the effect's colour went with it")
		require.Equal(t, "#00ff00", one.Colour.String())
	}
}

func TestADraftAndASavedSceneWriteTheSameThing(t *testing.T) {
	/*
		The rule, asserted rather than intended: a scene is a scene, and what
		reaches the hardware cannot depend on whether it arrived from an
		editor or from a list. Written as a comparison because that is the
		property -- the two paths agreeing -- rather than a repeat of the
		modes one of them happens to write today.
	*/
	applied, appliedServer := lit(t, reactive("evening"))
	_, err := applied.ApplyScene(context.Background(), "evening")
	require.NoError(t, err)

	/*
		Both from the same starting point: the device is in a per-LED mode
		and arriving at the effect. A redraft onto a device already showing
		the effect writes one packet rather than two (spec 057), so comparing
		a first apply against a second draft would be comparing two different
		questions.
	*/
	drafted, draftServer := lit(t)
	_, err = drafted.Preview(context.Background(), reactive(""), "editor", false)
	require.NoError(t, err)

	want := modesFor(appliedServer, "Keychron K4 HE")
	got := modesFor(draftServer, "Keychron K4 HE")
	require.Equal(t, want, got,
		"an edited draft and a saved scene reach the keyboard differently")
}

func TestADraftThatGrowsADeviceTakesItIntoTheLease(t *testing.T) {
	// An editor that adds a device to a scene means that device, and a lease
	// that did not follow would leave the reconciler free to correct
	// something the window is showing somebody.
	svc, _ := lit(t)

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
	svc, _ := lit(t)
	_, err := svc.Redraft(context.Background(), "nosuchtoken", reactive(""))
	require.Error(t, err)
}

func TestADraftCannotTakeADeviceSomebodyElseIsShowing(t *testing.T) {
	// One device, one preview -- whether the second claim arrives as a new
	// lease or as a draft that grew into one.
	svc, _ := lit(t)

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
