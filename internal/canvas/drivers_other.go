//go:build !windows

package canvas

import (
	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/corsairdram"
	"github.com/ushineko/sanshoku/msi"
)

/*
pawnioDrivers are the drivers that need PawnIO on Windows: Corsair memory on
the chipset's SMBus, through i2c-dev here, and an MSI board's Super I/O
header, which sanshoku finds and refuses off Windows (spec 082).
*/
func pawnioDrivers() []sanshoku.Driver {
	return []sanshoku.Driver{corsairdram.Driver{}, msi.Board{}}
}
