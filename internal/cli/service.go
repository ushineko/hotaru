package cli

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/ushineko/hotaru/internal/api"
)

func statusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "What the service is, and what it remembers",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := client(cmd)
			if err != nil {
				return err
			}
			status, err := client.Status(cmd.Context())
			if err != nil {
				return quiet(err)
			}
			if asJSON(cmd) {
				return emit(cmd, status)
			}

			cmd.Printf("hotaru %s\n", status.Version)
			if status.RulesFile != "" {
				cmd.Printf("  rules     %s\n", status.RulesFile)
			}
			// What the window's System card says, so the two agree (#184).
			if status.Scene != "" {
				cmd.Printf("  scene     %s, the last applied\n", status.Scene)
			}
			if status.Showing != "" {
				cmd.Printf("  screen    %s\n", status.Showing)
			}
			if len(status.Remembered) == 0 {
				// The inert state, said plainly: nothing has been asked for,
				// so nothing will be put back.
				cmd.Println("  remembers nothing yet, so it restores nothing")
			} else {
				cmd.Printf("  remembers %s\n", strings.Join(status.Remembered, ", "))
			}
			return nil
		},
	}
	withJSON(cmd)
	return cmd
}

func reconcileCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reconcile",
		Short: "Put the lights back to what was last asked for",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := client(cmd)
			if err != nil {
				return err
			}
			out, err := client.Reconcile(cmd.Context())
			if err != nil {
				return quiet(err)
			}
			if asJSON(cmd) {
				return emit(cmd, out)
			}

			switch {
			case out.Applied == 0 && out.Complete:
				cmd.Println("nothing to put back")
			case out.Complete:
				cmd.Printf("restored %d devices\n", out.Applied)
			default:
				cmd.Printf("restored %d devices; still waiting for %s\n",
					out.Applied, strings.Join(out.Missing, ", "))
			}
			for _, result := range out.Results {
				for _, problem := range result.Problems {
					cmd.PrintErrf("  %s\n", problem)
				}
			}
			return nil
		},
	}
	withJSON(cmd)
	return cmd
}

func reloadCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reload",
		Short: "Re-read the rules file",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := client(cmd)
			if err != nil {
				return err
			}
			out, err := client.Reload(cmd.Context())
			if err != nil {
				return quiet(err)
			}
			if asJSON(cmd) {
				return emit(cmd, out)
			}

			if out.RulesFile == "" {
				cmd.Println("no rules file; every device present is driven")
				return nil
			}
			cmd.Printf("read %s\n", out.RulesFile)
			for _, problem := range out.Problems {
				cmd.PrintErrf("  %s\n", problem)
			}
			return nil
		},
	}
	withJSON(cmd)
	return cmd
}

/*
display says what screen the cooler has, in one line.

The same sentence the window shows, because "which display was found" is a
question asked at the terminal as often as in the interface -- usually while
working out why nothing is being drawn.
*/
func display(cooling api.Cooling) string {
	switch {
	case cooling.Screen == "":
		return "none"
	case cooling.ScreenDetail != "":
		return cooling.Screen + " (" + cooling.ScreenDetail + ")"
	}
	return cooling.Screen
}

/*
coolingCommand reports what the liquid cooler says about itself.

A command of its own rather than a line in `status`, because `status` answers
"what is this service doing" and this answers "what is my machine doing" --
and because a number somebody wants to watch should be cheap to watch.
*/
func coolingCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cooling",
		Short: "What the liquid cooler reports",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := client(cmd)
			if err != nil {
				return err
			}
			cooling, err := client.Cooling(cmd.Context())
			if err != nil {
				return quiet(err)
			}
			if asJSON(cmd) {
				return emit(cmd, cooling)
			}

			// No cooler is an ordinary answer, not a failure: a machine
			// without one still has working lighting, and saying so plainly
			// is better than an error somebody has to interpret.
			if cooling.Absent {
				if cooling.Device != "" {
					cmd.Printf("%s: %s\n", cooling.Device, cooling.Detail)
					return nil
				}
				cmd.Println("No liquid cooler on this machine.")
				return nil
			}

			cmd.Printf("%s\n", cooling.Device)
			cmd.Printf("  coolant   %.1f C\n", cooling.Coolant)
			cmd.Printf("  pump      %d rpm (%d%%)\n", cooling.PumpRPM, cooling.PumpDuty)
			cmd.Printf("  fan       %d rpm (%d%%)\n", cooling.FanRPM, cooling.FanDuty)
			cmd.Printf("  display   %s\n", display(cooling))
			return nil
		},
	}
	withJSON(cmd)
	return cmd
}
