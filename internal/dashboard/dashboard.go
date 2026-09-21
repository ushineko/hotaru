package dashboard

import (
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
}

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
