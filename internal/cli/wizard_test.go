package cli_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/api"
	"github.com/ushineko/hotaru/internal/cli"
	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/config"
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
		"n", "y",    // write it
	}}

	require.NoError(t, cli.Map(t.Context(), client, person))

	// The file the conversation produced.
	rules, err := os.ReadFile(filepath.Join(home, "config", "hotaru", "hotaru.yml"))
	require.NoError(t, err)
	written := string(rules)

	require.Contains(t, written, `match: "kraken"`, "the shortest word that identifies it")
	require.Contains(t, written, `rad-rear: {zone: "Hue 2 Channel 2", leds: [0, 7]}`)
	require.Contains(t, written, `rad-mid: {zone: "Hue 2 Channel 2", leds: [8, 15]}`)
	require.Contains(t, written, `rad-front: {zone: "Hue 2 Channel 2", leds: [16, 23]}`)
	require.NotContains(t, written, "Hue 2 Channel 1",
		"a zone with nothing on it was named anyway")

	// And the questions were the ones a person can answer by looking.
	asked := person.questions()
	require.Contains(t, asked, "What is lit now?")
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
		"y",      // the map is right
		"n", "y", // write it
	}}
	require.NoError(t, cli.Map(t.Context(), client, person))

	asked := person.questions()
	require.Contains(t, asked, "[radiator]", "the answer from last time was not offered back")

	written, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(written), `radiator: {zone: "Hue 2 Channel 2"}`,
		"pressing return did not keep the name")
	require.Contains(t, string(written), `match: "something-else"`,
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
		"1", "y", "n", "y",
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
		"y", "n", "y", // right, and write it
	}}
	require.NoError(t, cli.Map(t.Context(), client, person))

	said := strings.Join(person.said, "\n")
	require.Contains(t, said, "about one light each")
	require.Contains(t, said, "the answer here is 1")

	rules, err := os.ReadFile(filepath.Join(home, "hotaru", "hotaru.yml"))
	require.NoError(t, err)
	require.Contains(t, string(rules), `first-stick: {zone: "RAM"}`,
		"the stick was named once rather than divided into ten")
	require.NotContains(t, string(rules), "leds:", "a range was invented for a single thing")
}

func TestWhatTheWizardWritesLoads(t *testing.T) {
	/*
		A file that does not load is worse than no file: the program comes up
		reporting a problem with something the user never typed.

		"match: 0x18" shipped exactly that way. It is a perfectly good way to
		tell four identical sticks of RAM apart, and a hexadecimal number to
		YAML, and the loader refused the whole file -- so the program came up
		complaining about something its user never typed. What the wizard
		writes goes back through the loader here.
	*/
	sticks := []devices.Device{}
	for _, address := range []string{"0x18", "0x19"} {
		sticks = append(sticks, devices.Device{
			Name:       "Corsair Dominator Platinum RGB DDR5 (" + address + ")",
			LEDCount:   12,
			Modes:      []devices.Mode{{Name: "Direct", PerLED: true}},
			Zones:      []devices.Zone{{Name: "Corsair DRAM", First: 0, Count: 12}},
			ActiveMode: "Direct",
		})
	}
	client := api.NewClient(serving(t, nil, openrgb.NewFake(sticks...)))
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)

	person := &scripted{answers: []string{
		"y", "stick0", "1", // the first stick
		"y", "stick1", "1", // the second
		"y", "n", "y",
	}}
	require.NoError(t, cli.Map(t.Context(), client, person))

	path := filepath.Join(home, "hotaru", "hotaru.yml")
	cfg, problems, err := config.Load(path)
	require.NoError(t, err, "the wizard wrote a file its own loader rejects")
	require.Empty(t, problems, "the wizard wrote a file its own loader complains about")

	require.Len(t, cfg.RulesFor("Corsair Dominator Platinum RGB DDR5 (0x18)"), 1)
	require.Len(t, cfg.RulesFor("Corsair Dominator Platinum RGB DDR5 (0x19)"), 1)
}

