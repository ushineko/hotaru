package service_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/dashboard"
	"github.com/ushineko/hotaru/internal/images"
	"github.com/ushineko/hotaru/internal/openrgb"
	"github.com/ushineko/hotaru/internal/scenes"
	"github.com/ushineko/hotaru/internal/service"
)

/*
Renaming, and the references a rename carries with it.

Every test here is about the second half. Moving one entry is a map delete and
a map write and would not be worth a spec; what is worth one is that the key
bound to the scene, the scene naming the screen and the screen drawing the
picture all still point at it afterwards. See spec 053.
*/

// renaming is a service with all three stores, which is what a rename needs:
// a picture's name is spelled in the other two.
func renaming(t *testing.T, saved ...scenes.Scene) (
	*service.Service, *scenes.Store, *dashboard.Store, *images.Library,
) {
	t.Helper()
	dir := t.TempDir()
	svc := service.New(nil, openrgb.NewFake(board(), keyboard()), "")

	kept, err := scenes.Open(filepath.Join(dir, "scenes.yml"))
	require.NoError(t, err)
	for _, scene := range saved {
		require.NoError(t, kept.Save(scene))
	}
	svc.SetScenes(kept)

	boards, err := dashboard.Open(filepath.Join(dir, "dashboards.yml"))
	require.NoError(t, err)
	svc.SetDashboards(boards)

	library, err := images.Open(filepath.Join(dir, "images"))
	require.NoError(t, err)
	svc.SetImages(library)

	return svc, kept, boards, library
}

// screenOf is what a saved scene says the panel should show.
func screenOf(t *testing.T, store *scenes.Store, name string) string {
	t.Helper()
	scene, err := store.Get(name)
	require.NoError(t, err)
	require.Equal(t, name, scene.Name)
	return scene.Screen
}

func TestRenamingASceneTakesItsKeysWithIt(t *testing.T) {
	/*
		The failure this prevents is a key that does nothing.

		A binding names a scene, and the script installed into the desktop
		carries the name rather than the key, so a rename that left the
		binding behind would leave the keypress firing a scene that is not
		there -- reported, when it happened by hand, as "the hotkey stopped
		working".
	*/
	svc, store, _, _ := renaming(t, scenes.Scene{Name: "evening", Colour: "blue"})
	require.NoError(t, svc.Bind("Ctrl+Alt+Shift+Num+1", "evening"))

	done, err := svc.RenameScene("evening", "dusk")
	require.NoError(t, err)
	require.Equal(t, 1, done.Keys)

	_, err = store.Get("evening")
	require.Error(t, err, "the old name is gone")
	moved, err := store.Get("dusk")
	require.NoError(t, err)
	require.Equal(t, "blue", moved.Colour, "and it took what it was with it")

	require.Equal(t, "dusk", store.Bindings()["Ctrl+Alt+Shift+Num+1"])
}

func TestRenamingAScreenTakesTheScenesThatNameItWithIt(t *testing.T) {
	// Two references, and neither is visible from the dashboards file: a
	// scene spells the screen it wants, and the panel remembers which one it
	// is drawing.
	svc, store, boards, _ := renaming(t,
		scenes.Scene{Name: "evening", Screen: "dashboard:mine"},
		scenes.Scene{Name: "morning", Screen: "dashboard:mine"},
		scenes.Scene{Name: "midday", Screen: "dashboard:coolant"},
	)
	require.NoError(t, boards.Save(dashboard.Dashboard{Name: "mine", Theme: "midnight"}))
	require.NoError(t, boards.Use("mine"))

	done, err := svc.RenameDashboard("mine", "ours")
	require.NoError(t, err)
	require.Equal(t, 2, done.Scenes)
	require.True(t, done.Active, "it was the one on the panel")

	require.Equal(t, "dashboard:ours", screenOf(t, store, "evening"))
	require.Equal(t, "dashboard:ours", screenOf(t, store, "morning"))
	require.Equal(t, "dashboard:coolant", screenOf(t, store, "midday"),
		"a scene naming another screen is left alone")

	require.Equal(t, "ours", boards.Active().Name, "and the panel followed")
	_, err = boards.Get("mine")
	require.Error(t, err)
}

func TestRenamingAPictureTakesItsScenesAndBackgroundsWithIt(t *testing.T) {
	/*
		Two different references to one file. A scene keeps the path, because
		it is what the panel is handed; a screen keeps the name, because a
		background is chosen from the library. Both have to move, or the
		rename shows up later as a scene with no picture and a screen with a
		plain colour where a photograph was.
	*/
	svc, store, boards, library := renaming(t)
	stored, err := library.Add("wallpaper", encoded(t))
	require.NoError(t, err)

	require.NoError(t, store.Save(scenes.Scene{Name: "evening", Screen: stored.Path}))
	require.NoError(t, boards.Save(dashboard.Dashboard{
		Name:       "mine",
		Background: dashboard.Background{Kind: dashboard.Picture, Picture: "wallpaper"},
	}))

	done, err := svc.RenameImage("wallpaper", "aurora")
	require.NoError(t, err)
	require.Equal(t, 1, done.Scenes)
	require.Equal(t, 1, done.Screens)

	require.Equal(t, library.Path("aurora"), screenOf(t, store, "evening"))
	moved, err := boards.Get("mine")
	require.NoError(t, err)
	require.Equal(t, "aurora", moved.Background.Picture)

	// And the file itself moved rather than being copied.
	_, err = library.Read("aurora")
	require.NoError(t, err)
	_, err = library.Read("wallpaper")
	require.Error(t, err)
}

