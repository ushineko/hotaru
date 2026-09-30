package cooler

import (
	"github.com/ushineko/sanshoku/hwmon"
)

/*
Processor is the CPU's temperature in degrees, from the kernel.

Read by label, never by hwmon index: the numbers move between boots, and a
program that remembers hwmon5 reports the wrong chip's temperature after a
reboot rather than failing. sanshoku's hwmon.CPU is the list of sensors tried,
Intel's package temperature first and AMD's after it.
*/
func Processor() (float64, error) {
	_, degrees, err := hwmon.First(hwmon.Root, hwmon.CPU)
	return degrees, err //nolint:wrapcheck // hwmon names every sensor it looked for
}
