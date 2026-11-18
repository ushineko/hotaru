package cli

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ushineko/hotaru/internal/api"
	"github.com/ushineko/hotaru/internal/config"
)

/*
What a board cannot tell hotaru, found by looking, one thing at a time (spec
078): a header's length, what is on a header, and the names a person wants.
Each lights what it is about while it runs, and writes through the service,
as the window does. They replace light map, whose answers were judgements
made on a question; here the answer is on the desk.
*/

// zoneArg is a device and one of its zones, from "device/zone".
func zoneArg(ctx context.Context, client *api.Client, arg string) (api.Device, api.Zone, error) {
	device, zone, ok := strings.Cut(arg, "/")
	if !ok || zone == "" {
		return api.Device{}, api.Zone{}, fmt.Errorf("%q: a device and a zone, device/zone", arg)
	}
	found, err := client.Devices(ctx)
	if err != nil {
		return api.Device{}, api.Zone{}, quiet(err)
	}
	target, ok := named(found, device)
	if !ok {
		return api.Device{}, api.Zone{}, fmt.Errorf("no device called %q", device)
	}
	for _, z := range target.Zones {
		if strings.EqualFold(z.Name, zone) {
			return target, z, nil
		}
	}
	return api.Device{}, api.Zone{}, fmt.Errorf("%s has no zone called %q; it has %s", target.Name, zone, zoneNames(target))
}

func zoneNames(device api.Device) string {
	names := make([]string, 0, len(device.Zones))
	for _, z := range device.Zones {
		names = append(names, z.Name)
	}
	return strings.Join(names, ", ")
}

// show lights assignments on one device without recording them.
func show(ctx context.Context, client *api.Client, device string, assignments []api.Assignment) error {
	_, err := client.Apply(ctx, api.ApplyRequest{Devices: []string{device}, Assignments: assignments, Preview: true})
	return quiet(err)
}

// lightsOf is a zone's lights first to last as a target.
func lightsOf(device, zone string, first, last int) string {
	return fmt.Sprintf("%s/%s[%d:%d]", device, zone, first, last)
}

// endMarked is a zone of n lit green, every tenth white and the last red:
// where the end sits is seen against the strip.
func endMarked(device, zone string, n int) []api.Assignment {
	out := []api.Assignment{{Target: lightsOf(device, zone, 0, n-1), Colour: "green"}}
	for i := 9; i < n-1; i += 10 {
		out = append(out, api.Assignment{Target: lightsOf(device, zone, i, i), Colour: "white"})
	}
	return append(out, api.Assignment{Target: lightsOf(device, zone, n-1, n-1), Colour: "red"})
}

/*
FindLength finds a header's length by looking (spec 078 R1): the zone lit
green, a white light every ten and the last red, moved until the red is on
the strip's last light, then written to the rules.
*/
func FindLength(ctx context.Context, client *api.Client, asker Asker, arg string) error {
	device, zone, err := zoneArg(ctx, client, arg)
	if err != nil {
		return err
	}
	if zone.Max == 0 {
		return fmt.Errorf("%s counts the lights on %s itself: %d", device.Name, zone.Name, zone.Count)
	}
	was := zone.Count
	n := max(was, 1)
	defer func() { _, _ = client.Reconcile(context.WithoutCancel(ctx)) }()

	asker.Say("%s on %s: green, a white light every ten, and the last one red.", zone.Name, device.Name)
	asker.Say("Move the end until the red is on the strip's last light.")
	for {
		if err := client.Length(ctx, device.Name, zone.Name, n); err != nil {
			return quiet(err)
		}
		if err := show(ctx, client, device.Name, endMarked(device.Name, zone.Name, n)); err != nil {
			return err
		}
		answer, err := asker.Ask(fmt.Sprintf("  %d lights: + - (one), ++ -- (ten), a number, Return to keep, q to leave:", n))
		if err != nil {
			return err
		}
		answer = strings.TrimSpace(answer)
		step := map[string]int{"+": 1, "-": -1, "++": 10, "--": -10}
		switch {
		case answer == "":
			if _, err := client.Rule(ctx, api.RuleRequest{Device: device.Name, Lengths: map[string]int{zone.Name: n}}); err != nil {
				return quiet(err)
			}
			asker.Say("%s has %d lights, written to the rules.", zone.Name, n)
			return nil
		case answer == "q":
			asker.Say("Left as it was. Nothing was written.")
			if was > 0 {
				return quiet(client.Length(context.WithoutCancel(ctx), device.Name, zone.Name, was))
			}
			return nil
		case step[answer] != 0:
			n += step[answer]
		default:
			typed, err := strconv.Atoi(answer)
			if err != nil {
				asker.Say("  +, -, ++, --, a number, Return or q.")
				continue
			}
			n = typed
		}
		n = min(max(n, 1), zone.Max)
	}
}

