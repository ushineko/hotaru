package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

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
			asker := NewTerminal(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout())
			if err := Map(cmd.Context(), client, asker, names...); err != nil {
				if errors.Is(err, context.Canceled) {
					// Somebody changed their mind. That is not an error, and
					// printing a URL at them about it is not an answer.
					cmd.Println("Stopped. Nothing was written.")
					return errStopped
				}
				return err
			}
			return nil
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

	// What was answered last time, offered back as the defaults. Making
	// somebody retype "rad-front" to keep it is how a program stops being
	// re-run.
	existing, path := existingRules(asker)

	remembered := len(mustStatus(ctx, client).Remembered) > 0
	defer restore(ctx, client, asker, remembered)

	asker.Say("Lighting things one at a time and asking what you can see.")
	asker.Say("Answer in your own words -- these become the names you use. Press return to skip anything.\n")

	var found []namedSegment
	learned := map[string]notes{}
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
				asker.Say("  Nothing lit, whichever way it was asked. Leaving %s out.", device.Name)
			} else {
				asker.Say("  %s would not light at all, so there is nothing to see. Leaving it out.", device.Name)
			}
			continue
		}
		note := notes{}
		if working.corrected {
			note.solidModes = working.order
			asker.Say("  Noted: %s only lights one particular way, and that is written down.", device.Name)
		}

		segments, err := mapDevice(ctx, client, asker, device, working.mode, existing, names(list))
		if err != nil {
			return err
		}
		if len(segments) == 0 {
			continue // nothing named on it, so nothing to treat specially
		}
		found = append(found, segments...)

		/*
			A device already set to stay dark until touched is asked about.

			Otherwise the choice can never be revisited: it is only offered
			when a device fails to go dark, and a device configured this way
			no longer fails. Spec 008's reconfiguration pattern has to reach
			this answer too.
		*/
		if was := configuredDark(existing, device, names(list)); was != "" {
			keep, err := asker.Confirm(fmt.Sprintf(
				"  The %s is set to stay dark until you touch it. Keep that?", knownName(device)))
			if err != nil {
				return err
			}
			switch {
			case keep:
				note.solidModes = withFallbacks(device, was, note.solidModes)
			default:
				alt, chosen, err := chooseDark(ctx, client, asker, device, working.mode, true)
				switch {
				case err != nil:
					return err
				case alt != "":
					note.solidModes = withFallbacks(device, alt, note.solidModes)
				case chosen:
					note.plainAgain = true // lit all the time, as it was before
				default:
					note.solidModes = withFallbacks(device, was, note.solidModes)
				}
			}
		}

		// Brightness is offered only where the mode that will be used takes
		// one, which is rare enough that most runs are never asked.
		level, dimmable, err := dimmer(ctx, client, asker, device, working.mode)
		if err != nil {
			return err
		}
		note.brightness = level
		note.undimmable = !dimmable
		learned[device.Name] = note
	}

	if len(found) == 0 {
		asker.Say("\nNothing was named, so there is nothing to write.")
		return nil
	}
	return finish(ctx, client, asker, found, list, learned, existing, path)
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
	asker.Say("── %s", device.Name)

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
			Preview: true,
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
		settle(ctx, asker) // a slow device has not shown the colour yet; spec 014
		lit, err := asker.Confirm(fmt.Sprintf("  Is anything on it lit %s now?", probeColour))
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
plainModes are the ways of lighting a device worth trying, in the order a real
write would try them.

The order is the point. Trying them in the device's own order let somebody
answer "yes, it is lit" about a way of lighting their board that an ordinary
`light set` never chooses -- so the wizard blessed it, wrote no correction, and
setting a colour still turned their strips off afterwards. The probe has to ask
about what will actually be used, or its answer is about something else.

