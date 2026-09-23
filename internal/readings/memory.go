package readings

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

/*
Memory in use, from /proc/meminfo.

**`MemAvailable`, not `MemFree`.** Free memory on a machine that has been up
an hour is a small number on almost every Linux box, because the kernel spends
what nobody is using on cache and gives it back on demand. A panel reporting
that would read alarming on a healthy machine and would be wrong. Available is
the kernel's own estimate of what a workload could take without swapping --
the column `free -h` labels "available" -- and used is what is left.

Unlike utilisation this is not a rate, so there is nothing to keep between
calls: every reading opens the file.
*/
type Memory struct{ path string }

// NewMemory reads the running kernel's figures.
func NewMemory() *Memory { return &Memory{path: "/proc/meminfo"} }

/*
Used is memory in use as a percentage and in gigabytes.

Both or neither. They come from the same two fields, so a file that answered
one and not the other would be a file that is not /proc/meminfo, and reporting
half of it would put a number on the panel with nothing behind it.
*/
func (m *Memory) Used() (percent, gigabytes float64, ok bool) {
	total, available, err := m.sample()
	if err != nil || total <= 0 {
		return 0, 0, false
	}

	used := total - available
	if used < 0 {
		// Available above total is not a machine, it is a file that is not
		// what this expects.
		return 0, 0, false
	}
	// meminfo is in kibibytes, and a gigabyte on a panel is a gibibyte: the
	// number beside it in every system monitor on the machine is that one.
	return 100 * used / total, used / (1024 * 1024), true
}

// sample reads the two fields, in kibibytes.
func (m *Memory) sample() (total, available float64, err error) {
	body, err := os.ReadFile(m.path) //nolint:gosec // procfs, or a test fixture
	if err != nil {
		return 0, 0, fmt.Errorf("read %s: %w", m.path, err)
	}

	var haveTotal, haveAvailable bool
	for line := range strings.SplitSeq(string(body), "\n") {
		name, rest, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		switch name {
		case "MemTotal":
			total, err = kibibytes(rest)
			haveTotal = true
		case "MemAvailable":
			available, err = kibibytes(rest)
			haveAvailable = true
		default:
			continue
		}
		if err != nil {
			return 0, 0, fmt.Errorf("%s is not what it was: %w", m.path, err)
		}
	}

	if !haveTotal || !haveAvailable {
		// MemAvailable arrived in Linux 3.14. An older kernel reports
		// absence, which the panel already draws as a placeholder, rather
		// than a figure derived from MemFree that would be wrong.
		return 0, 0, fmt.Errorf("no MemTotal and MemAvailable in %s", m.path)
	}
	return total, available, nil
}

// kibibytes reads "  32659284 kB" as a number.
func kibibytes(field string) (float64, error) {
	fields := strings.Fields(field)
	if len(fields) == 0 {
		return 0, fmt.Errorf("no number in %q", field)
	}
	kb, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, fmt.Errorf("%q is not a number of kilobytes: %w", fields[0], err)
	}
	return kb, nil
}
