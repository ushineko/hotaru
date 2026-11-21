/*
Command hotaru is the CLI, and the service it talks to.

	hotaru serve                     run the service
	hotaru light list                what is there
	hotaru light set red             one colour, everything in scope
	hotaru light set kraken/fan-top=red kraken/fan-bot=blue
	hotaru light off
	hotaru light health              why nothing is happening

The CLI writes no device, ever. It asks the service to, over the socket, which
is what makes "one actor on the hardware" true by construction rather than by
discipline -- there is no code path here that could reach a device even by
mistake, because this package imports nothing that can.
*/
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/spf13/cobra"
	"github.com/ushineko/hotaru/internal/cli"
	"github.com/ushineko/hotaru/internal/config"
	"github.com/ushineko/hotaru/internal/daemon"
	"github.com/ushineko/hotaru/internal/host"
	"github.com/ushineko/hotaru/internal/pawnhelper"
	"github.com/ushineko/hotaru/internal/version"
)

func main() { os.Exit(run()) }

/*
run is main with a return value.

os.Exit does not run deferred functions, so everything that must be released
is released inside host.Run, which returns the code -- which is what this
split buys and why main does nothing else.

host knows how the process was started (spec 072 R1). As a Windows service,
hotaru's files are the machine's, prepared before anything opens them, and
what it prints goes to its log. A person's command finds an installed
service's files the same way.
*/
func run() int {
	if len(os.Args) > 1 && os.Args[1] == pawnhelper.Command {
		return helper(os.Args[2:])
	}
	return host.Run(config.App, func(ctx context.Context) int {
		cmd := root()
		switch {
		case host.Managed():
			config.SetMachine(true)
			log, err := machineLog()
			if err != nil {
				host.Report(config.App, err.Error())
				return 1
			}
			defer func() { _ = log.Close() }()
			cmd.SetOut(log)
			cmd.SetErr(log)
		case host.Installed(config.App):
			config.SetMachine(true)
		}
		return execute(ctx, cmd)
	})
}

/*
helper is hotaru's PawnIO helper on Windows (spec 083), as its service runs
it: `hotaru pawnio-helper [--allow SID]`. It is not a command of the CLI's,
because it is not the service's and shares nothing with it: no files, no
log, no socket. What goes wrong goes to the event log.
*/
func helper(args []string) int {
	allow := ""
	if len(args) == 2 && args[0] == "--allow" {
		allow = args[1]
	} else if len(args) != 0 {
		_, _ = fmt.Fprintln(os.Stderr, "usage: hotaru "+pawnhelper.Command+" [--allow SID]")
		return 2
	}
	return host.Run(pawnhelper.Service, func(ctx context.Context) int {
		if err := pawnhelper.Main(ctx, allow); err != nil {
			host.Report(pawnhelper.Service, err.Error())
			_, _ = fmt.Fprintln(os.Stderr, "hotaru: "+err.Error())
			return 1
		}
		return 0
	})
}

// machineLog prepares the machine's directories and opens the log in them.
func machineLog() (io.WriteCloser, error) {
	if err := config.PrepareMachine(); err != nil {
		return nil, err
	}
	dir, err := config.StateDir()
	if err != nil {
		return nil, err
	}
	log, err := host.OpenLog(filepath.Join(dir, "hotaru.log"), host.LogLimit, time.Now)
	if err != nil {
		return nil, err
	}
	return log, nil
}

func execute(ctx context.Context, cmd *cobra.Command) int {
	if err := cmd.ExecuteContext(ctx); err != nil {
		// A silent error has already said everything it needs to on stdout --
		// "nothing changed" after a list of devices that did not change.
		// Anything else is one line: a keypress should not produce a wall of
		// text.
		if !cli.Silent(err) {
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "hotaru: "+err.Error())
		}
		return 1
	}
	return 0
}

func root() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "hotaru",
		Short:         "RGB lighting and AIO cooler control",
		Version:       version.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.PersistentFlags().String("socket", "", "the service's socket (default: "+defaultSocket()+")")
	cmd.AddCommand(daemon.Command())
	cmd.AddCommand(cli.Commands()...)
	return cmd
}

// defaultSocket is where --socket's help says the socket is, on this system
// (spec 072 R5.2).
func defaultSocket() string {
	if runtime.GOOS == "windows" {
		return `%ProgramData%\hotaru\run\hotaru.sock with the service installed, else %LOCALAPPDATA%\hotaru\run\hotaru.sock`
	}
	return "$XDG_RUNTIME_DIR/hotaru/hotaru.sock"
}
