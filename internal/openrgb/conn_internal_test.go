package openrgb

import (
	"testing"

	"github.com/ushineko/hotaru/internal/devices"
)

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

/*
A rescanned server lists every device twice.

Six devices arrive as twelve, and addressing the second copy means sending
every command to the same hardware twice -- which looks like a device that
takes two writes to change, and like nothing else at all.
*/
func TestADeviceListedTwiceIsTakenOnce(t *testing.T) {
	list := []devices.Device{
		{Name: "NZXT Kraken", LEDCount: 48},
		{Name: "Keychron K4 HE", LEDCount: 100},
		{Name: "nzxt kraken", LEDCount: 48}, // the same device, as the server spells it the second time
		{Name: "", LEDCount: 2},             // and a nameless entry, which addresses nothing
	}

	got := collapse(list)
	if len(got) != 2 {
		t.Fatalf("collapse kept %d devices, want 2: %+v", len(got), got)
	}
	if got[0].Name != "NZXT Kraken" || got[1].Name != "Keychron K4 HE" {
		t.Errorf("the first of each name was not the one kept: %+v", got)
	}
}
