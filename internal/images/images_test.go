package images_test

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/images"
)

// wallpaper is a wide photograph, which is what somebody actually has.
func wallpaper(t *testing.T, width, height int) []byte {
	t.Helper()

	picture := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			picture.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 128, A: 255})
		}
	}
	var out bytes.Buffer
	require.NoError(t, jpeg.Encode(&out, picture, nil))
	return out.Bytes()
}

func library(t *testing.T) *images.Library {
	t.Helper()
	l, err := images.Open(filepath.Join(t.TempDir(), "images"))
	require.NoError(t, err)
	return l
}

func TestAWallpaperBecomesSomethingThePanelTakes(t *testing.T) {
	/*
		The panel takes 640x640 GIFs and nothing else -- anything else
		transfers successfully and displays nothing at all, which is how it
		says no. A person with a wallpaper they like has no way in without
		this.
	*/
	stored, err := library(t).Add("wallpaper", wallpaper(t, 3840, 2160))
	require.NoError(t, err)
	require.Equal(t, "wallpaper", stored.Name)
	require.Positive(t, stored.Bytes)

	body, err := os.ReadFile(stored.Path)
	require.NoError(t, err)

	decoded, err := gif.DecodeAll(bytes.NewReader(body))
	require.NoError(t, err)
	require.Equal(t, images.Panel, decoded.Config.Width)
	require.Equal(t, images.Panel, decoded.Config.Height)
	require.Len(t, decoded.Image, 1, "a still picture became an animation")
}

func TestAWidePictureIsCroppedRatherThanLetterboxed(t *testing.T) {
	/*
		Bars across somebody's photograph look like a mistake; a centre crop
		looks like a photograph. Checked by the corners: a letterboxed image
		has empty bands top and bottom, and this one does not.
	*/
	stored, err := library(t).Add("wide", wallpaper(t, 1920, 480))
	require.NoError(t, err)

	body, err := os.ReadFile(stored.Path)
	require.NoError(t, err)
	decoded, err := gif.Decode(bytes.NewReader(body))
	require.NoError(t, err)

	top := decoded.At(images.Panel/2, 2)
	bottom := decoded.At(images.Panel/2, images.Panel-3)
	require.NotEqual(t, color.Black, top, "the top of the picture is a bar")
	require.NotEqual(t, color.Black, bottom, "the bottom of the picture is a bar")
}

func TestAPNGConvertsToo(t *testing.T) {
	picture := image.NewRGBA(image.Rect(0, 0, 100, 200))
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, picture))

	_, err := library(t).Add("shot", buf.Bytes())
	require.NoError(t, err)
}

func TestAnAnimationKeepsItsFrames(t *testing.T) {
	// And its timing: an animation that arrives at the wrong speed is a
	// different animation.
	frames := []*image.Paletted{}
	for range 3 {
		frame := image.NewPaletted(image.Rect(0, 0, 200, 120),
			color.Palette{color.Black, color.White})
		frames = append(frames, frame)
	}
	var source bytes.Buffer
	require.NoError(t, gif.EncodeAll(&source, &gif.GIF{
		Image: frames, Delay: []int{5, 5, 5}, LoopCount: 0,
	}))

	stored, err := library(t).Add("moving", source.Bytes())
	require.NoError(t, err)
	require.Equal(t, 3, stored.Frames)
	require.True(t, stored.Moves())

	body, err := os.ReadFile(stored.Path)
	require.NoError(t, err)
	decoded, err := gif.DecodeAll(bytes.NewReader(body))
	require.NoError(t, err)
	require.Equal(t, []int{5, 5, 5}, decoded.Delay, "the animation changed speed")
}

func TestSomethingThatIsNotAnImageIsRefusedByName(t *testing.T) {
	// A directory of holiday photographs contains one thing that is not a
	// photograph, and this message is how anybody finds out which.
	_, err := library(t).Add("notes", []byte("this is a text file"))
	require.ErrorContains(t, err, "not an image")
}

func TestAddingTheSameNameTwiceReplacesIt(t *testing.T) {
	l := library(t)
	first, err := l.Add("wallpaper", wallpaper(t, 800, 800))
	require.NoError(t, err)
	second, err := l.Add("wallpaper", wallpaper(t, 1600, 900))
	require.NoError(t, err)

	require.Equal(t, first.Path, second.Path)
	all, err := l.All()
	require.NoError(t, err)
	require.Len(t, all, 1)
}

func TestANameCannotClimbOutOfTheDirectory(t *testing.T) {
	// Not an escape, a replacement: anything that is not a letter or a number
	// becomes a dash, so a name is safe however it was typed and still reads
	// as itself in a listing.
	l := library(t)
	stored, err := l.Add("../../etc/passwd", wallpaper(t, 100, 100))
	require.NoError(t, err)
	require.Equal(t, filepath.Dir(stored.Path), l.Dir())
	require.Equal(t, "etcpasswd", stored.Name)
}

func TestTheLibraryListsAndForgets(t *testing.T) {
	l := library(t)
	_, err := l.Add("one", wallpaper(t, 400, 400))
	require.NoError(t, err)
	_, err = l.Add("two", wallpaper(t, 400, 400))
	require.NoError(t, err)

	all, err := l.All()
	require.NoError(t, err)
	require.Len(t, all, 2)
	require.Equal(t, "one", all[0].Name)

	require.NoError(t, l.Remove("one"))
	require.NoError(t, l.Remove("never-existed"))

	all, err = l.All()
	require.NoError(t, err)
	require.Len(t, all, 1)
}
