package devices

import "github.com/ushineko/hotaru/internal/colour"

/*
Assignment is one colour applied to one target.

A scene is a list of these. Order matters only where two overlap: the later one
wins, which is what makes "everything blue, except the top fan" two lines
instead of an enumeration of every fan.
*/
type Assignment struct {
	Target Target
	Colour colour.Colour
}

/*
Frame is a device's complete lighting state: one colour per LED.

The unit is the whole device, never a fragment, and three things depend on that.
A write of a full frame is atomic from the device's point of view, so a scene
cannot interleave with a reconcile and leave half of each showing. Coalescing
works, because "latest wins per device" is only meaningful when the latest is a
complete state rather than the most recent fragment of several. And
reconciliation can ask "is this device showing what it should?", which is a
question about a frame — the last instruction sent does not answer it.
*/
type Frame struct {
	Device  string
	Colours []colour.Colour
}

// Uniform reports the single colour of a frame that has one, and whether it
// does. A device with no mode that takes a colour per LED can still show this.
func (f Frame) Uniform() (colour.Colour, bool) {
	if len(f.Colours) == 0 {
		return colour.Colour{}, false
	}
	first := f.Colours[0]
	for _, c := range f.Colours[1:] {
		if c != first {
			return colour.Colour{}, false
		}
	}
	return first, true
}

/*
Dominant is the colour most of this frame is, and whether it has one at all.

A mode that takes one colour for the whole device cannot show a frame of a
hundred, and a scene that names such a mode has still said what it wants that
device to look like. The most common colour is the honest reduction: a keyboard
lit blue with a handful of amber keys is a blue keyboard, and the vendor's
leftover red is not an answer anybody asked for.

Ties go to the colour that appears first, so the reduction of a frame does not
depend on map order.
*/
func (f Frame) Dominant() (colour.Colour, bool) {
	if len(f.Colours) == 0 {
		return colour.Colour{}, false
	}
	count := map[colour.Colour]int{}
	for _, c := range f.Colours {
		count[c]++
	}
	best := f.Colours[0]
	for _, c := range f.Colours {
		if count[c] > count[best] {
			best = c
		}
	}
	return best, true
}

// PerLED reports whether showing this frame needs a mode that accepts a colour
// per LED — that is, whether it holds more than one colour.
func (f Frame) PerLED() bool {
	_, uniform := f.Uniform()
	return !uniform
}

// Equal reports whether two frames would look the same. Reconciliation asks
// this of a device's desired and observed state.
func (f Frame) Equal(other Frame) bool {
	if f.Device != other.Device || len(f.Colours) != len(other.Colours) {
		return false
	}
	for i, c := range f.Colours {
		if c != other.Colours[i] {
			return false
		}
	}
	return true
}

/*
Compose applies assignments to a device and returns the frame to write.

base is what the device is showing now, so LEDs nobody mentioned keep their
colour rather than going dark. A caller with nothing to start from passes nil
and gets black underneath, which is the honest reading of "I do not know what
this device is showing".

Assignments that this device cannot satisfy are returned rather than dropped:
one unknown segment name should cost that assignment and be reported, not take
the whole scene with it.
*/
func Compose(d *Device, rule Rule, base []colour.Colour, assignments []Assignment) (Frame, []error) {
	frame := Frame{Device: d.Name, Colours: make([]colour.Colour, d.LEDCount)}
	copy(frame.Colours, base)

	var problems []error
	for _, a := range assignments {
		spans, err := d.ResolveTarget(a.Target, rule)
		if err != nil {
			problems = append(problems, err)
			continue
		}
		for _, span := range spans {
			for i := span.First; i <= span.Last() && i < len(frame.Colours); i++ {
				frame.Colours[i] = a.Colour
			}
		}
	}
	return frame, problems
}

// Solid is the frame for one colour across a whole device.
func Solid(d *Device, c colour.Colour) Frame {
	frame := Frame{Device: d.Name, Colours: make([]colour.Colour, d.LEDCount)}
	for i := range frame.Colours {
		frame.Colours[i] = c
	}
	return frame
}
