package service_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/images"
	"github.com/ushineko/hotaru/internal/scenes"
	"github.com/ushineko/hotaru/internal/service"
)

// withPictures is a service that can also keep pictures.
func withPictures(t *testing.T, svc *service.Service) {
	t.Helper()
	library, err := images.Open(filepath.Join(t.TempDir(), "images"))
	require.NoError(t, err)
	svc.SetImages(library)
}

// encoded is a picture as somebody would hand one over: a red left half and a
// blue right half, which is the case an average destroys.
func encoded(t *testing.T) []byte {
	t.Helper()
	picture := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := range 64 {
		for x := range 64 {
			c := color.RGBA{R: 255, A: 255}
			if x >= 32 {
				c = color.RGBA{B: 255, A: 255}
			}
			picture.Set(x, y, c)
		}
	}
	var out bytes.Buffer
	require.NoError(t, png.Encode(&out, picture))
	return out.Bytes()
}

func TestASceneFromAPictureCoversEveryDeviceInScope(t *testing.T) {
	svc, _ := lit(t)
	withPictures(t, svc)
	_, err := svc.AddImage("halves", encoded(t))
	require.NoError(t, err)

	scene, err := svc.SceneFromImage(t.Context(), "halves", "themed", 1, nil)
	require.NoError(t, err)

	named := map[string]bool{}
	for _, assignment := range scene.Assignments {
		named[strings.SplitN(assignment.Target, "/", 2)[0]] = true
	}
	require.True(t, named["ASUS ROG MAXIMUS Z790 HERO"])
	require.True(t, named["Keychron K4 HE"])
}

func TestASceneFromAPictureSweepsAcrossIt(t *testing.T) {
	/*
		A run of lights is a run across the picture. Averaging a red left and
		a blue right gives one purple for the whole machine, which is the
		result this exists to avoid.
	*/
	svc, _ := lit(t)
	withPictures(t, svc)
	_, err := svc.AddImage("halves", encoded(t))
	require.NoError(t, err)

	scene, err := svc.SceneFromImage(t.Context(), "halves", "themed", 1, nil)
	require.NoError(t, err)

	var board []string
	for _, assignment := range scene.Assignments {
		if strings.HasPrefix(assignment.Target, "ASUS") {
			board = append(board, assignment.Colour)
		}
	}
	require.Len(t, board, 2, "four lights over two colours is two runs")
	require.True(t, redder(board[0]), "the left of the picture is red, got %s", board[0])
	require.True(t, bluer(board[1]), "the right of the picture is blue, got %s", board[1])
}

func TestAdjacentLightsThatAgreeAreOneRule(t *testing.T) {
	// A hundred keys across a four-colour picture is four assignments, not a
	// hundred: a scene somebody opens afterwards is a scene they can read.
	svc, _ := lit(t)
	withPictures(t, svc)
	_, err := svc.AddImage("halves", encoded(t))
	require.NoError(t, err)

	scene, err := svc.SceneFromImage(t.Context(), "halves", "themed", 1, nil)
	require.NoError(t, err)

	for _, assignment := range scene.Assignments {
		if assignment.Target == "ASUS ROG MAXIMUS Z790 HERO/Addressable 1[0:1]" {
			return
		}
	}
	require.Fail(t, "the two red lights were not merged", "%v", scene.Assignments)
}

func TestASceneFromAPictureShowsThatPicture(t *testing.T) {
	// A machine lit by one image with another on its screen is two themes at
	// once.
	svc, _ := lit(t)
	withPictures(t, svc)
	stored, err := svc.AddImage("halves", encoded(t))
	require.NoError(t, err)

	scene, err := svc.SceneFromImage(t.Context(), "halves", "themed", 1, nil)
	require.NoError(t, err)
	require.Equal(t, stored.Path, scene.Screen)

	saved, err := svc.Scenes()
	require.NoError(t, err)
	var kept []string
	for _, one := range saved {
		kept = append(kept, one.Name)
	}
	require.Contains(t, kept, "themed", "the scene was built but not kept")
}

func TestAPictureThatIsNotThereSaysSo(t *testing.T) {
	svc, _ := lit(t)
	withPictures(t, svc)

	_, err := svc.SceneFromImage(t.Context(), "absent", "themed", 1, nil)
	require.ErrorContains(t, err, "absent")
}

// redder and bluer read a written colour without caring how bright it came
// out: the picture's hue is the claim, not its value.
func redder(written string) bool {
	r, g, b := channels(written)
	return r > g && r > b
}

func bluer(written string) bool {
	r, g, b := channels(written)
	return b > r && b > g
}

func channels(written string) (r, g, b int) {
	var got [3]int
	for i := range 3 {
		part := written[1+i*2 : 3+i*2]
		for _, c := range part {
			got[i] *= 16
			switch {
			case c >= '0' && c <= '9':
				got[i] += int(c - '0')
			default:
				got[i] += int(c-'a') + 10
			}
		}
	}
	return got[0], got[1], got[2]
}

