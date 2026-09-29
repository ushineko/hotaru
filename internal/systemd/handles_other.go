//go:build !linux

package systemd

import "context"

// serverPID and heldBy know nothing without /proc, which is the honest answer
// rather than a guess. See Handles.Known.
func serverPID(context.Context) (int, bool) { return 0, false }

func heldBy(int) Handles { return Handles{} }

// bounce cannot restart what it cannot see. The caller offers the command.
func bounce(context.Context, Facts) error { return ErrNotRestartable }
