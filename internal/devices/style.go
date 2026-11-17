package devices

import "github.com/ushineko/hotaru/internal/colour"

/*
Style is a mode's own settings, as a write asks for them: each nil where the
caller has no opinion, and used only where the mode has it. A device asked for
a setting its mode does not have is not an error; it is a device that does not
have it.
*/
type Style struct {
	// Brightness is 0-100, a multiply on every colour drawn.
	Brightness *int
	// Colour is the mode's own colour, for a mode that keeps one.
	Colour *colour.Colour
	/*
		Colours are the mode's own colours where there are several, first to
		last, and Colour is then their first (spec 061). Nil is one colour or
		none, as Colour says.
	*/
	Colours []colour.Colour
	// Speed is in the mode's own units, within the range it gives.
	Speed *int
	// Pace trims a drawn effect's period for this write; zero is the
	// device rule's (spec 077 R3.9).
	Pace float64
	// Restart starts the effect again from its beginning, even where nothing
	// about it changed.
	Restart bool
}

// Palette is every colour the style names, first to last.
func (s Style) Palette() []colour.Colour {
	if len(s.Colours) > 0 {
		return append([]colour.Colour(nil), s.Colours...)
	}
	if s.Colour != nil {
		return []colour.Colour{*s.Colour}
	}
	return nil
}
