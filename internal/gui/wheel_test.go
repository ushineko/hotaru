package gui_test

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/api"
	"github.com/ushineko/hotaru/internal/gui"
)

func TestTheWheelRoundTripsAColour(t *testing.T) {
	/*
		Opening the picker on what a scene already says, and reading back what
		somebody pointed at, are the same conversion in opposite directions. A
		wheel that lands a shade off every time it is opened turns editing a
		scene into drifting it.
	*/
	for _, want := range []color.NRGBA{
		{R: 255, A: 255},
		{G: 255, A: 255},
		{B: 255, A: 255},
		{R: 32, G: 16, B: 64, A: 255},
		{R: 255, G: 209, B: 102, A: 255},
		{R: 255, G: 255, B: 255, A: 255},
		{A: 255},
	} {
		w := gui.NewWheel()
		w.Set(want)

		got, ok := w.Colour().(color.NRGBA)
		require.True(t, ok)
		require.InDelta(t, want.R, got.R, 1, "red drifted for %v", want)
		require.InDelta(t, want.G, got.G, 1, "green drifted for %v", want)
		require.InDelta(t, want.B, got.B, 1, "blue drifted for %v", want)
	}
}

func TestBrightnessIsTheSlidersHalfOfTheWheel(t *testing.T) {
	// Hue and saturation are the disc; value is not on it, which is why the
	// slider exists rather than being a third dimension nobody can point at.
	w := gui.NewWheel()
	w.Set(color.NRGBA{R: 255, A: 255})

	w.SetValue(0.5)
	got := w.Colour().(color.NRGBA)
	require.InDelta(t, 128, got.R, 2)
	require.Zero(t, got.G)
}

func TestTheWheelReportsEveryMove(t *testing.T) {
	// What makes the hardware follow the pointer: not the last colour, every
	// colour.
	w := gui.NewWheel()
	picks := 0
	w.OnPick = func(color.Color) { picks++ }

	w.SetValue(0.9)
	w.SetValue(0.8)
	require.Equal(t, 2, picks)
}

func TestAWholeMachineColourIsNotSixAssignments(t *testing.T) {
	/*
		A scene's commonest shape -- "make the machine blue" -- and it was not
		expressible in the editor: the picture offered devices and zones, so
		colouring all of them meant six lines that would each have to be
		corrected when a device was added.
	*/
	draft := gui.NewDraft()
	draft.SetEverything("#0000ff")

	scene := draft.Scene("all blue")
	require.Equal(t, "#0000ff", scene.Colour)
	require.Empty(t, scene.Assignments)
	require.False(t, draft.Empty())
}

func TestEverythingAndAnExceptionCoexist(t *testing.T) {
	// "Everything blue except the top fan" stays two facts.
	draft := gui.NewDraft()
	draft.SetEverything("blue")
	draft.Set("kraken/fan-top", "red")

	scene := draft.Scene("evening")
	require.Equal(t, "blue", scene.Colour)
	require.Len(t, scene.Assignments, 1)
}

func TestEditingKeepsAWholeMachineColour(t *testing.T) {
	// The round-trip risk again: a scene that paints everything must not lose
	// that when its exceptions are edited.
	draft := gui.DraftFrom(api.Scene{Name: "evening", Colour: "blue"})
	draft.Set("kraken", "red")

	scene := draft.Scene("evening")
	require.Equal(t, "blue", scene.Colour)
	require.Equal(t, "blue", draft.Everything())
}

func TestThePickerDoesNotDriftWhatItWasGiven(t *testing.T) {
	/*
		The control fed itself: showing a colour set the brightness slider,
		the slider reported a change, and the change went back into the wheel
		-- snapped to the slider's step on the way. So the colour that came
		out was a shade off the one that went in, on the hardware and in the
		numbers, and the wheel looked broken.
	*/
	for _, want := range []string{"#ff0000", "#201040", "#ffd166", "#7ec88c"} {
		p := gui.NewPicker(gui.ParseColour(want))
		require.Equal(t, want, p.Colour(), "the picker changed the colour it was opened on")
	}
}

func TestThePickerReportsWhatItIsShowing(t *testing.T) {
	// What the hardware is asked for while somebody drags.
	picked := make([]string, 0, 4)
	p := gui.NewPicker(gui.ParseColour("#ff0000"))
	p.OnPick = func(colour string) { picked = append(picked, colour) }

	p.Choose(gui.ParseColour("#00ff00"))
	require.Equal(t, []string{"#00ff00"}, picked)
	require.Equal(t, "#00ff00", p.Colour())
}

func TestAWheelOpenedOnNothingOpensOnWhite(t *testing.T) {
	/*
		Opening the picker sends its colour to the hardware straight away, so
		what it opens on matters. An empty colour parsed to the theme's
		disabled grey, which meant selecting a fan that had no colour yet
		turned the machine grey before anybody had chosen anything.
	*/
	white, ok := gui.ParseColour(gui.Opening("")).(color.NRGBA)
	require.True(t, ok)
	require.Equal(t, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, white)

	require.Equal(t, "#201040", gui.Opening("#201040"))
}

func TestThePickedColourIsTheColourUnderThePointer(t *testing.T) {
	/*
		The wheel is drawn square and centred in whatever the widget is given,
		and the picking maths has to use that same circle. It did not: Fyne
		placed the image at its natural size inside a widget of a different
		one, so the colour under the pointer was not the colour the pointer
		was over -- and the conversion maths, which was right, looked wrong.
	*/
	w := gui.NewWheel()
	w.Resize(fyne.NewSize(200, 120)) // deliberately not square

	// The rim at three o'clock is fully saturated red on an HSV disc.
	centre := fyne.NewPos(100, 60)
	w.Tapped(&fyne.PointEvent{Position: fyne.NewPos(centre.X+60, centre.Y)})

	got := w.Colour().(color.NRGBA)
	require.InDelta(t, 255, got.R, 2, "the rim at three o'clock was not red")
	require.InDelta(t, 0, got.G, 8)
	require.InDelta(t, 0, got.B, 8)
}

func TestTheCentreOfTheWheelIsWhite(t *testing.T) {
	// Saturation grows outward, so the middle is the absence of hue. Worth a
	// test because it is the half of an HSV disc people are surprised by: a
	// red that is only red on the rim is the wheel working.
	w := gui.NewWheel()
	w.Resize(fyne.NewSize(200, 200))
	w.Tapped(&fyne.PointEvent{Position: fyne.NewPos(100, 100)})

	got := w.Colour().(color.NRGBA)
	require.Equal(t, uint8(255), got.R)
	require.Equal(t, uint8(255), got.G)
	require.Equal(t, uint8(255), got.B)
}

func TestSaturationCanBeTunedWithoutTheRim(t *testing.T) {
	// The complaint the slider answers: a fully saturated colour exists only
	// on a rim somebody has to hit with a pointer.
	w := gui.NewWheel()
	w.Resize(fyne.NewSize(200, 200))
	w.Tapped(&fyne.PointEvent{Position: fyne.NewPos(140, 100)}) // part way out

	w.SetSaturation(1)
	got := w.Colour().(color.NRGBA)
	require.Equal(t, uint8(255), got.R)
	require.Zero(t, got.G)
	require.Zero(t, got.B)
}
