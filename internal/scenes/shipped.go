package scenes

/*
The nine scenes this machine has had on its numpad for two years, and the keys
they sit on.

Not invented here. They were read out of peripheral-battery-monitor's own
configuration during the migration -- colour and screen state per key -- because
the bank is what somebody's hands already know, and a rearchitecture that
changes what Ctrl+Alt+Num4 does has broken something no test would catch.

	"1": {"color": "red",     "lcd": "dashboard"}
	...
	"9": {"color": "off",     "lcd": "liquid"}

The monitor's second bank, on the shifted row, is not here. Every one of its
nine points at a GIF under one person's `~/Pictures`, so as defaults they would
be nine broken scenes on every other machine. Those keys are left unbound
instead, which is where somebody's own scenes go. See spec 016.
*/

// Shipped are the scenes every machine has, before anybody saves one.
func Shipped() []Scene {
	return []Scene{
		lit("red", "red", ScreenDashboard),
		lit("green", "green", ScreenDashboard),
		lit("blue", "blue", ScreenDashboard),
		lit("purple", "purple", ScreenDashboard),
		lit("cyan", "cyan", ScreenDashboard),
		lit("orange", "orange", ScreenDashboard),
		lit("white", "white", ScreenReadout),
		/*
			Magenta had a GIF of the Earth on the panel. The scene ships; the
			media does not, because hotaru ships scenes and not somebody's
			pictures -- so this one says nothing about the screen and leaves
			whatever is there.
		*/
		lit("magenta", "magenta", ""),
		{Name: "off", Off: true, Screen: ScreenReadout},
	}
}

// lit is one colour across everything in scope, with a screen state.
func lit(name, colour, screen string) Scene {
	return Scene{Name: name, Colour: colour, Screen: screen}
}

/*
DefaultBindings are the keys as this desk has always had them.

The shifted row -- Ctrl+Alt+Shift+Num1..9 -- is deliberately absent. The
monitor used it for its animation bank; hotaru reserves it for scenes somebody
writes themselves, and sitting on those sequences by default would take the
obvious home for them away.

The spelling is KDE's own, as it appears in kglobalshortcutsrc, because that
string is what gets registered and a translation layer here would be one more
place for a key to be claimed and do nothing.
*/
func DefaultBindings() map[string]string {
	return map[string]string{
		"Ctrl+Alt+Num+1": "red",
		"Ctrl+Alt+Num+2": "green",
		"Ctrl+Alt+Num+3": "blue",
		"Ctrl+Alt+Num+4": "purple",
		"Ctrl+Alt+Num+5": "cyan",
		"Ctrl+Alt+Num+6": "orange",
		"Ctrl+Alt+Num+7": "white",
		"Ctrl+Alt+Num+8": "magenta",
		"Ctrl+Alt+Num+9": "off",
	}
}

// Reserved are the sequences hotaru leaves alone, for somebody's own scenes.
// Listed so `hotaru keys` can say they are free rather than merely not
// mentioning them.
func Reserved() []string {
	return []string{
		"Ctrl+Alt+Shift+Num+1", "Ctrl+Alt+Shift+Num+2", "Ctrl+Alt+Shift+Num+3",
		"Ctrl+Alt+Shift+Num+4", "Ctrl+Alt+Shift+Num+5", "Ctrl+Alt+Shift+Num+6",
		"Ctrl+Alt+Shift+Num+7", "Ctrl+Alt+Shift+Num+8", "Ctrl+Alt+Shift+Num+9",
	}
}
