package dashboard

import (
	"image/color"
	"strings"

	"github.com/ushineko/hotaru/internal/readings"
)

/*
Dashboard is what the panel is asked to show.

Not a picture: a description of one. The service renders it, because the panel
takes a 640x640 GIF and the service is what makes them -- a second renderer in
the window would be a second answer about what the screen looks like, and the
one nobody checks is the one on screen.
*/
type Dashboard struct {
	/*
		Name is what it is called and how it is chosen.

		Never written: the map key in the file is the name, and a copy of it
		inside the entry is a second place for it to disagree. The settings
		codec encodes through JSON, so `json:"-"` is what keeps it out of the
		YAML too -- yaml tags there are ignored.
	*/
	Name string `json:"-"`

	// Shipped marks one hotaru carries in code rather than in somebody's
	// file. Never written.
	Shipped bool `json:"-"`

	// Arrangement is where things go. An unknown one draws as Ring, because
	// a dashboard written by a later version should still light up.
	Arrangement string `json:"arrangement,omitempty"`

	// Theme names the colours. An unknown one draws in the default.
	Theme string `json:"theme,omitempty"`

	// Background is what is behind the numbers.
	Background Background `json:"background,omitempty"`

	// Headline is the big number.
	Headline Slot `json:"headline,omitempty"`

	/*
		Rings are the arcs around the headline, outermost first.

		Each tracks a reading of its own rather than the headline's, because
		a gauge is worth more when it says something the number does not.
		Inner rings are drawn progressively thinner, which is what keeps four
		of them legible at arm's length: the eye reads the order as depth
		rather than as four arcs it has to tell apart by radius.

		Empty means one ring tracking the headline, which is what the panel
		has always drawn.
	*/
	Rings []readings.Source `json:"rings,omitempty"`

	// Slots are the smaller ones. More than the arrangement has room for are
	// dropped rather than drawn over each other.
	Slots []Slot `json:"slots,omitempty"`

	/*
		Units draws what each number is measured in beside it, small, on the
		line the separator sits on.

		Off by default, which is every dashboard written before spec 044. On,
		the generated label stops carrying the unit -- it is beside the number
		now, and a label repeating it is the redundancy the unit line was
		removed for.
	*/
	Units bool `json:"units,omitempty"`

	// Caption is a line of the author's own text. Empty draws nothing and
	// takes no space.
	Caption string `json:"caption,omitempty"`

	// Lettering is how the words and the numbers are drawn. Empty draws
	// them the way the arrangement and the theme say.
	Lettering Lettering `json:"lettering,omitempty"`
}

/*
Lettering is how a dashboard's text is drawn, over what the arrangement and
the theme already decided.

Split into labels and values because they are read differently: a label is a
word somebody has learned the shape of and glances past, and a value is the
thing they are actually looking at from across a room. Making one bigger is
usually a reason to leave the other alone.

Everything here is a *modification*, never a replacement. The arrangement
still chooses where text goes and how big it is relative to the rest, and the
theme still chooses the colours; this scales and recolours what they decided.
A panel whose sizes were set by looking at it in a case (spec 013) stays laid
out that way.
*/
type Lettering struct {
	// Font is the face: empty or "sans" for Go, "mono" for Go Mono,
	// "smallcaps" for Go Smallcaps. An unknown one draws in the default,
	// because a dashboard written by a later version should still light up.
	Font string `json:"font,omitempty"`

	// Labels are the words, Values the readings.
	Labels Text `json:"labels,omitempty"`
	Values Text `json:"values,omitempty"`
}

// Text is one category's lettering.
type Text struct {
	/*
		Size is a percentage of what the arrangement draws. Zero means 100.

		A percentage rather than points: the arrangement's sizes are spec
		013's, arrived at by looking at a panel in a case, and the headline
		being three times its label is a relationship worth keeping when
		somebody makes both bigger.
	*/
	Size int `json:"size,omitempty"`

	// Colour is "#rrggbb". Empty is the theme's own.
	Colour string `json:"colour,omitempty"`

	/*
		Outline is how many pixels of dark edge the text carries.

		Nil is the background's decision -- two pixels over a picture, none
		over the theme's own colours -- which is what the panel has always
		drawn. Zero is explicitly none, which is why this is a pointer: "not
		set" and "set to none" are different answers and a plain int cannot
		hold both.
	*/
	Outline *int `json:"outline,omitempty"`
}

// Scale is Size as a multiplier, clamped to what still fits the panel.
func (t Text) Scale() float64 {
	if t.Size == 0 {
		return 1
	}
	return float64(min(max(t.Size, MinSize), MaxSize)) / 100
}

// The range a size may be set to. Below the lower bound the text is unreadable
// at arm's length, which is the whole job; above the upper one it leaves the
// space the arrangement gave it and overlaps its neighbour.
const (
	MinSize = 50
	MaxSize = 150
)

