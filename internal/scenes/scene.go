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
	"encoding/json"
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

	/*
		Colour is one colour across every device in scope, applied before any
		assignments below.

		What the nine shipped scenes are, and what somebody means by "make the
		machine blue": the same thing `hotaru light set blue` does. Exceptions
		go in Assignments, so "everything blue except the top fan" stays two
		lines rather than an enumeration.
	*/
	Colour string `json:"colour,omitempty"`

	/*
		Off turns lighting off instead of colouring it.

		Off is not a colour. It resolves the device's own Off mode, falls back
		to black in Direct, and honours the correction for a keyboard that
		treats black as a dead backlight rather than as off -- none of which a
		colour assignment can ask for. A scene that is Off ignores its
		colours; the ninth key on this desk has turned the lights off for two
		years and it is not a shade of black.
	*/
	Off bool `json:"off,omitempty"`

	// Assignments are targets and colours, in order: a later one wins where
	// two cover the same LEDs, which is what makes "everything blue except the
	// top fan" two lines rather than an enumeration.
	Assignments []Assignment `json:"assignments,omitempty"`

	/*
		Effects name what each device should be doing, by device name.

		An effect is a decision about the device -- the keyboard ripples under
		typing -- and the colours are what it ripples in. A mode that cannot
		show every colour of the frame is written anyway and the frame is what
		gives way, because there is nothing to fall back to when the mode is
		the thing being asked for. See spec 050.

		The name is the device's own spelling of a mode it advertises. Effects
		hotaru renders itself -- a storm across every device at once, which no
		firmware mode can do because no device knows what the others are
		showing -- resolve here too, and are their own spec.
	*/
	Effects map[string]Effect `json:"effects,omitempty"`

	/*
		Screen is what the cooler's panel shows: "dashboard", "readout", or a
		path to a GIF.

		Empty means the scene says nothing about the screen, and the screen
		does not change. That is the only rule that makes the absent case
		predictable: a lighting scene written by somebody who never thought
		about the panel must not take the dashboard away at midday.
	*/
	Screen string `json:"screen,omitempty"`

	/*
		Distance is how far apart this scene's colours were pushed when it
		was built from a picture or a dashboard.

		Kept so it can be adjusted rather than re-guessed: what a person
		picks and what an LED shows are not the same thing, and the right
		number is whatever looks right on their machine. Zero and one both
		mean "as measured"; a scene nobody built this way has none.
	*/
	Distance float64 `yaml:"distance,omitempty" json:"distance,omitempty"`

	// Shipped marks one of the nine hotaru carries in code rather than in
	// anybody's file. Set when it is read, never stored.
	Shipped bool `json:"-"`
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
	/*
		ScreenDashboardPrefix names a particular dashboard: "dashboard:load".

		A prefix rather than a second field, so a scene written before there
		was more than one dashboard still means what it meant -- "dashboard"
		on its own is whichever one the panel is set to, and always was.
	*/
	ScreenDashboardPrefix = "dashboard:"
	ScreenReadout         = "readout"
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
	if s.Colour != "" || s.Off {
		// Everything in scope. Naming nothing is how the rest of hotaru says
		// "every device", and a lease over a scene like this covers them all.
		return nil
	}
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
func (s Scene) Effect(device string) Effect {
	for name, effect := range s.Effects {
		if strings.Contains(strings.ToLower(device), strings.ToLower(name)) {
			return effect
		}
	}
	return Effect{}
}

/*
Effect is what one device should be doing, and what it should be doing it in.

**A mode's name on its own is the whole of it for almost every scene**, which
is why that is still how one is written:

	effects:
	  Keychron K4 HE: Solid Reactive Multinexus

A mode that shows one colour of its own takes it from the frame otherwise --
the colour most of the frame is, per spec 050 -- and that is a reduction
rather than a choice. Somebody who has decided their keyboard ripples blue
says so:

	effects:
	  Keychron K4 HE:
	    mode: Solid Reactive Multinexus
	    colour: '#0000ff'
	    speed: 127

Both forms are read, and the short one is written wherever it is sufficient:
this file is read by people, and a mode's name is a line rather than a block.
*/
type Effect struct {
	// Mode is the device's own spelling of a mode it advertises.
	Mode string `json:"mode"`

	/*
		Colour is what a mode that carries its own colour is given.

		Empty is the ordinary case and means the frame decides. A mode that
		takes a colour per LED ignores this: its colour is the frame, and two
		answers to one question would make which one won depend on hardware.
	*/
	Colour string `json:"colour,omitempty"`

	/*
		Speed is how fast the mode runs, in the device's own units, for a mode
		that advertises a range.

		A pointer because zero is a speed -- the slowest one -- and "as fast as
		the vendor left it" has to be distinguishable from it. Nothing reads
		back what a speed looks like, so this is written and looked at rather
		than confirmed.
	*/
	Speed *int `json:"speed,omitempty"`
}

// Named reports whether this effect says anything at all. The zero value is a
// device a scene is silent about, which is not the same as one set to Direct.
func (e Effect) Named() bool { return e.Mode != "" }

// bare reports whether the mode's name is the whole of this effect, and so
// whether it can be written as one.
func (e Effect) bare() bool { return e.Colour == "" && e.Speed == nil }

// effectForm is the long form, as a plain struct: the type itself cannot be
// marshalled through the codecs without recursing into its own methods.
type effectForm struct {
	Mode   string `json:"mode"`
	Colour string `json:"colour,omitempty"`
	Speed  *int   `json:"speed,omitempty"`
}

/*
MarshalJSON writes the short form where the mode is the whole of the effect.

JSON rather than YAML, and it covers both: the settings codec encodes through
JSON, so the json tags name the fields in the file as well as on the wire.
*/
func (e Effect) MarshalJSON() ([]byte, error) {
	var (
		written []byte
		err     error
	)
	if e.bare() {
		written, err = json.Marshal(e.Mode)
	} else {
		written, err = json.Marshal(effectForm(e))
	}
	if err != nil {
		return nil, fmt.Errorf("write the effect %q: %w", e.Mode, err)
	}
	return written, nil
}

/*
UnmarshalJSON reads either form.

The short one first, because every scene written before this spec is in it and
a file somebody cannot read back is not a file -- see the store, which reports
a scene it cannot parse rather than replacing it.
*/
func (e *Effect) UnmarshalJSON(b []byte) error {
	var mode string
	if err := json.Unmarshal(b, &mode); err == nil {
		*e = Effect{Mode: mode}
		return nil
	}

	var form effectForm
	if err := json.Unmarshal(b, &form); err != nil {
		return fmt.Errorf("an effect is a mode's name, or a mode with a colour and a speed: %w", err)
	}
	*e = Effect(form)
	return nil
}