Solid-looking ones only. Setting Rainbow Wave to find out whether a device
lights would be a light show rather than a diagnostic, and nobody watching an
animation can answer "is it red" anyway.
*/
func plainModes(device api.Device) []string {
	// hotaru's own preference first, then anything else plain the device has.
	wanted := []string{"static", "direct", "custom", "solid color", "solid"}

	var out []string
	seen := map[string]bool{}
	for _, want := range wanted {
		for _, mode := range device.Modes {
			if strings.EqualFold(mode, want) && !seen[strings.ToLower(mode)] {
				seen[strings.ToLower(mode)] = true
				out = append(out, mode)
			}
		}
	}
	return out
}

/*
mapDevice lights one part of a device at a time and asks what lit up.

The first version lit every part at once, each a different colour, and asked
which was which. It saved rounds and cost sense: on a machine where two parts
of a device have nothing attached, somebody was asked to name "the red one"
while the only thing they could see was blue -- and "the green one" could have
meant a stick of RAM, a graphics card, or a header, because other devices in
the case were lit too.

So: everything else goes dark, one part is lit, and the question is what just
came on. No colour matching, nothing else competing for the answer, and
"nothing" is the expected answer for a header with nothing plugged into it.
*/
func mapDevice(ctx context.Context, client *api.Client, asker Asker, device api.Device, mode string,
	existing *config.Config, present []string,
) ([]namedSegment, error) {
	zones := device.Zones
	if len(zones) == 0 {
		zones = []api.Zone{{Name: "", First: 0, Count: device.LEDs}}
	}

	// Everything else off, so the only lit thing in the case is the thing
	// being asked about.
	if err := darken(ctx, client, present, device.Name); err != nil {
		return nil, err
	}
	if len(zones) > 1 {
		asker.Say("  It has %d parts that light separately. Going through them one at a time.", len(zones))
	}

	var named []namedSegment
	for _, zone := range zones {
		if err := light(ctx, client, asker, device.Name, mode, []api.Assignment{{
			Target: target(device.Name, zone.Name), Colour: probeColour,
		}}); err != nil {
			return nil, err
		}

		question := "  What is lit now?"
		if was := knownAs(existing, device.Name, zone.Name, present); was != "" {
			question = fmt.Sprintf("  What is lit now? [%s]", was)
		}
		asker.Say("    (press return if nothing is)")

		what, err := askNameOr(asker, question, knownAs(existing, device.Name, zone.Name, present))
		if err != nil {
			return nil, err
		}
		if what == "" {
			continue // nothing attached to this one
		}

		parts, err := split(ctx, client, asker, device, zone, what, mode)
		if err != nil {
			return nil, err
		}
		named = append(named, parts...)
	}
	return named, nil
}

// target names a zone, or the whole device where it has none.
func target(device, zone string) string {
	if zone == "" {
		return device
	}
	return device + "/" + zone
}

