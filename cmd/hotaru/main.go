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
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/ushineko/hotaru/internal/cli"
	"github.com/ushineko/hotaru/internal/daemon"
	"github.com/ushineko/hotaru/internal/version"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := root().ExecuteContext(ctx); err != nil {
		// A silent error has already said everything it needs to on stdout --
		// "nothing changed" after a list of devices that did not change.
		// Anything else is one line: a keypress should not produce a wall of
		// text.
		if !cli.Silent(err) {
			fmt.Fprintln(os.Stderr, "hotaru: "+err.Error())
		}
		os.Exit(1)
	}
}

func root() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "hotaru",
		Short:         "RGB lighting and AIO cooler control",
		Version:       version.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.PersistentFlags().String("socket", "", "the service's socket (default: $XDG_RUNTIME_DIR/hotaru/hotaru.sock)")
	cmd.AddCommand(daemon.Command())
	cmd.AddCommand(cli.Commands()...)
	return cmd
}
