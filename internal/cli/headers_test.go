package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hotaru/internal/api"
	"github.com/ushineko/hotaru/internal/canvas/canvastest"
	"github.com/ushineko/hotaru/internal/cli"
	"github.com/ushineko/hotaru/internal/config"
)

/*
The commands that replace light map (spec 078), as a person uses them: each
lights what it is about, takes the answer seen on the desk, and writes the
rules through the service.
*/

// rulesAt is a rules file holding text, under a config home of the test's
// own, and where it is.
func rulesAt(t *testing.T, text string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	path := filepath.Join(home, "hotaru", "hotaru.yml")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(text), 0o600))
	return path
}

func ruleIn(t *testing.T, path, device string) config.DeviceRule {
	t.Helper()
	cfg, _, err := config.Load(path)
	require.NoError(t, err)
	rules := cfg.RulesFor(device)
	require.NotEmpty(t, rules, "no rule for %s", device)
	return rules[len(rules)-1]
}

const board = "ASUS ROG MAXIMUS Z790 HERO"

func z790(header string, lights, most int) *canvastest.Board {
	return canvastest.NewBoard(board, canvastest.Zone("Aura Mainboard", 1), canvastest.Header(header, lights, most))
}

// The end is moved by one and by ten until it sits on the strip's last light,
// and Return writes it; the length is in use at once.
func TestALengthIsFoundByMovingTheEnd(t *testing.T) {
	path := rulesAt(t, "devices:\n  - match: \"maximus\"\n")
	client := api.NewClient(drawing(t, nil, z790("Addressable RGB Header 3", 10, 120)))
	person := &scripted{answers: []string{"++", "-", ""}}
	require.NoError(t, cli.FindLength(t.Context(), client, person, "maximus/Addressable RGB Header 3"))

	assert.Equal(t, map[string]int{"Addressable RGB Header 3": 19}, ruleIn(t, path, board).Lengths)
	found, err := client.Devices(t.Context())
	require.NoError(t, err)
	for _, d := range found {
		if d.Name == board {
			assert.Equal(t, 19, d.Zones[1].Count, "the length is not in use")
		}
	}
	assert.Contains(t, person.questions(), "20 lights", "++ did not move the end by ten")
}

// q leaves the length as it was and writes nothing; a zone the device counts
// is refused.
func TestALengthLeftWithQIsNotWritten(t *testing.T) {
	path := rulesAt(t, "devices:\n  - match: \"maximus\"\n")
	client := api.NewClient(drawing(t, nil, z790("Addressable RGB Header 3", 10, 120)))
	require.NoError(t, cli.FindLength(t.Context(), client, &scripted{answers: []string{"+", "q"}},
		"maximus/Addressable RGB Header 3"))
	assert.Empty(t, ruleIn(t, path, board).Lengths)

	err := cli.FindLength(t.Context(), client, &scripted{}, "maximus/Aura Mainboard")
	require.ErrorContains(t, err, "counts the lights")
}

// A header of sixteen said to hold two fans is two named rings of eight; a
// name already on the same lights is kept, a segment dividing it otherwise is
// dropped, and a toggle stays.
func TestAHeaderSaidToHoldFansIsDividedAndNamed(t *testing.T) {
	path := rulesAt(t, `devices:
  - match: "maximus"
    segments:
      top-front: {zone: "Addressable RGB Header 2", leds: [0, 7]}
      old-half: {zone: "Addressable RGB Header 2", leds: [0, 3]}
      caps: {zone: "Addressable RGB Header 2", leds: [15, 15]}
    toggles: [caps]
`)
	client := api.NewClient(drawing(t, nil, z790("Addressable RGB Header 2", 16, 120)))
	person := &scripted{answers: []string{""}}
	require.NoError(t, cli.SetFans(t.Context(), client, person, "maximus/Addressable RGB Header 2", 2))

	rule := ruleIn(t, path, board)
	assert.Equal(t, map[string]string{"Addressable RGB Header 2": "ring"}, rule.Shapes)
	require.Contains(t, rule.Segments, "top-front")
	assert.Equal(t, config.LEDs{First: 0, Last: 7}, *rule.Segments["top-front"].LEDs)
	require.Contains(t, rule.Segments, "header-2-fan-2")
	assert.Equal(t, config.LEDs{First: 8, Last: 15}, *rule.Segments["header-2-fan-2"].LEDs)
	assert.NotContains(t, rule.Segments, "old-half", "a segment dividing it otherwise was kept")
	assert.Contains(t, rule.Segments, "caps", "a toggle was dropped")
	assert.Contains(t, strings.Join(person.said, "\n"), "red: top-front")
}