func TestStoppingPutsTheLightsBackAndSaysNothingElse(t *testing.T) {
	/*
		Ctrl-C in the middle of a question used to produce two lines about a
		URL the user never typed, and leave the lights on whatever the wizard
		last lit -- because the cleanup used the context that had just been
		cancelled.
	*/
	server := openrgb.NewFake(cooler())
	socket := serving(t, nil, server)
	client := api.NewClient(socket)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	// Something to put back, so restoring has work to do.
	_, err := client.Apply(t.Context(), api.ApplyRequest{Colour: "teal"})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(t.Context())
	person := &stopper{cancel: cancel}

	err = cli.Map(ctx, client, person)
	require.ErrorIs(t, err, context.Canceled, "a cancelled wizard reported something else")

	require.Eventually(t, func() bool {
		showing, ok := server.Showing("NZXT Kraken 2024 ELITE Series RGB")
		return ok && showing.Colours[0] == colour.MustParse("teal")
	}, 3*time.Second, 20*time.Millisecond,
		"the lights were left on whatever the wizard had lit")

	require.NotContains(t, strings.Join(person.said, "\n"), "Could not put the lights back")
}

// stopper answers the first question by walking away, as Ctrl-C does.
type stopper struct {
	scripted
	cancel context.CancelFunc
}

func (s *stopper) Confirm(question string) (bool, error) {
	s.next(question)
	s.cancel()
	return false, context.Canceled
}

func TestWhatTheWizardLightsIsNotWhatTheMachineWants(t *testing.T) {
	/*
		The wizard's colours are questions, not intentions. Recording them as
		desired state meant "put the lights back" put the questions back --
		and a reboot would have restored the last thing the wizard happened to
		be lighting when somebody walked away.
	*/
	server := openrgb.NewFake(cooler())
	socket := serving(t, nil, server)
	client := api.NewClient(socket)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	_, err := client.Apply(t.Context(), api.ApplyRequest{Colour: "teal"})
	require.NoError(t, err)

	before, err := client.Status(t.Context())
	require.NoError(t, err)
	require.NotEmpty(t, before.Remembered)

	person := &scripted{answers: []string{"y", "nothing", "radiator", "1", "n"}}
	require.NoError(t, cli.Map(t.Context(), client, person))

	// The machine still wants what it wanted: teal, not the wizard's red.
	showing, ok := server.Showing("NZXT Kraken 2024 ELITE Series RGB")
	require.True(t, ok)
	require.Equal(t, colour.MustParse("teal"), showing.Colours[0],
		"the lights were left showing a question")
}

func TestAZoneNobodyCanSeeIsLeftOutRatherThanBisected(t *testing.T) {
	/*
		A dead end somebody actually hit: told there were two things on an
		empty header, the wizard asked whether each showed one colour, was told
		no, and went hunting for a boundary -- asking questions about lights
		that were not there, with no answer that would end it.

		Nothing lit means nothing on it. That is one question, and it comes
		before the hunt.
	*/
	board := devices.Device{
		Name:       "Z790 AORUS MASTER X",
		LEDCount:   30,
		Modes:      []devices.Mode{{Name: "Direct", PerLED: true}},
		Zones:      []devices.Zone{{Name: "ARGB_V2_1", First: 0, Count: 30}},
		ActiveMode: "Direct",
	}
	client := api.NewClient(serving(t, nil, openrgb.NewFake(board)))
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	person := &scripted{answers: []string{
		"y",         // the device lights
		"rgbstrip0", // named from what the probe lit
		"2",         // two things on it
		"n",         // each shows one colour? no -- nothing is showing
		"n",         // can you see it lit at all? no
	}}
	require.NoError(t, cli.Map(t.Context(), client, person))

	said := strings.Join(person.said, "\n")
	require.Contains(t, said, "Nothing on it, then")
	require.NotContains(t, person.questions(), "exactly one thing",
		"it went looking for a boundary on a header with nothing attached")
	require.Contains(t, said, "nothing to write")
}

