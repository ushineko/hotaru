package service_test

import (
	"testing"

	"github.com/ushineko/hotaru/internal/canvas/canvastest"
	"github.com/ushineko/hotaru/internal/colour"
)

/*
The machine the service tests draw on since spec 076: canvases, as sanshoku's
drivers give them, named as the desk's devices are. Each is a fresh fake.
*/

// auraBoard is a mainboard's own light and one header of 4.
func auraBoard() *canvastest.Board {
	b := canvastest.NewBoard("ASUS ROG MAXIMUS Z790 HERO",
		canvastest.Zone("Aura Mainboard", 1), canvastest.Header("Addressable 1", 0, 120))
	_ = b.Resize("Addressable 1", 4)
	return b
}

// k4 is a keyboard of two keys with two effects of its own.
func k4() *canvastest.Firmware {
	return canvastest.NewFirmware(canvastest.NewBoard("Keychron K4 HE", canvastest.Zone("Keyboard", 2)),
		"Solid Color", "Solid Reactive")
}

// genericStrip is three lights in one zone.
func genericStrip() *canvastest.Board {
	return canvastest.NewBoard("Generic Strip", canvastest.Zone("Strip", 3))
}

// shows is whether a fake shows c on each of its lights. Its keys are
// numbered from 1.
func shows(t *testing.T, d *canvastest.Device, c colour.Colour) bool {
	t.Helper()
	for id := 1; id <= d.Lights; id++ {
		p, ok := d.Showing(byte(id))
		if !ok || p.R != c.R || p.G != c.G || p.B != c.B {
			return false
		}
	}
	return true
}
