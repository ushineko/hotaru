package devices

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/ushineko/hotaru/internal/config"
)

/*
Target is what a colour is being applied to.

	kraken                      the whole device, every zone
	kraken/ring                 one zone
	kraken/ring[0:11]           a run of LEDs within a zone
	kraken/ring[1,4,7:9]        LEDs 1 and 4, and 7 through 9
	kraken/fan-top              a named segment, defined once in the rules

Whole-device is the shorthand for "every zone", not a separate concept, so a
simple scene and a granular one are the same mechanism at different depths.
*/
type Target struct {
	Device string
	// Part is a zone or segment name. Empty is the whole device. Which of the
	// two it is depends on the device and the rules, so it is decided at
	// resolution rather than guessed at parse time.
	Part string

	/*
		Picks are the LEDs named inside the brackets, as written.

		A list rather than one range, because pointing at four lights that are
		not next to each other is one intention and hotaru had no way to say
		it: "kraken/ring[1,4,7:9]" is one target, one assignment, and one line
		in a scene. Empty means the whole part.
	*/
	Picks []config.LEDs
}

var bracketed = regexp.MustCompile(`^(.*)\[([0-9,:\s]+)\]$`)

// ParseTarget reads one of the forms above.
func ParseTarget(s string) (Target, error) {
	text := strings.TrimSpace(s)
	if text == "" {
		return Target{}, fmt.Errorf("no target given")
	}

	device, part, hasPart := strings.Cut(text, "/")
	device = strings.TrimSpace(device)
	if device == "" {
		return Target{}, fmt.Errorf("%q has no device before the /", s)
	}
	t := Target{Device: device}
	if !hasPart {
		return t, nil
	}

	part = strings.TrimSpace(part)
	if m := bracketed.FindStringSubmatch(part); m != nil {
		picks, err := parsePicks(m[2], s)
		if err != nil {
			return Target{}, err
		}
		t.Part = strings.TrimSpace(m[1])
		t.Picks = picks
	} else {
		/*
			A part that ends in a bracket and did not parse as one is a
			mistake about the grammar, not a zone with an unusual name.
			"kraken/Ring[]" silently became a zone called "Ring[]" and failed
			later with "no zone called Ring[]", which is a true sentence that
			answers the wrong question.
		*/
		if strings.HasSuffix(part, "]") {
			return Target{}, fmt.Errorf("%q: the brackets hold light numbers, like [3] or [1,4:6]", s)
		}
		t.Part = part
	}
	if t.Part == "" && len(t.Picks) == 0 {
		return Target{}, fmt.Errorf("%q has nothing after the /", s)
	}
	return t, nil
}

/*
parsePicks reads what is between the brackets: numbers and ranges, separated by
commas.

	[7]          one light
	[0:11]       twelve of them
	[1,4,7:9]    two, then three

Order is kept as written rather than sorted. It costs nothing, and a target
that comes back in a different order from the one somebody typed is a target
they have to read twice to recognise.
*/
func parsePicks(inside, whole string) ([]config.LEDs, error) {
	var out []config.LEDs
	for _, item := range strings.Split(inside, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			return nil, fmt.Errorf("%q has an empty entry between its commas", whole)
		}

		first, last, isRange := strings.Cut(item, ":")
		if !isRange {
			n, err := strconv.Atoi(item)
			if err != nil {
				return nil, fmt.Errorf("%q: %q is not a light number", whole, item)
			}
			out = append(out, config.LEDs{First: n, Last: n})
			continue
		}

		from, err := strconv.Atoi(strings.TrimSpace(first))
		if err != nil {
			return nil, fmt.Errorf("%q: %q is not a light number", whole, first)
		}
		to, err := strconv.Atoi(strings.TrimSpace(last))
		if err != nil {
			return nil, fmt.Errorf("%q: %q is not a light number", whole, last)
		}
		if to < from {
			return nil, fmt.Errorf("%q ends before it starts", whole)
		}
		out = append(out, config.LEDs{First: from, Last: to})
	}
	return out, nil
}