func TestTheQuestionAsksForANameRatherThanAColour(t *testing.T) {
	// "What is red?" asks somebody to identify a colour. They are naming a
	// thing, and the difference is the whole readability of the conversation.
	client := api.NewClient(serving(t, nil, openrgb.NewFake(cooler())))
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	person := &scripted{answers: []string{"y", "nothing", "radiator", "1", "n"}}
	require.NoError(t, cli.Map(t.Context(), client, person))

	asked := person.questions()
	require.Contains(t, asked, "What is lit now?")
	require.NotContains(t, asked, "red one",
		"it asked somebody to match a colour, which another device in the case can also be")
	require.Contains(t, strings.Join(person.said, "\n"), "2 parts that light separately",
		"the change from 'does it light' to 'which is which' was not announced")
}

func TestAOneLightZoneIsNotAskedAboutDividing(t *testing.T) {
	/*
		A 12V header is one control signal: ten strips chained onto it are one
		colour and one thing to name. Asked how many things were on theirs,
		somebody answered 2 -- true about their desk, and not a question with
		an actionable answer, since there is nothing to divide.
	*/
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

	person := &scripted{answers: []string{"y", "desk strips", "y", "n", "y"}}
	require.NoError(t, cli.Map(t.Context(), client, person))

	require.NotContains(t, person.questions(), "chained",
		"somebody was asked to divide a zone with one light in it")

	rules, err := os.ReadFile(filepath.Join(home, "hotaru", "hotaru.yml"))
	require.NoError(t, err)
	require.Contains(t, string(rules), `desk-strips: {zone: "LED_C"}`)
}

func TestNothingElseIsLitWhileAQuestionIsBeingAsked(t *testing.T) {
	/*
		Lighting every part of a device at once, each a different colour, and
		asking which was which: somebody was asked to name "the red one" while
		the only thing they could see was blue, because two of that device's
		three parts had nothing attached. And "the green one" could have meant
		a stick of RAM, because the rest of the case was lit too.

		One thing lit, everything else dark, and the question is what came on.
	*/
	board := devices.Device{
		Name:     "Z790 AORUS MASTER X",
		LEDCount: 61,
		Modes:    []devices.Mode{{Name: "Direct", PerLED: true}},
		Zones: []devices.Zone{
			{Name: "ARGB_V2_1", First: 0, Count: 30},
			{Name: "ARGB_V2_2", First: 30, Count: 30},
			{Name: "LED_C", First: 60, Count: 1},
		},
		ActiveMode: "Direct",
	}
	server := openrgb.NewFake(board, cooler())
	client := api.NewClient(serving(t, nil, server))
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)

	person := &scripted{answers: []string{
		"y", // the board lights
		"",  // first header: nothing attached
		"",  // second header: nothing attached
		"desk strips",
		"y", "n", "y",
	}}
	require.NoError(t, cli.Map(t.Context(), client, person, "AORUS"))

	// The other device in the case was turned off before any question.
	showing, ok := server.Showing("NZXT Kraken 2024 ELITE Series RGB")
	require.True(t, ok)
	require.Equal(t, colour.Black, showing.Colours[0],
		"another device stayed lit while somebody was asked what they could see")

	// And each question was about one part, in turn.
	asked := person.questions()
	require.Equal(t, 3, strings.Count(asked, "What is lit now?"),
		"three parts, one question each")

	rules, err := os.ReadFile(filepath.Join(home, "hotaru", "hotaru.yml"))
	require.NoError(t, err)
	require.Contains(t, string(rules), `desk-strips: {zone: "LED_C"}`)
	require.NotContains(t, string(rules), "ARGB_V2_1", "an empty header was named")
}

