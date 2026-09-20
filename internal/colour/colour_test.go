package colour_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/colour"
)

func TestTheNamesMeanWhatTheyMeantInThePython(t *testing.T) {
	// A scene written for peripheral-battery-monitor must mean the same thing
	// here, so these values are carried over rather than re-chosen.
	for name, want := range map[string]colour.Colour{
		"red":    {R: 255},
		"orange": {R: 255, G: 85},
		"purple": {R: 128, B: 255},
		"teal":   {G: 255, B: 128},
		"white":  {R: 255, G: 255, B: 255},
	} {
		got, err := colour.Parse(name)
		require.NoError(t, err, name)
		require.Equal(t, want, got, name)
	}
}

func TestHexIsAcceptedInTheFormsPeopleActuallyType(t *testing.T) {
	for _, in := range []string{"#ff8800", "FF8800", " #Ff8800 ", "#f80"} {
		got, err := colour.Parse(in)
		require.NoError(t, err, in)
		require.Equal(t, colour.Colour{R: 255, G: 136}, got, in)
	}
}

func TestOffIsAnIntentAndSaysSo(t *testing.T) {
	// Handing back black would light a keyboard's backlight out rather than
	// turning the device off, which is the whole reason the distinction exists.
	_, err := colour.Parse("off")
	require.ErrorContains(t, err, "intent")
}

func TestWhatIsNotAColourIsRejectedWithAnExample(t *testing.T) {
	for _, in := range []string{"", "  ", "burgundy", "#12345", "#gggggg", "12345678"} {
		_, err := colour.Parse(in)
		require.Error(t, err, in)
	}
	_, err := colour.Parse("burgundy")
	require.ErrorContains(t, err, "#ff8800", "the message shows the form that works")
}

func TestAColourIsAStringEverywhereAPersonReadsIt(t *testing.T) {
	type scene struct {
		Colour colour.Colour `json:"colour"`
	}
	out, err := json.Marshal(scene{Colour: colour.MustParse("orange")})
	require.NoError(t, err)
	require.JSONEq(t, `{"colour":"#ff5500"}`, string(out))

	var back scene
	require.NoError(t, json.Unmarshal([]byte(`{"colour":"teal"}`), &back))
	require.Equal(t, colour.MustParse("teal"), back.Colour)

	require.Error(t, json.Unmarshal([]byte(`{"colour":"beige"}`), &back))
}

func TestNamesAreListedForHelpAndForPickers(t *testing.T) {
	names := colour.Names()
	require.Contains(t, names, "magenta")
	require.NotContains(t, names, "off")
	require.Equal(t, "black", names[0], "sorted, so the list is stable")
}