/*
darken turns off everything except the device being asked about.

Other devices in the case are lit too, often in the same colours: a question
about "the green one" can be answered with a stick of RAM. Nothing else being
lit removes the ambiguity rather than wording around it.
*/
func darken(ctx context.Context, client *api.Client, present []string, except string) error {
	for _, device := range present {
		if strings.EqualFold(device, except) {
			continue
		}
		if _, err := client.Apply(ctx, api.ApplyRequest{
			Colour: "black", Devices: []string{device}, Preview: true,
		}); err != nil {
			return quiet(err)
		}
	}
	return nil
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

	/*
		Only a line of lights can be several separate things.

		A keyboard is one keyboard with a hundred keys, and asked how many
		things were chained on it somebody quite reasonably wondered whether
		they were being asked about keys. The device says which of its parts
		are lines, which are grids and which are single lights; a question
		about chaining belongs only to the first.
	*/
	if zone.Shape != "" && zone.Shape != api.ShapeLine {
		return []namedSegment{whole}, nil
	}

	/*
		A zone with one light in it cannot be divided, whatever is plugged into
		it, so there is nothing to ask.

		The difference is electrical and hotaru already knows it: a 12V header
		carries one control signal and reports one LED, so ten strips chained
		onto it are one colour and one thing to name. An addressable header
		reports a light per LED, and there the boundary between two chained
		strips is a real place. Asking somebody to count things they cannot
		address invites an answer that is true about their case and useless
		here -- which is what happened.
	*/
	if zone.Count <= 1 {
		return []namedSegment{whole}, nil
	}

	/*
		Asked about things, not LEDs.

		"How many separate lights are on it" was read as "how many LEDs", which
		is a fair reading and the wrong answer: a stick of RAM with twelve LEDs
		is one thing. The question is whether several objects share this
		connector, which is what a daisy chain is and what a range exists to
		divide.
	*/
	asker.Say("    (several fans on one cable is how many fans; a single strip or stick is 1)")
	count, err := asker.Count(fmt.Sprintf("  How many separate things are chained on %s? [1]", what))
	if err != nil {
		return nil, err
	}
	if count <= 1 {
		return []namedSegment{whole}, nil
	}

	if zone.Count < count {
		/*
			More things than there are lights to address them with.

			A 12V header is one control for everything plugged into it: two
			strips on a splitter are two strips and one LED, and no amount of
			asking will separate them. Saying so is the whole answer -- the
			alternative is dividing one LED between two strips and reporting
			that the boundary could not be settled, which sounds like a fault.
		*/
		asker.Say("  %s has one light to set, so those %d change together.", what, count)
		return []namedSegment{whole}, nil
	}

	// A count that leaves about one LED each is almost always the other
	// reading of the question. Ask again rather than dividing on it.
	if zone.Count/count < 2 {
		asker.Say("    %d things sharing %d lights is about one light each, which is unusual.",
			count, zone.Count)
		asker.Say("    If you were counting the lights on it, that is %d, and the answer here is 1.", zone.Count)
		count, err = asker.Count(fmt.Sprintf("  How many separate things are chained on %s? [1]", what))
		if err != nil {
			return nil, err
		}
		if count <= 1 || zone.Count/count < 2 {
			return []namedSegment{whole}, nil
		}
	}

	if zone.Count%count != 0 {
		asker.Say("  %d lights do not divide evenly by %d, so where they split needs finding.", zone.Count, count)
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

	if err := light(ctx, client, asker, device.Name, mode, partAssignments(device.Name, zone, parts)); err != nil {
		return nil, err
	}
	right, err := asker.Confirm(fmt.Sprintf("  Each of the %d shows one colour?", count))
	if err != nil {
		return nil, err
	}
	if !right {
		// Before hunting for a boundary, check there is anything to see. A
		// header with nothing plugged into it answers "no" to every question
		// about what it is showing, and bisecting on that is a dead end with
		// no way out of it -- which is exactly what it was.
		lit, err := asker.Confirm("  Can you see it lit at all?")
		if err != nil {
			return nil, err
		}
		if !lit {
			asker.Say("    Nothing on it, then -- leaving it out.")
			return nil, nil
		}
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
		if err := light(ctx, client, asker, device.Name, mode, partAssignments(device.Name, zone, first)); err != nil {
			return nil, err
		}

		asker.Say("    The first %d lights are red and the rest are %s. Nothing else is lit.", k, palette[1])
		answer, err := asker.Choose(
			"  Does the red part cover exactly one thing, more than one, or part of one?",
			[]string{"exactly", "more", "part", "cannot-tell"})
		if err != nil {
			return nil, err
		}
		if answer == "cannot-tell" {
			asker.Say("    Leaving %s undivided, then.", what)
			return []namedSegment{{Device: device.Name, Zone: zone.Name, Name: what, Whole: true,
				First: 0, Last: zone.Count - 1}}, nil
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
			asker.Say("  Leaving %d lights for the rest.", rest.Last-rest.First+1)
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
func names(list []api.Device) []string {
	out := make([]string, 0, len(list))
	for _, device := range list {
		out = append(out, device.Name)
	}
	return out
}

func finish(ctx context.Context, client *api.Client, asker Asker, found []namedSegment,
	present []api.Device, learned map[string]notes, existing *config.Config, path string,
) error {
	presentNames := names(present)
	asker.Say("\nEverything you named, lit at once:")
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
		if err := light(ctx, client, asker, device, "", assignments); err != nil {
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

	if err := blanking(ctx, client, asker, present, learned); err != nil {
		return err
	}

	merged := merge(existing, found, learned, presentNames)
	rules := yamlFor(merged)
	asker.Say("\n%s", rules)
	return offerToWrite(ctx, client, asker, rules, path)
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
func light(ctx context.Context, client *api.Client, asker Asker, device, mode string, assignments []api.Assignment) error {
	_, err := client.Apply(ctx, api.ApplyRequest{
		Colour:      "black",
		Assignments: assignments,
		Devices:     []string{device},
		Mode:        mode,
		Preview:     true,
	})
	if err != nil {
		return quiet(err)
	}
	settle(ctx, asker)
	return nil
}

/*
lookDelay is how long a device is given to show a colour before somebody is
asked what they can see.

Nothing announces that a device is slow. The development machine's cooler ring
takes about half a second; its fans are immediate, and so is everything else on
that desk. Asked in the same breath as the write, the honest answer to "what is
lit now?" is whatever was there before -- which is what happened, and the ring
went unnamed because of it.

Long enough for the slowest thing measured here, short enough that nobody
notices waiting. See spec 014.
*/
const lookDelay = 500 * time.Millisecond

// settle waits for the hardware to catch up, or returns early if the run is
// being abandoned. It waits only where somebody is looking; see Watcher.
func settle(ctx context.Context, asker Asker) {
	if w, ok := asker.(Watcher); !ok || !w.Watching() {
		return
	}
	select {
	case <-ctx.Done():
	case <-time.After(lookDelay):
	}
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
	answer, err := asker.Ask(question)
	if err != nil {
		return "", err
	}
	return validName(asker, answer, question)
}

// validName checks an answer, asking again once when it is plainly not a name.
func validName(asker Asker, answer, question string) (string, error) {
	for range 2 {
		if isNothing(answer) {
			return "", nil
		}

		trimmed := strings.ToLower(strings.TrimSpace(answer))
		switch {
		case slug(answer) == "":
			// "??" is somebody saying they do not know, not a name.
			asker.Say("    Nothing to name there, then -- leaving it out.")
			return "", nil
		case len(trimmed) < 2:
			asker.Say("    %q is a bit short for a name -- what is it called?", answer)
		case trimmed == "yes" || trimmed == "no":
			asker.Say("    That looks like an answer to a different question. What is it called?")
		case isPaletteColour(trimmed):
			asker.Say("    %q is the colour it is lit; what is the thing called?", answer)
		default:
			return slug(answer), nil
		}

		next, err := asker.Ask(question)
		if err != nil {
			return "", err
		}
		answer = next
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
	/*
		Cleanup runs on the way out, including the way out through Ctrl-C --
		when the context that got us here is already cancelled. Using it would
		mean the lights stay on whatever the wizard last lit, and the failure
		reported as the same interrupt that caused it.
	*/
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()

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
offerToWrite saves the rules, over an existing file if there is one.

Editing a file somebody maintains is only rude when it happens behind their
back. Said plainly, with what was there kept beside it, a re-run is how they
change their mind -- which is the whole reason to run this a second time.

Comments do not survive: the file is regenerated from what hotaru understands.
That is said before anything is written, and the previous file is kept as
.yml.bak, so the cost is one somebody accepted rather than one imposed on them.
*/
func offerToWrite(ctx context.Context, client *api.Client, asker Asker, rules, path string) error {
	_, err := os.Stat(path)
	switch {
	case err == nil:
		asker.Say("%s already exists. Writing this replaces it, and any comments in it are lost;", path)
		asker.Say("the previous version is kept as %s.bak.", path)
	case !errors.Is(err, fs.ErrNotExist):
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
	// G703: the path is hotaru's own configuration path, resolved from XDG and
	// a constant filename, and the backup is that path plus a suffix. Nothing
	// a user typed reaches either.
	if previous, err := os.ReadFile(path); err == nil { //nolint:gosec
		if err := os.WriteFile(path+".bak", previous, 0o600); err != nil { //nolint:gosec
			return fmt.Errorf("keep a copy of %s: %w", path, err)
		}
	}
	if err := os.WriteFile(path, []byte(rules), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}

	/*
		And put it to work, rather than telling somebody to.

		The service reads its rules when it starts, so a file written here
		changes nothing until it is told. Leaving that as an instruction meant
		the lights were put back at the end of the wizard using the rules from
		before it ran -- which on one machine meant the correction it had just
		established was ignored, and the strips it had been lighting went out
		again as the last act of a successful run.
	*/
	if _, err := client.Reload(ctx); err != nil {
		asker.Say("Written to %s. Run `hotaru reload` to use it.", path)
		return nil //nolint:nilerr // written is the outcome; reloading is a convenience
	}

	asker.Say("Written to %s, and in use now.", path)
	return nil
}

/*
existingRules is the configuration as it stands, for defaults and for keeping
rules about devices this run does not ask about.

A file that will not parse is not a reason to refuse: the wizard is often what
somebody reaches for when their configuration has gone wrong, and starting from
empty is a worse answer than starting from nothing.
*/
func existingRules(asker Asker) (*config.Config, string) {
	path, err := config.RulesPath()
	if err != nil {
		return &config.Config{}, ""
	}

	cfg, problems, err := config.Load(path)
	if err != nil {
		asker.Say("Could not read %s (%v), so this starts from nothing.", path, err)
		return &config.Config{}, path
	}
	for _, problem := range problems {
		asker.Say("  %s", problem.Error())
	}
	return cfg, path
}

/*
askNameOr is askName with something to fall back on.

An empty answer keeps what was there last time, which is what "press return" is
for when a question already has an answer. Saying "none" is how somebody clears
one deliberately, and the difference matters: one is agreement, the other is a
decision.
*/
func askNameOr(asker Asker, question, fallback string) (string, error) {
	answer, err := asker.Ask(question)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(answer) == "" {
		return fallback, nil // agreement, or nothing, depending on what was there
	}
	return validName(asker, answer, question)
}

/*
notes are the corrections a device needs that no protocol reports.

Three facts, each learned the same way the rest of the wizard learns: do the
thing, and ask what happened in the room. See spec 014.
*/
type notes struct {
	solidModes []string
	neverBlank bool
	brightness *int
	// undimmable is set when the mode this device is driven in takes no
	// brightness. A rule from an earlier run may carry one -- this wizard
	// wrote one itself before it knew better -- and a setting that cannot
	// apply is worse in a file than absent.
	undimmable bool
	// plainAgain undoes a stay-dark mode chosen on an earlier run.
	plainAgain bool
}

// dimLevel is what "turned down" means when the wizard offers it. A number
// rather than a question: somebody asked to pick a percentage is being asked
// about hotaru rather than about their desk.
const dimLevel = 40

/*
darkish is a mode that renders mostly dark, if the device has one.

A keyboard that cannot be blanked can still be quiet: its reactive modes leave
unpressed keys unlit and ripple the colour under typing, which is what somebody
means by a keyboard that is off. The Python found the same thing and named
`solid splash` outright; matching on what the modes are called finds it on
hardware nobody has written down.
*/
/*
likelyDark are words that name a mode which lights up under use.

A hint for ordering, never a filter. These are the words this desk's Keychron
happens to use, and peripheral-battery-monitor settled on Solid Splash after
living with it: it ripples outward from each keypress rather than lighting only
the key struck, which reads as the board answering rather than blinking.

A SteelSeries Apex Pro names its effects differently, and so does everything
else. Matching on these words alone would offer nothing at all on hardware
nobody here has seen -- which is the over-fitting this project is supposed to
avoid.
*/
var likelyDark = []string{"splash", "reactive", "ripple", "typing", "press", "react", "key"}

/*
darkishModes is every way a device can be lit, likeliest-first.

Not a filtered list. hotaru cannot know which of a keyboard's twenty effects
leaves most of it dark: the names are the vendor's, the behaviour is in the
firmware, and the only instrument that can tell is somebody looking at the
board. So everything is offered, ordered so the good guesses come first, and
the person decides by watching.
*/
func darkishModes(device api.Device) []string {
	var out []string
	add := func(mode string) {
		if !slices.Contains(out, mode) && !strings.EqualFold(mode, "off") {
			out = append(out, mode)
		}
	}
	for _, want := range likelyDark {
		for _, mode := range device.Modes {
			if strings.Contains(strings.ToLower(mode), want) {
				add(mode)
			}
		}
	}
	// Then the rest, in the order the device lists them, so a machine whose
	// vocabulary nobody anticipated still has every option on the table.
	for _, mode := range device.Modes {
		add(mode)
	}
	return out
}

/*
chooseDark shows the ways a device can be lit, and lets somebody browse them.

A menu rather than a march. Offered one at a time with "Use this one?", five
times over, somebody decides about each without having seen the rest, cannot go
back to the one they liked, and is not even told which one they are looking at.
A list they can pick from, repeatedly, is how a person chooses between things
that look different on their desk.

The names are the vendor's own. "Solid Reactive Multinexus" means nothing to
anybody, but it is what OpenRGB calls it and it is the only handle that exists
for "the one with the ripple" once the light has moved on.

An empty mode with chosen set means the plain steady light: how somebody stops
using one of these after an earlier run set one.
*/
func chooseDark(ctx context.Context, client *api.Client, asker Asker, device api.Device,
	steady string, revisiting bool,
) (string, bool, error) {
	options := darkishModes(device)
	if len(options) == 0 {
		return "", false, nil
	}
	plain := len(options) + 1

	if revisiting {
		asker.Say("  Here is everything it can do, likeliest first.")
	} else {
		asker.Say("  It can be lit in these ways. The first few usually stay dark")
		asker.Say("  until you touch it, but only looking at it will tell.")
	}
	for i, mode := range options {
		asker.Say("    %d  %s", i+1, mode)
	}
	asker.Say("    %d  lit all the time, no reacting", plain)
	asker.Say("  Type on it while one is showing to see what it does.")

	for {
		answer, err := asker.Ask(fmt.Sprintf("  Show which? [1-%d, or return to leave it as it is]", plain))
		if err != nil {
			return "", false, err
		}
		if strings.TrimSpace(answer) == "" {
			asker.Say("  Leaving it as it was, then.")
			return "", false, nil
		}
		pick, err := strconv.Atoi(strings.TrimSpace(answer))
		if err != nil || pick < 1 || pick > plain {
			asker.Say("  Pick a number from the list, or press return to leave it alone.")
			continue
		}

		mode := steady
		if pick <= len(options) {
			mode = options[pick-1]
		}
		if err := light(ctx, client, asker, device.Name, mode, nil); err != nil {
			return "", false, err
		}
		use, err := asker.Confirm("  Use this one?")
		if err != nil {
			return "", false, err
		}
		if !use {
			continue // back to the list; changing your mind is the point
		}
		if pick == plain {
			return "", true, nil
		}
		return mode, true, nil
	}
}

/*
withFallbacks puts a chosen mode first and keeps somewhere to go behind it.

A single mode leaves nothing to fall back on if firmware changes under it, and
this list is what hotaru tries in order -- the Python's is
("solid splash", "direct", "solid color") for the same reason.
*/
func withFallbacks(device api.Device, chosen string, had []string) []string {
	if chosen == "" {
		return had
	}
	order := []string{chosen}
	for _, fallback := range append(without(had, chosen), "direct", "static") {
		known := slices.ContainsFunc(device.Modes, func(have string) bool {
			return strings.EqualFold(have, fallback)
		})
		already := slices.ContainsFunc(order, func(have string) bool {
			return strings.EqualFold(have, fallback)
		})
		if known && !already {
			order = append(order, fallback)
		}
	}
	return order
}

/*
configuredDark is the stay-dark mode a rules file already names for a device.

Matched against what the device advertises: a file naming a mode this hardware
does not have belongs to somebody else's machine, and asking about it would be
asking about nothing.
*/
func configuredDark(existing *config.Config, device api.Device, present []string) string {
	if existing == nil {
		return ""
	}
	match := matchFor(device.Name, present)
	for _, rule := range existing.Devices {
		if !strings.EqualFold(rule.Match, match) {
			continue
		}
		for _, mode := range rule.SolidModes {
			if slices.Contains(darkishModes(device), mode) {
				return mode
			}
		}
	}
	return ""
}

/*
habits asks the questions that decide how a device is treated, not what its
parts are called.

Every one is demonstrated. "Should this device be excluded from turning off?"
is a question about hotaru's configuration; "I have turned it off, is it dark?"
is a question about the room, and somebody answers it by looking up.
*/
func blanking(ctx context.Context, client *api.Client, asker Asker,
	list []api.Device, learned map[string]notes,
) error {
	if _, err := client.Apply(ctx, api.ApplyRequest{Off: true, Preview: true}); err != nil {
		return quiet(err)
	}
	settle(ctx, asker)

	/*
		One question for the whole machine. On hardware that blanks properly --
		which is most of it -- this is the only one asked, and the per-device
		round below never runs.

		The wording matters more than the mechanism. An earlier version said
		"Everything is off now. Is anything still lit?", which asserts the
		thing it is asking about: somebody looking at a keyboard glowing white
		answered no, because the program had just told them everything was
		off and white must therefore be what off looks like. That is the exact
		failure this question exists to catch, invited by the question.

		So it says what *should* have happened, and asks what did.
	*/
	asker.Say("\nEverything hotaru controls should be dark now.")
	stillLit, err := asker.Confirm("Is anything still glowing or lit up?")
	if err != nil {
		return err
	}
	if !stillLit {
		return nil
	}

	for _, device := range list {
		note, mapped := learned[device.Name]
		if !mapped {
			continue
		}
		lit, err := asker.Confirm(fmt.Sprintf("  Is the %s still glowing?", knownName(device)))
		if err != nil {
			return err
		}
		if !lit {
			continue
		}
		note.neverBlank = true
		asker.Say("  Noted: it is left alone instead of being turned off.")

		alt, chosen, err := chooseDark(ctx, client, asker, device, device.ActiveMode, false)
		if err != nil {
			return err
		}
		if chosen && alt != "" {
			note.solidModes = withFallbacks(device, alt, note.solidModes)
		}
		learned[device.Name] = note
	}
	return nil
}

// dimmer offers to turn a device down, for devices that have a brightness at
// all. Demonstrated, and never asked as a number.
func dimmer(ctx context.Context, client *api.Client, asker Asker, device api.Device, mode string) (*int, bool, error) {
	// Only where the mode hotaru will actually write in takes one. A device
	// with a dimmable animation and an undimmable Direct cannot be dimmed by
	// anything hotaru does, and offering it is a demonstration that
	// demonstrates nothing.
	if !slices.Contains(device.Dimmable, mode) {
		return nil, false, nil
	}
	level := dimLevel
	if _, err := client.Apply(ctx, api.ApplyRequest{
		Colour: probeColour, Devices: []string{device.Name}, Mode: mode,
		Preview: true, Brightness: &level,
	}); err != nil {
		return nil, true, quiet(err)
	}
	settle(ctx, asker)
	quieter, err := asker.Confirm(fmt.Sprintf("  %s can be turned down. Keep it dimmer?", knownName(device)))
	if err != nil {
		return nil, true, err
	}
	if !quieter {
		return nil, true, nil
	}
	return &level, true, nil
}

// knownName is the shortest thing a person would call this device.
func knownName(device api.Device) string {
	if f := strings.Fields(device.Name); len(f) > 0 {
		return strings.ToLower(f[0])
	}
	return device.Name
}

func without(list []string, drop string) []string {
	var out []string
	for _, item := range list {
		if !strings.EqualFold(item, drop) {
			out = append(out, item)
		}
	}
	return out
}