func TestTheWizardSpeaksNoJargon(t *testing.T) {
	/*
		Nothing it says should require knowing how any of this works. A person
		mapping their case knows about strips, fans and sticks; "zone", "LED"
		and the name of a lighting mode are this program's vocabulary, and
		using them asks somebody to learn it before they can answer.
	*/
	board := devices.Device{
		Name:     "Z790 AORUS MASTER X",
		LEDCount: 31,
		Modes:    []devices.Mode{{Name: "Static", PerLED: true}, {Name: "Direct", PerLED: true}},
		Zones: []devices.Zone{
			{Name: "ARGB_V2_1", First: 0, Count: 30},
			{Name: "LED_C", First: 30, Count: 1},
		},
		ActiveMode: "Rainbow Wave",
	}
	client := api.NewClient(serving(t, nil, openrgb.NewFake(board)))
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	person := &scripted{answers: []string{
		"n", "y", // not lit the first way, lit the second
		"", "desk strips", // nothing on the first part; the strips on the second
		"y", "y",
	}}
	require.NoError(t, cli.Map(t.Context(), client, person))

	// What it says, not what it shows: the file it prints for approval is a
	// configuration file, and its keys are necessarily this program's words.
	var spoken strings.Builder
	spoken.WriteString(strings.ToLower(person.questions()))
	for _, line := range person.said {
		if strings.Contains(line, "devices:") {
			continue
		}
		spoken.WriteString("\n" + strings.ToLower(line))
	}
	said := spoken.String()
	for _, jargon := range []string{"zone", " led", "leds", "rgb mode", "argb", "controller", "protocol"} {
		require.NotContains(t, said, jargon,
			"the wizard used a word somebody has to learn before they can answer")
	}
}

func TestTheProbeAsksAboutTheWayAWriteWillActuallyLightIt(t *testing.T) {
	/*
		A board that lights in Direct and not in Static, whose own list puts
		Direct first. Asked in the device's order, somebody sees red on the
		first try, says yes, and no correction is written -- and then setting a
		colour picks Static, as hotaru's preference order always would, and
		their strips go out. The probe has to ask about what a write will use.
	*/
	board := devices.Device{
		Name:     "Z790 AORUS MASTER X",
		LEDCount: 1,
		Modes: []devices.Mode{
			{Name: "Direct", PerLED: true}, // the device lists this one first
			{Name: "Static", PerLED: true},
		},
		Zones:      []devices.Zone{{Name: "LED_C", First: 0, Count: 1}},
		ActiveMode: "Rainbow Wave",
	}
	client := api.NewClient(serving(t, nil, openrgb.NewFake(board)))
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)

	person := &scripted{answers: []string{
		"n",           // Static, which hotaru would choose: nothing lights
		"y",           // Direct: there it is
		"desk strips", // and it is the strips
		"y", "n", "y",
	}}
	require.NoError(t, cli.Map(t.Context(), client, person))

	rules, err := os.ReadFile(filepath.Join(home, "hotaru", "hotaru.yml"))
	require.NoError(t, err)
	require.Contains(t, string(rules), "solid_modes: [direct, static]",
		"the correction was not written, so setting a colour will still pick the mode that lights nothing")
}

func TestWhatItWritesIsInUseBeforeItPutsTheLightsBack(t *testing.T) {
	/*
		The service reads its rules at startup, so a file the wizard writes
		changes nothing until it is told. Leaving that as an instruction meant
		the last act of a successful run was to put the lights back using the
		rules from before it ran -- and on one machine that meant the
		correction just established was ignored and the strips went out again.
	*/
	board := devices.Device{
		Name:     "Z790 AORUS MASTER X",
		LEDCount: 1,
		Modes: []devices.Mode{
			{Name: "Direct", PerLED: true},
			{Name: "Static", PerLED: true},
		},
		Zones:      []devices.Zone{{Name: "LED_C", First: 0, Count: 1}},
		ActiveMode: "Rainbow Wave",
	}
	// Static is accepted, reports Static, and lights nothing -- which no fake
	// can express, because it is a fact about a case and not about a
	// protocol. The person says so: "n" to Static, "y" to Direct.
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home) // before the service starts, as on a real machine

	server := openrgb.NewFake(board)
	client := api.NewClient(serving(t, nil, server))

	person := &scripted{answers: []string{"n", "y", "desk strips", "y", "n", "y"}}
	require.NoError(t, cli.Map(t.Context(), client, person))

	require.Contains(t, strings.Join(person.said, "\n"), "in use now",
		"somebody was told to run a command the program could run itself")

	// The proof: a write now picks the corrected mode without anyone
	// reloading anything.
	out, err := client.Apply(t.Context(), api.ApplyRequest{Colour: "purple"})
	require.NoError(t, err)
	require.Equal(t, "Direct", out.Results[0].Mode,
		"the correction the wizard had just established was not in use")
}

