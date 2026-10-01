/*
Package cli is every command that talks to a running hotaru.

It reaches no device package: no OpenRGB, no service, no devices. That is
asserted by a test rather than left to discipline, because "the service is the
only writer" is worth more as a fact about the import graph than as a sentence
in a document -- there is no code path here that could reach a device even by
mistake.

What it does do besides ask the service is read the machine, where the service
is sandboxed out of reading it: the desktop's own shortcut file, and whether
the OpenRGB server still has a connection to the hardware it thinks it has.
Neither writes a light.
*/
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/ushineko/hotaru/internal/api"
	"github.com/ushineko/hotaru/internal/config"
	"github.com/ushineko/hotaru/internal/stale"
)

// Commands are the client-side commands, for a root command to add.
func Commands() []*cobra.Command {
	return []*cobra.Command{
		lightCommand(), sceneCommand(), keysCommand(), imageCommand(),
		previewCommand(), statusCommand(), coolingCommand(), readingsCommand(),
		screenCommand(), dashboardCommand(),
		reconcileCommand(), reloadCommand(),
	}
}

func lightCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "light",
		Short: "Lighting",
	}
	cmd.AddCommand(listCommand(), setCommand(), offCommand(), healthCommand(), probeCommand(), mapCommand(),
		rescanCommand(), releaseCommand())
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
			_, _ = fmt.Fprintln(w, "DEVICE\tLEDS\tACTIVE\tSCOPE\tMODES")
			var previewing bool
			var handed []string
			for _, device := range list {
				scope := "-"
				switch {
				case device.HandedTo != "":
					// Listed, so a person can see why it is not written to.
					scope = "handed"
					handed = append(handed, fmt.Sprintf("%s is drawn by hotaru as %s; OpenRGB is not written to.",
						device.Name, device.HandedTo))
				case device.Preview != nil:
					// A device whose re-assertion is suspended looks exactly
					// like one that is simply behaving, so it says so.
					scope, previewing = "preview", true
				case device.InScope:
					scope = "yes"
				}
				_, _ = fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%s\n",
					device.Name, device.LEDs, active(device), scope, strings.Join(device.Modes, ", "))
			}
			if err := w.Flush(); err != nil {
				return fmt.Errorf("write the listing: %w", err)
			}
			if previewing {
				cmd.Println("\nSome devices are showing a draft. `hotaru preview` says who is holding it.")
			}
			if len(handed) > 0 {
				cmd.Println()
				for _, line := range handed {
					cmd.Println(line)
				}
			}
			return nil
		},
	}
	withJSON(cmd)
	return cmd
}

/*
active is what a device is doing, for the listing's column.

A device hotaru draws says so: its frame rate while an effect moves, and
"holding" while it does not, because a canvas that was sent one frame and
shows it is the ordinary state and costs nothing (spec 060 R5.2).
*/
func active(device api.Device) string {
	if device.Canvas == nil {
		return device.ActiveMode
	}
	return drawing(device.ActiveMode, *device.Canvas)
}

// drawing is one canvas's state in a few words: "Rainbow Wave, 17.9 fps" or
// "Static, holding".
func drawing(effect string, c api.Canvas) string {
	if effect == "" {
		effect = "nothing drawn"
	}
	if c.Drawing {
		return fmt.Sprintf("%s, %.1f fps", effect, c.Rate)
	}
	return effect + ", holding"
}

func releaseCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "release <device>",
		Short: "Hand a device hotaru draws back to its own lighting",
		Long: `Hand a device hotaru draws back to its firmware's own lighting.

A device hotaru draws holds the last frame it was sent, and stopping the
service leaves that frame showing. This gives the lighting back. On the first
such device that reboots the board: it drops off the bus for a moment and
comes back on its onboard effect. hotaru attaches it again and sends it
nothing until a scene asks.

	hotaru light release apex`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := client(cmd)
			if err != nil {
				return err
			}
			out, err := client.Release(cmd.Context(), args[0])
			if err != nil {
				return quiet(err)
			}
			if asJSON(cmd) {
				return emit(cmd, out)
			}
			cmd.Println(out.Detail)
			return nil
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
			req.Preview, _ = cmd.Flags().GetBool("preview")
			return apply(cmd, req)
		},
	}
	cmd.Flags().StringSlice("devices", nil, "only these devices, by name")
	withPreview(cmd)
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
			preview, _ := cmd.Flags().GetBool("preview")
			return apply(cmd, api.ApplyRequest{Off: true, Devices: names, Preview: preview})
		},
	}
	cmd.Flags().StringSlice("devices", nil, "only these devices, by name")
	withPreview(cmd)
	withJSON(cmd)
	return cmd
}

/*
withPreview adds the flag for a write the machine should not remember.

Setting a colour to see what lights up is not the same as saying the machine
should look like this. Without the distinction, trying three colours in a row
to find out which fan is which leaves the last one as what the machine restores
at boot -- and the reconciler puts the previous one back while somebody is
still looking at the case, which reads as hardware misbehaving. The wizard has
always written this way; the command line could not.
*/
func withPreview(cmd *cobra.Command) {
	cmd.Flags().Bool("preview", false, "write the colour without remembering it")
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
		for _, problem := range result.Problems {
			// Said first, and to stderr: an assignment that did not apply is
			// the part of the answer somebody needs to act on.
			cmd.PrintErrf("%s: %s\n", result.Device, problem)
		}
		if result.Unconfirmed != "" {
			cmd.PrintErrf("%s: unconfirmed — %s\n", result.Device, result.Unconfirmed)
		}
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
		case result.Superseded:
			cmd.Printf("%s: superseded by a later request\n", result.Device)
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

/*
withStale adds the one verdict the service cannot reach.

Asked here rather than there because reading the server's descriptors has to
happen in the server's own mount namespace, and the service is sandboxed away
from it while this command is an ordinary process of the user's. The rest of
health is the service's answer, untouched. See internal/stale and spec 058.

Anything that goes wrong is silence. This is an extra sentence on a report that
was already complete, and a health command that failed because a diagnosis of a
diagnosis did not work would be worse than one that says what it knows.
*/
func withStale(ctx context.Context, client *api.Client, health api.Health) api.Health {
	if health.State != stale.Healthy {
		return health
	}
	found, err := client.Devices(ctx)
	if err != nil {
		return health
	}
	names := make([]string, 0, len(found))
	for _, device := range found {
		names = append(names, device.Name)
	}

	verdict := (&stale.Checker{}).Check(ctx, names)
	health.State, health.Detail, health.Remedies = verdict.Apply(health.State, health.Detail, health.Remedies)
	return health
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
			health = withStale(cmd.Context(), client, health)

			if asJSON(cmd) {
				if err := emit(cmd, health); err != nil {
					return err
				}
			} else {
				cmd.Printf("%s: %s\n", health.State, health.Detail)
				for _, remedy := range health.Remedies {
					cmd.Printf("  %s\n", remedy)
				}
				if health.Protocol > 0 {
					cmd.Printf("  %s, protocol %d, %d devices, %d in scope\n",
						health.Address, health.Protocol, health.Devices, health.InScope)
				}
				for _, c := range health.Canvases {
					cmd.Printf("  drawing on %s: %s\n", c.Device, drawing(c.Effect, c))
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
	// errStopped is somebody pressing Ctrl-C, which is a decision rather than
	// a fault and has already been acknowledged on screen.
	errStopped = &silent{"stopped"}
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
	if err := enc.Encode(body); err != nil {
		return fmt.Errorf("write the answer: %w", err)
	}
	return nil
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
