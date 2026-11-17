package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/hotaru/internal/api"
	"github.com/ushineko/hotaru/internal/canvas/canvastest"
	"github.com/ushineko/hotaru/internal/cli"
	"github.com/ushineko/hotaru/internal/config"
)

/*
light map finds a header's length by halving: lit up to a point in green and
red after it, asked only whether any red shows, and writes it to the rules
(spec 077 R4.3). The answers below are a person looking at a strip of 18.
*/
func TestTheMapFindsAHeadersLength(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	board := canvastest.NewBoard("ASUS ROG MAXIMUS Z790 HERO",
		canvastest.Zone("Aura Mainboard", 1), canvastest.Header("Addressable RGB Header 3", 0, 500))
	client := api.NewClient(drawing(t, nil, board))

	// Of 120: is any red past 60? 30? 15? 22? 18? 16? 17?
	reds := []string{"n", "n", "y", "n", "n", "y", "y"}
	answers := append([]string{"y", "mainboard", "y"}, reds...)
	answers = append(answers, "rear fan", "1", "n", "y", "n", "y")
	person := &scripted{answers: answers}
	require.NoError(t, cli.Map(t.Context(), client, person))

	require.Equal(t, 7, strings.Count(person.questions(), "Is any red showing"))
	require.Contains(t, strings.Join(person.said, "\n"), "Addressable RGB Header 3 has 18 lights")
	cfg, _, err := config.Load(filepath.Join(home, "hotaru", "hotaru.yml"))
	require.NoError(t, err)
	rule := cfg.RulesFor("ASUS ROG MAXIMUS Z790 HERO")
	require.NotEmpty(t, rule, "nothing was written")
	require.Equal(t, map[string]int{"Addressable RGB Header 3": 18}, rule[0].Lengths)
}

// A header with nothing plugged in is found empty with one question.
func TestTheMapFindsAnEmptyHeader(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	board := canvastest.NewBoard("ASUS ROG MAXIMUS Z790 HERO",
		canvastest.Zone("Aura Mainboard", 1), canvastest.Header("Addressable RGB Header 4", 0, 500))
	client := api.NewClient(drawing(t, nil, board))
	person := &scripted{answers: []string{"y", "mainboard", "n"}}
	require.NoError(t, cli.Map(t.Context(), client, person))
	require.NotContains(t, person.questions(), "Is any red showing")
	require.Contains(t, strings.Join(person.said, "\n"), "Nothing on it")
}

// "No" at the end goes back to a device, with this run's answers as the
// defaults, instead of throwing every answer away (spec 077, the live run).
func TestTheMapGoesBackToADevice(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	board := canvastest.NewBoard("ASUS ROG MAXIMUS Z790 HERO",
		canvastest.Zone("Aura Mainboard", 1), canvastest.Header("Addressable RGB Header 3", 0, 500))
	client := api.NewClient(drawing(t, nil, board))

	reds := []string{"n", "n", "y", "n", "n", "y", "y"}
	answers := append([]string{"y", "mainboard", "y"}, reds...)
	answers = append(answers, "rear fan", "1", "n", "n", "1")
	answers = append(answers, "y", "", "y")
	answers = append(answers, reds...)
	answers = append(answers, "top fans", "1", "y", "n", "y")
	person := &scripted{answers: answers}
	require.NoError(t, cli.Map(t.Context(), client, person))

	cfg, _, err := config.Load(filepath.Join(home, "hotaru", "hotaru.yml"))
	require.NoError(t, err, person.questions())
	rule := cfg.RulesFor("ASUS ROG MAXIMUS Z790 HERO")
	require.NotEmpty(t, rule, person.questions())
	_, renamed := rule[0].Segments["top-fans"]
	_, old := rule[0].Segments["rear-fan"]
	require.True(t, renamed, "the second answer was not written")
	require.False(t, old, "the first answer was kept beside it")
	require.Contains(t, rule[0].Segments, "mainboard", "the default from this run was not offered")
}

