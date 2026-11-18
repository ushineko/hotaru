package images_test

import (
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/images"
)

// distinct counts the colours in a picture.
func distinct(img image.Image) int {
	seen := map[[3]uint32]bool{}
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			seen[[3]uint32{r, g, bl}] = true
		}
	}
	return len(seen)
}

/*
A still picture is kept in full colour beside its GIF (spec 080), and Still
reads it: the GIF holds the picture to 256 colours, and the original file is
not kept anywhere.
*/
func TestAStillPictureIsKeptInFullColour(t *testing.T) {
	l := library(t)
	_, err := l.Add("wallpaper", wallpaper(t, 1280, 720))
	require.NoError(t, err)

	still, err := l.Still("wallpaper")
	require.NoError(t, err)
	require.Equal(t, image.Rect(0, 0, images.Panel, images.Panel), still.Bounds())
	require.Greater(t, distinct(still), 256, "the still is the GIF, not the full-colour copy")

	all, err := l.All()
	require.NoError(t, err)
	require.Len(t, all, 1, "the copy is listed as a picture of its own")
}

// An animation is a GIF on every panel, so it has no copy, and Still is its
// first frame. A still replaced by an animation loses its old copy.
func TestAnAnimationHasNoFullColourCopy(t *testing.T) {
	l := library(t)
	_, err := l.Add("art", wallpaper(t, 800, 800))
	require.NoError(t, err)
	_, err = l.Add("art", moving(t, 3))
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(l.Dir(), "art.png"))
	require.True(t, os.IsNotExist(err), "a still's copy outlived the animation that replaced it")
	still, err := l.Still("art")
	require.NoError(t, err)
	require.LessOrEqual(t, distinct(still), 256)
}

// The copy goes where the picture goes: renamed with it, removed with it.
func TestTheCopyFollowsRenameAndRemove(t *testing.T) {
	l := library(t)
	_, err := l.Add("old", wallpaper(t, 800, 800))
	require.NoError(t, err)

	_, err = l.Rename("old", "new")
	require.NoError(t, err)
	still, err := l.Still("new")
	require.NoError(t, err)
	require.Greater(t, distinct(still), 256, "the copy did not move with the rename")

	require.NoError(t, l.Remove("new"))
	entries, err := os.ReadDir(l.Dir())
	require.NoError(t, err)
	for _, e := range entries {
		require.False(t, strings.HasPrefix(e.Name(), "new."), "%s outlived its picture", e.Name())
	}
}

// A picture added before spec 080 has only its GIF, and Still reads that.
func TestAPictureWithNoCopyIsReadFromItsGIF(t *testing.T) {
	l := library(t)
	_, err := l.Add("older", wallpaper(t, 800, 800))
	require.NoError(t, err)
	require.NoError(t, os.Remove(filepath.Join(l.Dir(), "older.png")))

	still, err := l.Still("older")
	require.NoError(t, err)
	require.Equal(t, image.Rect(0, 0, images.Panel, images.Panel), still.Bounds())
	require.LessOrEqual(t, distinct(still), 256)
}