// String is the form a person would type, and is what a listing shows.
func (t Target) String() string {
	switch {
	case t.Part == "" && len(t.Picks) == 0:
		return t.Device
	case len(t.Picks) == 0:
		return t.Device + "/" + t.Part
	}
	return fmt.Sprintf("%s/%s[%s]", t.Device, t.Part, Picks(t.Picks))
}

// Picks writes a list of runs the way it is typed: a bare number for one
// light, first:last for several, commas between.
func Picks(picks []config.LEDs) string {
	parts := make([]string, 0, len(picks))
	for _, pick := range picks {
		if pick.First == pick.Last {
			parts = append(parts, strconv.Itoa(pick.First))
			continue
		}
		parts = append(parts, fmt.Sprintf("%d:%d", pick.First, pick.Last))
	}
	return strings.Join(parts, ",")
}

/*
Span is a contiguous run of a device's LEDs: what a target resolves to.

Contiguous because every way of naming part of a device — a zone, a range
within one, a named segment — describes lights that sit next to each other.
*/
type Span struct {
	First int
	Count int
}

// Last is the final LED in the span.
func (s Span) Last() int { return s.First + s.Count - 1 }

/*
ResolveTarget turns a target into the LEDs it means on this device.

Segments are looked up before zones, because a segment is a name the user chose
and a zone is a name the vendor chose, and the user's should win on their own
machine. A name that is neither is reported with the names that exist, since
"unknown segment" without the alternatives is a puzzle rather than an error.
*/
func (d *Device) ResolveTarget(t Target, rule Rule) ([]Span, error) {
	whole := Span{First: 0, Count: d.LEDCount}
	if t.Part == "" {
		if len(t.Picks) > 0 {
			return d.clampAll(whole, t.Picks, t.String())
		}
		if d.LEDCount == 0 {
			return nil, unsupported(d.Name, "reports no LEDs")
		}
		return []Span{whole}, nil
	}

	var base Span
	switch segment, ok := rule.Segments[t.Part]; {
	case ok:
		zone, found := d.Zone(segment.Zone)
		if !found {
			return nil, unsupported(d.Name,
				"segment %q names zone %q, which this device does not have; it has %s",
				t.Part, segment.Zone, listOrNone(d.ZoneNames()))
		}
		base = Span{First: zone.First, Count: zone.Count}
		if segment.LEDs != nil {
			var err error
			if base, err = d.clamp(base, *segment.LEDs, "segment "+t.Part); err != nil {
				return nil, err
			}
		}
	default:
		zone, found := d.Zone(t.Part)
		if !found {
			return nil, unsupported(d.Name,
				"no segment or zone called %q; segments are %s and zones are %s",
				t.Part, listOrNone(segmentNames(rule)), listOrNone(d.ZoneNames()))
		}
		base = Span{First: zone.First, Count: zone.Count}
	}

	if len(t.Picks) > 0 {
		return d.clampAll(base, t.Picks, t.String())
	}
	return []Span{base}, nil
}

/*
clampAll resolves every run in a target, and refuses the whole target if any
one of them runs off the end.

All or nothing on purpose: "kraken/ring[1,4,99]" on a twenty-four light ring is
a mistake about the ring, and lighting two of the three would hide it behind
something that looked like it worked.
*/
func (d *Device) clampAll(base Span, picks []config.LEDs, what string) ([]Span, error) {
	out := make([]Span, 0, len(picks))
	for _, pick := range picks {
		span, err := d.clamp(base, pick, what)
		if err != nil {
			return nil, err
		}
		out = append(out, span)
	}
	return out, nil
}

// clamp turns a range written against a zone into absolute LED indices, and
// refuses one that runs off the end rather than lighting whatever is there.
func (d *Device) clamp(base Span, r config.LEDs, what string) (Span, error) {
	if r.First < 0 || r.Last >= base.Count {
		return Span{}, unsupported(d.Name,
			"%s asks for LEDs %d-%d, but that has %d (0-%d)",
			what, r.First, r.Last, base.Count, base.Count-1)
	}
	return Span{First: base.First + r.First, Count: r.Count()}, nil
}

func segmentNames(rule Rule) []string {
	out := make([]string, 0, len(rule.Segments))
	for name := range rule.Segments {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func listOrNone(names []string) string {
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, ", ")
}
