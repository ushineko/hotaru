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

	kraken                  the whole device, every zone
	kraken/ring             one zone
	kraken/ring[0:11]       an LED range within a zone
	kraken/fan-top          a named segment, defined once in the rules

Whole-device is the shorthand for "every zone", not a separate concept, so a
simple scene and a granular one are the same mechanism at different depths.
*/
type Target struct {
	Device string
	// Part is a zone or segment name. Empty is the whole device. Which of the
	// two it is depends on the device and the rules, so it is decided at
	// resolution rather than guessed at parse time.
	Part string
	LEDs *config.LEDs
}

var rangeSuffix = regexp.MustCompile(`^(.*)\[(\d+):(\d+)\]$`)

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
	if m := rangeSuffix.FindStringSubmatch(part); m != nil {
		first, _ := strconv.Atoi(m[2])
		last, _ := strconv.Atoi(m[3])
		if last < first {
			return Target{}, fmt.Errorf("%q ends before it starts", s)
		}
		t.Part = strings.TrimSpace(m[1])
		t.LEDs = &config.LEDs{First: first, Last: last}
	} else {
		t.Part = part
	}
	if t.Part == "" && t.LEDs == nil {
		return Target{}, fmt.Errorf("%q has nothing after the /", s)
	}
	return t, nil
}

// String is the form a person would type, and is what a listing shows.
func (t Target) String() string {
	switch {
	case t.Part == "" && t.LEDs == nil:
		return t.Device
	case t.LEDs == nil:
		return t.Device + "/" + t.Part
	default:
		return fmt.Sprintf("%s/%s[%d:%d]", t.Device, t.Part, t.LEDs.First, t.LEDs.Last)
	}
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
Resolve turns a target into the LEDs it means on this device.

Segments are looked up before zones, because a segment is a name the user chose
and a zone is a name the vendor chose, and the user's should win on their own
machine. A name that is neither is reported with the names that exist, since
"unknown segment" without the alternatives is a puzzle rather than an error.
*/
func (d *Device) ResolveTarget(t Target, rule Rule) (Span, error) {
	whole := Span{First: 0, Count: d.LEDCount}
	if t.Part == "" {
		if t.LEDs != nil {
			return d.clamp(whole, *t.LEDs, t.String())
		}
		if d.LEDCount == 0 {
			return Span{}, unsupported(d.Name, "reports no LEDs")
		}
		return whole, nil
	}

	var base Span
	switch segment, ok := rule.Segments[t.Part]; {
	case ok:
		zone, found := d.Zone(segment.Zone)
		if !found {
			return Span{}, unsupported(d.Name,
				"segment %q names zone %q, which this device does not have; it has %s",
				t.Part, segment.Zone, listOrNone(d.ZoneNames()))
		}
		base = Span{First: zone.First, Count: zone.Count}
		if segment.LEDs != nil {
			var err error
			if base, err = d.clamp(base, *segment.LEDs, "segment "+t.Part); err != nil {
				return Span{}, err
			}
		}
	default:
		zone, found := d.Zone(t.Part)
		if !found {
			return Span{}, unsupported(d.Name,
				"no segment or zone called %q; segments are %s and zones are %s",
				t.Part, listOrNone(segmentNames(rule)), listOrNone(d.ZoneNames()))
		}
		base = Span{First: zone.First, Count: zone.Count}
	}

	if t.LEDs != nil {
		return d.clamp(base, *t.LEDs, t.String())
	}
	return base, nil
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
