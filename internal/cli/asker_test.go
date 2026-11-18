package cli_test

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
)

/*
scripted is a person who has already decided what they will say.

A command that asks is tested by the conversation as well as the outcome: what
was asked, in what order, and what the machine was doing when it was asked.
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
