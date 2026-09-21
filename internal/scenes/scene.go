/*
Package scenes is named lighting, kept.

A scene is what somebody wants their machine to look like: colours addressed at
whatever depth they meant them -- a device, a zone, a range of LEDs, a segment
they named once -- plus the effect each device should be running and what the
cooler's screen should show. Applied as a unit, because a machine lit halfway
through a scene is not a state anybody asked for.

Named rather than numbered. The program this replaces had banks of nine because
numpad keys are numbers; a name survives a rebind, and slot numbers belong to
the layer that does the binding. See spec 015.
*/
package scenes

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/devices"
)

/*
Scene is one named lighting state.

Everything in it is optional except the name. A scene with no assignments and a
screen is a legitimate thing to want, and so is the reverse.
*/
type Scene struct {
	// Name is how somebody refers to it, and how a binding will.
	Name string `json:"-"`

	// Assignments are targets and colours, in order: a later one wins where
	// two cover the same LEDs, which is what makes "everything blue except the
	// top fan" two lines rather than an enumeration.
	Assignments []Assignment `json:"assignments,omitempty"`

	/*
		Effects name what each device should be doing, by device name.

		Preferred, never forced. A mode that cannot carry the frame is no use,
		and showing one colour where three were asked for would be a worse
		answer than choosing a mode that works -- so an effect that does not
		fit falls through exactly as hotaru's mode resolution already does.

		The name is the device's own spelling of a mode it advertises. Effects
		hotaru renders itself -- a storm across every device at once, which no
		firmware mode can do because no device knows what the others are
		showing -- resolve here too, and are their own spec.
	*/
	Effects map[string]string `json:"effects,omitempty"`

	/*
		Screen is what the cooler's panel shows: "dashboard", "readout", or a
		path to a GIF.

		Empty means the scene says nothing about the screen, and the screen
		does not change. That is the only rule that makes the absent case
		predictable: a lighting scene written by somebody who never thought
		about the panel must not take the dashboard away at midday.
	*/
	Screen string `json:"screen,omitempty"`
}

// Assignment is one colour on one target, in the spelling somebody would type.
// Stored as text because this file is read by people even though it is written
// by the program.
type Assignment struct {
	Target string `json:"target"`
	Colour string `json:"colour"`
}

// Screen states a scene can name, beyond a path to an image.
const (
	ScreenDashboard = "dashboard"
	ScreenReadout   = "readout"
)

/*
Resolve turns a scene's text into the assignments the lighting path takes.

Errors are per assignment and do not stop the others: one unreadable colour
costs that line, the way an unknown segment name already costs one assignment
rather than the whole scene.
*/
func (s Scene) Resolve() ([]devices.Assignment, []error) {
	var out []devices.Assignment
	var problems []error
	for i, a := range s.Assignments {
		target, err := devices.ParseTarget(a.Target)
		if err != nil {
			problems = append(problems, fmt.Errorf("assignment %d: %w", i+1, err))
			continue
		}
		c, err := colour.Parse(a.Colour)
		if err != nil {
			problems = append(problems, fmt.Errorf("assignment %d: %w", i+1, err))
			continue
		}
		out = append(out, devices.Assignment{Target: target, Colour: c})
	}
	return out, problems
}

/*
Devices are the device names a scene mentions, which is what a preview takes a
lease on.

Names as typed, not as enumerated: matching them against the hardware happens
where every other device name is matched, and a scene naming something absent
takes a lease on nothing rather than failing.
*/
func (s Scene) Devices() []string {
	seen := map[string]bool{}
	for _, a := range s.Assignments {
		if target, err := devices.ParseTarget(a.Target); err == nil {
			seen[target.Device] = true
		}
	}
	for device := range s.Effects {
		seen[device] = true
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Effect is the effect named for a device, matched the way device names are
// matched everywhere else here: case-insensitively, by substring, so a scene
// can say "keychron" rather than the vendor's full string.
func (s Scene) Effect(device string) string {
	for name, effect := range s.Effects {
		if strings.Contains(strings.ToLower(device), strings.ToLower(name)) {
			return effect
		}
	}
	return ""
}
