package daemon

import (
	"context"
	"strings"
	"time"

	"github.com/ushineko/hotaru/internal/service"
)

/*
Reconciler keeps the hardware showing what it should.

Two jobs that look alike and are not. **Restoring** happens once, after the
OpenRGB server becomes reachable, and puts back what was last asked for.
**Re-asserting** happens forever, on an interval, for the devices that do not
hold what they are told.

Restoring is deliberately not a start-up step that runs and finishes. OpenRGB
detects devices once, when it starts, and a cold boot has been observed finding
two devices out of six -- so a restore that reached fewer devices than were
recorded retries rather than declaring victory, and completes silently when the
rest turn up.
*/
type Reconciler struct {
	Service *service.Service

	// Backoff is how long to wait between restore attempts, growing to the
	// last value and staying there. Long enough not to be a busy loop; short
	// enough to catch devices that enumerate a few seconds after login.
	Backoff []time.Duration

	// Every is how often to check whether any device is due a re-assert.
	Every time.Duration

	// Report is where progress goes. One line per change of state, never one
	// per tick: a machine with no OpenRGB should not have its journal filled
	// with a fact that is not changing.
	Report func(format string, args ...any)
}

// DefaultBackoff and DefaultEvery are what the service runs with.
var (
	DefaultBackoff = []time.Duration{time.Second, 2 * time.Second, 5 * time.Second, 15 * time.Second, 30 * time.Second}
	DefaultEvery   = 5 * time.Second
)

/*
Run restores, then keeps re-asserting, until the context is cancelled.

With nothing recorded there is nothing to restore and nothing to re-assert, and
this loop costs a timer. That is the inert rule at run time: a fresh install
starts every mechanism and writes to no device.
*/
func (r *Reconciler) Run(ctx context.Context) {
	r.restore(ctx)
	r.reassert(ctx)
}

func (r *Reconciler) restore(ctx context.Context) {
	if r.Service.Desired().Empty() {
		return
	}

	var said string
	for attempt := 0; ctx.Err() == nil; attempt++ {
		got, err := r.Service.Reconcile(ctx, nil)
		switch {
		case err != nil:
			// No server yet. The connector reports that; saying it twice from
			// two goroutines helps nobody.
		case got.Complete():
			if got.Applied > 0 {
				r.report("restored %d devices to what they were showing", got.Applied)
			}
			return
		default:
			// Say it once per distinct set of missing devices, so a machine
			// waiting on one device says so once rather than every few seconds.
			now := strings.Join(got.Missing, ", ")
			if now != said {
				r.report("restore incomplete: %d devices put back, still waiting for %s", got.Applied, now)
				said = now
			}
		}

		if !r.wait(ctx, r.backoff(attempt)) {
			return
		}
	}
}

func (r *Reconciler) reassert(ctx context.Context) {
	tracker := service.NewReasserter()
	every := r.Every
	if every <= 0 {
		every = DefaultEvery
	}

	ticker := time.NewTicker(every)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			/*
				Lapsed previews first.

				A holder that stopped talking has left a draft on somebody's
				hardware with re-assertion suspended on its behalf, so the
				loop that would notice a device needing correction is exactly
				the loop that should notice one whose holder is gone.
			*/
			if _, err := r.Service.Expired(ctx); err != nil {
				r.report("could not end a lapsed preview: %v", err)
			}

			rules, err := r.Service.ReassertRules(ctx)
			if err != nil || len(rules) == 0 {
				continue
			}
			due := tracker.Due(now, rules)
			if len(due) == 0 {
				continue
			}
			// Nothing is said about a re-assert that worked. It happens every
			// minute for as long as the machine is on, and it is not news.
			if _, err := r.Service.Reconcile(ctx, due); err != nil {
				r.report("could not re-assert %s: %v", strings.Join(due, ", "), err)
			}
		}
	}
}

func (r *Reconciler) backoff(attempt int) time.Duration {
	table := r.Backoff
	if len(table) == 0 {
		table = DefaultBackoff
	}
	return table[min(attempt, len(table)-1)]
}

func (r *Reconciler) wait(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (r *Reconciler) report(format string, args ...any) {
	if r.Report != nil {
		r.Report(format, args...)
	}
}
