package canvas

import "testing"

// The cooler's lights are drawn on the cooler's handle; discovery opening the
// nzxt driver would hold the node a second time (spec 073 R1.1).
func TestDiscoveryNeverOpensTheCooler(t *testing.T) {
	for _, d := range drivers() {
		if d.Name() == "nzxt" {
			t.Fatal("canvas discovery scans the nzxt driver")
		}
	}
}
