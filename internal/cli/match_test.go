package cli_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/hotaru/internal/api"
	"github.com/ushineko/hotaru/internal/canvas/canvastest"
	"github.com/ushineko/hotaru/internal/cli"
	"github.com/ushineko/hotaru/internal/config"
)

/*
light match runs the effect on a device and the device it follows, and keeps
the pace somebody settled on in the rules (spec 077 R3.9).
*/
func TestMatchWritesThePaceItWasLeftAt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	rules := filepath.Join(home, "hotaru", "hotaru.yml")
	require.NoError(t, os.MkdirAll(filepath.Dir(rules), 0o700))
	require.NoError(t, os.WriteFile(rules, []byte("devices:\n  - match: maximus\n    follow: kraken\n    lengths: {\"Addressable 1\": 4}\n"), 0o600))

	cooler := canvastest.NewFirmware(canvastest.NewBoard("NZXT Kraken Elite V2", canvastest.Ring("Ring", 24)), "Fading", "Tai Chi")
	board := canvastest.NewBoard("ASUS ROG MAXIMUS Z790 HERO", canvastest.Zone("Addressable 1", 4))
	cfg, _, err := config.Load(rules)
	require.NoError(t, err)
	client := api.NewClient(drawing(t, cfg, cooler, board))

	person := &scripted{answers: []string{"s", "s", "", "y"}}
	require.NoError(t, cli.Match(t.Context(), client, person, "maximus", "Fading", "", "red"))

	written, _, err := config.Load(rules)
	require.NoError(t, err)
	rule := written.RulesFor("ASUS ROG MAXIMUS Z790 HERO")[0]
	require.Equal(t, map[string]float64{"Fading": 1.21}, rule.Pace, "two slower is 1.1 squared")
	require.Equal(t, "kraken", rule.Follow, "the rest of the rule was not kept")
	require.Equal(t, map[string]int{"Addressable 1": 4}, rule.Lengths)
	running, _ := cooler.Running()
	require.Equal(t, "Fading", running, "the leader did not run the effect")
}

// A device that follows nothing needs --with.
func TestMatchAsksWhatToMatchAgainst(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	client := api.NewClient(drawing(t, nil, canvastest.NewBoard("ASUS ROG MAXIMUS Z790 HERO", canvastest.Zone("Addressable 1", 4))))
	err := cli.Match(t.Context(), client, &scripted{}, "maximus", "Fading", "", "red")
	require.ErrorContains(t, err, "--with")
}
