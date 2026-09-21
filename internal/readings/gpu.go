package readings

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

/*
Graphics card utilisation.

AMD puts it in sysfs, where reading it costs a file open. NVIDIA's own driver
registers no hwmon and no busy file, so it comes from nvidia-smi -- the same
call that already fetches the temperature (spec 013), asked for two columns
instead of one.

That is the whole reason this lives beside the temperature reader rather than
asking separately: this project refuses to spawn a process per frame, and one
spawn per dashboard tick answering both numbers is the same cost it already
pays for one.
*/

// busyGlob is where AMD's driver exposes utilisation. A path pattern rather
// than a card index: the numbers move between boots.
const busyGlob = "/sys/class/drm/card*/device/gpu_busy_percent"

// Busy is the percentage the card reports through sysfs.
func Busy() (float64, bool) { return busyIn(busyGlob) }

func busyIn(pattern string) (float64, bool) {
	found, err := filepath.Glob(pattern)
	if err != nil {
		return 0, false
	}
	for _, path := range found {
		body, err := os.ReadFile(path) //nolint:gosec // sysfs, or a test fixture
		if err != nil {
			continue
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(string(body)), 64)
		if err != nil {
			continue
		}
		return v, true
	}
	return 0, false
}

/*
ParseSMI reads one line of nvidia-smi's CSV.

	40, 3 %

Temperature first, utilisation second, in the order they are asked for. The
unit travels with the number and is dropped here; a line that does not parse
is absence, because a format is not an API and this one has changed before.
*/
func ParseSMI(out string) (temperature, load float64, gotTemp, gotLoad bool) {
	line, _, _ := strings.Cut(strings.TrimSpace(out), "\n")
	parts := strings.Split(line, ",")
	if len(parts) > 0 {
		if v, err := strconv.ParseFloat(number(parts[0]), 64); err == nil {
			temperature, gotTemp = v, true
		}
	}
	if len(parts) > 1 {
		if v, err := strconv.ParseFloat(number(parts[1]), 64); err == nil {
			load, gotLoad = v, true
		}
	}
	return temperature, load, gotTemp, gotLoad
}

// number is the leading figure in a field like " 3 %".
func number(field string) string {
	fields := strings.Fields(field)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// SMIFlag is what nvidia-smi is asked for, in the order ParseSMI expects. A
// whole flag rather than a query to concatenate: a constant argument is one
// the subprocess checker can see is not somebody's input.
const SMIFlag = "--query-gpu=temperature.gpu,utilization.gpu"
