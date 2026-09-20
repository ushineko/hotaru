package cli

import (
	"fmt"
	"strings"
	"text/tabwriter"

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
			if status.Connected {
				cmd.Printf("  OpenRGB   %s, protocol %d\n", status.Address, status.Protocol)
			} else {
				cmd.Printf("  OpenRGB   not connected (%s)\n", status.Address)
			}
			if status.RulesFile != "" {
				cmd.Printf("  rules     %s\n", status.RulesFile)
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

func probeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "probe",
		Short: "Find out what each device can actually do",
		Long: `Find out what each device can actually do.

Sets modes to see which ones a device honours, and puts everything back
afterwards. What it cannot tell you is whether the lights physically changed:
a board that accepts Static and lights only its onboard LED reports Static
either way. For that, look at the machine.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := client(cmd)
			if err != nil {
				return err
			}
			names, _ := cmd.Flags().GetStringSlice("devices")
			findings, err := client.Probe(cmd.Context(), api.ProbeRequest{Devices: names})
			if err != nil {
				return quiet(err)
			}
			if asJSON(cmd) {
				return emit(cmd, findings)
			}

			for i, found := range findings {
				if i > 0 {
					cmd.Println()
				}
				cmd.Printf("%s\n", found.Device)
				if found.Error != "" {
					cmd.PrintErrf("  %s\n", found.Error)
				}

				w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
				for _, mode := range found.Modes {
					note := "took"
					if !mode.Took {
						note = "accepted, and did not take"
					}
					per := ""
					if mode.PerLED {
						per = "per-LED"
					}
					fmt.Fprintf(w, "  %s\t%s\t%s\n", mode.Name, note, per)
				}
				for _, zone := range found.Zones {
					fmt.Fprintf(w, "  zone %s\t%d LEDs\t(%d-%d)\n",
						zone.Name, zone.Count, zone.First, zone.First+zone.Count-1)
				}
				if err := w.Flush(); err != nil {
					return err
				}

				if found.Suggested != "" {
					cmd.Println("  suggested rule:")
					for _, line := range strings.Split(found.Suggested, "\n") {
						cmd.Printf("    %s\n", line)
					}
				}
			}
			if len(findings) == 0 {
				cmd.Println("No devices to probe. `hotaru light health` says why.")
			}
			return nil
		},
	}
	cmd.Flags().StringSlice("devices", nil, "only these devices, by name")
	withJSON(cmd)
	return cmd
}
