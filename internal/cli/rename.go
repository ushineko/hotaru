package cli

import (
	"github.com/spf13/cobra"
	"github.com/ushineko/hotaru/internal/api"
)

/*
Renaming, in the terminal.

A verb per kind of thing rather than one `hotaru rename`, because the kinds
already have their own commands and a scene called `aurora` and a picture
called `aurora` are both ordinary. Guessing between them is a guess that is
wrong at the worst moment. See spec 053.
*/

func sceneRenameCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "rename <name> <new name>",
		Short: "Rename a scene, keys and all",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := client(cmd)
			if err != nil {
				return err
			}
			done, err := client.RenameScene(cmd.Context(), args[0], args[1])
			if err != nil {
				return quiet(err)
			}
			renamed(cmd, done)
			return nil
		},
	}
}

func dashboardRenameCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "rename <name> <new name>",
		Short: "Rename a dashboard, and the scenes that name it",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := client(cmd)
			if err != nil {
				return err
			}
			done, err := client.RenameDashboard(cmd.Context(), args[0], args[1])
			if err != nil {
				return quiet(err)
			}
			renamed(cmd, done)
			return nil
		},
	}
}

func dashboardCloneCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "clone <name> <new name>",
		Short: "Copy a dashboard under another name",
		Long: `Copy a dashboard under another name.

A dashboard is a dozen decisions, and the same dozen with a different picture
behind them is a reasonable thing to want. The shipped ones can be cloned,
which is how somebody starts from one.

The copy is not put on the screen. Copying a dashboard is not asking to see
it.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := client(cmd)
			if err != nil {
				return err
			}
			one, err := client.CloneDashboard(cmd.Context(), args[0], args[1])
			if err != nil {
				return quiet(err)
			}
			cmd.Printf("%s is a copy of %s.\n", one.Name, args[0])
			return nil
		},
	}
}

func imageRenameCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "rename <name> <new name>",
		Short: "Rename a picture, and everything that shows it",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := client(cmd)
			if err != nil {
				return err
			}
			done, err := client.RenameImage(cmd.Context(), args[0], args[1])
			if err != nil {
				return quiet(err)
			}
			renamed(cmd, done)
			return nil
		},
	}
}

/*
renamed says what moved, and what moved with it.

The second line only when there is one. A rename of something nothing points
at is the ordinary case, and a line saying nothing was updated is a line
somebody reads every time to learn nothing.
*/
func renamed(cmd *cobra.Command, done api.Renamed) {
	cmd.Printf("renamed %s to %s\n", done.From, done.To)
	if done.Changed != "" {
		cmd.Printf("  %s\n", done.Changed)
	}
}
