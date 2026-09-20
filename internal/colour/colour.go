/*
Package colour is the colour vocabulary: names, hex, and the parsing of both.

Small on purpose. It knows nothing about devices, modes or scenes — a colour is
three bytes and the words people use for them, and every layer above is free to
decide what to do with one.
*/
package colour

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Colour is an 8-bit-per-channel RGB colour.
type Colour struct {
	R, G, B uint8
}

// Black is what every channel at zero looks like, and what "off" resolves to
// on a device that has no Off mode of its own.
var Black = Colour{}

/*
Names are the colours a person can type.

Carried from the Python this replaces, values included, so a scene written for
that program means the same thing here. They are the vivid corners rather than
a palette: an LED renders a muted screen colour as muddy brown, so the useful
names are the saturated ones.
*/
var names = map[string]Colour{
	"red":     {255, 0, 0},
	"green":   {0, 255, 0},
	"blue":    {0, 0, 255},
	"yellow":  {255, 255, 0},
	"cyan":    {0, 255, 255},
	"magenta": {255, 0, 255},
	"white":   {255, 255, 255},
	"orange":  {255, 85, 0},
	"purple":  {128, 0, 255},
	"pink":    {255, 0, 128},
	"lime":    {128, 255, 0},
	"teal":    {0, 255, 128},
	"black":   {0, 0, 0},
}

// Names is every name that parses, sorted, for help text and for a GUI that
// offers them.
func Names() []string {
	out := make([]string, 0, len(names))
	for name := range names {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

/*
Parse reads a colour name, "#rrggbb", or "#rgb".

Case and surrounding space do not matter, and the leading "#" is optional
because someone writing YAML by hand will leave it out at least once. The short
form doubles each digit, so "#f80" is "#ff8800" — the convention every other
tool uses, and the one a person expects when they try it.

It does not accept "off". Off is not a colour, it is an intent about a device,
and a device that cannot express it needs to say so rather than be handed
black — see the devices package.
*/
func Parse(s string) (Colour, error) {
	text := strings.ToLower(strings.TrimSpace(s))
	if text == "" {
		return Colour{}, fmt.Errorf("no colour given")
	}
	if c, ok := names[text]; ok {
		return c, nil
	}
	if text == "off" {
		return Colour{}, fmt.Errorf("%q is an intent, not a colour: ask the device to turn off", s)
	}

	digits := strings.TrimPrefix(text, "#")
	switch len(digits) {
	case 3:
		digits = string([]byte{digits[0], digits[0], digits[1], digits[1], digits[2], digits[2]})
	case 6:
	default:
		return Colour{}, fmt.Errorf("%q is not a colour name or a hex colour like #ff8800", s)
	}

	v, err := strconv.ParseUint(digits, 16, 32)
	if err != nil {
		return Colour{}, fmt.Errorf("%q is not a colour name or a hex colour like #ff8800", s)
	}
	// Masked explicitly: ParseUint has already bounded this to 24 bits, and
	// saying so is cheaper than explaining it to every reader and linter.
	return Colour{R: uint8(v >> 16 & 0xff), G: uint8(v >> 8 & 0xff), B: uint8(v & 0xff)}, nil
}

// MustParse is Parse for a constant the author already knows is good. It panics
// on anything else, so it belongs in tests and package-level values, never on a
// path that has read a file.
func MustParse(s string) Colour {
	c, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return c
}

// String is the hex form, which is what a log line and a JSON field both want.
func (c Colour) String() string { return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B) }

// MarshalText and UnmarshalText make a Colour a string in JSON and YAML, so a
// scene file holds "#ff8800" rather than three numbers nobody can read.
func (c Colour) MarshalText() ([]byte, error) { return []byte(c.String()), nil }

// UnmarshalText reads a colour name or hex string.
func (c *Colour) UnmarshalText(b []byte) error {
	got, err := Parse(string(b))
	if err != nil {
		return err
	}
	*c = got
	return nil
}
