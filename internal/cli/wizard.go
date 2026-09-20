package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/ushineko/hotaru/internal/api"
	"github.com/ushineko/hotaru/internal/config"
)

/*
The mapping wizard: the conversation that turns a machine's zones into names a
person would use out loud.

Nothing here can be derived. A zone with sixteen LEDs is two daisy-chained fans,
one fan with an unlit half, or an empty header, and the protocol reports the
same number for all three -- three of the development machine's nine zones have
nothing attached and every one accepts writes and reports success. The only
instrument is somebody looking at the machine, so this asks them.

The questions are the design. Never "which LEDs are the rear fan", which nobody
knows; always "what colour is the rear fan", which anyone can answer while
looking at it.
*/

// palette is what zones are lit with: four colours nobody has to squint at.
// Rehearsal produced "bottom cyan(?)" from something subtler, and then the
// same mistake again with blue, cyan and teal on adjacent fans.
var palette = []string{"red", "green", "blue", "white"}

func mapCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "map",
		Short: "Work out what is what, by looking at the machine",
		Long: `Work out what is what, by looking at the machine.

Lights each zone a different colour and asks what you can see. Your answers
name your own hardware: what comes out is ` + "`kraken/rad-front`" + `, not
` + "`kraken/Hue 2 Channel 2[16:23]`" + `.

Nothing is written without your say-so, and the lights go back afterwards.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := client(cmd)
			if err != nil {
				return err
			}
			names, _ := cmd.Flags().GetStringSlice("devices")
			asker := NewTerminal(cmd.InOrStdin(), cmd.OutOrStdout())
			return Map(cmd.Context(), client, asker, names...)
		},
	}
	cmd.Flags().StringSlice("devices", nil, "only these devices, by name")
	return cmd
}

// namedSegment is one thing the user named, and where it lives.
type namedSegment struct {
	Device string
	Zone   string
	Name   string
	First  int // within the zone
	Last   int
	Whole  bool // the whole zone, so no range is written
}

/*
Map runs the wizard.

Exported so the GUI can drive the same conversation with its own Asker: the
questions, the order and the arithmetic are the part worth sharing, and only
the way they are put to a person differs.
*/
func Map(ctx context.Context, client *api.Client, asker Asker, only ...string) error {
	health, err := client.Health(ctx)
	if err != nil {
		return quiet(err)
	}
	if health.State != "healthy" {
		// Mapping a machine hotaru cannot see would spend someone's attention
		// on a question with no answer.
		asker.Say("%s: %s", health.State, health.Detail)
		for _, remedy := range health.Remedies {
			asker.Say("  %s", remedy)
		}
		return errUnhealthy
	}

	list, err := client.Devices(ctx)
	if err != nil {
		return quiet(err)
	}

	remembered := len(mustStatus(ctx, client).Remembered) > 0
	defer restore(ctx, client, asker, remembered)

	asker.Say("Lighting each zone a colour and asking what you see.")
	asker.Say("Answer about your own hardware, in your own words. Press return to skip anything.\n")

	var found []namedSegment
	corrections := map[string][]string{}
	for _, device := range list {
		if !device.InScope || device.LEDs == 0 || !wanted(only, device.Name) {
			continue
		}

		// Before asking what is on a device, find out whether hotaru can light
		// it at all. On this author's board the default mode is accepted,
		// reported, and lights only the onboard LED -- so a user would answer
		// "nothing" to every zone and map half their case as empty.
		working, err := lightsUp(ctx, client, asker, device)
		if err != nil {
			return err
		}
		if working.none {
			// Say which modes were tried: on a device that will not light, the
			// useful next step is knowing what was already ruled out.
			if len(working.tried) > 0 {
				asker.Say("  Nothing lit in %s. Skipping %s.",
					strings.Join(working.tried, " or "), device.Name)
			} else {
				asker.Say("  %s would not take any plain mode, so there is nothing to see. Skipping it.",
					device.Name)
			}
			continue
		}
		if working.corrected {
			corrections[device.Name] = working.order
			asker.Say("  %s only lights in %s, so that will go in the rules.", device.Name, working.mode)
		}

		segments, err := mapDevice(ctx, client, asker, device, working.mode)
		if err != nil {
			return err
		}
		found = append(found, segments...)
	}

	if len(found) == 0 {
		asker.Say("\nNothing was named, so there is nothing to write.")
		return nil
	}
	present := make([]string, 0, len(list))
	for _, device := range list {
		present = append(present, device.Name)
	}
	return finish(ctx, client, asker, found, present, corrections)
}

/*
working is what it took to light a device, if anything did.

corrected means the mode hotaru would have chosen was not the one that lit it,
which is a correction worth writing down -- and one no read-back could have
found, because the mode hotaru chose was accepted and reported back.
*/
type working struct {
	mode      string
	order     []string
	corrected bool
	none      bool
	tried     []string
}

/*
probeColour is what a device is lit with while asking whether it lights.

Red rather than white. "Is it white?" is a harder question than it sounds on
hardware that is dim, tinted behind a filter, or lighting only some of itself,
and the question needs to be one a person can answer at a glance.
*/
const probeColour = "red"

/*
lightsUp finds a mode that visibly lights a device, by asking.

The one question no software can answer for itself. A board that accepts Static
and lights only its onboard LED reports Static either way; the probe cannot see
it, the read-back cannot see it, and the user can see nothing else.

Each mode is applied **exactly**, with no fall-through. Letting hotaru try the
next candidate would answer a different question and then ask the person about
it: the first run of this on someone else's machine asked about Direct four
times, because every other mode it tried fell through to Direct and was
reported as Direct.
*/
func lightsUp(ctx context.Context, client *api.Client, asker Asker, device api.Device) (working, error) {
	asker.Say("── %s (%d LEDs, %d zones)", device.Name, device.LEDs, len(device.Zones))

	var order, asked []string
	tried := map[string]bool{}

	for _, mode := range plainModes(device) {
		if tried[strings.ToLower(mode)] {
			continue
		}
		tried[strings.ToLower(mode)] = true

		out, err := client.Apply(ctx, api.ApplyRequest{
			Colour:  probeColour,
			Devices: []string{device.Name},
			Mode:    mode,
			Exactly: true,
		})
		if err != nil {
			return working{}, quiet(err)
		}
		if len(out.Results) == 0 || !out.Results[0].Applied {
			// The device would not take that mode at all, which is an answer
			// and not worth a question.
			continue
		}

		used := out.Results[0].Mode
		asked = append(asked, used)
		lit, err := asker.Confirm(fmt.Sprintf("  Is it lit %s now? (%s)", probeColour, used))
		if err != nil {
			return working{}, err
		}
		if lit {
			// The one that worked first, then the ones that did not, kept as
			// fallbacks: dropping them would leave the device one option and
			// nowhere to go if firmware changes under it.
			order = append([]string{strings.ToLower(used)}, order...)
			return working{mode: used, order: order, corrected: len(asked) > 1}, nil
		}
		order = append(order, strings.ToLower(used))
	}

	return working{none: true, tried: asked}, nil
}

/*
plainModes are the modes worth trying, in order.

Solid-looking ones only. Setting Rainbow Wave to find out whether a device
lights would be a light show rather than a diagnostic, and a user watching an
animation cannot answer "is it red" anyway.
*/
func plainModes(device api.Device) []string {
	var out []string
	for _, mode := range device.Modes {
		switch strings.ToLower(mode) {
		case "direct", "static", "custom", "solid color", "solid":
			out = append(out, mode)
		}
	}
	return out
}

// mapDevice asks about one device's zones, then about what is on them.
func mapDevice(ctx context.Context, client *api.Client, asker Asker, device api.Device, mode string) ([]namedSegment, error) {
	zones := device.Zones
	if len(zones) == 0 {
		zones = []api.Zone{{Name: "", First: 0, Count: device.LEDs}}
	}

	var named []namedSegment
	for batch := 0; batch < len(zones); batch += len(palette) {
		end := min(batch+len(palette), len(zones))
		group := zones[batch:end]

		if err := light(ctx, client, device.Name, mode, assignmentsFor(device.Name, group)); err != nil {
			return nil, err
		}

		for i, zone := range group {
			colour := palette[i]
			what, err := askName(asker, fmt.Sprintf("  What is %s?", colour))
			if err != nil {
				return nil, err
			}
			if what == "" {
				// An empty zone: the header reports LEDs and nothing is on it.
				continue
			}

			parts, err := split(ctx, client, asker, device, zone, what, mode)
			if err != nil {
				return nil, err
			}
			named = append(named, parts...)
		}
	}
	return named, nil
}

/*
split asks how many things are on a zone, and works out where they divide.

Asking the count first is what the vendors' wizards do, and it is right: a
daisy-chain is usually identical fans, so one answer and the LED count give the
division arithmetically. Bisection is the fallback for a division that is not
clean, not the opening move.
*/
func split(ctx context.Context, client *api.Client, asker Asker,
	device api.Device, zone api.Zone, what, mode string,
) ([]namedSegment, error) {
	whole := namedSegment{Device: device.Name, Zone: zone.Name, Name: what, Whole: true,
		First: 0, Last: zone.Count - 1}

	count, err := asker.Count(fmt.Sprintf("  How many separate lights are on %s? [1]", what))
	if err != nil {
		return nil, err
	}
	if count <= 1 {
		return []namedSegment{whole}, nil
	}

	if zone.Count%count != 0 {
		asker.Say("  %d LEDs does not divide by %d, so the boundaries need finding.", zone.Count, count)
		return bisect(ctx, client, asker, device, zone, what, count, mode)
	}

	size := zone.Count / count
	var parts []namedSegment
	for i := range count {
		parts = append(parts, namedSegment{
			Device: device.Name, Zone: zone.Name,
			First: i * size, Last: (i+1)*size - 1,
		})
	}

	if err := light(ctx, client, device.Name, mode, partAssignments(device.Name, zone, parts)); err != nil {
		return nil, err
	}
	right, err := asker.Confirm(fmt.Sprintf("  Each of the %d shows one colour?", count))
	if err != nil {
		return nil, err
	}
	if !right {
		return bisect(ctx, client, asker, device, zone, what, count, mode)
	}

	for i := range parts {
		name, err := askName(asker, fmt.Sprintf("  What is %s?", palette[i%len(palette)]))
		if err != nil {
			return nil, err
		}
		parts[i].Name = name
	}
	return keepNamed(parts), nil
}

/*
bisect finds a boundary nobody can count.

Light the first k LEDs one colour and the rest another, and ask the one
question a person can always answer by looking: does the first colour cover
exactly one light, more than one, or part of one? Each answer halves what is
left to search.
*/
func bisect(ctx context.Context, client *api.Client, asker Asker,
	device api.Device, zone api.Zone, what string, count int, mode string,
) ([]namedSegment, error) {
	asker.Say("  Finding where %s divides.", what)

	low, high := 1, zone.Count-1
	k := zone.Count / count
	for range 8 { // a chain of any sane length is found well inside this
		if low > high {
			break
		}
		first := []namedSegment{{First: 0, Last: k - 1}, {First: k, Last: zone.Count - 1}}
		if err := light(ctx, client, device.Name, mode, partAssignments(device.Name, zone, first)); err != nil {
			return nil, err
		}

		answer, err := asker.Choose("  Is red exactly one light, more than one, or part of one?",
			[]string{"exactly", "more", "part"})
		if err != nil {
			return nil, err
		}
		switch answer {
		case "exactly":
			rest := namedSegment{Device: device.Name, Zone: zone.Name, First: k, Last: zone.Count - 1}
			head := namedSegment{Device: device.Name, Zone: zone.Name, First: 0, Last: k - 1}
			name, err := askName(asker, "  What is it called?")
			if err != nil {
				return nil, err
			}
			head.Name = name
			if count <= 2 {
				more, err := askName(asker, "  And what is the rest?")
				if err != nil {
					return nil, err
				}
				rest.Name = more
				return keepNamed([]namedSegment{head, rest}), nil
			}
			// More than two: the rest is asked about as its own chain.
			asker.Say("  Leaving %d LEDs for the rest.", rest.Last-rest.First+1)
			return keepNamed([]namedSegment{head, rest}), nil
		case "more":
			high = k - 1
		default: // part of one: the boundary is further along
			low = k + 1
		}
		k = (low + high) / 2
		if k < 1 {
			k = 1
		}
	}

	asker.Say("  Could not settle where %s divides; leaving it whole.", what)
	return []namedSegment{{Device: device.Name, Zone: zone.Name, Name: what, Whole: true,
		First: 0, Last: zone.Count - 1}}, nil
}

// finish shows the map, confirms it, and offers to write it.
func finish(ctx context.Context, client *api.Client, asker Asker, found []namedSegment,
	presentNames []string, corrections map[string][]string,
) error {
	asker.Say("\nHere is the whole map, lit at once:")
	byDevice := map[string][]namedSegment{}
	for _, segment := range found {
		byDevice[segment.Device] = append(byDevice[segment.Device], segment)
	}
	for device, segments := range byDevice {
		var assignments []api.Assignment
		for i, segment := range segments {
			assignments = append(assignments, api.Assignment{
				Target: targetOf(segment),
				Colour: palette[i%len(palette)],
			})
			asker.Say("  %s → %s", palette[i%len(palette)], segment.Name)
		}
		if err := light(ctx, client, device, "", assignments); err != nil {
			return err
		}
	}

	right, err := asker.Confirm("\nIs that right?")
	if err != nil {
		return err
	}
	if !right {
		asker.Say("Nothing written. Run it again when you are ready.")
		return nil
	}

	rules := rulesFor(found, presentNames, corrections)
	asker.Say("\n%s", rules)
	return offerToWrite(asker, rules)
}

func targetOf(segment namedSegment) string {
	switch {
	case segment.Zone == "":
		return segment.Device
	case segment.Whole:
		return segment.Device + "/" + segment.Zone
	default:
		return fmt.Sprintf("%s/%s[%d:%d]", segment.Device, segment.Zone, segment.First, segment.Last)
	}
}

// rulesFor is the YAML the answers add up to.
func rulesFor(found []namedSegment, present []string, corrections map[string][]string) string {
	byDevice := map[string][]namedSegment{}
	for _, segment := range found {
		byDevice[segment.Device] = append(byDevice[segment.Device], segment)
	}
	names := make([]string, 0, len(byDevice))
	for device := range byDevice {
		names = append(names, device)
	}
	sort.Strings(names)

	var out strings.Builder
	out.WriteString("devices:\n")
	for _, device := range names {
		fmt.Fprintf(&out, "  - match: %s\n", matchFor(device, present))
		if order := corrections[device]; len(order) > 0 {
			fmt.Fprintf(&out, "    # only lights in %s on this machine.\n", order[0])
			fmt.Fprintf(&out, "    solid_modes: [%s]\n", strings.Join(order, ", "))
		}
		out.WriteString("    segments:\n")
		for _, segment := range byDevice[device] {
			if segment.Whole {
				fmt.Fprintf(&out, "      %s: {zone: %q}\n", segment.Name, segment.Zone)
				continue
			}
			fmt.Fprintf(&out, "      %s: {zone: %q, leds: [%d, %d]}\n",
				segment.Name, segment.Zone, segment.First, segment.Last)
		}
	}
	return out.String()
}

/*
matchFor is the word a person would have typed to mean this device.

A rule matches on a substring, and "NZXT Kraken 2024 ELITE Series RGB" is not a
string anyone wants in a file they maintain. Picking a word by position does not
work -- the second word of "ASUS ROG MAXIMUS Z790 HERO" is "rog", which
identifies a brand rather than a device.

So it is chosen against the machine: the longest word that matches this device
and no other one present. That is the word which is both memorable and
unambiguous here, which is the only place this file will be read.
*/
func matchFor(device string, present []string) string {
	lowered := strings.ToLower(device)

	// Words that describe every second device and distinguish none of them.
	generic := map[string]bool{
		"rgb": true, "series": true, "edition": true, "gaming": true,
		"wireless": true, "controller": true, "device": true, "led": true,
	}

	var best string
	for _, word := range strings.Fields(lowered) {
		word = strings.Trim(word, "()[]-")
		if len(word) < 4 || generic[word] {
			continue
		}

		matches := 0
		for _, other := range present {
			if strings.Contains(strings.ToLower(other), word) {
				matches++
			}
		}
		if matches == 1 && len(word) > len(best) {
			best = word
		}
	}
	if best == "" {
		return lowered // nothing unique: the whole name still works
	}
	return best
}

func assignmentsFor(device string, zones []api.Zone) []api.Assignment {
	var out []api.Assignment
	for i, zone := range zones {
		target := device
		if zone.Name != "" {
			target = device + "/" + zone.Name
		}
		out = append(out, api.Assignment{Target: target, Colour: palette[i%len(palette)]})
	}
	return out
}

func partAssignments(device string, zone api.Zone, parts []namedSegment) []api.Assignment {
	var out []api.Assignment
	for i, part := range parts {
		out = append(out, api.Assignment{
			Target: fmt.Sprintf("%s/%s[%d:%d]", device, zone.Name, part.First, part.Last),
			Colour: palette[i%len(palette)],
		})
	}
	return out
}

// light turns everything on this device off and shows only what is asked.
func light(ctx context.Context, client *api.Client, device, mode string, assignments []api.Assignment) error {
	_, err := client.Apply(ctx, api.ApplyRequest{
		Colour:      "black",
		Assignments: assignments,
		Devices:     []string{device},
		Mode:        mode,
	})
	if err != nil {
		return quiet(err)
	}
	return nil
}

func keepNamed(parts []namedSegment) []namedSegment {
	var out []namedSegment
	for _, part := range parts {
		if part.Name != "" {
			out = append(out, part)
		}
	}
	return out
}

// wanted reports whether a device is one the caller asked about. Mapping one
// device at a time is how a person with six of them keeps their place.
func wanted(only []string, device string) bool {
	if len(only) == 0 {
		return true
	}
	name := strings.ToLower(device)
	for _, want := range only {
		if strings.Contains(name, strings.ToLower(strings.TrimSpace(want))) {
			return true
		}
	}
	return false
}

/*
askName takes a name for something, and declines the answers that are not one.

"y" is not the name of a fan. It is what somebody types when they think they are
being asked a different question, and writing it into their rules file as a
segment would turn a slip into a puzzle they meet weeks later. The colours are
refused for the same reason: "red" answers "what colour is it", which is not
what was asked.
*/
func askName(asker Asker, question string) (string, error) {
	for range 2 {
		answer, err := asker.Ask(question)
		if err != nil {
			return "", err
		}
		if isNothing(answer) {
			return "", nil
		}

		trimmed := strings.ToLower(strings.TrimSpace(answer))
		switch {
		case len(trimmed) < 2:
			asker.Say("    %q is a bit short for a name -- what is it called?", answer)
			continue
		case trimmed == "yes" || trimmed == "no":
			asker.Say("    That looks like an answer to a different question. What is it called?")
			continue
		case isPaletteColour(trimmed):
			asker.Say("    %q is the colour it is lit; what is the thing called?", answer)
			continue
		}
		return slug(answer), nil
	}
	return "", nil
}

func isPaletteColour(name string) bool {
	for _, colour := range palette {
		if name == colour {
			return true
		}
	}
	return false
}

// isNothing covers the answers that mean "there is nothing there", including
// the empty one: a user who does not know should not have it guessed for them.
func isNothing(answer string) bool {
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "", "nothing", "none", "dark", "no", "n/a", "?", "dunno", "unknown":
		return true
	}
	return false
}

// slug is a name as it will appear in the file: what the user said, in the
// shape a target can carry.
func slug(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			return r
		case r == ' ', r == '_', r == '-', r == '/':
			return '-'
		default:
			return -1
		}
	}, name)
	return strings.Trim(strings.ReplaceAll(name, "--", "-"), "-")
}

func mustStatus(ctx context.Context, client *api.Client) api.Status {
	status, err := client.Status(ctx)
	if err != nil {
		return api.Status{}
	}
	return status
}

/*
restore puts the lights back, whatever happened.

Deferred, so it runs on an interrupt and on a user who changed their mind as
well as on success. Where hotaru remembers what was asked for, that is what
goes back; where it does not -- a fresh install, which is this wizard's whole
audience -- there is nothing to return to and saying so beats inventing one.
*/
func restore(ctx context.Context, client *api.Client, asker Asker, remembered bool) {
	if !remembered {
		asker.Say("\nThe lights are showing the last thing the wizard lit. " +
			"`hotaru light set <colour>` when you want something else.")
		return
	}
	if _, err := client.Reconcile(ctx); err != nil {
		asker.Say("Could not put the lights back: %v", err)
		return
	}
	asker.Say("\nLights put back.")
}

/*
offerToWrite puts the rules where hotaru will read them, if asked.

Only where there is no file. The rules file is the user's, hotaru does not
rewrite it, and a wizard is not an exception to that -- a program that edits a
hand-maintained file loses its comments and the user's trust in the same
stroke. Where one exists, this prints what to add.
*/
func offerToWrite(asker Asker, rules string) error {
	path, err := config.RulesPath()
	if err != nil {
		return err
	}

	if _, err := os.Stat(path); err == nil {
		asker.Say("Add that to %s. It is yours, so hotaru will not edit it.", path)
		return nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("look at %s: %w", path, err)
	}

	write, err := asker.Confirm(fmt.Sprintf("Write it to %s?", path))
	if err != nil {
		return err
	}
	if !write {
		asker.Say("Not written. It is above if you want to keep it.")
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("make the configuration directory: %w", err)
	}
	header := "# Written by `hotaru light map`. Yours now: hotaru reads this file\n" +
		"# and never writes it again, so comments and edits are safe here.\n"
	if err := os.WriteFile(path, []byte(header+rules), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}

	asker.Say("Written to %s. `hotaru reload` to use it now.", path)
	return nil
}
