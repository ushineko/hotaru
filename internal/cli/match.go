package cli

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ushineko/hotaru/internal/api"
	"github.com/ushineko/hotaru/internal/config"
)

/*
matchCommand matches the pace of a device's copy of an effect to its leader's,
by eye, and writes the trim (spec 077 R3.9).

A pace that looks right depends on the device's LEDs and diffusers and on who
is looking, and is hard to put into words; matching it beside the leader is
not.
*/
func matchCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "match <device> <effect>",
		Short: "Match a device's pace of an effect to its leader's, by eye",
		Long: `Match a device's pace of an effect to its leader's, by eye.

Runs the effect on the device and on the device it follows, together and from
the start. Type f to make the device faster, s slower, r to start both again,
and press Return to keep the pace; q leaves without keeping it. The pace is
written to the rules as the device's pace for that effect, and the lights go
back to what they were showing.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := client(cmd)
			if err != nil {
				return err
			}
			with, _ := cmd.Flags().GetString("with")
			shade, _ := cmd.Flags().GetString("colour")
			asker := NewTerminal(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout())
			err = Match(cmd.Context(), client, asker, args[0], args[1], with, shade)
			if errors.Is(err, context.Canceled) {
				cmd.Println("Stopped. Nothing was written.")
				return errStopped
			}
			return err
		},
	}
	cmd.Flags().String("with", "", "the device to match against; the device's follow rule where absent")
	cmd.Flags().String("colour", "red", "the effect's colour while matching")
	return cmd
}

// paceStep is how much one f or s changes the pace: a tenth.
const paceStep = 1.1

/*
Match runs effect on the device named by device and on its leader, and lets a
person trim the device's pace until the two look alike, then writes the trim.
*/
func Match(ctx context.Context, client *api.Client, asker Asker, device, effect, with, shade string) error {
	cfg, path := existingRules(asker)
	found, err := client.Devices(ctx)
	if err != nil {
		return quiet(err)
	}
	target, ok := named(found, device)
	if !ok {
		return fmt.Errorf("no device called %q", device)
	}
	if with == "" {
		for _, rule := range cfg.RulesFor(target.Name) {
			if rule.Follow != "" {
				with = rule.Follow
			}
		}
	}
	if with == "" {
		return fmt.Errorf("%s follows nothing: say which device to match it against with --with", target.Name)
	}
	leader, ok := named(found, with)
	if !ok {
		return fmt.Errorf("no device called %q to match against", with)
	}
	if !hasMode(target, effect) || !hasMode(leader, effect) {
		return fmt.Errorf("%s and %s do not both run %q", target.Name, leader.Name, effect)
	}

	pace := 1.0
	for _, rule := range cfg.RulesFor(target.Name) {
		for name, factor := range rule.Pace {
			if strings.EqualFold(name, effect) {
				pace = factor
			}
		}
	}

	show := func() error {
		_, err := client.Apply(ctx, api.ApplyRequest{
			Colour: shade, Devices: []string{target.Name, leader.Name}, Mode: effect, Exactly: true,
			Preview: true, Restart: true, Pace: map[string]float64{target.Name: pace},
		})
		return quiet(err)
	}
	// Whatever happens, the lights go back to what was last asked of them.
	defer func() { _, _ = client.Reconcile(context.WithoutCancel(ctx)) }()

	asker.Say("%s on %s and %s, from the start. Watch them side by side.", effect, target.Name, leader.Name)
	for {
		if err := show(); err != nil {
			return err
		}
		answer, err := asker.Ask(fmt.Sprintf("  pace %.2f: f faster, s slower, r again, Return to keep, q to leave:", pace))
		if err != nil {
			return err
		}
		switch strings.ToLower(strings.TrimSpace(answer)) {
		case "f":
			pace /= paceStep
		case "s":
			pace *= paceStep
		case "r":
		case "q":
			asker.Say("Left as it was. Nothing was written.")
			return nil
		case "":
			return keepPace(ctx, client, asker, cfg, path, target.Name, effect, pace)
		default:
			asker.Say("  f, s, r, Return or q.")
		}
	}
}

// keepPace writes a device's pace for an effect to the rules.
func keepPace(ctx context.Context, client *api.Client, asker Asker, cfg *config.Config, path, device, effect string, pace float64) error {
	if path == "" {
		return errors.New("there is no rules file to write the pace to")
	}
	pace = math.Round(pace*100) / 100
	rule := ruleFor(cfg, device)
	if rule.Pace == nil {
		rule.Pace = map[string]float64{}
	}
	for name := range rule.Pace {
		if strings.EqualFold(name, effect) {
			delete(rule.Pace, name)
		}
	}
	rule.Pace[effect] = pace
	asker.Say("%s's pace for %s is %.2f.", device, effect, pace)
	return offerToWrite(ctx, client, asker, yamlFor(cfg), path)
}

// ruleFor is the rule a device's own settings go in: the last that matches
// it, or a new one for it.
func ruleFor(cfg *config.Config, device string) *config.DeviceRule {
	for i := len(cfg.Devices) - 1; i >= 0; i-- {
		if strings.Contains(strings.ToLower(device), strings.ToLower(cfg.Devices[i].Match)) {
			return &cfg.Devices[i]
		}
	}
	cfg.Devices = append(cfg.Devices, config.DeviceRule{Match: strings.ToLower(device)})
	return &cfg.Devices[len(cfg.Devices)-1]
}

// named is the device a name means, by any part of it, leaving out a name a
// device was formerly called.
func named(found []api.Device, name string) (api.Device, bool) {
	for _, d := range found {
		if d.HandedTo == "" && strings.Contains(strings.ToLower(d.Name), strings.ToLower(name)) {
			return d, true
		}
	}
	return api.Device{}, false
}

// hasMode is whether a device runs a mode of that name.
func hasMode(d api.Device, mode string) bool {
	for _, m := range d.Modes {
		if strings.EqualFold(m, mode) {
			return true
		}
	}
	return false
}

/*
lengthCommand gives a zone whose length its device cannot read a length, for
as long as the service runs: to try one before writing it to the rules'
lengths, which light map does for you (spec 077 R4.3).
*/
func lengthCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "length <device>/<zone> <lights>",
		Short: "Try a length for a zone its device cannot count",
		Long: `Try a length for a zone its device cannot count, such as an addressable
header's strip or fans, until the service stops. Write it to the rules'
lengths to keep it; ` + "`hotaru light map`" + ` finds and writes it for you.

	hotaru light length maximus/"Addressable RGB Header 3" 20`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := client(cmd)
			if err != nil {
				return err
			}
			device, zone, ok := strings.Cut(args[0], "/")
			if !ok || zone == "" {
				return fmt.Errorf("%q: a device and a zone, device/zone", args[0])
			}
			var lights int
			if _, err := fmt.Sscan(args[1], &lights); err != nil || lights < 0 {
				return fmt.Errorf("%q: a number of lights", args[1])
			}
			found, err := client.Devices(cmd.Context())
			if err != nil {
				return quiet(err)
			}
			target, ok := named(found, device)
			if !ok {
				return fmt.Errorf("no device called %q", device)
			}
			if err := client.Length(cmd.Context(), target.Name, zone, lights); err != nil {
				return quiet(err)
			}
			cmd.Printf("%s's %s has %d lights until the service stops.\n", target.Name, zone, lights)
			return nil
		},
	}
	return cmd
}
