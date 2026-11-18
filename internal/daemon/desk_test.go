package daemon_test

import (
	"github.com/ushineko/hotaru/internal/canvas/canvastest"
	"github.com/ushineko/hotaru/internal/colour"
)

/*
The machine the daemon tests draw on since spec 076: canvases, as sanshoku's
drivers give them. Each is a fresh fake, showing nothing, as a device that
has just powered on.
*/

// mouseCanvas is two lights in one zone.
func mouseCanvas() *canvastest.Board {
	return canvastest.NewBoard("G502 X PLUS", canvastest.Zone("Mouse", 2))
}

// stripCanvas is two lights in one zone.
func stripCanvas() *canvastest.Board {
	return canvastest.NewBoard("Generic Strip", canvastest.Zone("Strip", 2))
}

// first is whether a fake's first light shows c.
func first(d *canvastest.Device, c colour.Colour) bool {
	p, ok := d.Showing(1)
	return ok && p.R == c.R && p.G == c.G && p.B == c.B
}
