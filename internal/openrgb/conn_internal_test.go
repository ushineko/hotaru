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
takes two writes to change, and like nothing else at all. The same location is
what makes them the same device.
*/
func TestADeviceListedTwiceIsTakenOnce(t *testing.T) {
	list := at(
		devices.Device{Name: "NZXT Kraken", Location: "HID: /dev/hidraw4", LEDCount: 48},
		devices.Device{Name: "Keychron K4 HE", Location: "HID: /dev/hidraw7", LEDCount: 100},
		devices.Device{Name: "nzxt kraken", Location: "HID: /dev/hidraw4", LEDCount: 48}, // the same one again
		devices.Device{Name: "", Location: "nowhere", LEDCount: 2},                       // nameless
	)

	got := collapse(list)
	if len(got) != 2 {
		t.Fatalf("collapse kept %d devices, want 2: %+v", len(got), got)
	}
	if got[0].device.Name != "NZXT Kraken" || got[1].device.Name != "Keychron K4 HE" {
		t.Errorf("the first of each name was not the one kept: %+v", got)
	}
}

/*
Four sticks of RAM are four devices with one name between them.

Found on the second machine, which is what that test exists for: Corsair's DDR5
reports the same name at four I2C addresses, and a motherboard turned up as two
controllers on different hidraw nodes. Collapsing by name hid three sticks and
half a board, and nothing said so -- writes to the one that survived succeeded,
and the rest of the machine stayed dark.
*/
func TestSeveralOfTheSameModelAreSeveralDevices(t *testing.T) {
	var raw []devices.Device
	for _, address := range []string{"0x18", "0x19", "0x1A", "0x1B"} {
		raw = append(raw, devices.Device{
			Name:     "Corsair Dominator Platinum RGB DDR5",
			Location: "I2C: SMBus I801 adapter at 0000:00:1f.4 (/dev/i2c-21), address " + address,
			LEDCount: 12,
		})
	}
	raw = append(raw,
		devices.Device{Name: "Z790 AORUS MASTER X", Location: "HID: /dev/hidraw10", Serial: "0x57010100", LEDCount: 1},
		devices.Device{Name: "Z790 AORUS MASTER X", Location: "HID: /dev/hidraw8", Serial: "0x57020100", LEDCount: 1},
		devices.Device{Name: "Razer Mouse Dock Pro", Location: "HID: /dev/hidraw0", LEDCount: 9},
	)

	got := collapse(at(raw...))
	if len(got) != 7 {
		t.Fatalf("collapse kept %d devices, want 7: %+v", len(got), got)
	}

	// Each one is addressable, because a name is how everything above this
	// layer names a device.
	names := map[string]bool{}
	for _, entry := range got {
		if names[entry.device.Name] {
			t.Errorf("two devices share the name %q, so one of them cannot be addressed", entry.device.Name)
		}
		names[entry.device.Name] = true
	}
	for _, want := range []string{
		"Corsair Dominator Platinum RGB DDR5 (0x18)",
		"Corsair Dominator Platinum RGB DDR5 (0x1B)",
		"Z790 AORUS MASTER X (0x57010100)",
		"Z790 AORUS MASTER X (0x57020100)",
		"Razer Mouse Dock Pro", // alone, so it keeps the name it came with
	} {
		if !names[want] {
			t.Errorf("no device called %q; got %v", want, keys(names))
		}
	}
}

// at numbers devices as a listing would, so a test can speak in devices and the
// code can speak in where it found them.
func at(list ...devices.Device) []located {
	out := make([]located, 0, len(list))
	for i, device := range list {
		out = append(out, located{index: uint32(i), device: device})
	}
	return out
}

func keys(in map[string]bool) []string {
	out := make([]string, 0, len(in))
	for name := range in {
		out = append(out, name)
	}
	return out
}