// Edge is how thick an outline to draw, given whether the background is a
// picture. See Text.Outline.
func (t Text) Edge(overPicture bool) int {
	if t.Outline != nil {
		return max(*t.Outline, 0)
	}
	if overPicture {
		return DefaultOutline
	}
	return 0
}

// DefaultOutline is the edge text carries over a picture. Two pixels at 640:
// enough to survive a bright background, small enough that the digits do not
// close up at the headline's size.
const DefaultOutline = 2

/*
chosen are the colours this lettering names, for the palette.

A paletted image draws in the nearest colour it has, so a colour that is not
in the palette is a colour somebody asked for and did not get. Both are added
whether or not they are used: the cost is two entries, and the alternative is
working out which text will be drawn before the drawing starts.
*/
func (l Lettering) chosen() []color.RGBA {
	var out []color.RGBA
	for _, text := range []Text{l.Labels, l.Values} {
		if c, ok := parseColour(text.Colour); ok {
			out = append(out, c)
		}
	}
	return out
}

// Fonts are the faces a dashboard may name, in the order the editor offers
// them. Compiled in, because a panel that drew in a font somebody else
// installed would draw differently on the next machine.
func Fonts() []string { return []string{"sans", "mono", "smallcaps"} }

/*
Slot is what one place on the panel says.

One reading, or two. CPU load and CPU temperature are one thought, and so are
memory used and memory free; drawn as `11 / 59` under a label that says
`CPU % / C` they take the room of one number and answer two questions.

Label is the dashboard's own words where it has them and the reading's where
it does not, and it is the *only* place words are drawn. There used to be a
unit under every value, taken from the readings package and editable nowhere,
which is one line spent saying something the author never chose. The label
says it now, and the author can spell it how they like.
*/
type Slot struct {
	Source readings.Source `json:"source"`

	// Second is the other half of a pair. Empty is one reading, which is
	// what every dashboard written before this said.
	Second readings.Source `json:"second,omitempty"`

	/*
		Separator goes between the two values. Empty is DefaultSeparator.

		The author's, because the label beside it is: somebody writing
		`CPU % / C` above their numbers wants a slash between them, and
		somebody writing `CPU %  C` wants two spaces. One of them would
		otherwise be writing a label that does not match the value under it.
	*/
	Separator string `json:"separator,omitempty"`

	Label string `json:"label,omitempty"`
}

/*
DefaultSeparator is what joins a pair when the author has said nothing.

A middle dot with a space either side. ` / ` was the first guess and this is
what somebody chose after looking at both on the panel, which is the only
place the question could be answered.
*/
const DefaultSeparator = " · "

/*
Words is the label this slot draws.

The author's, or one built from what the readings are called and measured in.
The built one carries the unit, which is what the unit line used to do: a slot
nobody has edited still says whether it is degrees or percent.

Two readings with the same name are named once -- `CPU % · °C` rather than
`CPU % · CPU °C`, because the second CPU is a word the eye has to read to
learn nothing.

**Divided by whatever divides the numbers.** A label reading `CPU % / °C`
over a value reading `12 · 63` is two answers to the same question, and the
one the eye believes is the one in the label.
*/
func (s Slot) Words(units bool) string {
	if s.Label != "" {
		return s.Label
	}

	label, unit := readings.Describe(s.Source)
	if units {
		// Beside the number now; a label repeating it is the redundancy the
		// unit line was removed for.
		unit = ""
	}
	if s.Second == "" {
		return strings.TrimSpace(label + " " + unit)
	}

	other, otherUnit := readings.Describe(s.Second)
	if units {
		otherUnit = ""
	}
	if other == label {
		other = ""
	}
	if other == "" && otherUnit == "" {
		// Two readings of one thing, named once, measured in nothing worth
		// saying: "CPU" rather than "CPU / ".
		return strings.TrimSpace(label + " " + unit)
	}
	return strings.TrimSpace(label+" "+unit) + s.Join() + strings.TrimSpace(other+" "+otherUnit)
}

// Join is the separator this slot puts between its two values.
func (s Slot) Join() string {
	if s.Separator == "" {
		return DefaultSeparator
	}
	return s.Separator
}

/*
Text is the value this slot draws: one reading, or two joined.

The placeholder for a reading the machine did not have is the reading
package's, per half. A pair with one sensor missing reads `-- · 59`, which
says which half went away -- where dropping to a single placeholder would
report both as absent on the evidence of one.

This is the value as a *string*, for the CLI and for anything comparing two
frames. What the panel draws is Fields, which is the same text in boxes that
do not move.
*/
func (s Slot) Text(r readings.Reading) string {
	if s.Second == "" {
		return r.Text(s.Source)
	}
	return r.Text(s.Source) + s.Join() + r.Text(s.Second)
}

