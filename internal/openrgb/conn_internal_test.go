package openrgb

import "testing"

/*
The wire carries C strings, terminator included.

This is the shape of the bug the live test found: the comparison that decides
which mode to send is case-insensitive but not NUL-insensitive, so a rule asking
for "static" matched nothing on hardware that answers "Static\x00", and every
device fell through to "no mode that shows a solid colour".
*/
func TestNamesFromTheWireLoseTheirTerminator(t *testing.T) {
	for in, want := range map[string]string{
		"Direct\x00": "Direct",
		"MSI GeForce RTX 4090 Suprim Liquid X\x00": "MSI GeForce RTX 4090 Suprim Liquid X",
		"Solid Splash\x00":                         "Solid Splash",
		"  Static\x00  ":                           "Static",
		"Static":                                   "Static",
		"":                                         "",
	} {
		if got := clean(in); got != want {
			t.Errorf("clean(%q) = %q, want %q", in, got, want)
		}
	}
}
