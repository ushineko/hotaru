package cli

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

/*
keysCommand is the hotkeys: what is bound, what is free, and what is in the way.

The third one is why this command exists at all. A shortcut another program
still claims registers successfully and then does nothing when it is pressed,
with nothing anywhere saying why -- which is a fault that has cost this desk
several days, and is invisible without somebody looking in a file.
*/
func keysCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "keys",
		Short: "Scenes on hotkeys",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := client(cmd)
			if err != nil {
				return err
			}
			keys, err := client.Keys(cmd.Context())
			if err != nil {
				return quiet(err)
			}

			if keys.Desktop != "" {
				cmd.Printf("The desktop integration is not running: %s\n\n", keys.Desktop)
			}
			if len(keys.Bindings) == 0 {
				cmd.Println("Nothing is bound.")
			} else {
				w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
				_, _ = fmt.Fprintln(w, "KEY\tSCENE")
				for _, binding := range keys.Bindings {
					note := ""
					if binding.Missing {
						note = "  (no such scene)"
					}
					_, _ = fmt.Fprintf(w, "%s\t%s%s\n", binding.Key, binding.Scene, note)
				}
				if err := w.Flush(); err != nil {
					return fmt.Errorf("write the listing: %w", err)
				}
			}

			if len(keys.Reserved) > 0 {
				cmd.Printf("\n%s are left free for your own scenes.\n",
					strings.Join([]string{keys.Reserved[0], "…", keys.Reserved[len(keys.Reserved)-1]}, " "))
			}
			if len(keys.Claimed) > 0 {
				/*
					The failure this command exists for. Said as plainly as
					possible, because the symptom -- a key that does nothing
					while everything reports success -- gives no hint at all.
				*/
				cmd.Println("\nSomething else is holding these keys, and hotaru's will not fire while it does:")
				for _, claim := range keys.Claimed {
					cmd.Printf("  %s\n", claim)
				}
				cmd.Println("\n`hotaru keys release` removes those entries. Nothing else in the file is touched.")
			}
			return nil
		},
	}
	cmd.AddCommand(keysBindCommand(), keysUnbindCommand(), keysReleaseCommand())
	return cmd
}

func keysBindCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "bind <key> <scene>",
		Short: "Put a scene on a key",
		Long: `Put a scene on a key.

	hotaru keys bind "Ctrl+Alt+Shift+Num+1" evening

The key is spelled the way KDE spells it. Ctrl+Alt+Shift+Num+1 to +9 are left
free by default, which is where your own scenes go.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := client(cmd)
			if err != nil {
				return err
			}
			if err := client.Bind(cmd.Context(), args[0], args[1]); err != nil {
				return quiet(err)
			}
			cmd.Printf("%s applies %s. It takes effect the next time the desktop starts hotaru's script; "+
				"`hotaru keys` says whether it is installed.\n", args[0], args[1])
			return nil
		},
	}
}

func keysUnbindCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "unbind <key>",
		Short: "Take a scene off a key",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := client(cmd)
			if err != nil {
				return err
			}
			if err := client.Bind(cmd.Context(), args[0], ""); err != nil {
				return quiet(err)
			}
			cmd.Printf("%s does nothing now.\n", args[0])
			return nil
		},
	}
}

func keysReleaseCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "release",
		Short: "Remove another program's claim on these keys",
		Long: `Remove another program's claim on these keys.

KDE records a shortcut when a program registers one, and those records outlive
the program: unloading its script does not remove them and neither does
restarting the desktop. While one is there, hotaru's own registration succeeds
and the key does nothing.

This removes exactly those entries from ~/.config/kglobalshortcutsrc and leaves
every other line in the file alone. It asks first.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := client(cmd)
			if err != nil {
				return err
			}
			yes, _ := cmd.Flags().GetBool("yes")
			if !yes {
				keys, err := client.Keys(cmd.Context())
				if err != nil {
					return quiet(err)
				}
				if len(keys.Claimed) == 0 {
					cmd.Println("Nothing else is holding hotaru's keys.")
					return nil
				}

				cmd.Println("These entries will be removed from ~/.config/kglobalshortcutsrc:")
				for _, claim := range keys.Claimed {
					cmd.Printf("  %s\n", claim)
				}
				asker := NewTerminal(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout())
				agreed, err := asker.Confirm("Remove them?")
				if err != nil {
					return err
				}
				if !agreed {
					cmd.Println("Left alone.")
					return nil
				}
			}

			removed, err := client.ReleaseKeys(cmd.Context())
			if err != nil {
				return quiet(err)
			}
			if removed == 0 {
				cmd.Println("Nothing else is holding hotaru's keys.")
				return nil
			}
			cmd.Printf("Removed %d entr%s. Log out and back in, or restart the desktop, "+
				"for the keys to come free.\n", removed, plural(removed))
			return nil
		},
	}
	cmd.Flags().Bool("yes", false, "do not ask")
	return cmd
}

func plural(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}
