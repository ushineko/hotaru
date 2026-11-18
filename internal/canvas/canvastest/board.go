package canvastest

import (
	"fmt"

	"github.com/ushineko/sanshoku/lighting"
)

/*
Board is a canvas that names its zones (lighting.Zoned), as every sanshoku
driver hotaru draws does since spec 075: a mainboard's own light and its
headers, a keyboard's one zone, a mousepad's three. A zone with a Max is
resizable, as an addressable header is; Resize changes its count and the
lights after it move along.
*/
type Board struct {
	*Device
	zones []lighting.Zone
}

// Zone is a zone of n lights, for NewBoard.
func Zone(name string, n int) lighting.Zone { return lighting.Zone{Name: name, Count: n} }

// Header is a resizable zone of n lights, at most most, for NewBoard.
func Header(name string, n, most int) lighting.Zone {
	return lighting.Zone{Name: name, Count: n, Max: most}
}

// NewBoard is a canvas called name with the zones given, in order.
func NewBoard(name string, zones ...lighting.Zone) *Board {
	b := &Board{Device: New(0), zones: append([]lighting.Zone(nil), zones...)}
	b.Name, b.Path = name, "fake:"+name
	b.layout()
	return b
}

// layout numbers the zones' lights in order. The caller holds mu, or owns b.
func (b *Board) layout() {
	first := 0
	for i := range b.zones {
		b.zones[i].First = first
		first += b.zones[i].Count
	}
	b.Lights = first
}

// Zones is the zones, as they are now.
func (b *Board) Zones() []lighting.Zone {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]lighting.Zone(nil), b.zones...)
}

// Resize sets a resizable zone's length, refusing any other zone or a length
// out of its range.
func (b *Board) Resize(zone string, lights int) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	for i, z := range b.zones {
		if z.Name != zone {
			continue
		}
		if !z.Resizable() {
			return fmt.Errorf("%q is not resizable: %w", zone, lighting.ErrNoZone)
		}
		if lights < z.Min || lights > z.Max {
			return fmt.Errorf("%d lights for %q: out of range", lights, zone)
		}
		b.zones[i].Count = lights
		b.layout()
		return nil
	}
	return fmt.Errorf("%q: %w", zone, lighting.ErrNoZone)
}
