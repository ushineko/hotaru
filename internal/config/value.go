package config

import (
	"encoding/json"
	"fmt"
	"time"
)

/*
Duration is a time.Duration written the way a person writes one: "60s", "5m".

A bare number would be nanoseconds, which is nobody's intent in a file that
says how often to re-assert a mouse's colour.
*/
type Duration time.Duration

// UnmarshalJSON accepts "90s" and also a plain number of seconds.
func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		parsed, err := time.ParseDuration(s)
		if err != nil {
			return fmt.Errorf("not a duration: %q", s)
		}
		*d = Duration(parsed)
		return nil
	}
	var secs float64
	if err := json.Unmarshal(b, &secs); err != nil {
		return fmt.Errorf("not a duration: %s", b)
	}
	*d = Duration(time.Duration(secs * float64(time.Second)))
	return nil
}

// MarshalJSON writes the form a person would have typed.
func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

// Duration is the value as a time.Duration.
func (d Duration) Duration() time.Duration { return time.Duration(d) }

/*
LEDs is an inclusive LED range within a zone, written [first, last].

Inclusive because it describes lights someone counted by looking at them, and
"[0, 11] is the top fan" survives being read aloud in a way a half-open range
does not.
*/
type LEDs struct {
	First int
	Last  int
}

// UnmarshalJSON accepts [first, last].
func (r *LEDs) UnmarshalJSON(b []byte) error {
	var pair []int
	if err := json.Unmarshal(b, &pair); err != nil {
		return fmt.Errorf("not a [first, last] pair: %s", b)
	}
	if len(pair) != 2 {
		return fmt.Errorf("a [first, last] pair has two values, got %d", len(pair))
	}
	r.First, r.Last = pair[0], pair[1]
	return nil
}

// MarshalJSON writes [first, last].
func (r LEDs) MarshalJSON() ([]byte, error) { return json.Marshal([2]int{r.First, r.Last}) }

// Count is how many LEDs the range covers.
func (r LEDs) Count() int { return r.Last - r.First + 1 }

func (r LEDs) String() string { return fmt.Sprintf("[%d:%d]", r.First, r.Last) }
