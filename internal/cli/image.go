package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

/*
imageCommand is the pictures the cooler's screen can show.

The panel takes 640x640 GIFs and nothing else, and says no by displaying
nothing at all -- so a wallpaper has to be converted before it can be shown,
and the conversion is worth keeping rather than redoing. hotaru does it and
stores the result.
*/
func imageCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "image",
		Short: "Pictures for the cooler's screen",
	}
	cmd.AddCommand(imageListCommand(), imageAddCommand(),
		imageShowCommand(), imageRemoveCommand())
	return cmd
}

func imageListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "What pictures are stored",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := client(cmd)
			if err != nil {
				return err
			}
			stored, err := client.Images(cmd.Context())
			if err != nil {
				return quiet(err)
			}
			if len(stored) == 0 {
				cmd.Println("No pictures. `hotaru image add <name> <file>` converts one.")
				return nil
			}

			out := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(out, "NAME\tSIZE\tFRAMES")
			for _, image := range stored {
				moves := "still"
				if image.Frames > 1 {
					moves = fmt.Sprintf("%d frames", image.Frames)
				}
				_, _ = fmt.Fprintf(out, "%s\t%s\t%s\n", image.Name, size(image.Bytes), moves)
			}
			if err := out.Flush(); err != nil {
				return fmt.Errorf("write the listing: %w", err)
			}
			return nil
		},
	}
}

func imageAddCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <name> <file>",
		Short: "Convert a picture and keep it",
		Long: `Convert a picture and keep it.

Any JPEG, PNG or GIF. It is scaled to the panel's 640x640 and cropped to the
middle rather than letterboxed -- bars across a photograph look like a mistake
-- and stored as a GIF, because this firmware drops a still image within
seconds and holds a GIF indefinitely.

The name is what you refer to it by afterwards, in a scene or on the screen.

--convert-to writes the converted GIF to a file and keeps nothing, which is
how to look at a crop before committing to it. The window shows the same thing
in a dialog.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := client(cmd)
			if err != nil {
				return err
			}
			source, err := os.ReadFile(args[1]) //nolint:gosec // a path the user typed
			if err != nil {
				return fmt.Errorf("read %s: %w", args[1], err)
			}

			if out, _ := cmd.Flags().GetString("convert-to"); out != "" {
				converted, frames, err := client.ConvertImage(cmd.Context(), source)
				if err != nil {
					return quiet(err)
				}
				// The path is the one the person running this typed, on
				// their own machine, as the point of the flag.
				if err := os.WriteFile(out, converted, 0o600); err != nil { //nolint:gosec
					return fmt.Errorf("write %s: %w", out, err)
				}
				cmd.Printf("%s is %s, %d frame(s). Nothing was kept.\n",
					out, size(int64(len(converted))), frames)
				return nil
			}

			stored, err := client.AddImage(cmd.Context(), args[0], source)
			if err != nil {
				return quiet(err)
			}
			cmd.Printf("%s is %s. `hotaru image show %s` puts it on the screen.\n",
				stored.Name, size(stored.Bytes), stored.Name)
			return nil
		},
	}
	cmd.Flags().String("convert-to", "",
		"write the converted picture to this file and keep nothing")
	return cmd
}

func imageShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show <name>",
		Short: "Put a stored picture on the screen",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := client(cmd)
			if err != nil {
				return err
			}
			if err := client.ShowImage(cmd.Context(), args[0]); err != nil {
				return quiet(err)
			}
			cmd.Printf("Showing %s. `hotaru screen dashboard` puts the dashboard back.\n", args[0])
			return nil
		},
	}
}

func imageRemoveCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Forget a stored picture",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := client(cmd)
			if err != nil {
				return err
			}
			if err := client.RemoveImage(cmd.Context(), args[0]); err != nil {
				return quiet(err)
			}
			cmd.Printf("%s is gone.\n", args[0])
			return nil
		},
	}
}

// size is a byte count as somebody reads it. The number matters here: the
// panel's refresh floor scales with it.
func size(bytes int64) string {
	switch {
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(bytes)/(1<<10))
	}
	return fmt.Sprintf("%d bytes", bytes)
}