func TestOnlyALineOfLightsIsAskedAboutChaining(t *testing.T) {
	/*
		"How many separate things are chained on keyboard?" -- asked of a
		device with a hundred addressable keys, somebody quite reasonably
		wondered whether they were being asked about keys.

		They were not: the question is about separate objects sharing one
		connector, which only a line of lights can be. A keyboard is a grid,
		and a grid is one object however many lights it has.
	*/
	keyboard := devices.Device{
		Name:     "Keychron K4 HE",
		LEDCount: 100,
		Modes:    []devices.Mode{{Name: "Direct", PerLED: true}},
		Zones: []devices.Zone{
			{Name: "Keyboard", Shape: devices.ShapeGrid, First: 0, Count: 100},
		},
		ActiveMode: "Direct",
	}
	fans := devices.Device{
		Name:     "NZXT Kraken",
		LEDCount: 24,
		Modes:    []devices.Mode{{Name: "Direct", PerLED: true}},
		Zones: []devices.Zone{
			{Name: "Hue 2 Channel 2", Shape: devices.ShapeLine, First: 0, Count: 24},
		},
		ActiveMode: "Direct",
	}
	client := api.NewClient(serving(t, nil, openrgb.NewFake(keyboard, fans)))
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	person := &scripted{answers: []string{
		"y", "keyboard", // no chaining question follows a grid
		"y", "radiator", "1", // a line gets one
		"n", "n",
	}}
	require.NoError(t, cli.Map(t.Context(), client, person))

	asked := person.questions()
	require.NotContains(t, asked, "chained on keyboard",
		"somebody was asked how many things were chained on a keyboard")
	require.Contains(t, asked, "chained on radiator",
		"a line of lights is exactly where the question belongs")
}

// keyboardThatWillNotBlank is the Keychron: it advertises a reactive mode, and
// on the real board a dark frame is taken as "the host has stopped talking"
// and replaced by the firmware's own white.
func keyboardThatWillNotBlank() devices.Device {
	return devices.Device{
		Name:     "Keychron K4 HE",
		LEDCount: 2,
		Modes: []devices.Mode{
			{Name: "Direct", PerLED: true},
			{Name: "Solid Splash"},
		},
		Zones:      []devices.Zone{{Name: "Keyboard", Shape: devices.ShapeGrid, First: 0, Count: 2}},
		ActiveMode: "Direct",
	}
}

func TestADeviceThatWillNotGoDarkIsRecorded(t *testing.T) {
	/*
		`hotaru light off` left this board glowing white, and the wizard wrote
		a file with no trace of it. hotaru has always had never_blank; nothing
		ever asked the one question that sets it. Spec 014.
	*/
	client := api.NewClient(serving(t, nil, openrgb.NewFake(keyboardThatWillNotBlank())))
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)

	person := &scripted{answers: []string{
		"y",        // lit red?
		"keyboard", // what is lit
		"y",        // is that right?
		"y",        // anything still glowing?
		"y",        // the keyboard is
		"",         // leave the way it is lit alone
		"y",        // write it
	}}
	require.NoError(t, cli.Map(t.Context(), client, person))

	rules, err := os.ReadFile(filepath.Join(home, "hotaru", "hotaru.yml"))
	require.NoError(t, err)
	require.Contains(t, string(rules), "never_blank: true")
}

func TestADeviceThatGoesDarkIsNotWrittenAbout(t *testing.T) {
	// The other half: a file that claims a device was corrected when it was
	// not is worse than one that says nothing.
	client := api.NewClient(serving(t, nil, openrgb.NewFake(keyboardThatWillNotBlank())))
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)

	person := &scripted{answers: []string{"y", "keyboard", "y", "n", "y"}}
	require.NoError(t, cli.Map(t.Context(), client, person))

	rules, err := os.ReadFile(filepath.Join(home, "hotaru", "hotaru.yml"))
	require.NoError(t, err)
	require.NotContains(t, string(rules), "never_blank")
}