func TestASceneFromADashboardNamesThatDashboard(t *testing.T) {
	/*
		So applying it puts that dashboard up. The lights were built from how
		it looked at the time, which is the honest split: editing the
		dashboard afterwards changes the screen without changing the lights,
		and `recolour` brings them back in line.
	*/
	svc, _ := lit(t)
	svc.SetCooler(&panel{})
	withScreens(t, svc)

	scene, err := svc.SceneFromDashboard(t.Context(), "quiet", "evening", 1, nil)
	require.NoError(t, err)
	require.Equal(t, "dashboard:quiet", scene.Screen)
	require.NotEmpty(t, scene.Assignments)
}

func TestSeparationChangesTheColoursAndNothingElse(t *testing.T) {
	svc, _ := lit(t)
	withPictures(t, svc)
	_, err := svc.AddImage("halves", encoded(t))
	require.NoError(t, err)

	plain, err := svc.SceneFromImage(t.Context(), "halves", "themed", 1, nil)
	require.NoError(t, err)
	apart, err := svc.SceneFromImage(t.Context(), "halves", "themed", 2.5, nil)
	require.NoError(t, err)

	require.Equal(t, plain.Screen, apart.Screen)
	require.InDelta(t, 2.5, apart.Distance, 0.001, "the separation was not kept")
	require.NotEqual(t, plain.Assignments[0].Colour, apart.Assignments[0].Colour,
		"separating the colours changed none of them")
}

func TestRecolouringTakesTheSceneAsItsSource(t *testing.T) {
	/*
		The knob has no right answer, so it has to be adjustable after the
		fact: what somebody is judging is not on screen anywhere except the
		case itself.
	*/
	svc, _ := lit(t)
	withPictures(t, svc)
	_, err := svc.AddImage("halves", encoded(t))
	require.NoError(t, err)

	made, err := svc.SceneFromImage(t.Context(), "halves", "themed", 1, nil)
	require.NoError(t, err)

	again, err := svc.RecolourScene(t.Context(), "themed", 3)
	require.NoError(t, err)
	require.Equal(t, made.Screen, again.Screen, "recolouring changed what the scene shows")
	require.InDelta(t, 3, again.Distance, 0.001)
	require.NotEqual(t, made.Assignments[0].Colour, again.Assignments[0].Colour)
}

func TestASceneWithNothingToTakeColoursFromSaysSo(t *testing.T) {
	// Rather than inventing a source.
	svc, _ := lit(t, scenes.Scene{Name: "plain", Colour: "blue"})
	withPictures(t, svc)

	_, err := svc.RecolourScene(t.Context(), "plain", 2)
	require.ErrorContains(t, err, "nothing to take its colours from")
}

// painting is a service with a picture in it, which is what every test below
// starts from.
func painting(t *testing.T) *service.Service {
	t.Helper()
	svc, _ := lit(t)
	withPictures(t, svc)
	_, err := svc.AddImage("halves", encoded(t))
	require.NoError(t, err)
	return svc
}

func TestASceneFromAPictureCarriesTheEffectsItWasGiven(t *testing.T) {
	/*
		A picture says what colour each light should be and nothing about
		what a device should be doing with it, so a keyboard asked to ripple
		is a decision the caller makes. Nothing is the default: every device
		shows the colours it was given.
	*/
	svc := painting(t)

	plain, err := svc.SceneFromImage(t.Context(), "halves", "themed", 1, nil)
	require.NoError(t, err)
	require.Empty(t, plain.Effects, "a scene invented an effect nobody asked for")

	with, err := svc.SceneFromImage(t.Context(), "halves", "rippling", 1,
		map[string]string{"Keychron": "Typing Heatmap"})
	require.NoError(t, err)
	require.Equal(t, map[string]string{"Keychron": "Typing Heatmap"}, with.Effects)
	require.NotEmpty(t, with.Assignments, "the colours were lost with the effect added")

	// And it is kept, because the scene is saved on the way out.
	saved, err := svc.Scene("rippling")
	require.NoError(t, err)
	require.Equal(t, "Typing Heatmap", saved.Effects["Keychron"])
}

func TestRecolouringKeepsTheEffects(t *testing.T) {
	// Recolouring is about the colours. A keyboard's mode is not one of
	// them, and losing it would make the separation slider destructive.
	svc := painting(t)

	_, err := svc.SceneFromImage(t.Context(), "halves", "themed", 1,
		map[string]string{"Keychron": "Typing Heatmap"})
	require.NoError(t, err)

	again, err := svc.RecolourScene(t.Context(), "themed", 2.5)
	require.NoError(t, err)
	require.Equal(t, "Typing Heatmap", again.Effects["Keychron"],
		"recolouring dropped what the devices were doing")
}
