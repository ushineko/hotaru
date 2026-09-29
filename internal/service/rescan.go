package service

import (
	"context"
	"errors"
	"fmt"
	"time"
)

/*
NotOurs is an OpenRGB server this user is not in a position to restart.

A system unit needs root, and a lighting daemon asking for a password is not a
thing this program does; a server somebody started by hand has no unit to
restart at all. Neither is a failure, so this carries the command a person
would type instead. See spec 058 R7.
*/
type NotOurs struct{ Command string }

func (e *NotOurs) Error() string {
	if e.Command == "" {
		return "this OpenRGB server is not one hotaru can restart"
	}
	return "this OpenRGB server is not one hotaru can restart. Bounce it yourself with `" + e.Command + "`"
}

/*
Rescan is what a bounce did.

Devices is what the server found the second time, which is the number the whole
operation exists to change: a server that has re-detected its hardware is one
holding live descriptors for it.
*/
type Rescan struct {
	Devices int
	Applied int
	Missing []string
}

/*
rescanTimeout is how long to wait for the server to start answering at all.

Generous. OpenRGB takes its SIGTERM timeout to die -- ten seconds, measured --
the unit may have a readiness gate of its own, and the service redials on a
backoff after that.
*/
const rescanTimeout = 90 * time.Second

/*
rescanSettle is how long the devices then get to turn up.

A server answers before it has finished finding hardware. Asked immediately
after a restart it has been seen reporting none of six, and reporting it
without an error -- so "the server replied" is not "the server is ready", and a
rescan that stopped at the first reply announced that it had restored nothing
while the restore it had triggered was still running.

Bounded rather than open-ended, because a device that was unplugged and left
unplugged is never going to turn up, and the answer then is to say which ones
are missing rather than to wait on them forever.
*/
const rescanSettle = 25 * time.Second

// rescanPoll is how often to look while waiting.
const rescanPoll = 250 * time.Millisecond

/*
Rescan restarts the OpenRGB server and puts the lights back.

The only operation in hotaru that changes the machine rather than reading it,
and it happens because somebody asked for it by name. A device that has been
unplugged and plugged back in cannot be recovered any other way: the server
detects hardware once, at startup, so its picture of the machine is fixed until
it starts again.

Deliberately not automatic. A restart drops every other device's lighting for
as long as it takes, and choosing that moment belongs to whoever is looking at
the lights.
*/
func (s *Service) Rescan(ctx context.Context) (Rescan, error) {
	env := s.environment()
	if env == nil {
		return Rescan{}, &NotOurs{}
	}
	if err := env.Bounce(ctx); err != nil {
		return Rescan{}, err
	}

	// The connection that comes back is a different one. The daemon redials on
	// its own -- that is what makes a bounce survivable at all -- so this waits
	// for the result rather than reaching for the socket itself.
	if err := s.waitForServer(ctx); err != nil {
		return Rescan{}, err
	}
	return s.putBack(ctx)
}

/*
putBack restores the lights, and keeps restoring them until everything
remembered has turned up.

The same shape the reconciler uses at boot, and for the same reason: hardware
enumerates over several seconds, so one attempt reaches whatever happened to be
present at the moment it ran. Reporting that as the result is how a rescan came
to announce nothing restored while the devices were still arriving.

What it has when the settle runs out is what it reports. A device that was
unplugged and left that way is missing rather than late, and naming it is more
use than waiting for it.
*/
func (s *Service) putBack(ctx context.Context) (Rescan, error) {
	ctx, cancel := context.WithTimeout(ctx, rescanSettle)
	defer cancel()

	ticker := time.NewTicker(rescanPoll)
	defer ticker.Stop()

	var best Rescan
	for {
		out, err := s.putBackOnce(ctx)
		switch {
		case err != nil:
			// The server can go away again mid-settle. Not this operation's
			// to report: the daemon is already redialling.
		case out.Devices >= best.Devices && out.Applied >= best.Applied:
			best = out
			if len(out.Missing) == 0 {
				return best, nil
			}
		}

		select {
		case <-ctx.Done():
			return best, nil
		case <-ticker.C:
		}
	}
}

// putBackOnce is one look at the server that came back, and one go at putting
// the lights on it.
func (s *Service) putBackOnce(ctx context.Context) (Rescan, error) {
	_, client, addr := s.current()
	if client == nil {
		return Rescan{}, unreachable(addr)
	}
	found, err := client.Devices(ctx)
	if err != nil {
		return Rescan{}, fmt.Errorf("ask the server that came back what it has: %w", err)
	}

	restored, err := s.Reconcile(ctx, nil)
	if err != nil {
		return Rescan{Devices: len(found)}, err
	}
	return Rescan{Devices: len(found), Applied: restored.Applied, Missing: restored.Missing}, nil
}

/*
waitForServer blocks until there is a working connection again.

Working, not merely present: the service can be holding a socket to the server
that is on its way out, and a device listing is the cheapest question that only
a live one answers.
*/
func (s *Service) waitForServer(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, rescanTimeout)
	defer cancel()

	ticker := time.NewTicker(rescanPoll)
	defer ticker.Stop()

	for {
		_, client, _ := s.current()
		if client != nil && !client.Gone() {
			if _, err := client.Devices(ctx); err == nil {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return errors.New("the OpenRGB server was restarted and did not come back")
		case <-ticker.C:
		}
	}
}

func (s *Service) environment() Environment {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.env
}