func TestTheAlternativeToBlankingIsOffered(t *testing.T) {
	// "Off" on a keyboard is a mode, not a colour: a reactive effect leaves
	// unpressed keys dark, which is what somebody means by off.
	client := api.NewClient(serving(t, nil, openrgb.NewFake(keyboardThatWillNotBlank())))
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)

	person := &scripted{answers: []string{
		"y", "keyboard",
		"y",      // is that right?
		"y", "y", // something still glowing; it is the keyboard
		"1", "y", // the first on the menu, and keep it
		"y", // write it
	}}
	require.NoError(t, cli.Map(t.Context(), client, person))

	rules, err := os.ReadFile(filepath.Join(home, "hotaru", "hotaru.yml"))
	require.NoError(t, err)
	require.Contains(t, string(rules), "solid_modes: [Solid Splash")
}

func TestAMachineThatBlanksIsAskedOnlyOnce(t *testing.T) {
	/*
		Question count is the cost. Most hardware turns off properly, and on
		that hardware the whole subject is one question for the machine rather
		than one per device.
	*/
	one := func(name string) devices.Device {
		return devices.Device{
			Name: name, LEDCount: 1,
			Modes:      []devices.Mode{{Name: "Direct", PerLED: true}},
			Zones:      []devices.Zone{{Name: "LED", Shape: devices.ShapeSingle, First: 0, Count: 1}},
			ActiveMode: "Direct",
		}
	}
	client := api.NewClient(serving(t, nil, openrgb.NewFake(one("Alpha Board"), one("Beta Board"))))
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	person := &scripted{answers: []string{
		"y", "front light", // Alpha
		"y", "rear light", // Beta
		"y", // is that right?
		"n", // nothing still lit
		"n", // do not write
	}}
	require.NoError(t, cli.Map(t.Context(), client, person))

	asked := strings.Count(person.questions(), "still glowing or lit up")
	require.Equal(t, 1, asked, "a machine that blanks was interrogated device by device")
}

func TestADeviceWithNoBrightnessIsNotOfferedOne(t *testing.T) {
	// A question nobody can act on is not a question. Spec 008's rule, and
	// the reason this one is gated on what the device reports.
	client := api.NewClient(serving(t, nil, openrgb.NewFake(cooler())))
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	person := &scripted{answers: []string{"y", "desk strip", "y", "n", "n"}}
	require.NoError(t, cli.Map(t.Context(), client, person))
	require.NotContains(t, person.questions(), "turned down")
}

func TestTheOffQuestionDoesNotAssertItsOwnAnswer(t *testing.T) {
	/*
		It used to say "Everything is off now. Is anything still lit?".

		Somebody looking at a keyboard glowing white answered no: the program
		had just told them everything was off, so white must be what off looks
		like on this board. That is precisely the fault the question exists to
		find, invited by the question.
	*/
	client := api.NewClient(serving(t, nil, openrgb.NewFake(keyboardThatWillNotBlank())))
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	person := &scripted{answers: []string{"y", "keyboard", "y", "n", "n"}}
	require.NoError(t, cli.Map(t.Context(), client, person))

	require.NotContains(t, person.questions(), "Everything is off now",
		"the wizard stated the outcome it was asking about")
	require.Contains(t, strings.Join(person.said, " "), "should be dark",
		"somebody has to be told what they are looking for")
}

func TestDimmingIsOfferedOnlyForTheModeThatWillBeUsed(t *testing.T) {
	/*
		A Keychron's Direct mode -- the one hotaru writes a colour in -- takes
		no brightness, while its animated cycle modes all do. Gated on "does
		the device have a dimmable mode", hotaru offered to dim it, dimmed
		nothing, and wrote a brightness that could never apply.
	*/
	device := keyboardThatWillNotBlank()
	device.Modes = []devices.Mode{
		{Name: "Direct", PerLED: true},        // used, not dimmable
		{Name: "Cycle All", Brightness: true}, // dimmable, never used
	}
	client := api.NewClient(serving(t, nil, openrgb.NewFake(device)))
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	person := &scripted{answers: []string{"y", "keyboard", "y", "n", "n"}}
	require.NoError(t, cli.Map(t.Context(), client, person))
	require.NotContains(t, person.questions(), "turned down",
		"a device was offered a dimming that would do nothing")
}

