package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/config"
)

// The desk this was developed on, as a user would write it. Not a default:
// hotaru ships none of this, and a machine without the file gets everything.
const exampleRules = `
scope:
  - kraken
  - geforce
devices:
  - match: maximus
    solid_modes: [direct, static]
  - match: keychron
    never_blank: true
    brightness: 100
  - match: g502
    reassert: 60s
  - match: kraken
    segments:
      fan-top: {zone: ring, leds: [0, 11]}
      fan-mid: {zone: ring, leds: [12, 23]}
      pump:    {zone: logo}
`

func write(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "hotaru.yml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

func TestAMachineWithNoConfigFileIsConfigured(t *testing.T) {
	cfg, problems, err := config.Load(filepath.Join(t.TempDir(), "absent.yml"))
	require.NoError(t, err)
	require.Empty(t, problems)

	// The whole point: no file means every device, not no devices.
	require.True(t, cfg.InScope("Some Stranger's Keyboard"))
	require.Empty(t, cfg.RulesFor("Some Stranger's Keyboard"))
}

func TestAnEmptyFileIsTheSameAsNoFile(t *testing.T) {
	cfg, problems, err := config.Load(write(t, "\n  \n"))
	require.NoError(t, err)
	require.Empty(t, problems)
	require.True(t, cfg.InScope("anything"))
}

func TestTheExampleRulesParse(t *testing.T) {
	cfg, problems, err := config.Load(write(t, exampleRules))
	require.NoError(t, err)
	require.Empty(t, problems)

	require.True(t, cfg.InScope("NZXT Kraken 2024 ELITE Series RGB"))
	require.True(t, cfg.InScope("MSI GeForce RTX 4090 Suprim Liquid X"))
	require.False(t, cfg.InScope("Keychron K4 HE"), "narrowed away by scope")

	board := cfg.RulesFor("ASUS ROG MAXIMUS Z790 HERO")
	require.Len(t, board, 1)
	require.Equal(t, []string{"direct", "static"}, board[0].SolidModes)

	keyboard := cfg.RulesFor("Keychron K4 HE")
	require.Len(t, keyboard, 1)
	require.True(t, keyboard[0].NeverBlank)
	require.Equal(t, 100, *keyboard[0].Brightness)

	mouse := cfg.RulesFor("Logitech G502 X PLUS")
	require.Len(t, mouse, 1)
	require.Equal(t, time.Minute, mouse[0].Reassert.Duration())

	cooler := cfg.RulesFor("NZXT Kraken 2024 ELITE Series RGB")
	require.Len(t, cooler, 1)
	require.Equal(t, "ring", cooler[0].Segments["fan-mid"].Zone)
	require.Equal(t, 12, cooler[0].Segments["fan-mid"].LEDs.First)
	require.Equal(t, 12, cooler[0].Segments["fan-mid"].LEDs.Count())
	require.Nil(t, cooler[0].Segments["pump"].LEDs, "a whole zone has no range")
}

func TestMatchingIsCaseInsensitiveBecauseDevicesShoutTheirNames(t *testing.T) {
	cfg, _, err := config.Load(write(t, "devices:\n  - match: KRAKEN\n    never_blank: true\n"))
	require.NoError(t, err)
	require.Len(t, cfg.RulesFor("NZXT Kraken 2024 ELITE Series RGB"), 1)
}

func TestEveryMatchingRuleIsReturnedInFileOrder(t *testing.T) {
	cfg, _, err := config.Load(write(t, `
devices:
  - match: kraken
    never_blank: true
  - match: nzxt
    brightness: 50
`))
	require.NoError(t, err)
	rules := cfg.RulesFor("NZXT Kraken 2024 ELITE")
	require.Len(t, rules, 2)
	require.Equal(t, "kraken", rules[0].Match)
	require.Equal(t, "nzxt", rules[1].Match)
}

func TestABadEntryCostsThatEntryAndNothingElse(t *testing.T) {
	cfg, problems, err := config.Load(write(t, `
devices:
  - match: kraken
    never_blank: true
  - never_blank: true
  - match: gpu
    brightness: 900
  - match: strip
    segments:
      bad: {leds: [5, 1]}
  - match: keychron
    reassert: 30s
what-is-this: 7
`))
	require.NoError(t, err, "a bad entry is not a bad file")

	// The good rules survived, including the ones after the bad ones.
	require.Len(t, cfg.Devices, 4)
	require.Equal(t, time.Second*30, cfg.RulesFor("Keychron K4 HE")[0].Reassert.Duration())

	// The bad ones were reported, each naming itself.
	var why []string
	for _, p := range problems {
		why = append(why, p.Error())
	}
	require.Len(t, why, 4)
	require.Contains(t, why[0], "devices[1]")
	require.Contains(t, why[0], "needs a device name")
	require.Contains(t, why[1], "brightness 900")
	require.Contains(t, why[2], "no zone")
	require.Contains(t, why[3], "unknown section")

	// A rule whose brightness was rejected is still used for everything else.
	require.Nil(t, cfg.RulesFor("gpu")[0].Brightness)
}

func TestAnUnparseableFileIsReportedAndLeftAloneOnDisk(t *testing.T) {
	// The promise is that hotaru never writes this file. The moment that is
	// most easily broken is the one where a user has mistyped their own YAML:
	// fynedesygn's settings.Store renames such a file to .bad, which is why
	// this package reads the file itself rather than opening a Store on it.
	path := write(t, "devices: [ unclosed\n")
	before, err := os.ReadFile(path)
	require.NoError(t, err)

	_, _, err = config.Load(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), path)

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, after, "the user's file was modified")

	_, err = os.Stat(path + ".bad")
	require.ErrorIs(t, err, os.ErrNotExist, "hotaru moved the user's file aside")
}

