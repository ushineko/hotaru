package dashboard

import "image/color"

/*
Theme is the colours, and only the colours.

Geometry is the arrangement's business. Splitting them this way means a theme
can be added without anybody looking at a panel to check it still fits, and an
arrangement can be added without choosing eight colours to go with it.

The coolant grade -- green, amber, red -- is deliberately not here. It is the
one colour on this screen that carries meaning rather than style, and it has
to agree with what the alerts say: a screen showing calm while a notification
says otherwise is worse than either alone.
*/
type Theme struct {
	// Name is what the editor calls it.
	Name string
	// BG is the plain background and the darkest colour on the panel.
	BG color.RGBA
	// Muted is labels and units; Accent is a value that carries no grade;
	// Bright is the lit tick; Edge is the unfilled part of the ring.
	Muted, Accent, Bright, Edge color.RGBA
	// Sky is the starfield's nebulae, tinted. Empty draws the shipped ones.
	Sky []color.RGBA
	/*
		Ramp is how far the background climbs towards the sky over the
		palette's 24 gradient steps, as an amount added to BG.

		Written down rather than derived from Accent because the default
		theme's ramp is spec 013's, and a derived one differing by a few
		values per channel re-quantises the whole starfield -- which showed
		up as a frame 24.9 KB instead of 23.9 KB, and a panel that settles in
		three seconds instead of two.
	*/
	Ramp color.RGBA
}

/*
Themes are the collection, in the order the editor offers them.

Four, because a theme is judged on a panel in a case and each of these was.
A longer list would mostly be colours nobody has seen lit.
*/
func Themes() []Theme {
	return []Theme{
		{
			Name: "midnight",
			BG:   color.RGBA{12, 14, 18, 255}, Muted: color.RGBA{130, 140, 155, 255},
			Accent: color.RGBA{120, 190, 255, 255}, Bright: color.RGBA{236, 239, 244, 255},
			Edge: color.RGBA{60, 68, 82, 255},
			Ramp: color.RGBA{100, 80, 140, 255},
		},
		{
			Name: "ice",
			BG:   color.RGBA{10, 18, 24, 255}, Muted: color.RGBA{120, 150, 165, 255},
			Accent: color.RGBA{150, 225, 255, 255}, Bright: color.RGBA{240, 250, 255, 255},
			Edge: color.RGBA{48, 76, 90, 255},
			Ramp: color.RGBA{50, 95, 135, 255},
			Sky: []color.RGBA{
				{40, 90, 170, 255}, {30, 130, 160, 255},
				{60, 110, 190, 255}, {35, 100, 150, 255},
			},
		},
		{
			Name: "amber",
			BG:   color.RGBA{20, 14, 8, 255}, Muted: color.RGBA{160, 135, 105, 255},
			Accent: color.RGBA{255, 190, 90, 255}, Bright: color.RGBA{255, 240, 220, 255},
			Edge: color.RGBA{85, 64, 40, 255},
			Ramp: color.RGBA{135, 85, 35, 255},
			Sky: []color.RGBA{
				{150, 70, 30, 255}, {170, 110, 30, 255},
				{130, 50, 40, 255}, {110, 70, 25, 255},
			},
		},
		{
			Name: "mono",
			BG:   color.RGBA{10, 10, 10, 255}, Muted: color.RGBA{125, 125, 125, 255},
			Accent: color.RGBA{225, 225, 225, 255}, Bright: color.RGBA{255, 255, 255, 255},
			Edge: color.RGBA{60, 60, 60, 255},
			Ramp: color.RGBA{70, 70, 70, 255},
			Sky: []color.RGBA{
				{50, 50, 50, 255}, {40, 40, 40, 255},
				{58, 58, 58, 255}, {34, 34, 34, 255},
			},
		},
	}
}

// DefaultTheme is what an unnamed or unknown theme draws as. Unknown rather
// than refused: a dashboard written by a later version should still light up.
const DefaultTheme = "midnight"

// ThemeOf finds a theme by name, falling back to the default.
func ThemeOf(name string) Theme {
	all := Themes()
	for _, theme := range all {
		if theme.Name == name {
			return theme
		}
	}
	for _, theme := range all {
		if theme.Name == DefaultTheme {
			return theme
		}
	}
	return all[0]
}
