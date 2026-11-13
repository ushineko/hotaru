//go:build !linux

package services

import "context"

// serverPID and heldBy know nothing without /proc, which is the honest answer
// rather than a guess. See Handles.Known.
func serverPID(context.Context) (int, bool) { return 0, false }

func heldBy(int) Handles { return Handles{} }
