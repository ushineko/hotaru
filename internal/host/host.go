/*
Package host is how hotaru itself was started, and the only code that knows.

On Linux the service is a systemd unit of `Type=simple`, which asks nothing of
the program, or a process in a terminal; both stop on a signal. On Windows it
is either a process in a terminal or a service of the Windows service manager,
which has to be answered: told that the service runs, and obeyed when it says
stop. Everything else in hotaru sees a context that ends when it should stop
(spec 072 R1).

Installing the service is not here. Packaging does that: the AUR's unit on
Arch, fynstall on Windows.
*/
package host

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

/*
Run runs main and returns its exit code.

Under the Windows service manager, name is the service's name, and main's
context ends when the manager asks the service to stop or the computer shuts
down. Anywhere else the context ends on Ctrl+C or SIGTERM.
*/
func Run(name string, main func(ctx context.Context) int) int {
	if Managed() {
		return runManaged(name, main)
	}
	return runHere(main)
}

/*
runHere is a process in a terminal, or a systemd unit.

The signal handler is released before Run returns, rather than by a defer in
main, because main calls os.Exit, which runs no deferred function.
*/
func runHere(main func(ctx context.Context) int) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return main(ctx)
}
