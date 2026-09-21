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

	scene, err := svc.SceneFromImage(t.Context(), "halves", "themed")
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

	scene, err := svc.SceneFromImage(t.Context(), "halves", "themed")
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

	scene, err := svc.SceneFromImage(t.Context(), "halves", "themed")
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

	scene, err := svc.SceneFromImage(t.Context(), "halves", "themed")
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

	_, err := svc.SceneFromImage(t.Context(), "absent", "themed")
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
