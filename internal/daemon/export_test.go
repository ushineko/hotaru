package daemon

import (
	"context"
	"time"

	"github.com/ushineko/hotaru/internal/cooler"
	"github.com/ushineko/hotaru/internal/openrgb"
	"github.com/ushineko/hotaru/internal/service"
)

// Dialer is dialer, so a test can supply a server that is not a socket.
type Dialer = func(ctx context.Context, address string) (openrgb.Client, error)

// Supervise is supervise, for a test: reconnecting is the behaviour, and a
// test that could not take the server away would be a test of nothing.
func Supervise(ctx context.Context, svc *service.Service, address string,
	report func(string, ...any), dial Dialer, every time.Duration,
) {
	supervise(ctx, svc, address, report, dial, every)
}

// WaitForCooler is waitForCooler, for a test: what it does is wait, and a
// test that could not drive the waiting would be a test of nothing.
func WaitForCooler(ctx context.Context, open func(context.Context) (*cooler.Cooler, error),
	backoff []time.Duration, report func(string, ...any),
) *cooler.Cooler {
	return waitForCooler(ctx, open, backoff, report)
}
