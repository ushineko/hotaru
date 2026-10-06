package gui

import (
	"fmt"
	"strconv"
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

	/*
		colours are the assignments being edited, oldest first.

		A list rather than a map because the order is what the scene means:
		the service applies assignments in turn, so where two overlap the
		later one shows. It was a map written out sorted, and a colour for
		the whole mouse mat sorted ahead of the colour each of its lights
		already had -- which then painted over it (#183). Setting a target
		again is still a correction, not a second assignment: it moves to
		the end with its new colour.
	*/
	colours []assignment

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
func NewDraft() *Draft { return &Draft{} }

// assignment is one target and its colour.
type assignment struct{ target, colour string }

/*
DraftFrom opens an existing scene for editing.

Everything that is not a colour comes along: the effects, the screen state, and
the name it will be offered to save under.
*/
func DraftFrom(scene api.Scene) *Draft {
	d := &Draft{From: scene.Name, rest: scene}
	for _, a := range scene.Assignments {
		// In the scene's own order, which is what it means.
		d.drop(a.Target)
		d.colours = append(d.colours, assignment{a.Target, a.Colour})
	}
	// The assignments live in the list now; keeping a second copy would let
	// the two disagree.
	d.rest.Assignments = nil
	return d
}

/*
SetEffect says what a device should be doing with the colours. An empty mode
takes the effect back out, which is how somebody undoes one.

**The mode alone, keeping whatever colour and speed were already set.** A
person changing a keyboard from one reactive mode to another has not changed
their mind about the colour, and making them pick it again would be the
editor forgetting something it was told.
*/
func (d *Draft) SetEffect(device, mode string) {
	if strings.TrimSpace(mode) == "" {
		delete(d.rest.Effects, device)
		return
	}
	effect := d.rest.Effects[device]
	effect.Mode = mode
	d.setEffect(device, effect)
}

/*
SetEffectColour is the colour an effect that shows one runs in. Empty gives
the device back to spec 050's fallback: the colour most of its frame is.

Silently nothing for a device with no effect, because the control that sets
this is only drawn for a device that has one.
*/
func (d *Draft) SetEffectColour(device, colour string) {
	d.SetEffectColours(device, []string{colour})
}

/*
SetEffectColours is every colour an effect runs in, first to last (spec 061).
None gives the device back to the fallback, as an empty colour does.

Colours set here are somebody's choice, so a mark that they were picked from
a picture goes: recolouring the scene keeps them rather than picking again.
*/
func (d *Draft) SetEffectColours(device string, colours []string) {
	effect, has := d.rest.Effects[device]
	if !has {
		return
	}
	effect.SetPalette(colours)
	effect.ColoursFrom = ""
	d.setEffect(device, effect)
}

// SetEffectSpeed is how fast the effect runs, in the device's own units. A nil
// speed is "as the device has it", which is not the same as the slowest one.
func (d *Draft) SetEffectSpeed(device string, speed *int) {
	effect, has := d.rest.Effects[device]
	if !has {
		return
	}
	effect.Speed = speed
	d.setEffect(device, effect)
}

/*
SetEffectWhole sets a device's effect and its settings together, for a caller
holding the whole value -- the chooser dialog, which edits all three.

An effect with no mode takes the device back out, exactly as an empty mode
does: a colour for a mode nobody named is a setting with nothing to apply to.
*/
func (d *Draft) SetEffectWhole(device string, effect api.Effect) {
	if !effect.Named() {
		delete(d.rest.Effects, device)
		return
	}
	d.setEffect(device, effect)
}

func (d *Draft) setEffect(device string, effect api.Effect) {
	if d.rest.Effects == nil {
		d.rest.Effects = map[string]api.Effect{}
	}
	d.rest.Effects[device] = effect
}

// Effect is what this draft says a device should be doing. The zero value is a
// device it says nothing about.
func (d *Draft) Effect(device string) api.Effect { return d.rest.Effects[device] }

// Effects are every device this draft says something about.
func (d *Draft) Effects() map[string]api.Effect { return d.rest.Effects }

/*
Set gives a target a colour, after everything set before it. An empty colour
removes it, which is how somebody takes an exception back out of a scene.

**A whole device or a whole zone replaces what was set inside it.** Somebody
who colours the whole mouse mat after its lights were coloured one by one has
changed their mind about the mat, and leaving the lights' lines in would keep
assignments that no longer show anywhere. Inside means the target's own
lights as this window writes them: `mat` covers `mat/Left[0]` and
`mat/logo`, and `mat/Left` covers `mat/Left[0:3]`. The whole scene's colour
is not a target and keeps its exceptions -- see SetEverything.
*/
func (d *Draft) Set(target, colour string) {
	d.drop(target)
	if strings.TrimSpace(colour) == "" {
		return
	}
	kept := d.colours[:0]
	for _, a := range d.colours {
		if !inside(a.target, target) {
			kept = append(kept, a)
		}
	}
	kept = append(kept, assignment{target, colour})
	d.colours = kept
}

// drop takes a target out, wherever it is.
func (d *Draft) drop(target string) {
	for i, a := range d.colours {
		if a.target == target {
			d.colours = append(d.colours[:i], d.colours[i+1:]...)
			return
		}
	}
}

/*
inside is whether one target names lights within another that is a whole
device or a whole zone.

Spelled as this window spells targets: a device by its name, a zone after a
slash, lights in brackets. A target somebody typed with a shorter name --
`kraken/ring[3]` for the NZXT Kraken -- is not recognised as inside the full
name and is left alone. That costs only the tidying: Set still puts the newer
colour after it, so it still shows.
*/
func inside(target, whole string) bool {
	if strings.Contains(whole, "[") || target == whole {
		return false
	}
	target, whole = strings.ToLower(target), strings.ToLower(whole)
	if !strings.Contains(whole, "/") {
		return strings.HasPrefix(target, whole+"/") || strings.HasPrefix(target, whole+"[")
	}
	return strings.HasPrefix(target, whole+"[")
}

/*
Clone is a copy to go back to, for a colour wheel that is cancelled.

Undoing by setting the old colour again is not enough once Set can take other
lines out: the per-light colours a whole-device colour replaced have to come
back with it.
*/
func (d *Draft) Clone() *Draft {
	out := *d
	out.colours = append([]assignment(nil), d.colours...)
	return &out
}

// Restore puts back what a Clone saved, in place, so everything holding this
// draft sees it.
func (d *Draft) Restore(saved *Draft) {
	d.colours = append([]assignment(nil), saved.colours...)
	d.rest = saved.rest
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
	for _, a := range d.colours {
		if a.target == target {
			return a.colour, true
		}
	}
	return "", false
}

/*
Light is the colour the draft gives one light: the newest line that covers
it, else the whole scene's colour.

What the editor draws each light in. Looking up the light's own target
string was not enough: a whole-device colour, a whole-zone colour and a light
written `Left[0]` rather than `Left[0:0]` all left the light drawn in what the
hardware was showing, so a colour that had been set looked as if it had not
(#183). Segments are left out: which lights a rule names is the service's to
say, and a light it would colour is drawn as before.
*/
func (d *Draft) Light(device, zone string, light int) (string, bool) {
	for i := len(d.colours) - 1; i >= 0; i-- {
		if covers(d.colours[i].target, device, zone, light) {
			return d.colours[i].colour, true
		}
	}
	if d.rest.Colour != "" {
		return d.rest.Colour, true
	}
	return "", false
}

// covers is whether a target, as this window writes it, includes one light.
func covers(target, device, zone string, light int) bool {
	name, part, hasPart := strings.Cut(target, "/")
	if !strings.EqualFold(name, device) {
		return false
	}
	if !hasPart {
		return true
	}
	named, lights, hasLights := strings.Cut(part, "[")
	if !strings.EqualFold(named, zone) {
		return false
	}
	if !hasLights {
		return true
	}
	for _, run := range strings.Split(strings.TrimSuffix(lights, "]"), ",") {
		first, last, ok := span(run)
		if ok && first <= light && light <= last {
			return true
		}
	}
	return false
}

// span reads one run of a target's lights: `3` or `3:5`.
func span(run string) (first, last int, ok bool) {
	from, to, isRun := strings.Cut(strings.TrimSpace(run), ":")
	first, err := strconv.Atoi(from)
	if err != nil {
		return 0, 0, false
	}
	if !isRun {
		return first, first, true
	}
	last, err = strconv.Atoi(to)
	if err != nil {
		return 0, 0, false
	}
	return first, last, true
}

// Empty reports whether there is anything to preview or save.
func (d *Draft) Empty() bool { return len(d.colours) == 0 && d.rest.Colour == "" && !d.rest.Off }

// Targets are the targets this draft touches, oldest first: the order the
// scene applies them in, and stable between rebuilds until something is set.
func (d *Draft) Targets() []string {
	out := make([]string, 0, len(d.colours))
	for _, a := range d.colours {
		out = append(out, a.target)
	}
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
	for _, a := range d.colours {
		out.Assignments = append(out.Assignments,
			api.SceneAssignment{Target: a.target, Colour: a.colour})
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