func TestARenameSaysWhatElseItChanged(t *testing.T) {
	// The counts are the whole report: a caller who is not told how many
	// references were rewritten has been asked to take it on trust.
	svc, _, boards, library := renaming(t,
		scenes.Scene{Name: "evening", Screen: "dashboard:mine"},
		scenes.Scene{Name: "morning", Screen: "dashboard:mine"},
	)
	require.NoError(t, boards.Save(dashboard.Dashboard{Name: "mine"}))

	done, err := svc.RenameDashboard("mine", "ours")
	require.NoError(t, err)
	require.Equal(t, "2 scenes updated", done.Changed())

	// And nothing pointing at it says nothing, rather than saying none.
	_, err = library.Add("spare", encoded(t))
	require.NoError(t, err)
	quiet, err := svc.RenameImage("spare", "unused")
	require.NoError(t, err)
	require.Empty(t, quiet.Changed())
}

func TestAShippedThingCannotBeRenamed(t *testing.T) {
	/*
		There is nothing to move. A shipped name is a string in the binary,
		and a file entry renaming one would be an override of a name plus the
		shipped entry coming back on the next release -- which is two scenes
		where somebody asked for one.
	*/
	svc, store, boards, _ := renaming(t)

	_, err := svc.RenameScene("off", "dark")
	require.ErrorIs(t, err, service.ErrShipped)
	_, err = store.Get("off")
	require.NoError(t, err, "and it is still there under its own name")

	_, err = svc.RenameDashboard("coolant", "liquid")
	require.ErrorIs(t, err, service.ErrShipped)
	_, err = boards.Get("coolant")
	require.NoError(t, err)
}

func TestARenameOntoATakenNameChangesNothing(t *testing.T) {
	// Overwriting would be a delete nobody typed.
	svc, store, boards, library := renaming(t,
		scenes.Scene{Name: "evening", Colour: "blue"},
		scenes.Scene{Name: "morning", Colour: "amber"},
	)

	_, err := svc.RenameScene("evening", "morning")
	require.Error(t, err)
	for name, want := range map[string]string{"evening": "blue", "morning": "amber"} {
		kept, err := store.Get(name)
		require.NoError(t, err)
		require.Equal(t, want, kept.Colour, "%s is untouched", name)
	}

	// A shipped name is taken too: saving over one is a different decision
	// from renaming into it.
	_, err = svc.RenameScene("evening", "off")
	require.Error(t, err)

	require.NoError(t, boards.Save(dashboard.Dashboard{Name: "mine"}))
	_, err = svc.RenameDashboard("mine", "coolant")
	require.Error(t, err)

	_, err = library.Add("wallpaper", encoded(t))
	require.NoError(t, err)
	_, err = library.Add("aurora", encoded(t))
	require.NoError(t, err)
	_, err = svc.RenameImage("wallpaper", "aurora")
	require.Error(t, err)
	_, err = library.Read("wallpaper")
	require.NoError(t, err, "and the picture it would have replaced is still there")
}

func TestCloningAScreenCopiesItWithoutShowingIt(t *testing.T) {
	// The use case in one test: the same dozen decisions under another name,
	// so the picture behind them is the only thing left to change.
	svc, _, boards, _ := renaming(t)
	require.NoError(t, boards.Save(dashboard.Dashboard{
		Name: "mine", Theme: "midnight", Arrangement: "stacked", Caption: "hello",
		Background: dashboard.Background{Kind: dashboard.Picture, Picture: "wallpaper"},
	}))
	require.NoError(t, boards.Use("mine"))

	made, err := svc.CloneDashboard("mine", "ours")
	require.NoError(t, err)
	require.Equal(t, "ours", made.Name)
	require.Equal(t, "midnight", made.Theme)
	require.Equal(t, "stacked", made.Arrangement)
	require.Equal(t, "hello", made.Caption)
	require.Equal(t, "wallpaper", made.Background.Picture)

	was, err := boards.Get("mine")
	require.NoError(t, err)
	require.Equal(t, "hello", was.Caption, "the original is left alone")
	require.Equal(t, "mine", boards.Active().Name, "and is still the one on the panel")
}

func TestAClonedShippedScreenIsNotShipped(t *testing.T) {
	// Cloning a shipped screen is the one way to start from one, and what
	// comes out is in somebody's file and behaves like it: it can be edited,
	// renamed and deleted.
	svc, _, boards, _ := renaming(t)

	made, err := svc.CloneDashboard("coolant", "mine")
	require.NoError(t, err)
	require.False(t, made.Shipped)

	stored, err := boards.Get("mine")
	require.NoError(t, err)
	require.False(t, stored.Shipped)

	_, err = svc.RenameDashboard("mine", "ours")
	require.NoError(t, err, "and it can be renamed, which the shipped one cannot")
}

func TestTheAppliedSceneFollowsARename(t *testing.T) {
	// The label is what the window shows as "last applied". Renaming the
	// scene it names would otherwise leave it saying the name of a scene
	// that is not there.
	svc, _, _, _ := renaming(t, scenes.Scene{
		Name:        "evening",
		Assignments: []scenes.Assignment{{Target: "ASUS", Colour: "blue"}},
	})
	_, err := svc.ApplyScene(context.Background(), "evening")
	require.NoError(t, err)
	require.Equal(t, "evening", svc.Applied())

	_, err = svc.RenameScene("evening", "dusk")
	require.NoError(t, err)
	require.Equal(t, "dusk", svc.Applied())
}