/*
Field is one piece of a drawn value: the text, and how much room to keep for
it whatever it happens to say this time.

Chars is a count of characters, turned into pixels by whoever is drawing --
the digits are tabular in every face the dashboard offers, so a count is a
width. Zero means the piece is literal and gets exactly what it measures,
which is what the separator is.
*/
type Field struct {
	Text  string
	Chars int

	/*
		Small marks a piece drawn smaller than the number it belongs to: a
		unit, at the headline's size, would otherwise be as large as the
		figure it qualifies and read as a second number.

		Centred in the same band as everything else on the line, so it sits
		where the separator sits, which is where the eye already is.
	*/
	Small bool

	// Divider marks the piece between two readings. What is before it grows
	// leftwards and what is after it grows right, so the two numbers stay
	// against the thing that separates them.
	Divider bool
}

/*
UnitScale is how large a unit is drawn against the number beside it.

Picked by eye on the panel. Small enough not to compete with the figure,
large enough to read across a desk -- which is the whole job, and the reason
this is a number somebody looked at rather than one derived from anything.
*/
const UnitScale = 0.45

/*
UnitGap is the space between a number and its unit, as a fraction of the
number's size.

Spec 044 drew the two as one run and they came out touching; this is the hair
of space that separates the figure from what it is measured in. Scaled with
the text, like the unit itself, so it is the same gap wherever it is drawn --
and, like the unit's size, a number picked by looking at the panel.
*/
const UnitGap = 0.06

/*
Fields is the value as boxes rather than as a string.

**The reason the panel stops wobbling.** A value drawn to the width of
whatever it says this time moves every character when the character count
changes, and the character count changes on every reading that crosses ten or
a hundred. Each number gets the width it *can* take instead, so the digits sit
in the same columns from one frame to the next and the assembly's total width
never depends on the reading.

A field is a minimum. A number wider than its reservation is drawn in full and
pushes -- one shift at the extreme, rather than a digit lost.
*/
func (s Slot) Fields(r readings.Reading, units bool) []Field {
	out := []Field{{Text: r.Text(s.Source), Chars: readings.Width(s.Source)}}
	out = append(out, unitOf(s.Source, units)...)
	if s.Second == "" {
		return out
	}

	out = append(out, Field{Text: s.Join(), Divider: true},
		Field{Text: r.Text(s.Second), Chars: readings.Width(s.Second)})
	return append(out, unitOf(s.Second, units)...)
}

/*
unitOf is the small piece after a number, or nothing.

Nothing twice over: when the dashboard does not want units, and when the
reading has none to give. A source measured in no particular thing would
otherwise reserve a gap after its number for a string that is empty.
*/
func unitOf(source readings.Source, units bool) []Field {
	if !units {
		return nil
	}
	_, unit := readings.Describe(source)
	if unit == "" {
		return nil
	}
	return []Field{{Text: unit, Small: true}}
}

// Background kinds.
const (
	Starfield = "starfield"
	Plain     = "plain"
	Picture   = "picture"
)

/*
Background is what is drawn behind the numbers.

A picture costs refresh rate rather than nothing: the push floor scales with
frame size (spec 013), so a photograph behind the readings is a panel that
settles more slowly. That is a real trade and the editor says so.
*/
type Background struct {
	// Kind is Starfield, Plain or Picture. Empty means Starfield.
	Kind string `json:"kind,omitempty"`

	// Picture names one in the image library. A picture that has been
	// removed costs the background and not the frame: it falls back to the
	// theme's plain colour.
	Picture string `json:"picture,omitempty"`

	// Dim is how far the picture is darkened, as a percentage. Zero is the
	// default rather than "not at all": a photograph at full brightness
	// under white text is the case the outline exists to survive, and it
	// should not have to.
	Dim int `json:"dim,omitempty"`
}

// DefaultDim is how much a picture is darkened when the author says nothing.
// Chosen by looking at the panel with a bright wallpaper behind the numbers.
const DefaultDim = 55

// Darkness is the fraction of the picture that survives.
func (b Background) Darkness() float64 {
	dim := b.Dim
	if dim <= 0 {
		dim = DefaultDim
	}
	if dim > 100 {
		dim = 100
	}
	return 1 - float64(dim)/100
}

/*
Outlined reports whether text needs a dark outline around every glyph.

Only over a picture. The starfield was drawn with these numbers in mind and
an outline would only make it heavier; a photograph is somebody else's and
will sooner or later put a light region under light text, at which point the
number is gone. An outline costs eight offset draws per string and makes the
text legible over anything.
*/
func (b Background) Outlined() bool { return b.Kind == Picture }
