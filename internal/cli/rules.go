package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ushineko/hotaru/internal/config"
)

/*
Writing the rules file back out.

The wizard used to refuse to touch a file that already existed, on the reasoning
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

// yamlFor renders a whole configuration, so a re-run keeps the rules for
// devices it did not ask about.
func yamlFor(cfg *config.Config) string {
	var out strings.Builder
	out.WriteString("# Written by `hotaru light map`.\n")
	out.WriteString("# Re-running it offers these answers as the defaults.\n")

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
		fmt.Fprintf(&out, "  - match: %s\n", rule.Match)
		if len(rule.SolidModes) > 0 {
			// Why the order is not the default one. Without this, a line that
			// took somebody standing in front of their machine to establish
			// reads like an arbitrary preference, and the next person deletes
			// it.
			out.WriteString("    # the first of these is the mode this device was seen to light in.\n")
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
		if len(rule.Segments) == 0 {
			continue
		}

		out.WriteString("    segments:\n")
		for _, name := range sorted(rule.Segments) {
			segment := rule.Segments[name]
			if segment.LEDs == nil {
				fmt.Fprintf(&out, "      %s: {zone: %q}\n", name, segment.Zone)
				continue
			}
			fmt.Fprintf(&out, "      %s: {zone: %q, leds: [%d, %d]}\n",
				name, segment.Zone, segment.LEDs.First, segment.LEDs.Last)
		}
	}
	return out.String()
}

func sorted(segments map[string]config.Segment) []string {
	out := make([]string, 0, len(segments))
	for name := range segments {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

/*
merge puts what the wizard learned into the configuration that was there.

A rule for a device the wizard did not ask about is left exactly as it was:
somebody mapping one stick of RAM should not lose the rules for their cooler.
A device it did map has its segments replaced rather than added to, because the
answers just given are the current truth about that hardware.
*/
func merge(existing *config.Config, learned []namedSegment, corrections map[string][]string, present []string) *config.Config {
	out := &config.Config{Scope: existing.Scope}
	out.Devices = append(out.Devices, existing.Devices...)

	byDevice := map[string][]namedSegment{}
	for _, segment := range learned {
		byDevice[segment.Device] = append(byDevice[segment.Device], segment)
	}

	for device, segments := range byDevice {
		match := matchFor(device, present)
		rule := findRule(out, match)
		if rule == nil {
			out.Devices = append(out.Devices, config.DeviceRule{Match: match})
			rule = &out.Devices[len(out.Devices)-1]
		}

		if order := corrections[device]; len(order) > 0 {
			rule.SolidModes = order
		}
		rule.Segments = map[string]config.Segment{}
		for _, segment := range segments {
			entry := config.Segment{Zone: segment.Zone}
			if !segment.Whole {
				entry.LEDs = &config.LEDs{First: segment.First, Last: segment.Last}
			}
			rule.Segments[segment.Name] = entry
		}
	}
	return out
}

func findRule(cfg *config.Config, match string) *config.DeviceRule {
	for i := range cfg.Devices {
		if strings.EqualFold(cfg.Devices[i].Match, match) {
			return &cfg.Devices[i]
		}
	}
	return nil
}

/*
knownAs is what this zone was called last time, for offering as a default.

A person re-running the wizard has already answered these questions once, and
making them type "rad-front" again to keep it is the sort of thing that stops
people re-running anything.
*/
func knownAs(cfg *config.Config, device, zone string, present []string) string {
	rule := findRule(cfg, matchFor(device, present))
	if rule == nil {
		return ""
	}
	for name, segment := range rule.Segments {
		if strings.EqualFold(segment.Zone, zone) && segment.LEDs == nil {
			return name
		}
	}
	return ""
}
