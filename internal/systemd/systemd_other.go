//go:build !linux

package systemd

import "context"

// look knows nothing on a machine without systemd, which is the honest answer
// rather than a guess. The remedies are then empty, and hotaru says what it can
// see instead of what someone else's machine might do about it.
func look(context.Context) Facts { return Facts{Lingering: true} }
