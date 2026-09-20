package service

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/ushineko/hotaru/internal/devices"
	"github.com/ushineko/hotaru/internal/state"
)

/*
Recorder is where desired state is kept. The service records what a user asked
for; something else decides where that lives.
*/
type Recorder interface {
	Snapshot() state.Snapshot
	Record(name string, device state.Device) error
	Forget(name string) error
}

// SetRecorder gives the service somewhere to remember what was asked for.
func (s *Service) SetRecorder(r Recorder) {
	s.mu.Lock()
	s.recorder = r
	s.mu.Unlock()
}

// Desired is what the lights should currently be showing.
func (s *Service) Desired() state.Snapshot {
	s.mu.RLock()
	recorder := s.recorder
	s.mu.RUnlock()
	if recorder == nil {
		return state.Snapshot{}
	}
	return recorder.Snapshot()
}

/*
Restore is the outcome of reconciling toward desired state.

Missing is the part that matters. A restore that reached fewer devices than
were recorded is **unfinished**, not successful: OpenRGB enumerates once at
server start, and a cold boot has been seen finding two devices out of six.
Reporting that as a success is how a machine ends up half-lit with everything
claiming to be fine.
*/
type Restore struct {
	Results []Result
	Applied int
	Missing []string
}

// Complete reports whether every remembered device was reached.
func (r Restore) Complete() bool { return len(r.Missing) == 0 }

/*
Reconcile puts the lights back to what was last asked for.

Not an apply: nothing here is a new user choice. Desired state is not rewritten,
timestamps do not move, and a caller cannot tell a reconcile from the original
request by looking at what was remembered — which is what keeps a re-assert from
being mistaken for an instruction.

With nothing recorded it writes nothing at all. A fresh install has nothing to
restore, so it touches no device, and that is the inert rule in one sentence
rather than a special case.
*/
func (s *Service) Reconcile(ctx context.Context, only []string) (Restore, error) {
	desired := s.Desired()
	if desired.Empty() {
		return Restore{}, nil
	}

	_, client, addr := s.current()
	if client == nil {
		return Restore{}, unreachable(addr)
	}
	found, err := client.Devices(ctx)
	if err != nil {
		return Restore{}, err
	}

	present := make(map[string]*devices.Device, len(found))
	for i := range found {
		present[found[i].Name] = &found[i]
	}

	var restore Restore
	for _, name := range sorted(desired.Names()) {
		if len(only) > 0 && !named(only, name) {
			continue
		}
		device, here := present[name]
		if !here {
			// Recorded, and not on the server. The device may enumerate later,
			// which is why this is reported rather than forgotten.
			restore.Missing = append(restore.Missing, name)
			continue
		}

		want := desired.Devices[name]
		result := s.through(ctx, device.Name, func(ctx context.Context) Result {
			return s.writeFrame(ctx, client, device, want.Frame(name), false, "")
		})
		if result.Applied {
			restore.Applied++
		}
		restore.Results = append(restore.Results, result)
	}
	return restore, nil
}

/*
Reasserter re-sends colours to devices that do not hold them.

Some hardware forgets: a wireless mouse restores its onboard colour when it
wakes, and OpenRGB sets a volatile effect it has no way to make stick. The only
remedy is to send it again, on an interval, forever.

Scoped to devices whose rule asks for it. Re-asserting everything would write
constantly to hardware that has no such problem, and a write nobody needs is
still a write that can collide with something else.
*/
type Reasserter struct {
	mu   sync.Mutex
	last map[string]time.Time
}

// NewReasserter tracks when each device was last re-sent.
func NewReasserter() *Reasserter { return &Reasserter{last: map[string]time.Time{}} }

/*
Due is the devices whose re-assert interval has elapsed.

A device is due the first time it is seen with a rule: hotaru has no idea what
happened while it was not running, and the cheapest way to find out is to send
what should be there.
*/
func (r *Reasserter) Due(now time.Time, rules map[string]time.Duration) []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	var due []string
	for name, interval := range rules {
		if interval <= 0 {
			continue
		}
		last, seen := r.last[name]
		if !seen || now.Sub(last) >= interval {
			due = append(due, name)
			r.last[name] = now
		}
	}
	sort.Strings(due)
	return due
}

// Forget drops a device, so it is due again when it returns.
func (r *Reasserter) Forget(name string) {
	r.mu.Lock()
	delete(r.last, name)
	r.mu.Unlock()
}

/*
ReassertRules is the interval per device, for devices that have one.

Read from the configuration against the devices actually present, so a rule for
hardware this machine does not have costs nothing.
*/
func (s *Service) ReassertRules(ctx context.Context) (map[string]time.Duration, error) {
	views, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string]time.Duration{}
	for _, view := range views {
		if view.InScope && view.Rule.Reassert > 0 {
			out[view.Device.Name] = view.Rule.Reassert
		}
	}
	return out, nil
}

func sorted(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}