/*
fanNames is a name for each of n parts of a zone: the one a segment already
has for those very lights, else the zone's name and the fan's number.
*/
func fanNames(cfg *config.Config, device api.Device, zone api.Zone, size, n int) []string {
	out := make([]string, n)
	for i := range n {
		first, last := i*size, (i+1)*size-1
		for _, rule := range cfg.RulesFor(device.Name) {
			for name, seg := range rule.Segments {
				if strings.EqualFold(seg.Zone, zone.Name) && seg.LEDs != nil && seg.LEDs.First == first && seg.LEDs.Last == last {
					out[i] = name
				}
			}
		}
		if out[i] == "" {
			out[i] = fmt.Sprintf("%s-fan-%d", slug(strings.TrimPrefix(strings.ToLower(zone.Name), "addressable rgb ")), i+1)
		}
	}
	return out
}

/*
SetFans says a header holds n fans (spec 078 R2.1): the zone divided into n
equal parts, each named, the zone a ring, each part lit a colour to check.
Segments in the zone that are not one of the parts are removed: they divided
it another way.
*/
func SetFans(ctx context.Context, client *api.Client, asker Asker, arg string, n int) error {
	device, zone, err := zoneArg(ctx, client, arg)
	if err != nil {
		return err
	}
	if len(zone.Parts) > 0 {
		return fmt.Errorf("%s counts what is on %s itself: %d parts", device.Name, zone.Name, len(zone.Parts))
	}
	if n < 1 || zone.Count%n != 0 {
		low := zone.Count / max(n, 1) * max(n, 1)
		return fmt.Errorf("%d lights do not divide into %d fans; %d or %d do (light length sets it)",
			zone.Count, n, low, low+max(n, 1))
	}
	cfg, _ := existingRules(asker)
	size := zone.Count / n
	names := fanNames(cfg, device, zone, size, n)

	segments := map[string]*api.RuleSegment{}
	for _, rule := range cfg.RulesFor(device.Name) {
		for name, seg := range rule.Segments {
			if strings.EqualFold(seg.Zone, zone.Name) && !slices.Contains(names, name) && !slices.Contains(rule.Toggles, name) {
				segments[name] = nil
			}
		}
	}
	var lit []api.Assignment
	for i, name := range names {
		first, last := i*size, (i+1)*size-1
		segments[name] = &api.RuleSegment{Zone: zone.Name, First: &first, Last: &last}
		lit = append(lit, api.Assignment{Target: lightsOf(device.Name, zone.Name, first, last), Colour: palette[i%len(palette)]})
	}
	ring := string(api.ShapeRing)
	if _, err := client.Rule(ctx, api.RuleRequest{
		Device: device.Name, Shapes: map[string]string{zone.Name: ring}, Segments: segments,
	}); err != nil {
		return quiet(err)
	}
	defer func() { _, _ = client.Reconcile(context.WithoutCancel(ctx)) }()
	if err := show(ctx, client, device.Name, lit); err != nil {
		return err
	}
	asker.Say("%s holds %d fans of %d lights, written to the rules:", zone.Name, n, size)
	for i, name := range names {
		asker.Say("  %s: %s", palette[i%len(palette)], name)
	}
	_, err = asker.Ask("Each fan is one colour? Return when you have looked; `hotaru light name` renames them.")
	return err
}

// SetStrip says a header holds a strip (spec 078 R2.2).
func SetStrip(ctx context.Context, client *api.Client, asker Asker, arg string) error {
	device, zone, err := zoneArg(ctx, client, arg)
	if err != nil {
		return err
	}
	if len(zone.Parts) > 0 {
		return fmt.Errorf("%s counts what is on %s itself: %d parts", device.Name, zone.Name, len(zone.Parts))
	}
	if _, err := client.Rule(ctx, api.RuleRequest{
		Device: device.Name, Shapes: map[string]string{zone.Name: string(api.ShapeLine)},
	}); err != nil {
		return quiet(err)
	}
	asker.Say("%s holds a strip, written to the rules.", zone.Name)
	return nil
}

// namedPart is one thing a name can be given: a zone, or lights of one.
type namedPart struct {
	zone        string
	first, last int
	whole       bool
}

