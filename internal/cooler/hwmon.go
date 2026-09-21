package cooler

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

/*
Sensor is one labelled temperature the kernel already exposes.

peripheral-battery-monitor asked OpenLinkHub over HTTP for the CPU package
temperature, because that is what Python could reach. The kernel has it in a
file: `coretemp` for the package, `nct6798` for the board, `corsairpsu` for the
supply. So OpenLinkHub stops being a dependency of hotaru -- see spec 012.

Read by **label**, never by hwmon index. The numbers move between boots, and a
program that remembers hwmon5 reports the wrong chip's temperature after a
reboot rather than failing.
*/
type Sensor struct {
	Chip  string // the hwmon name, e.g. "coretemp"
	Label string // e.g. "Package id 0"
}

// CPUPackage is the reading the dashboard shows and the Python got from a
// daemon.
var CPUPackage = Sensor{Chip: "coretemp", Label: "Package id 0"}

// Temperature reads one labelled sensor, in degrees.
func (s Sensor) Temperature() (int, error) { return s.read("/sys/class/hwmon") }

func (s Sensor) read(root string) (int, error) {
	chips, err := filepath.Glob(filepath.Join(root, "hwmon*"))
	if err != nil {
		return 0, fmt.Errorf("look for sensors: %w", err)
	}
	for _, chip := range chips {
		name, err := os.ReadFile(filepath.Join(chip, "name")) //nolint:gosec // sysfs
		if err != nil || strings.TrimSpace(string(name)) != s.Chip {
			continue
		}
		labels, err := filepath.Glob(filepath.Join(chip, "temp*_label"))
		if err != nil {
			return 0, fmt.Errorf("look inside %s: %w", s.Chip, err)
		}
		for _, label := range labels {
			b, err := os.ReadFile(label) //nolint:gosec // sysfs
			if err != nil || strings.TrimSpace(string(b)) != s.Label {
				continue
			}
			return milli(strings.TrimSuffix(label, "_label") + "_input")
		}
	}
	return 0, fmt.Errorf("no sensor %q on %s", s.Label, s.Chip)
}

// milli reads a hwmon temperature, which is thousandths of a degree.
func milli(path string) (int, error) {
	b, err := os.ReadFile(path) //nolint:gosec // sysfs
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", path, err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", path, err)
	}
	return n / 1000, nil
}
