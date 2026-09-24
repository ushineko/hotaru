package daemon

import (
	"context"
	"time"

	"github.com/ushineko/hotaru/internal/cooler"
)

// WaitForCooler is waitForCooler, for a test: what it does is wait, and a
// test that could not drive the waiting would be a test of nothing.
func WaitForCooler(ctx context.Context, open func(context.Context) (*cooler.Cooler, error),
	backoff []time.Duration, report func(string, ...any),
) *cooler.Cooler {
	return waitForCooler(ctx, open, backoff, report)
}