func (p namedPart) target(device string) string {
	if p.whole {
		return device + "/" + p.zone
	}
	return lightsOf(device, p.zone, p.first, p.last)
}

func (p namedPart) segment() *api.RuleSegment {
	if p.whole {
		return &api.RuleSegment{Zone: p.zone}
	}
	first, last := p.first, p.last
	return &api.RuleSegment{Zone: p.zone, First: &first, Last: &last}
}

/*
parts is what a device's names are given to, in order: each zone, or each of
its fans where the device counts them or the rules divide it, as hotaru draws
it; with the name each has now.
*/
func parts(cfg *config.Config, device api.Device) ([]namedPart, []string) {
	var out []namedPart
	var names []string
	nameOf := func(p namedPart) string {
		for _, rule := range cfg.RulesFor(device.Name) {
			for name, seg := range rule.Segments {
				if !strings.EqualFold(seg.Zone, p.zone) && !formerZone(rule, seg.Zone, p.zone) {
					continue
				}
				if p.whole && seg.LEDs == nil || !p.whole && seg.LEDs != nil && seg.LEDs.First == p.first && seg.LEDs.Last == p.last {
					return name
				}
			}
		}
		return ""
	}
	for _, zone := range device.Zones {
		var split []namedPart
		first := 0
		for _, n := range zone.Parts {
			split = append(split, namedPart{zone: zone.Name, first: first, last: first + n - 1})
			first += n
		}
		if len(split) < 2 {
			split = nil
			for _, rule := range cfg.RulesFor(device.Name) {
				for _, seg := range rule.Segments {
					if strings.EqualFold(seg.Zone, zone.Name) && seg.LEDs != nil {
						split = append(split, namedPart{zone: zone.Name, first: seg.LEDs.First, last: seg.LEDs.Last})
					}
				}
			}
			slices.SortFunc(split, func(a, b namedPart) int { return a.first - b.first })
		}
		if len(split) < 2 {
			split = []namedPart{{zone: zone.Name, whole: true}}
		}
		for _, p := range split {
			out = append(out, p)
			names = append(names, nameOf(p))
		}
	}
	return out, names
}

// formerZone is whether a segment's zone is the one a device's zone was
// formerly called (spec 073).
func formerZone(rule config.DeviceRule, was, zone string) bool {
	if rule.Formerly == nil {
		return false
	}
	for old, now := range rule.Formerly.Zones {
		if strings.EqualFold(old, was) && strings.EqualFold(now, zone) {
			return true
		}
	}
	return false
}

/*
NameOne names a zone or lights of one (spec 078 R3.1), lit while it is asked:
"device/zone" or "device/zone[a:b]".
*/
func NameOne(ctx context.Context, client *api.Client, asker Asker, arg, name string) error {
	name = slug(name)
	if name == "" {
		return errors.New("a name is lowercase words and dashes")
	}
	head, picks, ranged := strings.Cut(arg, "[")
	device, zone, err := zoneArg(ctx, client, head)
	if err != nil {
		return err
	}
	part := namedPart{zone: zone.Name, whole: true}
	if ranged {
		a, b, ok := strings.Cut(strings.TrimSuffix(picks, "]"), ":")
		first, errA := strconv.Atoi(a)
		last, errB := strconv.Atoi(b)
		if !ok || errA != nil || errB != nil || first < 0 || last < first || last >= zone.Count {
			return fmt.Errorf("%q: lights first:last, within the zone's %d", arg, zone.Count)
		}
		part = namedPart{zone: zone.Name, first: first, last: last}
	}
	defer func() { _, _ = client.Reconcile(context.WithoutCancel(ctx)) }()
	if err := show(ctx, client, device.Name, []api.Assignment{{Target: part.target(device.Name), Colour: "white"}}); err != nil {
		return err
	}
	ok, err := asker.Confirm(fmt.Sprintf("Is the white one %s?", name))
	if err != nil || !ok {
		return err
	}
	if _, err := client.Rule(ctx, api.RuleRequest{
		Device: device.Name, Segments: map[string]*api.RuleSegment{name: part.segment()},
	}); err != nil {
		return quiet(err)
	}
	asker.Say("%s is %s, written to the rules.", part.target(device.Name), name)
	return nil
}

