package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/ushineko/hotaru/internal/api"
)

/*
previewCommand is the way out of a draft nobody is holding any more.

`hotaru scene preview` ends when it does, and that covers the ordinary case.
This is for the other one: something took a preview over the socket -- a script,
twenty lines of curl, a GUI mid-development -- and the lights are sitting on a
draft. Somebody at a terminal needs to be able to see that and end it without
knowing which program did it.
*/
func previewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "preview",
		Short: "Drafts somebody is holding on the lights",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := client(cmd)
			if err != nil {
				return err
			}
			held, err := previews(cmd, client)
			if err != nil {
				return err
			}
			if len(held) == 0 {
				cmd.Println("Nothing is previewing. The lights are showing what was last applied.")
				return nil
			}
			for _, preview := range held {
				cmd.Printf("%s: %s is showing %s on %s\n",
					preview.Token, holder(preview), scene(preview),
					strings.Join(preview.Devices, ", "))
			}
			return nil
		},
	}
	cmd.AddCommand(previewReleaseCommand(), previewRenewCommand())
	return cmd
}

func previewReleaseCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "release [token]",
		Short: "End a preview and put the lights back",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := client(cmd)
			if err != nil {
				return err
			}

			tokens := args
			if all, _ := cmd.Flags().GetBool("all"); all || len(args) == 0 {
				held, err := previews(cmd, client)
				if err != nil {
					return err
				}
				tokens = nil
				for _, preview := range held {
					tokens = append(tokens, preview.Token)
				}
			}
			if len(tokens) == 0 {
				cmd.Println("Nothing is previewing.")
				return nil
			}

			for _, token := range tokens {
				restore, err := client.ReleasePreview(cmd.Context(), token)
				if err != nil {
					return quiet(err)
				}
				cmd.Printf("%s: %d device(s) back to what was last applied.\n", token, restore.Applied)
			}
			return nil
		},
	}
	cmd.Flags().Bool("all", false, "end every preview, whoever is holding it")
	return cmd
}

func previewRenewCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "renew <token>",
		Short: "Keep a preview alive a while longer",
		Long: `Keep a preview alive a while longer.

For a client that cannot sit on an open connection -- a shell script, or
anything one-shot. A preview taken that way lapses in a few seconds unless it
is renewed, so that a program which dies does not leave the lights on a draft.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := client(cmd)
			if err != nil {
				return err
			}
			if err := client.RenewPreview(cmd.Context(), args[0]); err != nil {
				return quiet(err)
			}
			return nil
		},
	}
}

// previews are the leases currently held, taken from the device listing: a
// preview is a property of the devices it covers, and there is no second place
// to ask.
func previews(cmd *cobra.Command, client *api.Client) ([]api.Preview, error) {
	found, err := client.Devices(cmd.Context())
	if err != nil {
		return nil, quiet(err)
	}
	seen := map[string]api.Preview{}
	for _, device := range found {
		if device.Preview != nil {
			seen[device.Preview.Token] = *device.Preview
		}
	}
	out := make([]api.Preview, 0, len(seen))
	for _, preview := range seen {
		out = append(out, preview)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Token < out[j].Token })
	return out, nil
}

func holder(preview api.Preview) string {
	if preview.Holder == "" {
		return "something"
	}
	return preview.Holder
}

func scene(preview api.Preview) string {
	if preview.Scene == "" {
		return "a draft"
	}
	return fmt.Sprintf("%q", preview.Scene)
}
