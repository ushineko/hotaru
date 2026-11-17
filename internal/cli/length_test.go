package cli_test

import (
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
