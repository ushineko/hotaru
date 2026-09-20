package devices

import "strings"

/*
Want is what a caller is asking a device to do.

PerLED is not a preference: it is a fact about the frame being sent. A frame
with two different colours in it cannot be shown by a mode that takes one
colour for the whole device, and sending it anyway is how someone ends up with
three fans in whichever colour happened to be first.
*/
type Want struct {
	Off    bool
	PerLED bool
}

/*
SolidCandidates is every mode that could show this frame, best first.

A list rather than a single answer, because a device can accept a mode and not
honour it: the ASUS board takes Static, reports success, and leaves its
addressable headers dark. The caller writes the first candidate, reads the
active mode back, and moves to the next if what landed is not what it asked
for. That is how a machine nobody has configured still gets working lighting —
the fall-through discovers what a rule would otherwise have had to know.
*/
func (d *Device) SolidCandidates(rule Rule, want Want) []string {
	var out []string
	seen := map[string]bool{}

	add := func(name string) {
		mode, ok := d.Mode(name)
		if !ok || seen[mode.Name] {
			return
		}
		if want.PerLED && !mode.PerLED {
			return
		}
		seen[mode.Name] = true
		out = append(out, mode.Name) // the device's own spelling, for comparison later
	}

	/*
		The named order first -- but where it is hotaru's default rather than
		the user's, a mode whose colour hotaru sets per LED comes before one
		that keeps its colour in the mode.

		Both can show a solid colour. Only the first can be read back and
		checked, and the second failed silently for a whole evening on an NZXT
		cooler: Static took, reported success, and displayed the red its vendor
		had left in it. A rule naming solid_modes is a person who has looked at
		their machine, and still wins. See spec 009.
	*/
	order := rule.solidOrder()
	if !rule.namesSolidModes() {
		order = d.perLEDFirst(order)
	}
	for _, name := range order {
		add(name)
	}
	// Anything else the device advertises that can carry the frame. A device
	// whose vendor named its modes unusually is still driveable: the list is
	// preference, not permission.
	for _, pass := range []bool{true, false} {
		for _, mode := range d.Modes {
			if mode.PerLED != pass {
				continue
			}
			if want.PerLED && !mode.PerLED {
				continue
			}
			if isEffect(mode.Name) {
				continue
			}
			add(mode.Name)
		}
	}
	return out
}

// perLEDFirst reorders names so that the modes hotaru can drive colour by
// colour, and therefore verify, are tried before the ones it cannot.
func (d *Device) perLEDFirst(names []string) []string {
	out := make([]string, 0, len(names))
	for _, pass := range []bool{true, false} {
		for _, name := range names {
			mode, ok := d.Mode(name)
			if ok && mode.PerLED == pass {
				out = append(out, name)
			} else if !ok && !pass {
				out = append(out, name) // unknown here; add() drops it
			}
		}
	}
	return out
}

// offCandidates is the same for turning a device off.
func (d *Device) offCandidates() []string {
	var out []string
	for _, name := range DefaultOffModes {
		if mode, ok := d.Mode(name); ok {
			out = append(out, mode.Name)
		}
	}
	return out
}

/*
Resolve picks the mode to write, or says why the device cannot do this.

The failure is a skip with a reason, never a substitute: a device that cannot
show three colours is reported, not handed the average of them. Approximating
here would mean a user asking for three fans in three colours, being told it
worked, and looking at something else.
*/
func (d *Device) Resolve(rule Rule, want Want) (string, error) {
	if want.Off {
		if rule.NeverBlank {
			return "", unsupported(d.Name,
				"this device is never blanked: off would resolve to black, which kills a backlight rather than dimming it")
		}
		candidates := d.offCandidates()
		if len(candidates) == 0 {
			return "", unsupported(d.Name, "no mode that turns it off; it advertises %s",
				strings.Join(d.ModeNames(), ", "))
		}
		return candidates[0], nil
	}

	candidates := d.SolidCandidates(rule, want)
	if len(candidates) == 0 {
		if want.PerLED {
			return "", unsupported(d.Name,
				"no mode that takes a colour per LED, so it cannot show more than one colour at a time; it advertises %s",
				strings.Join(d.ModeNames(), ", "))
		}
		return "", unsupported(d.Name, "no mode that shows a solid colour; it advertises %s",
			strings.Join(d.ModeNames(), ", "))
	}
	return candidates[0], nil
}

/*
isEffect keeps animations out of the fall-through.

A device that cannot do static or direct should not quietly end up in Rainbow
Wave because that was the next mode in its list. The named preference order is
where a deliberate effect belongs.
*/
func isEffect(name string) bool {
	switch strings.ToLower(name) {
	case "static", "direct", "custom", "solid color", "solid":
		return false
	case "off":
		return true // off is a mode, but never a way to show a colour
	}
	return true
}
