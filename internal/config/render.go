package config

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

/*
Writing the rules file back out.

light map used to refuse to touch a file that already existed, on the reasoning
that the file is the user's and hotaru does not edit it. That was the right
principle taken from the wrong place. `fynedesygn` holds it strictly because it
is a library: a side effect there surprises the program embedding it, and there
is nobody present to ask. An application has somebody in front of it, and asking
is exactly what it is for.

And the pattern it unlocks is an ordinary one -- read the existing settings,
offer them as the defaults, write the answers back. Every installer and every
`--reconfigure` flag works that way, because it is how a person changes one
thing without restating the other nine.

What is still true: this regenerates the file, so comments do not survive. That
is said plainly before anything is written, which is the difference between a
cost someone accepted and one that was imposed on them.
*/

// Render writes a whole configuration as the rules file, so a rewrite keeps
// every rule and key it did not change.
func Render(cfg *Config) string {
	var out strings.Builder
	out.WriteString("# Written by hotaru.\n")
	out.WriteString("# Comments are not kept when hotaru writes this file.\n")

	if len(cfg.Scope) > 0 {
		out.WriteString("\nscope:\n")
		for _, name := range cfg.Scope {
			fmt.Fprintf(&out, "  - %s\n", name)
		}
	}
	if len(cfg.Devices) == 0 {
		return out.String()
	}

	out.WriteString("\ndevices:\n")
	for _, rule := range cfg.Devices {
		// Quoted, always. "0x18" is a perfectly good way to tell four
		// identical sticks of RAM apart and a hexadecimal number to YAML, and
		// a file that does not load is worse than one that is fussy.
		fmt.Fprintf(&out, "  - match: %q\n", rule.Match)
		if len(rule.SolidModes) > 0 {
			// Why the order is not the default one. Without this, a line that
			// took somebody standing in front of their machine to establish
			// reads like an arbitrary preference, and the next person deletes
			// it.
			out.WriteString("    # how this device is lit, in order of preference.\n")
			fmt.Fprintf(&out, "    solid_modes: [%s]\n", strings.Join(rule.SolidModes, ", "))
		}
		if rule.NeverBlank {
			out.WriteString("    never_blank: true\n")
		}
		if rule.Brightness != nil {
			fmt.Fprintf(&out, "    brightness: %d\n", *rule.Brightness)
		}
		if rule.Reassert > 0 {
			fmt.Fprintf(&out, "    reassert: %s\n", rule.Reassert.Duration())
		}
		if rule.FrameInterval > 0 {
			fmt.Fprintf(&out, "    frame_interval: %s\n", rule.FrameInterval.Duration())
		}
		if rule.Twin != "" {
			fmt.Fprintf(&out, "    twin: %q\n", rule.Twin)
		}
		if rule.Follow != "" {
			fmt.Fprintf(&out, "    follow: %q\n", rule.Follow)
		}
		if rule.Lead {
			out.WriteString("    lead: true\n")
		}
		/*
			The keys whose values are maps, as JSON, which YAML reads as its own
			flow style: whatever a key holds is written back exactly, so a
			re-run of light map or a trim from light match loses nothing a
			person or another command set (spec 077).
		*/
		for _, field := range []struct {
			name  string
			value any
			set   bool
		}{
			{"formerly", rule.Formerly, rule.Formerly != nil},
			{"lengths", rule.Lengths, len(rule.Lengths) > 0},
			{"shapes", rule.Shapes, len(rule.Shapes) > 0},
			{"pace", rule.Pace, len(rule.Pace) > 0},
			{"colour", rule.Colour, rule.Colour != nil},
		} {
			if !field.set {
				continue
			}
			value, err := json.Marshal(field.value)
			if err != nil {
				continue // a value that came from JSON marshals back
			}
			fmt.Fprintf(&out, "    %s: %s\n", field.name, value)
		}
		if len(rule.Segments) == 0 {
			continue
		}

		out.WriteString("    segments:\n")
		for _, name := range sortedSegments(rule.Segments) {
			segment := rule.Segments[name]
			if segment.LEDs == nil {
				fmt.Fprintf(&out, "      %s: {zone: %q}\n", yamlKey(name), segment.Zone)
				continue
			}
			fmt.Fprintf(&out, "      %s: {zone: %q, leds: [%d, %d]}\n",
				yamlKey(name), segment.Zone, segment.LEDs.First, segment.LEDs.Last)
		}
		if len(rule.Toggles) > 0 {
			// With the note, because a toggle is the one entry here that is
			// not decoration: somebody reading this later should not have to
			// work out why two lights get their own control in the window.
			out.WriteString("    # segments that are switches rather than decoration.\n")
			fmt.Fprintf(&out, "    toggles: [%s]\n", strings.Join(rule.Toggles, ", "))
		}
	}
	return out.String()
}

/*
yamlKey is a segment name as a YAML mapping key.

Quoted unless it is plainly a word. A person naming a fan "1" or "0x18" has
said something reasonable, and it is this file's job to carry it rather than to
reinterpret it as a number.
*/
func yamlKey(name string) string {
	plain := name != "" && name[0] >= 'a' && name[0] <= 'z'
	for _, r := range name {
		lower := r >= 'a' && r <= 'z'
		digit := r >= '0' && r <= '9'
		if !lower && !digit && r != '-' {
			plain = false
			break
		}
	}
	if plain {
		return name
	}
	return fmt.Sprintf("%q", name)
}

func sortedSegments(segments map[string]Segment) []string {
	out := make([]string, 0, len(segments))
	for name := range segments {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