func TestAChoiceOfDarkModesIsOffered(t *testing.T) {
	/*
		A keyboard has several ways to stay dark until touched, and which one
		somebody wants is taste. The wizard used to pick the first it matched
		-- "Solid Reactive Simple", because it happened to come first in the
		device's list -- and wrote it down as though it had been chosen.
	*/
	device := keyboardThatWillNotBlank()
	device.Modes = []devices.Mode{
		{Name: "Direct", PerLED: true},
		{Name: "Solid Reactive Simple"},
		{Name: "Solid Splash"},
	}
	client := api.NewClient(serving(t, nil, openrgb.NewFake(device)))
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)

	person := &scripted{answers: []string{
		"y", "keyboard",
		"y",      // is that right?
		"y", "y", // something still glowing; it is the keyboard
		"2", "y", // the second on the menu, and keep it
		"y", // write it
	}}
	require.NoError(t, cli.Map(t.Context(), client, person))

	rules, err := os.ReadFile(filepath.Join(home, "hotaru", "hotaru.yml"))
	require.NoError(t, err)
	require.Contains(t, string(rules), "Solid Reactive Simple",
		"the second offer was accepted and the first was written instead")
	require.Contains(t, string(rules), "direct",
		"a single mode leaves nowhere to go if it stops working")
}

