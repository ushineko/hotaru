package cli

import (
	"encoding/base64"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/ushineko/hotaru/internal/api"
)

/*
screenCommand is the cooler's panel.

Deliberately small. It shows a picture, hands the panel to the cooler or back
to hotaru's dashboard, and sets the two things the device remembers. What it
does not do is render anything: the dashboard is the service's, and a command
that quietly started one would make "show me this picture" mean something
different tomorrow.

The screen has one picture on it, so showing one stops the dashboard until
`hotaru screen dashboard` asks for it back. A picture replaced two seconds
later was not shown.
*/
func screenCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "screen",
		Short: "The cooler's screen",
	}
	cmd.AddCommand(screenShowCommand(), screenOffCommand(),
		screenDashboardCommand(), screenAppearanceCommand())
	return cmd
}

func screenShowCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <file.gif>",
		Short: "Put a picture on the screen",
		Long: `Put a picture on the screen.

A GIF, animated or not. The cooler's firmware does not keep a still image --
it reverts to its own display within seconds -- so a single-frame GIF is how a
static picture stays up.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := client(cmd)
			if err != nil {
				return err
			}
			gif, err := os.ReadFile(args[0]) //nolint:gosec // a path the user typed
			if err != nil {
				return fmt.Errorf("read %s: %w", args[0], err)
			}
			if err := client.Screen(cmd.Context(), api.ScreenRequest{
				Image: base64.StdEncoding.EncodeToString(gif),
			}); err != nil {
				return quiet(err)
			}
			cmd.Printf("Showing %s. Run `hotaru screen dashboard` to put the dashboard back.\n", args[0])
			return nil
		},
	}
	return cmd
}

func screenOffCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "readout",
		Short: "Give the screen back to the cooler",
		Long: `Give the screen back to the cooler.

Its own coolant display, which is what it shows when nothing else is using it.
hotaru does this on the way out too, so a machine that has stopped running it
is not left showing a stale picture.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := client(cmd)
			if err != nil {
				return err
			}
			if err := client.Screen(cmd.Context(), api.ScreenRequest{Readout: true}); err != nil {
				return quiet(err)
			}
			cmd.Println("The screen is showing the cooler's own display again.")
			return nil
		},
	}
}

func screenDashboardCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "dashboard",
		Short: "Put hotaru's dashboard back on the screen",
		Long: `Put hotaru's dashboard back on the screen.

What the screen shows by default: coolant temperature, the processor, the
graphics card and the pump. Showing a picture or asking for the cooler's own
display stops it, and this starts it again.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := client(cmd)
			if err != nil {
				return err
			}
			if err := client.Screen(cmd.Context(), api.ScreenRequest{Dashboard: true}); err != nil {
				return quiet(err)
			}
			cmd.Println("The dashboard is back.")
			return nil
		},
	}
}

func screenAppearanceCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set",
		Short: "How bright the screen is, and which way up",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := client(cmd)
			if err != nil {
				return err
			}
			var request api.ScreenRequest
			if cmd.Flags().Changed("brightness") {
				level, _ := cmd.Flags().GetInt("brightness")
				request.Brightness = &level
			}
			if cmd.Flags().Changed("rotate") {
				degrees, _ := cmd.Flags().GetInt("rotate")
				request.Orientation = &degrees
			}
			if request.Brightness == nil && request.Orientation == nil {
				return fmt.Errorf("give --brightness, --rotate, or both")
			}
			if err := client.Screen(cmd.Context(), request); err != nil {
				return quiet(err)
			}
			cmd.Println("Done.")
			return nil
		},
	}
	cmd.Flags().Int("brightness", 100, "how bright, 0 to 100")
	cmd.Flags().Int("rotate", 0, "which way up: 0, 90, 180 or 270")
	return cmd
}
