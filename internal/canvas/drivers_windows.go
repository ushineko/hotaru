package canvas

import (
	"github.com/ushineko/sanshoku"

	"github.com/ushineko/hotaru/internal/pawnhelper"
)

/*
pawnioDrivers is hotaru's PawnIO helper (spec 083): an MSI board's RGB
header and memory on the SMBus are reached through PawnIO, which serves only
an administrator, so the helper service opens them and the service draws on
them through it.
*/
func pawnioDrivers() []sanshoku.Driver { return []sanshoku.Driver{pawnhelper.Driver()} }
