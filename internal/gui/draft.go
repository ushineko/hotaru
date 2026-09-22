package gui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ushineko/hotaru/internal/api"
)

/*
Draft is a scene being edited.

It lives in the window. Nothing in it has been written to a device or to a
file, and that is the whole point: a colour picker that wrote as it moved would
be sixty writes a second and sixty recorded intentions, and staging is what
makes one usable at all.

Three states, not two -- a draft is not a preview, and a preview is not a saved
scene.
*/
type Draft struct {
	// From is the scene this was opened from, empty for a new one. Kept so
	// saving offers the same name.
	From string

	// colours are the assignments being edited, by target. A map because
	// setting the same target twice is a correction rather than a second
	// assignment.
	colours map[string]string

	/*
		rest is everything the editor does not edit, carried through
		untouched: the effect each device runs and what the screen shows.

		An editor that understands part of a format and rewrites the whole
		thing quietly drops what it did not understand. This is the one place
		that can happen here, so it is the one thing this type is careful
		about.
	*/
	rest api.Scene
}

// NewDraft starts an empty one.
func NewDraft() *Draft { return &Draft{colours: map[string]string{}} }

/*
DraftFrom opens an existing scene for editing.

Everything that is not a colour comes along: the effects, the screen state, and
the name it will be offered to save under.
*/
func DraftFrom(scene api.Scene) *Draft {
	d := &Draft{From: scene.Name, colours: map[string]string{}, rest: scene}
	for _, a := range scene.Assignments {
		d.colours[a.Target] = a.Colour
	}
	// The assignments live in the map now; keeping a second copy would let
	// the two disagree.
	d.rest.Assignments = nil
	return d
}

/*
SetEffect says what a device should be doing with the colours. An empty mode
takes the effect back out, which is how somebody undoes one.
*/
func (d *Draft) SetEffect(device, mode string) {
	if strings.TrimSpace(mode) == "" {
		delete(d.rest.Effects, device)
		return
	}
	if d.rest.Effects == nil {
		d.rest.Effects = map[string]string{}
	}
	d.rest.Effects[device] = mode
}

// Effect is what this draft says a device should be doing, empty for nothing.
func (d *Draft) Effect(device string) string { return d.rest.Effects[device] }

// Effects are every device this draft says something about.
func (d *Draft) Effects() map[string]string { return d.rest.Effects }

// Set gives a target a colour. An empty colour removes it, which is how
// somebody takes an exception back out of a scene.
func (d *Draft) Set(target, colour string) {
	if strings.TrimSpace(colour) == "" {
		delete(d.colours, target)
		return
	}
	d.colours[target] = colour
}

/*
SetEverything paints every device in scope, which is a scene's commonest shape.

Not an assignment: it is the scene's own colour, applied before any of them, so
"everything blue except the top fan" stays two facts rather than six.
*/
func (d *Draft) SetEverything(colour string) { d.rest.Colour = colour }

// Everything is the colour every device in scope is given, if any.
func (d *Draft) Everything() string { return d.rest.Colour }

// SetScreen says what the scene puts on the panel: the dashboard, the
// firmware's readout, a path to a picture, or nothing at all.
func (d *Draft) SetScreen(what string) { d.rest.Screen = what }

// Screen is what the scene says about the panel.
func (d *Draft) Screen() string { return d.rest.Screen }

// Colour is what a target is showing in the draft, if anything.
func (d *Draft) Colour(target string) (string, bool) {
	c, ok := d.colours[target]
	return c, ok
}

// Empty reports whether there is anything to preview or save.
func (d *Draft) Empty() bool { return len(d.colours) == 0 && d.rest.Colour == "" && !d.rest.Off }

// Targets are the targets this draft touches, in a stable order so a listing
// does not reshuffle itself between rebuilds.
func (d *Draft) Targets() []string {
	out := make([]string, 0, len(d.colours))
	for target := range d.colours {
		out = append(out, target)
	}
	sort.Strings(out)
	return out
}

/*
Scene is the draft as the service takes it.

Named for the caller: a preview wants something to show in a listing, and a
save wants the name somebody typed.
*/
func (d *Draft) Scene(name string) api.Scene {
	out := d.rest
	out.Name = name
	out.Assignments = nil
	for _, target := range d.Targets() {
		out.Assignments = append(out.Assignments,
			api.SceneAssignment{Target: target, Colour: d.colours[target]})
	}
	return out
}

// Summary is what the draft does, in one line, for the editor's own heading.
func (d *Draft) Summary() string {
	switch {
	case d.rest.Off:
		return "the lights go off"
	case len(d.colours) == 0 && d.rest.Colour != "":
		return "everything " + d.rest.Colour
	case len(d.colours) == 0:
		return "nothing yet"
	}
	return fmt.Sprintf("%d target(s)", len(d.colours))
}