// A zone the device divides itself (the Kraken's fans) still has each part
// named, lit a colour apiece; the live run wrote them unnamed.
func TestTheMapNamesEachFanTheDeviceCounts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	cooler := canvastest.NewBoard("NZXT Kraken Elite V2", canvastest.Ring("Ring", 24), canvastest.Ring("Fan", 8, 8, 8))
	client := api.NewClient(drawing(t, nil, cooler))
	person := &scripted{answers: []string{"y", "ring", "fans", "rear-top", "middle-top", "front-top", "n", "y", "n", "y"}}
	require.NoError(t, cli.Map(t.Context(), client, person))

	cfg, _, err := config.Load(filepath.Join(home, "hotaru", "hotaru.yml"))
	require.NoError(t, err, person.questions())
	rule := cfg.RulesFor("NZXT Kraken Elite V2")
	require.NotEmpty(t, rule, person.questions())
	for name, leds := range map[string][2]int{"rear-top": {0, 7}, "middle-top": {8, 15}, "front-top": {16, 23}} {
		segment, ok := rule[0].Segments[name]
		require.True(t, ok, "%s was not named: %s", name, person.questions())
		require.Equal(t, "Fan", segment.Zone)
		require.Equal(t, leds, [2]int{segment.LEDs.First, segment.LEDs.Last})
	}
}

/*
A name the device was formerly called is not asked about as a device of its
own, and the names its segments had under the old zones are offered back
(the live run asked about the Kraken's old name, and offered nothing).
*/
func TestTheMapSkipsAFormerNameAndOffersItsNames(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	rules := filepath.Join(home, "hotaru", "hotaru.yml")
	require.NoError(t, os.MkdirAll(filepath.Dir(rules), 0o700))
	require.NoError(t, os.WriteFile(rules, []byte(`devices:
  - match: "kraken"
    formerly:
      name: "NZXT Kraken 2024 ELITE Series RGB"
      zones: {"Hue 2 Channel 1": "Ring", "Hue 2 Channel 2": "Fan"}
    segments:
      front-top: {zone: "Hue 2 Channel 2", leds: [16, 23]}
      middle-top: {zone: "Hue 2 Channel 2", leds: [8, 15]}
      rear-top: {zone: "Hue 2 Channel 2", leds: [0, 7]}
      ring: {zone: "Hue 2 Channel 1"}
`), 0o600))
	cfg, _, err := config.Load(rules)
	require.NoError(t, err)
	cooler := canvastest.NewBoard("NZXT Kraken Elite V2", canvastest.Ring("Ring", 24), canvastest.Ring("Fan", 8, 8, 8))
	client := api.NewClient(drawing(t, cfg, cooler))

	person := &scripted{answers: []string{"y", "", "fans", "", "", "", "n", "y", "n", "y"}}
	require.NoError(t, cli.Map(t.Context(), client, person))
	asked := person.questions()
	require.NotContains(t, strings.Join(person.said, "\n"), "── NZXT Kraken 2024", "the former name was gone through")
	require.Contains(t, asked, "[rear-top]")
	require.Contains(t, asked, "[ring]")

	written, _, err := config.Load(rules)
	require.NoError(t, err)
	rule := written.RulesFor("NZXT Kraken Elite V2")[0]
	require.Contains(t, rule.Segments, "front-top")
	require.Equal(t, "Fan", rule.Segments["front-top"].Zone)
	require.NotNil(t, rule.Formerly, "the former name was dropped")
}

// A keyboard's switches (caps and num lock), named by hand, are kept when
// the keyboard is mapped again; the live run lost them.
func TestTheMapKeepsAKeyboardsSwitches(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	rules := filepath.Join(home, "hotaru", "hotaru.yml")
	require.NoError(t, os.MkdirAll(filepath.Dir(rules), 0o700))
	require.NoError(t, os.WriteFile(rules, []byte(`devices:
  - match: "keychron"
    segments:
      keyboard: {zone: "Keys"}
      caps: {zone: "Keys", leds: [2, 2]}
    toggles: [caps]
`), 0o600))
	cfg, _, err := config.Load(rules)
	require.NoError(t, err)
	keyboard := canvastest.NewKeyboard(8)
	keyboard.Name = "Keychron K4 HE"
	client := api.NewClient(drawing(t, cfg, keyboard))

	person := &scripted{answers: []string{"y", "", "n", "y", "n", "y"}}
	require.NoError(t, cli.Map(t.Context(), client, person))
	written, _, err := config.Load(rules)
	require.NoError(t, err, person.questions())
	rule := written.RulesFor("Keychron K4 HE")[0]
	require.Contains(t, rule.Segments, "caps", person.questions())
	require.Contains(t, rule.Segments, "keyboard")
	require.Equal(t, []string{"caps"}, rule.Toggles)
}
