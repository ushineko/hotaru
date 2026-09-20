package cli_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/api"
	"github.com/ushineko/hotaru/internal/cli"
	"github.com/ushineko/hotaru/internal/devices"
	"github.com/ushineko/hotaru/internal/openrgb"
)

/*
scripted is a person who has already decided what they will say.

The wizard's whole quality is the wording and ordering of its questions, so the
test asserts the conversation rather than the outcome alone: what was asked, in
what order, and what the machine was doing when each question was put.
*/
type scripted struct {
	mu      sync.Mutex
	answers []string
	asked   []string
	said    []string
	at      int
}

func (s *scripted) Say(format string, args ...any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.said = append(s.said, strings.TrimSpace(fmt.Sprintf(format, args...)))
}

func (s *scripted) next(question string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.asked = append(s.asked, question)
	if s.at >= len(s.answers) {
		return ""
	}
	answer := s.answers[s.at]
	s.at++
	return answer
}

func (s *scripted) Ask(question string) (string, error) { return s.next(question), nil }

func (s *scripted) Confirm(question string) (bool, error) {
	answer := strings.ToLower(s.next(question))
	return answer == "y" || answer == "yes", nil
}

func (s *scripted) Count(question string) (int, error) {
	answer := strings.TrimSpace(s.next(question))
	switch answer {
	case "", "1":
		return 1, nil
	case "2":
		return 2, nil
	case "3":
		return 3, nil
	}
	return 1, nil
}

func (s *scripted) Choose(question string, options []string) (string, error) {
	answer := strings.ToLower(s.next(question))
	for _, option := range options {
		if strings.HasPrefix(option, answer) && answer != "" {
			return option, nil
		}
	}
	return options[0], nil
}

// questions is what the person was actually asked, which is the part whose
// wording matters. What the wizard says along the way is reported separately.
func (s *scripted) questions() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.Join(s.asked, "\n")
}

// cooler is the development machine's, near enough: two channels of 24, one
// carrying three daisy-chained fans and one carrying nothing.
func cooler() devices.Device {
	return devices.Device{
		Name:     "NZXT Kraken 2024 ELITE Series RGB",
		LEDCount: 48,
		Modes: []devices.Mode{
			{Name: "Static", PerLED: false},
			{Name: "Direct", PerLED: true},
		},
		Zones: []devices.Zone{
			{Name: "Hue 2 Channel 1", First: 0, Count: 24},
			{Name: "Hue 2 Channel 2", First: 24, Count: 24},
		},
		ActiveMode: "Direct",
	}
}

func TestTheWizardMapsAMachineFromWhatAPersonCanSee(t *testing.T) {
	server := openrgb.NewFake(cooler())
	socket := serving(t, nil, server)
	client := api.NewClient(socket)

	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))

	person := &scripted{answers: []string{
		"y",         // is anything lit? -- yes, so the mode hotaru chose is fine
		"nothing",   // what is red? -- channel 1 has nothing on it
		"radiator",  // what is green? -- channel 2
		"3",         // how many separate lights on it
		"y",         // each shows one colour
		"rad rear",  // what is red
		"rad mid",   // green
		"rad front", // blue
		"y",         // is the whole map right
		"y",         // write it
	}}

	require.NoError(t, cli.Map(t.Context(), client, person))

	// The file the conversation produced.
	rules, err := os.ReadFile(filepath.Join(home, "config", "hotaru", "hotaru.yml"))
	require.NoError(t, err)
	written := string(rules)

	require.Contains(t, written, "match: kraken", "the shortest word that identifies it")
	require.Contains(t, written, `rad-rear: {zone: "Hue 2 Channel 2", leds: [0, 7]}`)
	require.Contains(t, written, `rad-mid: {zone: "Hue 2 Channel 2", leds: [8, 15]}`)
	require.Contains(t, written, `rad-front: {zone: "Hue 2 Channel 2", leds: [16, 23]}`)
	require.NotContains(t, written, "Hue 2 Channel 1",
		"a zone with nothing on it was named anyway")

	// And the questions were the ones a person can answer by looking.
	asked := person.questions()
	require.Contains(t, asked, "What is red?")
	require.Contains(t, asked, "How many separate lights are on radiator?")
	require.NotContains(t, asked, "LED",
		"a question asked somebody to count LEDs, which is the thing nobody does twice")
	require.NotRegexp(t, `\[\d+:\d+\]`, asked,
		"a question quoted an LED range at a person, which is the notation the names exist to replace")
}

func TestTheWizardAsksHowManyBeforeItSplitsAnything(t *testing.T) {
	// The vendors' question, and the right one: a daisy-chain is usually
	// identical fans, so one answer and the LED count give the division.
	server := openrgb.NewFake(cooler())
	client := api.NewClient(serving(t, nil, server))
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	person := &scripted{answers: []string{"y", "nothing", "strip", "1", "y", "n"}}
	require.NoError(t, cli.Map(t.Context(), client, person))

	asked := person.questions()
	require.Contains(t, asked, "How many separate lights are on strip?")
	require.NotContains(t, asked, "exactly one light",
		"it bisected a chain it had been told was one thing")
}

