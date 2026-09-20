package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

/*
Asker is a conversation with a person.

An interface so the wizard can be driven by a test without a terminal: its
whole quality is in the wording and ordering of half a dozen questions, and
those are worth asserting rather than hoping about.
*/
type Asker interface {
	// Say tells the user something.
	Say(format string, args ...any)
	// Ask takes free text. An empty answer is a real answer -- it means the
	// user does not know, which is different from a guess.
	Ask(question string) (string, error)
	// Confirm takes yes or no, defaulting to no: the wizard should never
	// proceed because somebody pressed return to make it stop asking.
	Confirm(question string) (bool, error)
	// Count takes a number, defaulting to one.
	Count(question string) (int, error)
	// Choose takes one of the offered answers.
	Choose(question string, options []string) (string, error)
}

// terminal is an Asker over a real pair of streams.
type terminal struct {
	in  *bufio.Reader
	out io.Writer
}

// NewTerminal is an Asker reading from in and writing to out.
func NewTerminal(in io.Reader, out io.Writer) Asker {
	return &terminal{in: bufio.NewReader(in), out: out}
}

func (t *terminal) Say(format string, args ...any) {
	_, _ = fmt.Fprintf(t.out, format+"\n", args...)
}

func (t *terminal) Ask(question string) (string, error) {
	_, _ = fmt.Fprintf(t.out, "%s ", question)
	line, err := t.in.ReadString('\n')
	if err != nil && line == "" {
		return "", fmt.Errorf("read the answer: %w", err)
	}
	return strings.TrimSpace(line), nil
}

func (t *terminal) Confirm(question string) (bool, error) {
	answer, err := t.Ask(question + " [y/N]")
	if err != nil {
		return false, err
	}
	switch strings.ToLower(answer) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

func (t *terminal) Count(question string) (int, error) {
	answer, err := t.Ask(question)
	if err != nil {
		return 0, err
	}
	// Anything that is not a number means one: "just the one", "1 fan" and a
	// bare return are all the same answer, and arguing with a person about
	// the format of a number they already gave would be its own kind of rude.
	fields := strings.Fields(answer)
	if len(fields) == 0 {
		return 1, nil
	}
	n, _ := strconv.Atoi(fields[0])
	if n < 1 {
		return 1, nil
	}
	return n, nil
}

/*
Choose asks for one of the offered answers, and asks again when it gets
something else.

Taking the first option on an unrecognised answer is how a person who typed
"none" was told their hardware was exactly one light. An answer nobody offered
means the question did not land, and guessing at it compounds that rather than
recovering from it.
*/
func (t *terminal) Choose(question string, options []string) (string, error) {
	for attempt := range 3 {
		answer, err := t.Ask(fmt.Sprintf("%s [%s]", question, strings.Join(options, "/")))
		if err != nil {
			return "", err
		}
		answer = strings.ToLower(strings.TrimSpace(answer))
		for _, option := range options {
			// A leading letter is enough: nobody wants to type "more than one".
			if answer == strings.ToLower(option) ||
				(answer != "" && strings.HasPrefix(strings.ToLower(option), answer)) {
				return option, nil
			}
		}
		if attempt < 2 {
			_, _ = fmt.Fprintf(t.out, "    Please answer with one of: %s.\n",
				strings.Join(options, ", "))
		}
	}
	return "", errNoAnswer
}

// errNoAnswer ends a conversation that is not getting anywhere, rather than
// proceeding on an answer nobody gave.
var errNoAnswer = errors.New("no answer to that question, so nothing was changed")
