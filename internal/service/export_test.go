package service

import (
	"context"
	"time"
)

// SetFlashClock replaces the time a firmware write is measured against and
// how it waits, for a test, and returns how to put them back.
func SetFlashClock(now func() time.Time, wait func(context.Context, time.Duration) bool) func() {
	oldNow, oldWait := flashNow, flashWait
	flashNow, flashWait = now, wait
	return func() { flashNow, flashWait = oldNow, oldWait }
}