func TestAZoneReportedDarkIsLeftUnnamed(t *testing.T) {
	// Three of the development machine's nine zones have nothing attached,
	// and every one accepts writes and reports success. Naming one would make
	// a scene that silently does less than it says.
	server := openrgb.NewFake(cooler())
	client := api.NewClient(serving(t, nil, server))
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	person := &scripted{answers: []string{"y", "nothing", "nothing"}}
	require.NoError(t, cli.Map(t.Context(), client, person))
	require.Contains(t, strings.Join(person.said, "\n"), "nothing to write")
}

func TestAnUnhealthyMachineIsNotAskedAboutAtAll(t *testing.T) {
	client := api.NewClient(serving(t, nil, nil))
	person := &scripted{}

	err := cli.Map(t.Context(), client, person)
	require.Error(t, err)
	require.True(t, cli.Silent(err))
	require.Empty(t, person.asked, "a machine hotaru cannot see was still asked about")
	require.Contains(t, strings.Join(person.said, "\n"), "unreachable")
}

func TestNothingIsWrittenWithoutASayingSo(t *testing.T) {
	server := openrgb.NewFake(cooler())
	client := api.NewClient(serving(t, nil, server))
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)

	// The map is right, and the user declines to write it.
	person := &scripted{answers: []string{"y", "nothing", "strip", "1", "y", "n"}}
	require.NoError(t, cli.Map(t.Context(), client, person))

	_, err := os.Stat(filepath.Join(home, "hotaru", "hotaru.yml"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestAnExistingRulesFileIsPrintedAtRatherThanEdited(t *testing.T) {
	// The file is the user's. A wizard that rewrote it would lose their
	// comments and their trust in one stroke.
	server := openrgb.NewFake(cooler())
	client := api.NewClient(serving(t, nil, server))

	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	path := filepath.Join(home, "hotaru", "hotaru.yml")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	original := "# mine\ndevices: []\n"
	require.NoError(t, os.WriteFile(path, []byte(original), 0o600))

	person := &scripted{answers: []string{"y", "nothing", "strip", "1", "y"}}
	require.NoError(t, cli.Map(t.Context(), client, person))

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, original, string(after), "the user's file was edited")
	require.Contains(t, strings.Join(person.said, "\n"), "It is yours")
}

func TestTheRuleNamesADeviceTheWayAPersonWould(t *testing.T) {
	// Picking a word by position does not work: the second word of
	// "ASUS ROG MAXIMUS Z790 HERO" is "rog", which names a brand rather than
	// a device, and the first word of the cooler is its vendor. The word is
	// chosen against the machine instead -- the longest one that matches this
	// device and no other present.
	server := openrgb.NewFake(cooler(), devices.Device{
		Name:     "ASUS ROG MAXIMUS Z790 HERO",
		LEDCount: 16,
		Modes:    []devices.Mode{{Name: "Direct", PerLED: true}},
		Zones:    []devices.Zone{{Name: "Addressable RGB Header 2", First: 0, Count: 16}},
	})
	client := api.NewClient(serving(t, nil, server))
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)

	person := &scripted{answers: []string{
		"y", "nothing", "radiator", "1", // the cooler lights; its two channels
		"y", "front", "1", // the board lights; its header
		"y", "y",
	}}
	require.NoError(t, cli.Map(t.Context(), client, person))

	rules, err := os.ReadFile(filepath.Join(home, "hotaru", "hotaru.yml"))
	require.NoError(t, err)
	require.Contains(t, string(rules), "match: maximus", "not rog, and not asus")
	require.Contains(t, string(rules), "match: kraken", "not nzxt, and not series")
}

func TestTheWizardFindsAModeThatActuallyLightsTheDevice(t *testing.T) {
	/*
		The failure this exists for, and the one the author walked into while
		rehearsing: hotaru's default order picks Static for an ASUS board, the
		write is accepted, the mode is reported back, and the addressable
		headers stay dark. A user mapping that machine would answer "nothing"
		to every zone and map half their case as empty.

		No read-back can catch it, because the mode did change. Only a person
		looking at the machine can, so the wizard asks first.
	*/
	board := devices.Device{
		Name:     "ASUS ROG MAXIMUS Z790 HERO",
		LEDCount: 16,
		Modes: []devices.Mode{
			{Name: "Static", PerLED: true},
			{Name: "Direct", PerLED: true},
		},
		Zones:      []devices.Zone{{Name: "Addressable RGB Header 2", First: 0, Count: 16}},
		ActiveMode: "Rainbow Wave",
	}
	client := api.NewClient(serving(t, nil, openrgb.NewFake(board)))
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)

	person := &scripted{answers: []string{
		"n",     // lit in Static? -- no: the headers are dark and nothing says so
		"y",     // lit in Direct? -- yes
		"front", // what is red
		"1",
		"y", // the map is right
		"y", // write it
	}}
	require.NoError(t, cli.Map(t.Context(), client, person))

	rules, err := os.ReadFile(filepath.Join(home, "hotaru", "hotaru.yml"))
	require.NoError(t, err)
	require.Contains(t, string(rules), "solid_modes: [direct, static]",
		"the correction a read-back could never have found")
	require.Contains(t, string(rules), "only lights in direct on this machine",
		"and why it is there")

	asked := person.questions()
	require.Contains(t, asked, "Is it lit red now? (Static)")
	require.Contains(t, asked, "Is it lit red now? (Direct)",
		"each mode is asked about on its own; the first run elsewhere asked about Direct four times")
}

