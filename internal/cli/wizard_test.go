package cli_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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
	// Anything that is not a number is one, as the terminal does it.
	n, _ := strconv.Atoi(answer)
	if n < 1 {
		return 1, nil
	}
	return n, nil
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
		"3",         // how many things are chained on it
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
	require.Contains(t, asked, "How many separate things are chained on radiator?")
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
	require.Contains(t, asked, "How many separate things are chained on strip?")
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

func TestAReRunOffersTheAnswersFromLastTime(t *testing.T) {
	/*
		Editing a file somebody maintains is only rude when it happens behind
		their back. Read the existing answers, offer them as the defaults,
		write the result back: it is how every installer and every
		--reconfigure works, because it is how a person changes one thing
		without restating the other nine.
	*/
	client := api.NewClient(serving(t, nil, openrgb.NewFake(cooler())))
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)

	path := filepath.Join(home, "hotaru", "hotaru.yml")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(`
devices:
  - match: kraken
    segments:
      radiator: {zone: "Hue 2 Channel 2"}
  - match: something-else
    never_blank: true
`), 0o600))

	person := &scripted{answers: []string{
		"y",       // it lights
		"nothing", // channel 1
		"",        // channel 2: press return to keep what it was called
		"1",
		"y", // the map is right
		"y", // write it
	}}
	require.NoError(t, cli.Map(t.Context(), client, person))

	asked := person.questions()
	require.Contains(t, asked, "[radiator]", "the answer from last time was not offered back")

	written, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(written), `radiator: {zone: "Hue 2 Channel 2"}`,
		"pressing return did not keep the name")
	require.Contains(t, string(written), "match: something-else",
		"a rule about a device this run never asked about was lost")

	// And what was there before is kept, because regenerating the file loses
	// anything hotaru does not model -- comments included.
	backup, err := os.ReadFile(path + ".bak")
	require.NoError(t, err)
	require.Contains(t, string(backup), "something-else")

	said := strings.Join(person.said, "\n")
	require.Contains(t, said, "any comments in it are lost",
		"the cost of regenerating was not stated before it was paid")
}

func TestDecliningLeavesTheExistingFileExactlyAsItWas(t *testing.T) {
	client := api.NewClient(serving(t, nil, openrgb.NewFake(cooler())))
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)

	path := filepath.Join(home, "hotaru", "hotaru.yml")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	original := "# mine, with a comment\ndevices: []\n"
	require.NoError(t, os.WriteFile(path, []byte(original), 0o600))

	person := &scripted{answers: []string{"y", "nothing", "strip", "1", "y", "n"}}
	require.NoError(t, cli.Map(t.Context(), client, person))

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, original, string(after), "a declined write changed the file anyway")

	_, err = os.Stat(path + ".bak")
	require.ErrorIs(t, err, os.ErrNotExist, "a declined write left a backup of nothing")
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

func TestACountThatMeantLEDsIsQueriedRatherThanDividedOn(t *testing.T) {
	/*
		Real confusion from a real machine: asked "how many separate lights are
		on first-stick", its owner answered 10, meaning the LEDs they could
		count. Twelve LEDs across ten things is about one LED each, which is
		not a thing anybody has, so it is worth asking again before dividing
		the stick into ten pieces.
	*/
	stick := devices.Device{
		Name:       "Corsair Dominator Platinum RGB DDR5 (0x18)",
		LEDCount:   12,
		Modes:      []devices.Mode{{Name: "Direct", PerLED: true}},
		Zones:      []devices.Zone{{Name: "RAM", First: 0, Count: 12}},
		ActiveMode: "Direct",
	}
	client := api.NewClient(serving(t, nil, openrgb.NewFake(stick)))
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)

	person := &scripted{answers: []string{
		"y",           // it lights
		"first stick", // what is red
		"10",          // how many things -- meaning LEDs
		"1",           // asked again, with the arithmetic shown
		"y", "y",      // right, and write it
	}}
	require.NoError(t, cli.Map(t.Context(), client, person))

	said := strings.Join(person.said, "\n")
	require.Contains(t, said, "about one LED each")
	require.Contains(t, said, "the answer here is 1")

	rules, err := os.ReadFile(filepath.Join(home, "hotaru", "hotaru.yml"))
	require.NoError(t, err)
	require.Contains(t, string(rules), `first-stick: {zone: "RAM"}`,
		"the stick was named once rather than divided into ten")
	require.NotContains(t, string(rules), "leds:", "a range was invented for a single thing")
}