func TestReadingRulesWritesNothingAtAll(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hotaru.yml")
	require.NoError(t, os.WriteFile(path, []byte(exampleRules), 0o600))

	_, _, err := config.Load(path)
	require.NoError(t, err)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "reading the rules created a file")
}

func TestScopeIsNarrowedNeverEnabled(t *testing.T) {
	cfg, _, err := config.Load(write(t, "scope: [kraken]\n"))
	require.NoError(t, err)
	require.True(t, cfg.InScope("NZXT Kraken"))
	require.False(t, cfg.InScope("Keychron K4 HE"))

	// A rule on a device outside scope does not bring it back in: rules
	// correct behaviour, they do not select devices.
	cfg, _, err = config.Load(write(t, "scope: [kraken]\ndevices:\n  - match: keychron\n    never_blank: true\n"))
	require.NoError(t, err)
	require.False(t, cfg.InScope("Keychron K4 HE"))
}

func TestTheShippedExampleParsesAndMeansWhatItSays(t *testing.T) {
	// The example is a real machine's file, and a documented example that does
	// not load is worse than none: someone copies it, it fails, and the
	// program looks broken rather than the sample.
	cfg, problems, err := config.Load(filepath.Join("..", "..", "examples", "hotaru.yml"))
	require.NoError(t, err)
	require.Empty(t, problems)

	board := cfg.RulesFor("ASUS ROG MAXIMUS Z790 HERO")
	require.Len(t, board, 1)
	require.Equal(t, []string{"direct", "static"}, board[0].SolidModes)
	require.Equal(t, "Addressable RGB Header 2", board[0].Segments["front-top"].Zone)
	require.Equal(t, 0, board[0].Segments["front-top"].LEDs.First)
	require.Equal(t, 8, board[0].Segments["front-bot"].LEDs.Count())
	require.Nil(t, board[0].Segments["rear"].LEDs, "a whole zone needs no range")

	// The three radiator fans, daisy-chained into one channel of the cooler.
	cooler := cfg.RulesFor("NZXT Kraken 2024 ELITE Series RGB")
	require.Len(t, cooler, 1)
	for name, first := range map[string]int{"rad-rear": 0, "rad-mid": 8, "rad-front": 16} {
		segment := cooler[0].Segments[name]
		require.Equal(t, "Hue 2 Channel 2", segment.Zone, name)
		require.Equal(t, first, segment.LEDs.First, name)
		require.Equal(t, 8, segment.LEDs.Count(), name)
	}
	require.NotContains(t, cooler[0].Segments, "channel-1",
		"a segment that addresses nothing is a scene silently doing less than it says")

	require.True(t, cfg.RulesFor("Keychron K4 HE")[0].NeverBlank)
	require.Equal(t, time.Minute, cfg.RulesFor("G502 X PLUS")[0].Reassert.Duration())

	// And with the example loaded, everything is still in scope: the file
	// corrects behaviour, it does not select devices.
	require.True(t, cfg.InScope("Some Device Nobody Mentioned"))
}
