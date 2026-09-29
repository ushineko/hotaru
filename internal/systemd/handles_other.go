//go:build !linux

package systemd

import "context"

// held knows nothing without /proc, which is the honest answer rather than a
// guess. See Handles.Known.
func held(context.Context) Handles { return Handles{} }

// bounce cannot restart what it cannot see. The caller offers the command.
func bounce(context.Context, Facts) error { return ErrNotRestartable }
