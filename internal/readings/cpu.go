package readings

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
)

/*
CPU utilisation, from /proc/stat.

The kernel counts jiffies since boot, so utilisation is a *rate* and needs two
samples to exist at all. The first call after start-up has nothing to
difference against and reports absence -- the same absence a missing sensor
produces, which the panel already knows how to draw. Reporting the since-boot
average instead would be a number that is always true and never useful: a
machine up for a week reads 4% while it compiles.
*/
type CPU struct {
	mu                 sync.Mutex
	path               string
	lastBusy, lastIdle float64
	seen               bool
}

// NewCPU reads the running kernel's counters.
func NewCPU() *CPU { return &CPU{path: "/proc/stat"} }

/*
Load is the percentage busy since the last call.

Between two dashboard ticks that is an average over about two seconds, which
is what a panel read across a room wants: an instant would flicker between 3
and 100 on an idle machine.
*/
func (c *CPU) Load() (float64, bool) {
	busy, idle, err := c.sample()
	if err != nil {
		return 0, false
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	was, wasIdle, seen := c.lastBusy, c.lastIdle, c.seen
	c.lastBusy, c.lastIdle, c.seen = busy, idle, true
	if !seen {
		return 0, false
	}

	total := (busy - was) + (idle - wasIdle)
	if total <= 0 {
		// Two samples inside one jiffy. Not an error, just nothing to say.
		return 0, false
	}
	return 100 * (busy - was) / total, true
}

/*
sample reads the aggregate line.

	cpu  user nice system idle iowait irq softirq steal guest guest_nice

Idle is idle plus iowait: a processor waiting on a disk is not working, and
counting iowait as busy reads 100% through a large copy on an otherwise
sleeping machine. Guest time is already included in user, so adding it again
would double-count a virtual machine's load.
*/
func (c *CPU) sample() (busy, idle float64, err error) {
	body, err := os.ReadFile(c.path) //nolint:gosec // procfs, or a test fixture
	if err != nil {
		return 0, 0, fmt.Errorf("read %s: %w", c.path, err)
	}
	for line := range strings.SplitSeq(string(body), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 || fields[0] != "cpu" {
			continue
		}
		var total float64
		for i, field := range fields[1:] {
			v, err := strconv.ParseFloat(field, 64)
			if err != nil {
				return 0, 0, fmt.Errorf("/proc/stat is not what it was: %w", err)
			}
			if i == 3 || i == 4 { // idle, iowait
				idle += v
				continue
			}
			if i >= 8 { // guest, guest_nice: already counted in user and nice
				continue
			}
			total += v
		}
		return total, idle, nil
	}
	return 0, 0, fmt.Errorf("no aggregate line in %s", c.path)
}