func TestADeviceThatLightsInNoModeIsSkippedRatherThanMapped(t *testing.T) {
	dark := devices.Device{
		Name:     "Mystery Controller",
		LEDCount: 8,
		Modes:    []devices.Mode{{Name: "Static", PerLED: true}, {Name: "Direct", PerLED: true}},
		Zones:    []devices.Zone{{Name: "Header", First: 0, Count: 8}},
	}
	client := api.NewClient(serving(t, nil, openrgb.NewFake(dark)))
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	person := &scripted{answers: []string{"n", "n"}}
	require.NoError(t, cli.Map(t.Context(), client, person))

	said := strings.Join(person.said, "\n")
	require.Contains(t, said, "Nothing lit in Static or Direct",
		"a device that would not light should say what was already ruled out")
	require.Contains(t, said, "nothing to write")
	require.NotContains(t, person.questions(), "What is red?",
		"a device nobody can see was asked about anyway")
}

func TestAnAnswerToADifferentQuestionIsNotTakenAsAName(t *testing.T) {
	// "y" is not the name of a fan. It is what somebody types when they think
	// they are being asked something else, and writing it into their rules
	// file would turn a slip into a puzzle they meet weeks later.
	client := api.NewClient(serving(t, nil, openrgb.NewFake(cooler())))
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)

	person := &scripted{answers: []string{
		"y",        // is anything lit
		"yes",      // what is red? -- an answer to a different question
		"nothing",  // asked again
		"red",      // what is green? -- the colour, not the thing
		"radiator", // asked again
		"1", "y", "y",
	}}
	require.NoError(t, cli.Map(t.Context(), client, person))

	said := strings.Join(person.said, "\n")
	require.Contains(t, said, "answer to a different question")
	require.Contains(t, said, "is the colour it is lit")

	rules, err := os.ReadFile(filepath.Join(home, "hotaru", "hotaru.yml"))
	require.NoError(t, err)
	require.NotContains(t, string(rules), "yes:", "a slip became a segment name")
	require.NotContains(t, string(rules), "red:", "a colour became a segment name")
	require.Contains(t, string(rules), "radiator:")
}

func TestOneDeviceAtATime(t *testing.T) {
	// Six devices is a long conversation to hold in your head, and a person
	// who has just learned something about their cooler should be able to
	// write it down without answering for the mousepad first.
	server := openrgb.NewFake(cooler(), devices.Device{
		Name:     "Corsair MM700",
		LEDCount: 3,
		Modes:    []devices.Mode{{Name: "Direct", PerLED: true}},
		Zones:    []devices.Zone{{Name: "Left", First: 0, Count: 3}},
	})
	client := api.NewClient(serving(t, nil, server))
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	person := &scripted{answers: []string{"y", "nothing", "radiator", "1", "y", "n"}}
	require.NoError(t, cli.Map(t.Context(), client, person, "kraken"))

	require.NotContains(t, strings.Join(person.said, "\n"), "MM700",
		"a device nobody asked about was mapped anyway")
}

func TestThingsOnOneControlAreNotSplit(t *testing.T) {
	// A 12V header is one control for everything plugged into it. Two strips
	// on a splitter are two strips and one LED, and dividing that LED between
	// them would report a boundary that could not be settled -- which sounds
	// like a fault rather than the plain fact it is.
	board := devices.Device{
		Name:       "Z790 AORUS MASTER X",
		LEDCount:   1,
		Modes:      []devices.Mode{{Name: "Direct", PerLED: true}},
		Zones:      []devices.Zone{{Name: "LED_C", First: 0, Count: 1}},
		ActiveMode: "Direct",
	}
	client := api.NewClient(serving(t, nil, openrgb.NewFake(board)))
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)

	person := &scripted{answers: []string{
		"y",           // it lights
		"desk strips", // what is red
		"2",           // there are two of them
		"y",           // the map is right
		"y",           // write it
	}}
	require.NoError(t, cli.Map(t.Context(), client, person))

	require.Contains(t, strings.Join(person.said, "\n"), "controlled together")
	require.NotContains(t, person.questions(), "exactly one light",
		"it tried to find a boundary between two things sharing one control")

	rules, err := os.ReadFile(filepath.Join(home, "hotaru", "hotaru.yml"))
	require.NoError(t, err)
	require.Contains(t, string(rules), `desk-strips: {zone: "LED_C"}`,
		"the pair is named once, as the one thing it can be set as")
}
