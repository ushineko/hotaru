package cli

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/ushineko/hotaru/internal/api"
	"github.com/ushineko/hotaru/internal/dashboard"
	yaml "go.yaml.in/yaml/v3"
)

/*
The cooler's screen, as the terminal offers it.

Editing a dashboard is a form, and a form is the window's job. What the
terminal has is the rest of it: seeing what there is, reading one out,
choosing which is drawn, removing one, and writing a frame to a file so
somebody on a machine with no window can still look at it before it goes on
the panel.
*/
func dashboardCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dashboard",
		Short: "What the cooler's screen draws",
	}
	cmd.AddCommand(
		dashboardListCommand(), dashboardShowCommand(), dashboardSaveCommand(),
		dashboardUseCommand(), dashboardSceneCommand(),
		dashboardPreviewCommand(), dashboardDeleteCommand(),
	)
	return cmd
}

func dashboardListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "What the screen can be asked to draw",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := client(cmd)
			if err != nil {
				return err
			}
			got, err := client.Dashboards(cmd.Context())
			if err != nil {
				return quiet(err)
			}

			out := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(out, "NAME\tARRANGEMENT\tBACKGROUND\t")
			for _, one := range got.Dashboards {
				name := one.Name
				if one.Name == got.Active {
					name += " *"
				}
				if one.Shipped {
					name += "  (shipped)"
				}
				_, _ = fmt.Fprintf(out, "%s\t%s\t%s\t\n", name, arrangementOf(one), backgroundOf(one))
			}
			if err := out.Flush(); err != nil {
				return fmt.Errorf("write the listing: %w", err)
			}
			cmd.Println("\n* is the one on the screen.")
			return nil
		},
	}
}

func arrangementOf(one api.Dashboard) string {
	if one.Arrangement == "" {
		return "ring"
	}
	return one.Arrangement
}

func backgroundOf(one api.Dashboard) string {
	switch one.Background.Kind {
	case "plain":
		return "plain"
	case "picture":
		return "picture: " + one.Background.Picture
	}
	return "starfield"
}

func dashboardShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show <name>",
		Short: "What one dashboard says",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := client(cmd)
			if err != nil {
				return err
			}
			got, err := client.Dashboards(cmd.Context())
			if err != nil {
				return quiet(err)
			}
			for _, one := range got.Dashboards {
				if !strings.EqualFold(one.Name, args[0]) {
					continue
				}
				describe(cmd, one)
				return nil
			}
			return fmt.Errorf("no dashboard called %q", args[0])
		},
	}
}

func describe(cmd *cobra.Command, one api.Dashboard) {
	cmd.Println(one.Name)
	cmd.Printf("  %s, %s, %s\n", arrangementOf(one), themeOf(one), backgroundOf(one))
	cmd.Printf("  headline: %s\n", slotLine(one.Headline))
	if len(one.Rings) > 0 {
		cmd.Printf("  rings: %s\n", strings.Join(one.Rings, ", "))
	}
	for _, slot := range one.Slots {
		cmd.Printf("  slot: %s\n", slotLine(slot))
	}
	if one.Caption != "" {
		cmd.Printf("  caption: %s\n", one.Caption)
	}
}

func themeOf(one api.Dashboard) string {
	if one.Theme == "" {
		return "midnight"
	}
	return one.Theme
}

func slotLine(slot api.DashboardSlot) string {
	line := slot.Source
	if slot.Second != "" {
		// The separator as it will be drawn, so the line reads the way the
		// panel will: "cpu_pct / cpu_c".
		join := slot.Separator
		if join == "" {
			join = dashboard.DefaultSeparator
		}
		line += join + slot.Second
	}
	if slot.Label != "" {
		line += " as " + slot.Label
	}
	return line
}

