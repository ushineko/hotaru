package cli_test

import (
	"testing"
	"time"

	"github.com/ushineko/hotaru/internal/canvas/canvastest"
	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/config"
	"github.com/ushineko/hotaru/internal/service/servicetest"
	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/lighting"
)

/*
The machine the CLI tests draw on since spec 076: canvases, as sanshoku's
drivers give them, behind a service served over a socket. Each fixture is a
fresh fake.
*/

// drawing serves a service with cfg that draws on devs, and returns the
// socket's path.
func drawing(t *testing.T, cfg *config.Config, devs ...sanshoku.Device) string {
	t.Helper()
	return serve(t, servicetest.New(t, cfg, devs...))
}

// auraBoard is a mainboard header of four lights.
func auraBoard() *canvastest.Board {
	return canvastest.NewBoard("ASUS ROG MAXIMUS Z790 HERO", canvastest.Zone("Addressable 1", 4))
}

// k4 is a keyboard of two keys with an effect of its own that shows one
// colour, the shape an effect's own settings exist for.
func k4() *canvastest.Firmware {
	return canvastest.NewFirmware(canvastest.NewBoard("Keychron K4 HE", canvastest.Zone("Keyboard", 2)),
		"Solid Color", "Solid Reactive")
}

// shows is the colour a fake shows on a light, by its position from 0.
func shows(t *testing.T, d *canvastest.Device, at int) (colour.Colour, bool) {
	t.Helper()
	p, ok := d.Showing(byte(at + 1)) // a fake's keys are numbered from 1
	return colour.Colour{R: p.R, G: p.G, B: p.B}, ok
}

// eventually and every bound a wait for an animator to draw.
const (
	eventually = 2 * time.Second
	every      = 5 * time.Millisecond
)

// kraken is the development machine's cooler as OpenRGB had it, near enough:
// two channels of 24, one carrying three daisy-chained fans and one carrying
// nothing.
func kraken() *canvastest.Board {
	return canvastest.NewBoard("NZXT Kraken 2024 ELITE Series RGB",
		canvastest.Zone("Hue 2 Channel 1", 24), canvastest.Zone("Hue 2 Channel 2", 24))
}

// zoned is a canvas called name with the zones given.
func zoned(name string, zones ...lighting.Zone) *canvastest.Board {
	return canvastest.NewBoard(name, zones...)
}

// keychron is a keyboard of n keys whose firmware runs the effects given.
func keychron(n int, effects ...string) *canvastest.Firmware {
	return canvastest.NewFirmware(canvastest.NewBoard("Keychron K4 HE", canvastest.Zone("Keyboard", n)), effects...)
}
