package cooler

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

/*
TestReportTheScreensMemory prints the panel's slot table.

In the package rather than beside the other live tests because the slot report
is not exported and should not be: it is the device's own memory map, and the
only reason to look at it is a picture that did not appear.

Opt-in and read-only. It writes nothing, which is why it can be run while
something is on the screen -- and what is on the screen at the time is usually
the question.
*/
func TestReportTheScreensMemory(t *testing.T) {
	if os.Getenv("HOTARU_SCREEN_MEMORY") == "" {
		t.Skip("set HOTARU_SCREEN_MEMORY=1 to read the panel's slot table")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	c, err := Open(ctx)
	if err != nil {
		t.Skipf("no cooler: %v", err)
	}
	defer func() { _ = c.Close() }()

	screen, err := c.Screen()
	if err != nil {
		t.Fatalf("open the screen: %v", err)
	}
	defer func() { _ = screen.Close() }()

	slots, err := screen.slots(ctx)
	if err != nil {
		t.Fatalf("read the slots: %v", err)
	}

	used := 0
	for i, reply := range slots {
		offset, size := u16(reply[17:19]), u16(reply[19:21])
		state := "occupied"
		if vacant(reply) {
			state = "vacant"
		} else {
			used += size
		}
		t.Logf("slot %2d  %-8s  addr %5d  size %5d  (%s)",
			i, state, offset, size, human(size))
	}
	t.Logf("%d of %d packets accounted for (%s of %s)",
		used, capacity, human(used), human(capacity))
}

func human(packets int) string {
	return fmt.Sprintf("%.1f MB", float64(packets)*float64(packetLen)/(1024*1024))
}
