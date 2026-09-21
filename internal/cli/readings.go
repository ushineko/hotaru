package cli

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

/*
readingsCommand prints what the machine will say about itself.

The reason it exists: a dashboard slot can be filled with any of these, and
somebody choosing one wants to know whether their machine actually reports it
before they put it on a panel and wonder why it draws a dash. A graphics card
with no utilisation reading looks exactly like a working one until something
asks.
*/
func readingsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "readings",
		Short: "What the machine can put on the screen",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := client(cmd)
			if err != nil {
				return err
			}
			taken, err := client.Readings(cmd.Context())
			if err != nil {
				return quiet(err)
			}

			out := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(out, "SOURCE\tNAME\tVALUE")
			for _, one := range taken {
				_, _ = fmt.Fprintf(out, "%s\t%s\t%s\n",
					one.Source, one.Label, strings.TrimSpace(one.Text+" "+one.Unit))
			}
			if err := out.Flush(); err != nil {
				return fmt.Errorf("write the listing: %w", err)
			}
			return nil
		},
	}
}
