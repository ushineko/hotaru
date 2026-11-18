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

// Every device OpenRGB drew on the desk is found (spec 075 R1.1).
func TestDiscoveryScansEveryLightingDriver(t *testing.T) {
	names := map[string]bool{}
	for _, d := range drivers() {
		names[d.Name()] = true
	}
	for _, want := range []string{"steelseries", "aula", "asus", "corsair", "keychron", "msi", "logitech"} {
		if !names[want] {
			t.Errorf("canvas discovery does not scan %s", want)
		}
	}
}