// A length that does not divide is refused with the lengths that do; a strip
// is a line.
func TestFansThatDoNotDivideAreRefusedAndAStripIsALine(t *testing.T) {
	path := rulesAt(t, "devices:\n  - match: \"maximus\"\n")
	client := api.NewClient(drawing(t, nil, z790("Addressable RGB Header 2", 16, 120)))
	err := cli.SetFans(t.Context(), client, &scripted{}, "maximus/Addressable RGB Header 2", 3)
	require.ErrorContains(t, err, "15 or 18")

	require.NoError(t, cli.SetStrip(t.Context(), client, &scripted{}, "maximus/Addressable RGB Header 2"))
	assert.Equal(t, map[string]string{"Addressable RGB Header 2": "line"}, ruleIn(t, path, board).Shapes)
}

// A range is lit and named once the person says it is the one; a no writes
// nothing.
func TestARangeIsNamedOnceItIsSeen(t *testing.T) {
	path := rulesAt(t, "devices:\n  - match: \"maximus\"\n")
	client := api.NewClient(drawing(t, nil, z790("Addressable RGB Header 3", 20, 120)))
	require.NoError(t, cli.NameOne(t.Context(), client, &scripted{answers: []string{"n"}},
		"maximus/Addressable RGB Header 3[0:9]", "Rear Fan"))
	assert.Empty(t, ruleIn(t, path, board).Segments)

	require.NoError(t, cli.NameOne(t.Context(), client, &scripted{answers: []string{"y"}},
		"maximus/Addressable RGB Header 3[0:9]", "Rear Fan"))
	seg := ruleIn(t, path, board).Segments["rear-fan"]
	assert.Equal(t, "Addressable RGB Header 3", seg.Zone)
	assert.Equal(t, config.LEDs{First: 0, Last: 9}, *seg.LEDs)
}

/*
A device's zones and fans are gone through in turn, with the names they have,
under the zones the device was formerly known by too: Return keeps one, a word
replaces it, and - removes it.
*/
func TestADevicesNamesAreGoneThroughInTurn(t *testing.T) {
	path := rulesAt(t, `devices:
  - match: "kraken"
    formerly:
      name: "NZXT Kraken 2024 ELITE Series RGB"
      zones: {"Hue 2 Channel 1": "Ring", "Hue 2 Channel 2": "Fan"}
    segments:
      ring: {zone: "Hue 2 Channel 1"}
      rear-top: {zone: "Hue 2 Channel 2", leds: [0, 7]}
      middle-top: {zone: "Hue 2 Channel 2", leds: [8, 15]}
`)
	cooler := canvastest.NewBoard("NZXT Kraken Elite V2", canvastest.Ring("Ring", 24), canvastest.Ring("Fan", 8, 8, 8))
	client := api.NewClient(drawing(t, nil, cooler))
	person := &scripted{answers: []string{"", "back-fan", "-", "front-top"}}
	require.NoError(t, cli.NameAll(t.Context(), client, person, "kraken"))

	asked := person.questions()
	assert.Contains(t, asked, "[ring]")
	assert.Contains(t, asked, "[rear-top]")
	rule := ruleIn(t, path, "NZXT Kraken Elite V2")
	assert.Contains(t, rule.Segments, "ring", "Return did not keep it")
	assert.Contains(t, rule.Segments, "back-fan")
	assert.NotContains(t, rule.Segments, "rear-top", "the old name stayed beside the new")
	assert.NotContains(t, rule.Segments, "middle-top", "- did not remove it")
	require.Contains(t, rule.Segments, "front-top")
	assert.Equal(t, config.LEDs{First: 16, Last: 23}, *rule.Segments["front-top"].LEDs)
	assert.NotNil(t, rule.Formerly, "the former name was dropped")
}

// light map says what replaced it.
func TestLightMapSaysWhatReplacedIt(t *testing.T) {
	out, err := run(t, drawing(t, nil, z790("Addressable RGB Header 3", 20, 120)), "light", "map")
	require.Error(t, err)
	for _, command := range []string{"light length", "light fans", "light strip", "light name"} {
		assert.Contains(t, out, command)
	}
}
