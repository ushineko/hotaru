package gui

import (
	"image"
	"image/color"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/stretchr/testify/require"
	"github.com/ushineko/fynedesygn/imagecache"
	"github.com/ushineko/hotaru/internal/api"
)

/*
What the window remembers about a picture, and what a rename has to forget.

A thumbnail is cached under a key built from the picture's name, so a rename
leaves the old key holding a picture nothing can ask for again. Spec 053, AC10.
*/
func TestRenamingAPictureForgetsItsThumbnail(t *testing.T) {
	was := api.Image{
		Name: "wallpaper", Path: "/nowhere/wallpaper.gif",
		Bytes: 1024, Added: time.Unix(0, 0).UTC(),
	}
	drawn := image.NewRGBA(image.Rect(0, 0, 1, 1))
	drawn.Set(0, 0, color.RGBA{R: 255, A: 255})

	held, err := imagecache.Shared.Get(thumbKey(was),
		func() (image.Image, error) { return drawn, nil })
	require.NoError(t, err)
	require.NotNil(t, held, "the cache is holding the thumbnail")

	forgetThumbnail(was)

	// notYet is how the window asks whether the cache has one without
	// drawing it, which is exactly the question being asked here.
	_, err = imagecache.Shared.Get(thumbKey(was), notYet)
	require.ErrorIs(t, err, errNoShotYet, "and now it is not")
}

/*
The key changes with the name, which is the reason forgetting is needed at
all: a rename cannot re-use the entry, it can only leave it behind.
*/
func TestAThumbnailKeyIsPerName(t *testing.T) {
	was := api.Image{Name: "wallpaper", Bytes: 1024, Added: time.Unix(0, 0).UTC()}
	now := was
	now.Name = "aurora"
	require.NotEqual(t, thumbKey(was), thumbKey(now))
}

/*
A picture's tile is wide enough for the buttons on it.

They are centred in a tile of a fixed width, so a row of icons wider than the
tile hangs over both edges -- and the ones at the ends are "make a scene from
it" and "remove it". Spec 053 put a fourth icon there, which is what makes
this worth asserting rather than looking at.
*/
func TestAPictureTileHoldsItsOwnButtons(t *testing.T) {
	_ = test.NewApp()

	icons := make([]fyne.CanvasObject, 0, 4)
	for range 4 {
		button := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {})
		button.Importance = widget.LowImportance
		icons = append(icons, button)
	}

	wide := container.NewHBox(icons...).MinSize().Width
	t.Logf("the buttons need %.0f of the tile's %d", wide, int(tileWide))
	require.LessOrEqual(t, wide, float32(tileWide),
		"the buttons on a picture's tile are wider than the tile")
}

/*
Taking the name a copy offers is an answer, not a no-op.

The bug this is here for: the dialog was given one name for both what to put
in the field and what means "nothing was asked for". A rename is the case
where those are the same name, so it worked; a copy fills the field with a
suggestion, and accepting the suggestion looked like leaving the name alone.
The dialog closed, nothing was copied, and nothing said so.
*/
func TestTakingTheNameACopyOffersIsAnAnswer(t *testing.T) {
	copying := naming{
		Title: "Copy planet1", Confirm: "Copy it",
		Filled: "planet1-copy", Unchanged: "planet1",
	}
	require.Equal(t, "planet1-copy", copying.wanted(true, copying.Filled),
		"the offered name is what the copy should be called")

	for _, nothing := range []struct {
		why   string
		ok    bool
		typed string
	}{
		{"cancelled", false, "planet1-copy"},
		{"left empty", true, ""},
		{"spaces alone", true, "   "},
		{"the name it already has", true, "planet1"},
	} {
		require.Empty(t, copying.wanted(nothing.ok, nothing.typed), nothing.why)
	}
}

/*
A rename is the case where the field and the unchanged name agree, which is
what made the bug above invisible: every path through a rename is right.
*/
func TestARenameThatChangesNothingAsksForNothing(t *testing.T) {
	renaming := naming{Filled: "planet1", Unchanged: "planet1"}
	require.Empty(t, renaming.wanted(true, "planet1"), "the same name is no rename")
	require.Equal(t, "pluto", renaming.wanted(true, " pluto "), "and a new one is trimmed")
}