/*
dashboardSaveCommand writes a dashboard from a file.

The window's editor is a form and this is not trying to be one. What it is
for is a machine with no window: the file is the same YAML the service keeps,
so the way to make one is to copy what `hotaru dashboard show` describes, or
to lift a block out of dashboards.yml and hand it back edited.
*/
func dashboardSaveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "save <name>",
		Short: "Write a dashboard from a file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			from, _ := cmd.Flags().GetString("from")
			if from == "" {
				return errors.New("--from says which file describes it")
			}
			body, err := os.ReadFile(from) //nolint:gosec // a path the user gave
			if err != nil {
				return fmt.Errorf("read %s: %w", from, err)
			}
			var one api.Dashboard
			if err := yaml.Unmarshal(body, &one); err != nil {
				return fmt.Errorf("read %s: %w", from, err)
			}
			one.Name = args[0]

			client, err := client(cmd)
			if err != nil {
				return err
			}
			if err := client.SaveDashboard(cmd.Context(), one); err != nil {
				return quiet(err)
			}
			cmd.Printf("%s is saved. `hotaru dashboard use %s` puts it on the screen.\n",
				one.Name, one.Name)
			return nil
		},
	}
	cmd.Flags().String("from", "", "a YAML file describing the dashboard")
	return cmd
}

/*
dashboardSceneCommand makes a scene whose lights match a dashboard.

The same idea as `hotaru image scene`, with the frame the panel would draw as
the picture: a screen full of amber reads across the case as amber, which is
what somebody choosing a dashboard and then a set of colours was doing by
hand.

The scene names the dashboard, so applying it puts that dashboard up.
*/
func dashboardSceneCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scene <dashboard> <name>",
		Short: "Make a scene whose lights match a dashboard",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := client(cmd)
			if err != nil {
				return err
			}
			distance, _ := cmd.Flags().GetFloat64("distance")
			effects, err := effectsFlag(cmd)
			if err != nil {
				return err
			}
			scene, err := client.SceneFromDashboard(cmd.Context(), args[0], args[1], distance, effects)
			if err != nil {
				return quiet(err)
			}
			cmd.Printf("%s: %d assignment(s) from %s. `hotaru scene apply %s` lights it.\n",
				scene.Name, len(scene.Assignments), args[0], scene.Name)
			return nil
		},
	}
	distanceFlag(cmd)
	effectFlag(cmd)
	return cmd
}

func dashboardUseCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "use <name>",
		Short: "Put one on the screen",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := client(cmd)
			if err != nil {
				return err
			}
			one, err := client.UseDashboard(cmd.Context(), args[0])
			if err != nil {
				return quiet(err)
			}
			cmd.Printf("The screen is drawing %s.\n", one.Name)
			return nil
		},
	}
}

func dashboardPreviewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "preview <name>",
		Short: "Write a frame to a file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			to, _ := cmd.Flags().GetString("to")
			if to == "" {
				return fmt.Errorf("--to says where to write the frame")
			}
			client, err := client(cmd)
			if err != nil {
				return err
			}
			frame, err := client.PreviewDashboard(cmd.Context(), args[0], nil)
			if err != nil {
				return quiet(err)
			}
			body, err := base64.StdEncoding.DecodeString(frame.Image)
			if err != nil {
				return fmt.Errorf("that frame does not decode: %w", err)
			}
			if err := os.WriteFile(to, body, 0o600); err != nil {
				return fmt.Errorf("write %s: %w", to, err)
			}
			cmd.Printf("%s: %s, %.0fs between pushes.\n", to, size(int64(frame.Bytes)), frame.Floor)
			return nil
		},
	}
	cmd.Flags().String("to", "", "the file to write the frame to")
	return cmd
}

func dashboardDeleteCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <name>",
		Short: "Forget a dashboard",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := client(cmd)
			if err != nil {
				return err
			}
			if err := client.DeleteDashboard(cmd.Context(), args[0]); err != nil {
				return quiet(err)
			}
			cmd.Printf("%s is gone.\n", args[0])
			return nil
		},
	}
}
