package images_test

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/color/palette"
	"image/gif"
	"image/jpeg"
	"image/png"
	"math/rand/v2"
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

func TestASlideshowIsOneAnimation(t *testing.T) {
	/*
		A stack of wallpapers dropped on the program is a question, and this
		is the second answer: each picture held, crossfaded into the next, and
		the last fading back to the first so the loop has no jump in it.
	*/
	sources := [][]byte{
		wallpaper(t, 800, 600),
		wallpaper(t, 1024, 768),
		wallpaper(t, 640, 640),
	}

	stored, err := library(t).AddSlideshow("reel", sources)
	require.NoError(t, err)
	require.True(t, stored.Moves())
	require.Greater(t, stored.Frames, len(sources),
		"a slideshow with no crossfade is a slide sorter")

	body, err := os.ReadFile(stored.Path)
	require.NoError(t, err)
	decoded, err := gif.DecodeAll(bytes.NewReader(body))
	require.NoError(t, err)
	require.Equal(t, images.Panel, decoded.Config.Width)
}

func TestAHeldPictureIsOneFrameNotMany(t *testing.T) {
	/*
		The monitor's finding, carried over: a held photograph is one frame
		carrying the whole hold, and only the fade steps carry the fade's
		short delay. Giving every frame the same duration means a smoother
		fade can only be bought by making it slower.
	*/
	converted, _, err := images.Slideshow([][]byte{
		wallpaper(t, 400, 400), wallpaper(t, 400, 400),
	})
	require.NoError(t, err)

	decoded, err := gif.DecodeAll(bytes.NewReader(converted))
	require.NoError(t, err)

	var holds int
	for _, delay := range decoded.Delay {
		if delay >= images.Hold {
			holds++
		}
	}
	require.Equal(t, 2, holds, "the two photographs are two held frames")
}

func TestASlideshowSharesOnePalette(t *testing.T) {
	/*
		The other half of the same finding. Frames that share a colour table
		delta-encode against each other and frames that do not are each a
		fresh image -- which is what made the monitor's crossfades five steps
		long instead of twenty-four.
	*/
	converted, _, err := images.Slideshow([][]byte{
		wallpaper(t, 500, 500), wallpaper(t, 500, 500), wallpaper(t, 500, 500),
	})
	require.NoError(t, err)

	decoded, err := gif.DecodeAll(bytes.NewReader(converted))
	require.NoError(t, err)

	first := decoded.Image[0].Palette
	for i, frame := range decoded.Image {
		require.Equal(t, len(first), len(frame.Palette),
			"frame %d carries a palette of its own", i)
	}
}

func TestOnePictureIsNotASlideshow(t *testing.T) {
	// Asking for a reel of one is asking for a picture, and answering with a
	// crossfade from a photograph to itself would be a program being clever.
	converted, frames, err := images.Slideshow([][]byte{wallpaper(t, 300, 300)})
	require.NoError(t, err)
	require.Equal(t, 1, frames)
	require.NotEmpty(t, converted)
}

func TestAReelTooSmoothToFitLosesSmoothnessNotPictures(t *testing.T) {
	/*
		The panel holds about 24 MB and says nothing when handed more: spec
		013's finding, which is why there is a budget here at all. What gives
		when a reel overruns is the fade, because a slideshow missing a
		photograph is not the slideshow somebody asked for.
	*/
	var sources [][]byte
	for i := range 8 {
		sources = append(sources, noise(t, i))
	}

	converted, _, err := images.Slideshow(sources)
	require.NoError(t, err)
	require.LessOrEqual(t, len(converted), images.Budget, "the reel will not fit the panel")

	decoded, err := gif.DecodeAll(bytes.NewReader(converted))
	require.NoError(t, err)

	var holds int
	for _, delay := range decoded.Delay {
		if delay >= images.Hold {
			holds++
		}
	}
	require.Equal(t, len(sources), holds, "a picture was dropped to make the reel fit")
	require.Less(t, len(decoded.Image), len(sources)*(images.MostSteps+1),
		"the fade was never shortened, so nothing about fitting was tested")
}

// noise is a photograph that will not compress: every pixel unrelated to its
// neighbours, which is the worst case a reel can be handed and the only way to
// make one overrun the panel on purpose.
func noise(t *testing.T, seed int) []byte {
	t.Helper()

	random := rand.New(rand.NewPCG(uint64(seed), 7)) //nolint:gosec // a fixture, not a secret
	picture := image.NewRGBA(image.Rect(0, 0, 700, 700))
	for y := range 700 {
		for x := range 700 {
			picture.Set(x, y, color.RGBA{
				R: uint8(random.UintN(256)), G: uint8(random.UintN(256)),
				B: uint8(random.UintN(256)), A: 255,
			})
		}
	}
	var out bytes.Buffer
	require.NoError(t, png.Encode(&out, picture))
	return out.Bytes()
}

func TestCountingFramesAgreesWithDecodingThem(t *testing.T) {
	/*
		The count is read off the file's blocks rather than decoded, because
		decoding is what made listing the library cost a second and a third
		of it: `gif.DecodeAll` undoes the LZW compression of every frame to
		tell you how many there are, and the window asks for that list
		whenever the section is drawn.

		So the cheap count has to agree with the expensive one. This is the
		test that says it does.
	*/
	dir := t.TempDir()
	library, err := images.Open(dir)
	require.NoError(t, err)

	want := map[string]int{}
	for _, count := range []int{1, 2, 17} {
		name := fmt.Sprintf("animation%d", count)
		kept, err := library.Add(name, moving(t, count))
		require.NoError(t, err)

		// What the format actually holds, which is what the listing must say.
		body, err := os.ReadFile(kept.Path)
		require.NoError(t, err)
		decoded, err := gif.DecodeAll(bytes.NewReader(body))
		require.NoError(t, err)
		want[name] = len(decoded.Image)
	}

	// And a file that is not a GIF at all, which must not fail the listing.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "broken.gif"),
		[]byte("not a picture"), 0o600))
	want["broken"] = 1

	listed, err := library.All()
	require.NoError(t, err)
	require.Len(t, listed, len(want))
	for _, one := range listed {
		require.Equal(t, want[one.Name], one.Frames, "%s", one.Name)
	}
}

// moving is a GIF of a given number of frames, as a source to convert.
func moving(t *testing.T, count int) []byte {
	t.Helper()

	animation := &gif.GIF{}
	for i := range count {
		frame := image.NewPaletted(image.Rect(0, 0, 32, 32), palette.Plan9)
		frame.Set(i%32, 0, color.RGBA{R: 255, A: 255})
		animation.Image = append(animation.Image, frame)
		animation.Delay = append(animation.Delay, 10)
	}
	var out bytes.Buffer
	require.NoError(t, gif.EncodeAll(&out, animation))
	return out.Bytes()
}
