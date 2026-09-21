package gui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

/*
Spot is a part of the machine somebody has pointed at: a device, one of its
zones, or a run of lights inside one.

A structured target rather than the string hotaru writes, because a selection
of several has to be *merged* before it is written -- and merging
"kraken/ring[3:3]", "kraken/ring[4:4]" and "kraken/ring[5:5]" back into
"kraken/ring[3:5]" means taking the string apart again. Keep the parts.
*/
type Spot struct {
	Device string
	// Zone is empty for the whole device.
	Zone string
	// First and Last are lights within the zone. Both -1 for the whole zone.
	First, Last int
}

// WholeDevice is a spot covering everything a device has.
func WholeDevice(name string) Spot { return Spot{Device: name, First: -1, Last: -1} }

// WholeZone is a spot covering one zone.
func WholeZone(device, zone string) Spot {
	return Spot{Device: device, Zone: zone, First: -1, Last: -1}
}

// Lights is a spot covering a run within a zone.
func Lights(device, zone string, first, last int) Spot {
	return Spot{Device: device, Zone: zone, First: first, Last: last}
}

// Target is the spot as hotaru writes it, which is what a scene stores.
func (s Spot) Target() string {
	switch {
	case s.Zone == "":
		return s.Device
	case s.First < 0:
		return s.Device + "/" + s.Zone
	}
	return fmt.Sprintf("%s/%s[%d:%d]", s.Device, s.Zone, s.First, s.Last)
}

// Describe is the spot as a person reads it.
func (s Spot) Describe() string {
	switch {
	case s.Device == everything:
		return "everything in scope"
	case s.Zone == "":
		return s.Device
	case s.First < 0:
		return s.Zone
	case s.First == s.Last:
		return fmt.Sprintf("%s, light %d", s.Zone, s.First)
	}
	return fmt.Sprintf("%s, lights %d to %d", s.Zone, s.First, s.Last)
}

/*
Selection is what is about to be coloured.

A set rather than one thing, because "these four lights" is one intention and
four trips through a colour picker is not. Clicking adds, clicking again takes
away, and what comes out is merged: three adjacent lights become one run, which
is what somebody would have typed and what the scene should say.
*/
type Selection struct{ spots []Spot }

// Toggle adds a spot, or removes it if it is already there.
func (s *Selection) Toggle(spot Spot) {
	for i, existing := range s.spots {
		if existing == spot {
			s.spots = append(s.spots[:i], s.spots[i+1:]...)
			return
		}
	}
	s.spots = append(s.spots, spot)
}

// Has reports whether a spot is selected, which is how a block knows to draw
// itself as chosen.
func (s *Selection) Has(spot Spot) bool {
	for _, existing := range s.spots {
		if existing == spot {
			return true
		}
	}
	return false
}

// Clear empties it.
func (s *Selection) Clear() { s.spots = nil }

// Empty reports whether anything is selected.
func (s *Selection) Empty() bool { return len(s.spots) == 0 }

// Len is how many spots are selected, which the card says out loud.
func (s *Selection) Len() int { return len(s.spots) }

// Spots are the selected spots, in the order they were clicked.
func (s *Selection) Spots() []Spot { return append([]Spot(nil), s.spots...) }

/*
Targets are the selection as hotaru writes it: one target per zone, however
many lights were clicked in it.

Four lights that are not next to each other are one intention, and hotaru's
target grammar says so -- `kraken/ring[1,4,7:9]` is one target, one assignment,
and one line in a scene. Adjacent picks collapse into runs on the way out,
because `[3,4,5]` and `[3:5]` are the same lights and only one of them is what
somebody would have typed.

The collapsing happens here rather than at the point of clicking, so that
clicking stays reversible: a light can be unpicked without unpicking its
neighbours.
*/
func (s *Selection) Targets() []string {
	runs := map[string][]Spot{}
	var order []string
	var out []string

	for _, spot := range s.spots {
		if spot.First < 0 {
			// A whole device or a whole zone is already one target.
			out = append(out, spot.Target())
			continue
		}
		key := spot.Device + "/" + spot.Zone
		if _, seen := runs[key]; !seen {
			order = append(order, key)
		}
		runs[key] = append(runs[key], spot)
	}

	for _, key := range order {
		spots := runs[key]
		out = append(out, fmt.Sprintf("%s[%s]", key, merge(spots)))
	}
	return out
}

/*
merge turns a zone's picked lights into the fewest runs that cover them, as
hotaru writes them.

	3, 4, 5      -> 3:5
	1, 4, 7, 8   -> 1,4,7:8
*/
func merge(spots []Spot) string {
	sort.Slice(spots, func(i, j int) bool { return spots[i].First < spots[j].First })

	var parts []string
	run := spots[0]
	for _, spot := range spots[1:] {
		if spot.First <= run.Last+1 {
			// Adjacent, or overlapping where a zone was drawn as runs.
			run.Last = max(run.Last, spot.Last)
			continue
		}
		parts = append(parts, written(run))
		run = spot
	}
	return strings.Join(append(parts, written(run)), ",")
}

// written is one run as it is typed: a bare number for a single light.
func written(run Spot) string {
	if run.First == run.Last {
		return strconv.Itoa(run.First)
	}
	return fmt.Sprintf("%d:%d", run.First, run.Last)
}