/*
NameAll goes through a device's zones and fans, each lit, offering its name
(spec 078 R3.2): Return keeps it, a word replaces it, and "-" removes it.
*/
func NameAll(ctx context.Context, client *api.Client, asker Asker, device string) error {
	found, err := client.Devices(ctx)
	if err != nil {
		return quiet(err)
	}
	target, ok := named(found, device)
	if !ok {
		return fmt.Errorf("no device called %q", device)
	}
	cfg, _ := existingRules(asker)
	all, names := parts(cfg, target)
	defer func() { _, _ = client.Reconcile(context.WithoutCancel(ctx)) }()

	segments := map[string]*api.RuleSegment{}
	for i, p := range all {
		if err := show(ctx, client, target.Name, []api.Assignment{{Target: p.target(target.Name), Colour: "white"}}); err != nil {
			return err
		}
		question := "What is the white one?"
		if names[i] != "" {
			question = fmt.Sprintf("What is the white one? [%s]", names[i])
		}
		answer, err := asker.Ask(question + " (- to remove)")
		if err != nil {
			return err
		}
		answer = strings.TrimSpace(answer)
		switch {
		case answer == "":
		case answer == "-":
			if names[i] != "" {
				segments[names[i]] = nil
			}
		case slug(answer) == "":
			asker.Say("  Not a name; left as it was.")
		default:
			if names[i] != "" && names[i] != slug(answer) {
				segments[names[i]] = nil
			}
			segments[slug(answer)] = p.segment()
		}
	}
	if len(segments) == 0 {
		asker.Say("Nothing changed.")
		return nil
	}
	if _, err := client.Rule(ctx, api.RuleRequest{Device: target.Name, Segments: segments}); err != nil {
		return quiet(err)
	}
	asker.Say("Written to the rules.")
	return nil
}

func lengthsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "length <device>/<zone> [lights]",
		Short: "Set the length of a header its board cannot count",
		Long: `Set the length of a zone its device cannot count, such as an addressable
header's strip or fans. With no number, the zone is lit green, with a white
light every ten and the last one red: move the end with + and -, or ++ and --
by ten, until the red is on the strip's last light, and press Return to write
it to the rules. With a number, that length is tried until the service stops.

	hotaru light length maximus/"Addressable RGB Header 3"`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := client(cmd)
			if err != nil {
				return err
			}
			if len(args) == 1 {
				return interactive(cmd, func(asker Asker) error { return FindLength(cmd.Context(), client, asker, args[0]) })
			}
			return tryLength(cmd, client, args[0], args[1])
		},
	}
	return cmd
}

func fansCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "fans <device>/<zone> <how many>",
		Short: "Say a header holds fans, and how many",
		Long: `Say a header holds fans: its lights are divided into that many equal parts,
each drawn as a ring, and each lit a colour so the division can be checked.
Each is named; a name already given to the same lights is kept.

	hotaru light fans maximus/"Addressable RGB Header 2" 2`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := client(cmd)
			if err != nil {
				return err
			}
			n, err := strconv.Atoi(args[1])
			if err != nil {
				return fmt.Errorf("%q: a number of fans", args[1])
			}
			return interactive(cmd, func(asker Asker) error { return SetFans(cmd.Context(), client, asker, args[0], n) })
		},
	}
}

func stripCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "strip <device>/<zone>",
		Short: "Say a header holds a strip",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := client(cmd)
			if err != nil {
				return err
			}
			return interactive(cmd, func(asker Asker) error { return SetStrip(cmd.Context(), client, asker, args[0]) })
		},
	}
}

func nameCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "name <device>[/<zone>[first:last]] [name]",
		Short: "Name a part of a device, seeing it lit",
		Long: `Name a part of a device, so a scene can say rear-fan=red. With a zone or a
range and a name, it is lit white and named once you say it is the one. With
only a device, each of its zones and fans is lit in turn: Return keeps its
name, a word replaces it, and - removes it. Names are optional: a scene can
always say device/zone.

	hotaru light name maximus/"Addressable RGB Header 3" rear-fan
	hotaru light name kraken`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := client(cmd)
			if err != nil {
				return err
			}
			if len(args) == 1 {
				return interactive(cmd, func(asker Asker) error { return NameAll(cmd.Context(), client, asker, args[0]) })
			}
			return interactive(cmd, func(asker Asker) error { return NameOne(cmd.Context(), client, asker, args[0], args[1]) })
		},
	}
}

// interactive runs fn with a terminal asker, saying a stop plainly.
func interactive(cmd *cobra.Command, fn func(Asker) error) error {
	err := fn(NewTerminal(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout()))
	if errors.Is(err, context.Canceled) {
		cmd.Println("Stopped. Nothing more was written.")
		return errStopped
	}
	return err
}
