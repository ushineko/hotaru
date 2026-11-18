package service_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/canvas/canvastest"
	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/scenes"
	"github.com/ushineko/hotaru/internal/service"
	"github.com/ushineko/hotaru/internal/service/servicetest"
)

const graphicsName = "Test Graphics Card"

func TestACanvasBreathesBetweenTheColoursItIsGiven(t *testing.T) {
	svc := service.New(nil)
	attachDrawn(t, svc)

	effect := scenes.Effect{Mode: "Breathing"}
	effect.SetPalette([]string{"#ff0000", "#0000ff"})
	results, err := svc.Apply(t.Context(), service.Request{
		Assignments: solid(drawnName, "white"),
		Effects:     map[string]scenes.Effect{drawnName: effect},
	})
	require.NoError(t, err)
	require.True(t, results[0].Applied, results[0].Skipped)

	view := viewOf(t, svc, drawnName)
	breathing, _ := view.Device.Mode("Breathing")
	require.Equal(t, 4, breathing.ColoursMax)
	require.Equal(t, []colour.Colour{colour.MustParse("#ff0000"), colour.MustParse("#0000ff")}, breathing.Colours)
}

func TestACanvasSpectrumWithNoColoursIsNotGivenTheFramesColour(t *testing.T) {
	/*
		The frame's colour is the fallback for a mode that shows one of its
		own. Spectrum shows the wheel without colours, and given the frame's
		one it would hold still in it.
	*/
	svc := service.New(nil)
	attachDrawn(t, svc)

	_, err := svc.Apply(t.Context(), service.Request{
		Assignments: solid(drawnName, "white"),
		Effects:     map[string]scenes.Effect{drawnName: {Mode: "Spectrum"}},
	})
	require.NoError(t, err)

	view := viewOf(t, svc, drawnName)
	spectrum, _ := view.Device.Mode("Spectrum")
	require.Equal(t, 8, spectrum.ColoursMax)
	require.Empty(t, spectrum.Colours, "Spectrum was handed the frame's colour")
}

// threeToOne is a picture three parts red to one part blue, so the order of
// its colours by coverage is not a tie.
func threeToOne(t *testing.T) []byte {
	t.Helper()
	picture := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := range 64 {
		for x := range 64 {
			c := color.RGBA{R: 255, A: 255}
			if x >= 48 {
				c = color.RGBA{B: 255, A: 255}
			}
			picture.Set(x, y, c)
		}
	}
	var out bytes.Buffer
	require.NoError(t, png.Encode(&out, picture))
	return out.Bytes()
}

func picturing(t *testing.T) *service.Service {
	t.Helper()
	svc, _ := litCanvas(t)
	servicetest.Attach(t, svc, canvastest.NewBoard(graphicsName, canvastest.Zone("Card", 3)))
	withPictures(t, svc)
	_, err := svc.AddImage("sunset", threeToOne(t))
	require.NoError(t, err)
	return svc
}

func TestMakingASceneGivesAnEffectThePicturesColours(t *testing.T) {
	// R5.1: as many as the mode takes, the colour covering most first.
	svc := picturing(t)

	scene, err := svc.SceneFromImage(t.Context(), "sunset", "themed", 1,
		map[string]scenes.Effect{"Graphics": {Mode: "Breathing"}, "Keychron": {Mode: "Solid Reactive"}})
	require.NoError(t, err)

	// The picture is stored as a GIF, so its red is a red and not #ff0000.
	breathing := scene.Effects["Graphics"]
	picked := breathing.Palette()
	require.GreaterOrEqual(t, len(picked), 2)
	require.LessOrEqual(t, len(picked), 4, "more than a canvas's Breathing takes")
	require.True(t, redder(picked[0]), "the colour covering most is not first: %v", picked)
	require.True(t, bluer(picked[1]), "%v", picked)
	require.Equal(t, scenes.ColoursFromPicture, breathing.ColoursFrom)

	// A mode that takes one gets the one covering most.
	one := scene.Effects["Keychron"].Palette()
	require.Len(t, one, 1)
	require.True(t, redder(one[0]), "%v", one)
}

func TestTheColoursAPictureGivesAreTheOnesThePreviewShowed(t *testing.T) {
	// R5.3: the window shows the colours before the scene is made.
	svc := picturing(t)

	shown, err := svc.Picked("sunset", 4, 1) // a canvas's Breathing takes 4
	require.NoError(t, err)
	scene, err := svc.SceneFromImage(t.Context(), "sunset", "themed", 1,
		map[string]scenes.Effect{"Graphics": {Mode: "Breathing"}})
	require.NoError(t, err)
	require.Equal(t, shown, scene.Effects["Graphics"].Palette())

	_, err = svc.Picked("absent", 2, 1)
	require.ErrorContains(t, err, "absent")
}

func TestRecolouringPicksPickedColoursAgainAndKeepsChosenOnes(t *testing.T) {
	// R5.2: picked colours follow the picture; chosen ones are somebody's.
	svc := picturing(t)

	_, err := svc.SceneFromImage(t.Context(), "sunset", "themed", 1, map[string]scenes.Effect{
		"Graphics": {Mode: "Breathing"},
		"Keychron": {Mode: "Solid Reactive", Colour: "#00ff00"},
	})
	require.NoError(t, err)

	// Somebody's edit of the file left stale picked colours behind.
	stale, err := svc.Scene("themed")
	require.NoError(t, err)
	effect := stale.Effects["Graphics"]
	effect.SetPalette([]string{"#123456", "#654321"})
	stale.Effects["Graphics"] = effect
	require.NoError(t, svc.SaveScene(t.Context(), stale))

	again, err := svc.RecolourScene(t.Context(), "themed", 1.5)
	require.NoError(t, err)
	picked := again.Effects["Graphics"].Palette()
	require.GreaterOrEqual(t, len(picked), 2)
	require.True(t, redder(picked[0]) && bluer(picked[1]), "the stale colours were kept: %v", picked)
	require.Equal(t, []string{"#00ff00"}, again.Effects["Keychron"].Palette(), "a chosen colour was picked over")
	require.Empty(t, again.Effects["Keychron"].ColoursFrom)
}
