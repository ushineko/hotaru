package dashboard

import (
	"image/color"

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
Slot is one reading, as this dashboard wants it said.

Label and Unit are the dashboard's own words where it has them. Empty means
the reading's: "Coolant" and "°C" are already written down once, in the
readings package, and a dashboard that had to repeat them would be a second
place for them to be wrong.
*/
type Slot struct {
	Source readings.Source `json:"source"`
	Label  string          `json:"label,omitempty"`
	Unit   string          `json:"unit,omitempty"`
}

// Words are the label and unit this slot draws.
func (s Slot) Words() (label, unit string) {
	label, unit = readings.Describe(s.Source)
	if s.Label != "" {
		label = s.Label
	}
	if s.Unit != "" {
		unit = s.Unit
	}
	return label, unit
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
