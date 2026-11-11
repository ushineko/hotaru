//go:build !windows

package host

import (
	"context"
	"fmt"
	"os"
)

// Managed is false: no system but Windows has a service manager that must
// be answered. A systemd unit of Type=simple asks nothing of the program.
func Managed() bool { return false }

// Installed is false: on Linux the client finds the service where it always
// has, so there is nothing to look up.
func Installed(string) bool { return false }

func runManaged(string, func(context.Context) int) int { return 1 }

// Report writes msg to standard error, which systemd keeps in the journal.
func Report(name, msg string) { fmt.Fprintln(os.Stderr, name+": "+msg) }
