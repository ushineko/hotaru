package cli

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/ushineko/hotaru/internal/desktop"
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
			bound := map[string]bool{}
			for _, binding := range keys.Bindings {
				bound[binding.Key] = true
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

			// Only the ones still free: saying a key is reserved for your
			// own scenes while it is applying one of them reads as a bug.
			var free []string
			for _, key := range keys.Reserved {
				if !bound[key] {
					free = append(free, key)
				}
			}
			if len(free) > 0 {
				cmd.Printf("\n%s are left free for your own scenes.\n",
					strings.Join([]string{free[0], "…", free[len(free)-1]}, " "))
			}
			if len(keys.Claimed) > 0 {
				/*
					The failure this command exists for. Said as plainly as
					possible, because the symptom -- a key that does nothing
					while everything reports success -- gives no hint at all.
				*/
				/*
					Worth saying and easy to overstate. Measured on the
					development machine: hotaru registered its keys with
					eighteen of these present and every key worked, because
					the grab belongs to the loaded script rather than to the
					line. They are leftovers -- and KDE will refuse a
					sequence to a *different* component, so a key that does
					nothing while everything reports success is still the
					first thing to check here.
				*/
				cmd.Println("\nAnother program still has these keys recorded:")
				for _, claim := range keys.Claimed {
					cmd.Printf("  %s\n", claim)
				}
				cmd.Println("\nThey are leftovers: the program that registered them is not necessarily")
				cmd.Println("running, and hotaru's own keys may work anyway. If one does nothing while")
				cmd.Println("everything reports success, this is why. `hotaru keys release` asks KDE to")
				cmd.Println("forget them.")
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

This asks KDE to forget exactly those entries, through the daemon that owns
them, and touches nothing else. It asks first.

Deleting the lines from kglobalshortcutsrc does not work: kglobalaccel keeps
the table in memory and writes it out whenever anything registers a shortcut,
so the entries come back within seconds. Going through the daemon is also the
only way that does not need you to log out.

It happens here rather than in the service, which has write access to its own
two directories and nothing else -- a property worth keeping rather than a
limitation to work around.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := client(cmd)
			if err != nil {
				return err
			}
			keys, err := client.Keys(cmd.Context())
			if err != nil {
				return quiet(err)
			}

			path, err := desktop.ShortcutsFile()
			if err != nil {
				return err
			}
			wanted := make([]string, 0, len(keys.Bindings)+len(keys.Reserved))
			for _, binding := range keys.Bindings {
				wanted = append(wanted, binding.Key)
			}
			wanted = append(wanted, keys.Reserved...)

			claims, err := desktop.Claimed(path, wanted)
			if err != nil {
				return err
			}
			if len(claims) == 0 {
				cmd.Println("Nothing else is holding hotaru's keys.")
				return nil
			}

			cmd.Printf("These entries will be removed from %s:\n", path)
			for _, claim := range claims {
				cmd.Printf("  %s holds %s (in %s)\n", claim.Entry, claim.Key, claim.Component)
			}
			yes, _ := cmd.Flags().GetBool("yes")
			if !yes {
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

			conn, err := desktop.Session()
			if err != nil {
				return err
			}
			defer func() { _ = conn.Close() }()

			removed, err := desktop.Release(conn, claims)
			if err != nil {
				return err
			}
			cmd.Printf("KDE has forgotten %d entr%s.\n", removed, plural(removed))
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
