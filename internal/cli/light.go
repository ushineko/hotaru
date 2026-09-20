/*
Package cli is every command that talks to a running hotaru.

It imports the API client and nothing else of hotaru's: no device packages, no
OpenRGB, no service. That is asserted by a test rather than left to discipline,
because "the service is the only writer" is worth more as a fact about the
import graph than as a sentence in a document -- there is no code path here that
could reach a device even by mistake.
*/
package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/ushineko/hotaru/internal/api"
	"github.com/ushineko/hotaru/internal/config"
)

// Commands are the client-side commands, for a root command to add.
func Commands() []*cobra.Command {
	return []*cobra.Command{lightCommand(), statusCommand(), reconcileCommand(), reloadCommand()}
}

func lightCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "light",
		Short: "Lighting",
	}
	cmd.AddCommand(listCommand(), setCommand(), offCommand(), healthCommand(), probeCommand())
	return cmd
}

func listCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "What the service can see",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := client(cmd)
			if err != nil {
				return err
			}
			list, err := client.Devices(cmd.Context())
			if err != nil {
				return quiet(err)
			}
			if asJSON(cmd) {
				return emit(cmd, list)
			}

			if len(list) == 0 {
				cmd.Println("No devices. `hotaru light health` says why.")
				return nil
			}

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "DEVICE\tLEDS\tACTIVE\tSCOPE\tMODES")
			for _, device := range list {
				scope := "-"
				if device.InScope {
					scope = "yes"
				}
				fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%s\n",
					device.Name, device.LEDs, device.ActiveMode, scope, strings.Join(device.Modes, ", "))
			}
			return w.Flush()
		},
	}
	withJSON(cmd)
	return cmd
}

func setCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set <colour> | <target>=<colour>...",
		Short: "Set a colour",
		Long: `Set a colour.

	hotaru light set red                       every device in scope
	hotaru light set kraken/fan-top=red        one part of one device
	hotaru light set blue kraken/fan-top=red   everything blue, except the top fan

A target is a device, a zone, an LED range, or a segment named in the rules
file: kraken, kraken/ring, kraken/ring[0:11], kraken/fan-top.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			req := api.ApplyRequest{}
			for _, arg := range args {
				target, colour, isAssignment := strings.Cut(arg, "=")
				switch {
				case isAssignment:
					req.Assignments = append(req.Assignments, api.Assignment{Target: target, Colour: colour})
				case req.Colour == "":
					req.Colour = arg
				default:
					return fmt.Errorf("%q: one colour for everything, then target=colour for the exceptions", arg)
				}
			}
			names, _ := cmd.Flags().GetStringSlice("devices")
			req.Devices = names
			return apply(cmd, req)
		},
	}
	cmd.Flags().StringSlice("devices", nil, "only these devices, by name")
	withJSON(cmd)
	return cmd
}

func offCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "off",
		Short: "Turn lighting off",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			names, _ := cmd.Flags().GetStringSlice("devices")
			return apply(cmd, api.ApplyRequest{Off: true, Devices: names})
		},
	}
	cmd.Flags().StringSlice("devices", nil, "only these devices, by name")
	withJSON(cmd)
	return cmd
}

/*
apply sends the request and reports one line per device.

Three outcomes and three shapes, because they are not the same thing: a device
that changed says which mode did it, a device that could not says why, and a
device that errored says what went wrong. Exit is non-zero when nothing
changed — a scene that lit nothing has not worked, whatever the server said.
*/
func apply(cmd *cobra.Command, req api.ApplyRequest) error {
	client, err := client(cmd)
	if err != nil {
		return err
	}
	out, err := client.Apply(cmd.Context(), req)
	if err != nil {
		return quiet(err)
	}
	if asJSON(cmd) {
		if err := emit(cmd, out); err != nil {
			return err
		}
		if out.Changed == 0 {
			return errNothingChanged
		}
		return nil
	}

	for _, result := range out.Results {
		switch {
		case result.Applied:
			cmd.Printf("%s: %s\n", result.Device, result.Mode)
			if len(result.Attempts) > 1 {
				// The interesting case: something was accepted and did not
				// take. Saying so is how a user learns their hardware.
				for _, attempt := range result.Attempts[:len(result.Attempts)-1] {
					cmd.Printf("  tried %s first; the device stayed in %s\n", attempt.Mode, attempt.Active)
				}
			}
		case result.Error != "":
			cmd.PrintErrf("%s: %s\n", result.Device, result.Error)
		default:
			cmd.Printf("%s: skipped, %s\n", result.Device, result.Skipped)
		}
	}
	if out.Changed == 0 {
		return errNothingChanged
	}
	return nil
}

func healthCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "health",
		Short: "Why nothing is happening",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := client(cmd)
			if err != nil {
				return err
			}
			health, err := client.Health(cmd.Context())
			if err != nil {
				return quiet(err)
			}
			if asJSON(cmd) {
				if err := emit(cmd, health); err != nil {
					return err
				}
			} else {
				cmd.Printf("%s: %s\n", health.State, health.Detail)
				if health.Protocol > 0 {
					cmd.Printf("  %s, protocol %d, %d devices, %d in scope\n",
						health.Address, health.Protocol, health.Devices, health.InScope)
				}
			}
			if health.State != "healthy" {
				return errUnhealthy
			}
			return nil
		},
	}
	withJSON(cmd)
	return cmd
}

// Sentinels for the two "it worked but nothing happened" exits. Both print
// nothing extra: the lines above them already said it.
var (
	errNothingChanged = &silent{"nothing changed"}
	errUnhealthy      = &silent{"not healthy"}
)

type silent struct{ why string }

func (e *silent) Error() string { return e.why }

func withJSON(cmd *cobra.Command) {
	cmd.Flags().Bool("json", false, "machine-readable output")
}

func asJSON(cmd *cobra.Command) bool {
	on, _ := cmd.Flags().GetBool("json")
	return on
}

func emit(cmd *cobra.Command, body any) error {
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(body)
}

// client is the connection to the service every command here uses.
func client(cmd *cobra.Command) (*api.Client, error) {
	socket, _ := cmd.Flags().GetString("socket")
	if socket == "" {
		got, err := config.SocketPath()
		if err != nil {
			return nil, err
		}
		socket = got
	}
	return api.NewClient(socket), nil
}

// quiet turns a NotRunning into the one line it deserves: a shell that cannot
// reach the service should say that, not print a transport error about a path
// the user never typed.
func quiet(err error) error {
	var down *api.NotRunning
	if errors.As(err, &down) {
		return errors.New(down.Error())
	}
	return err
}

// Silent reports whether an error has already said everything it needs to on
// stdout, so a caller exits non-zero without printing it again.
func Silent(err error) bool {
	var s *silent
	return errors.As(err, &s)
}