func TestABrightnessThatCannotApplyIsRemovedOnARerun(t *testing.T) {
	/*
		An earlier run of this very wizard wrote `brightness: 40` for a
		keyboard whose Direct mode takes no brightness. A setting that cannot
		apply is worse in a file than absent: somebody reads it and believes
		it.
	*/
	device := keyboardThatWillNotBlank()
	device.Modes = []devices.Mode{{Name: "Direct", PerLED: true}}

	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	require.NoError(t, os.MkdirAll(filepath.Join(home, "hotaru"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(home, "hotaru", "hotaru.yml"), []byte(
		"devices:\n  - match: \"keychron\"\n    brightness: 40\n    segments:\n      keyboard: {zone: \"Keyboard\"}\n"), 0o600))

	client := api.NewClient(serving(t, nil, openrgb.NewFake(device)))
	person := &scripted{answers: []string{"y", "keyboard", "y", "n", "y"}}
	require.NoError(t, cli.Map(t.Context(), client, person))

	rules, err := os.ReadFile(filepath.Join(home, "hotaru", "hotaru.yml"))
	require.NoError(t, err)
	require.NotContains(t, string(rules), "brightness",
		"a brightness this device cannot use survived a run that knew better")
}

func TestAPreviousDarkModeChoiceCanBeRevisited(t *testing.T) {
	/*
		The offer only appeared when a device failed to go dark -- and a device
		already set to stay dark until touched no longer fails, so the choice
		could be made once and never changed. Somebody re-running the wizard
		to change it was told nothing and asked nothing.

		Spec 008's reconfiguration pattern has to reach this answer too.
	*/
	device := keyboardThatWillNotBlank()
	device.Modes = []devices.Mode{
		{Name: "Direct", PerLED: true},
		{Name: "Solid Reactive Simple"},
		{Name: "Solid Splash"},
	}
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	require.NoError(t, os.MkdirAll(filepath.Join(home, "hotaru"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(home, "hotaru", "hotaru.yml"), []byte(
		"devices:\n  - match: \"keychron\"\n    solid_modes: [Solid Reactive Simple]\n"+
			"    never_blank: true\n    segments:\n      keyboard: {zone: \"Keyboard\"}\n"), 0o600))

	client := api.NewClient(serving(t, nil, openrgb.NewFake(device)))
	person := &scripted{answers: []string{
		"y", "keyboard",
		"n",      // no, do not keep the mode it is set to
		"1", "y", // the first on the menu is Splash, which is preferred
		"y", // is that right?
		"n", // nothing still glowing
		"y", // write it
	}}
	require.NoError(t, cli.Map(t.Context(), client, person))

	require.Contains(t, person.questions(), "Keep that?",
		"a run that was reconfiguring was never asked about the existing choice")

	rules, err := os.ReadFile(filepath.Join(home, "hotaru", "hotaru.yml"))
	require.NoError(t, err)
	require.Contains(t, string(rules), "Solid Splash", "the new choice was not written")
}

func TestKeepingAPreviousDarkModeLeavesItAlone(t *testing.T) {
	// The other half: answering yes must not quietly rewrite the choice.
	device := keyboardThatWillNotBlank()
	device.Modes = []devices.Mode{
		{Name: "Direct", PerLED: true},
		{Name: "Solid Splash"},
	}
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	require.NoError(t, os.MkdirAll(filepath.Join(home, "hotaru"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(home, "hotaru", "hotaru.yml"), []byte(
		"devices:\n  - match: \"keychron\"\n    solid_modes: [Solid Splash]\n"+
			"    segments:\n      keyboard: {zone: \"Keyboard\"}\n"), 0o600))

	client := api.NewClient(serving(t, nil, openrgb.NewFake(device)))
	person := &scripted{answers: []string{"y", "keyboard", "y", "y", "n", "y"}}
	require.NoError(t, cli.Map(t.Context(), client, person))

	rules, err := os.ReadFile(filepath.Join(home, "hotaru", "hotaru.yml"))
	require.NoError(t, err)
	require.Contains(t, string(rules), "Solid Splash")
}

func TestAKeyboardWithUnfamiliarModeNamesStillGetsAChoice(t *testing.T) {
	/*
		The candidate list was matched on "splash" and "reactive", which is
		this desk's Keychron's vocabulary. A SteelSeries Apex Pro names its
		effects differently, and would have been offered nothing at all --
		on a keyboard that certainly has something.

		hotaru cannot know which effect leaves a board mostly dark. It offers
		everything, likeliest first, and somebody watching decides.
	*/
	device := devices.Device{
		Name:     "SteelSeries Apex Pro",
		LEDCount: 2,
		Modes: []devices.Mode{
			{Name: "Direct", PerLED: true},
			{Name: "ColorShift"},
			{Name: "Ripple"},
			{Name: "Breathing"},
		},
		Zones:      []devices.Zone{{Name: "Keyboard", Shape: devices.ShapeGrid, First: 0, Count: 2}},
		ActiveMode: "Direct",
	}
	client := api.NewClient(serving(t, nil, openrgb.NewFake(device)))
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	person := &scripted{answers: []string{
		"y", "keyboard",
		"y",      // is that right?
		"y", "y", // something still glowing; it is the keyboard
		"", // look at the list, then leave it alone
		"n",
	}}
	require.NoError(t, cli.Map(t.Context(), client, person))

	shown := strings.Join(person.said, "\n")
	require.Contains(t, shown, "Ripple", "a likely mode was not offered")
	require.Contains(t, shown, "ColorShift",
		"a mode nobody anticipated was hidden, on hardware nobody here has seen")
	require.Contains(t, shown, "Breathing")
}

func TestTheModeOfferIsNeverEmptyWhereTheDeviceHasModes(t *testing.T) {
	// The failure this replaces: an empty candidate list told somebody their
	// device could do nothing about it, which was never true.
	device := devices.Device{
		Name:     "Nameless Board",
		LEDCount: 1,
		Modes: []devices.Mode{
			{Name: "Direct", PerLED: true},
			{Name: "Mode 2"},
		},
		Zones:      []devices.Zone{{Name: "Keys", Shape: devices.ShapeSingle, First: 0, Count: 1}},
		ActiveMode: "Direct",
	}
	client := api.NewClient(serving(t, nil, openrgb.NewFake(device)))
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	person := &scripted{answers: []string{"y", "light", "y", "y", "y", "", "n"}}
	require.NoError(t, cli.Map(t.Context(), client, person))
	require.Contains(t, strings.Join(person.said, "\n"), "Mode 2")
}
