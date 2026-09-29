//go:build linux

package systemd

import (
	"context"
	"fmt"
	"time"
)

/*
bounceTimeout is how long a restart is given.

Generous on purpose. OpenRGB does not stop on SIGTERM -- on the machine this
was written for, systemd waited its full ten seconds and then killed it -- and
the unit may have a readiness gate of its own before the server counts as
started. A bounce that gave up at the two seconds a question gets would report
a failure while the restart it asked for was still going through.
*/
const bounceTimeout = 60 * time.Second

func bounce(ctx context.Context, f Facts) error {
	privileged, args, ok := restartPlan(f, passwordlessSudo(ctx))
	if !ok {
		return ErrNotRestartable
	}

	run := systemctlFor
	if privileged {
		run = sudoFor
	}
	if _, err := run(ctx, bounceTimeout, args...); err != nil {
		return fmt.Errorf("restart %s: %w", f.Unit, err)
	}
	return nil
}

/*
passwordlessSudo is whether this user can reach root without being asked.

`sudo -n` is the question and the answer: it does exactly what it would do for
real, minus the prompt, and fails immediately where a password would be
required. Nothing is cached from it, because sudo's own timestamp is the cache
and a lighting daemon should not be keeping a second one.

False on any error at all -- no sudo installed, no rule, an expired timestamp.
The caller then offers the command, which is what somebody in that position
needs anyway.
*/
func passwordlessSudo(ctx context.Context) bool {
	_, err := sudoFor(ctx, askTimeout, "-n", "true")
	return err == nil
}
