package render

import "github.com/ushineko/sanshoku/lighting"

/*
ansiColumn is where a key's centre sits across a standard US (ANSI) layout,
in key widths from the left edge, by the name a driver gives the key.

A canvas lists its lights in the device's own order, which on the first
canvas device is A to Z and then the digits: a wave that followed that order
ran across the alphabet, not across the board. Key names are the generic
part, so a wave reads them where it can. The table is the common layout, not
any one product's; a key it does not name keeps its place in the device's
order (spec 060 R2.3).
*/
var ansiColumn = map[string]float64{
	"Escape": 0.5, "F1": 2.5, "F2": 3.5, "F3": 4.5, "F4": 5.5, "F5": 7, "F6": 8,
	"F7": 9, "F8": 10, "F9": 11.5, "F10": 12.5, "F11": 13.5, "F12": 14.5,

	"`": 0.5, "1": 1.5, "2": 2.5, "3": 3.5, "4": 4.5, "5": 5.5, "6": 6.5, "7": 7.5,
	"8": 8.5, "9": 9.5, "0": 10.5, "-": 11.5, "=": 12.5, "Backspace": 14,

	"Tab": 0.75, "Q": 2, "W": 3, "E": 4, "R": 5, "T": 6, "Y": 7, "U": 8, "I": 9,
	"O": 10, "P": 11, "[": 12, "]": 13, `\`: 14.25,

	"Caps Lock": 0.875, "A": 2.25, "S": 3.25, "D": 4.25, "F": 5.25, "G": 6.25,
	"H": 7.25, "J": 8.25, "K": 9.25, "L": 10.25, ";": 11.25, "'": 12.25, "Enter": 13.875,

	"Left Shift": 1.125, "Z": 2.75, "X": 3.75, "C": 4.75, "V": 5.75, "B": 6.75,
	"N": 7.75, "M": 8.75, ",": 9.75, ".": 10.75, "/": 11.75, "Right Shift": 13.625,

	"Left Control": 0.625, "Left GUI": 1.875, "Left Alt": 3.125, "Space": 6.875,
	"Right Alt": 10.625, "Right GUI": 11.875, "Right Control": 14.375,

	"Print Screen": 15.75, "Scroll Lock": 16.75, "Pause": 17.75,
	"Insert": 15.75, "Home": 16.75, "Page Up": 17.75,
	"Delete": 15.75, "End": 16.75, "Page Down": 17.75,
	"Up": 16.75, "Left": 15.75, "Down": 16.75, "Right": 17.75,
}

// ansiWidth is the layout's width in key widths, Escape to Page Up.
const ansiWidth = 18.25

/*
positions is each light's place across the device, from 0 to 1.

A light the layout names sits at its column. One it does not, and every
light when the layout names none of them, sits at its place in the device's
order, so a canvas with unnamed lights draws exactly as it did before.
*/
func positions(keys []lighting.Key) []float64 {
	out := make([]float64, len(keys))
	n := float64(max(len(keys), 1))
	for i, k := range keys {
		if col, ok := ansiColumn[k.Name]; ok {
			out[i] = col / ansiWidth
		} else {
			out[i] = float64(i) / n
		}
	}
	return out
}
