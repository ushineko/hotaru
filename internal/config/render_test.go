package config_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/config"
)

func TestTogglesSurviveTheFileBeingRewritten(t *testing.T) {
	/*
		hotaru renders the whole configuration back when it writes the rules,
		so anything this renderer does not know about is something a person
		loses on the next write -- and they would not notice until their
		toggles stopped having a control in the window.
	*/
	written := config.Render(&config.Config{Devices: []config.DeviceRule{{
		Match: "keychron",
		Segments: map[string]config.Segment{
			"caps": {Zone: "Keyboard", LEDs: &config.LEDs{First: 55, Last: 55}},
		},
		Toggles: []string{"caps"},
	}}})
	require.Contains(t, written, "toggles: [caps]")

	again, problems, err := config.Parse("rules.yml", []byte(written))
	require.NoError(t, err)
	require.Empty(t, problems, "the file this renderer wrote does not load cleanly")
	require.Equal(t, []string{"caps"}, again.Devices[0].Toggles)
}

/*
Writing the rules back keeps every key a rule can hold: light map and light
match regenerate the file, and a key they do not write is a setting they
silently delete (spec 077). Every field of a rule is set here, and the test
fails when a new one is added without a value below.
*/
func TestEveryRuleKeySurvivesBeingWrittenBack(t *testing.T) {
	half, gamma, level := 0.5, 2.15, 80
	rule := config.DeviceRule{
		Match:         "maximus",
		SolidModes:    []string{"static"},
		NeverBlank:    true,
		Brightness:    &level,
		Reassert:      config.Duration(time.Minute),
		Segments:      map[string]config.Segment{"rear": {Zone: "Addressable RGB Header 3"}},
		Toggles:       []string{"rear"},
		FrameInterval: config.Duration(100 * time.Millisecond),
		Twin:          "Old Board",
		Formerly:      &config.Former{Name: "Old Board", Zones: map[string]string{"A": "B"}},
		Lengths:       map[string]int{"Addressable RGB Header 3": 20},
		Shapes:        map[string]string{"Addressable RGB Header 3": "ring"},
		Pace:          map[string]float64{"Breathing": 2.2},
		Follow:        "kraken",
		Colour:        &config.ColourProfile{Saturation: &half, Curve: &config.Curve{Green: &gamma}},
	}
	value := reflect.ValueOf(rule)
	for i := range value.NumField() {
		require.False(t, value.Field(i).IsZero(), "%s has no value in this test", value.Type().Field(i).Name)
	}

	written := config.Render(&config.Config{Devices: []config.DeviceRule{rule}})
	back, problems, err := config.Parse("hotaru.yml", []byte(written))
	require.NoError(t, err)
	require.Empty(t, problems, "%s", written)
	require.Equal(t, rule, back.Devices[0], "%s", written)
}
