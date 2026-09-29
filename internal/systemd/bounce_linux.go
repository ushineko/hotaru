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
	if !f.Restartable() {
		return ErrNotRestartable
	}
	if _, err := systemctlFor(ctx, bounceTimeout, "--user", "restart", f.Unit); err != nil {
		return fmt.Errorf("restart %s: %w", f.Unit, err)
	}
	return nil
}
