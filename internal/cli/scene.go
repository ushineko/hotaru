package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/ushineko/hotaru/internal/api"
)

/*
sceneCommand is named lighting.

A scene is what somebody wants their machine to look like, kept under a name so
it survives a rebind and a reboot. Preview is deliberately a separate verb from
apply: one is a question, the other is an intention, and conflating them is how
a program ends up treating everything somebody glanced at as a preference.
*/
func sceneCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scene",
		Short: "Named lighting",
	}
	cmd.AddCommand(sceneListCommand(), sceneShowCommand(), sceneWriteCommand(),
		sceneApplyCommand(), scenePreviewCommand(), sceneSaveCommand(),
		sceneRecolourCommand(), sceneStyleCommand(), sceneAdoptCommand(),
		sceneDeleteCommand())
	return cmd
}

func sceneListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "What scenes are saved",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := client(cmd)
			if err != nil {
				return err
			}
			saved, err := client.Scenes(cmd.Context())
			if err != nil {
				return quiet(err)
			}
			if len(saved) == 0 {
				cmd.Println("No scenes saved. `hotaru scene save <name>` keeps what the lights are showing now.")
				return nil
			}

			out := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(out, "NAME\tLIGHTS\tEFFECTS\tSCREEN")
			for _, scene := range saved {
				_, _ = fmt.Fprintf(out, "%s\t%s\t%s\t%s\n",
					scene.Name, lights(scene), effects(scene), scene.Screen)
			}
			if err := out.Flush(); err != nil {
				return fmt.Errorf("write the listing: %w", err)
			}
			return nil
		},
	}
}

func sceneShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show <name>",
		Short: "What a scene does",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := client(cmd)
			if err != nil {
				return err
			}
			scene, err := find(cmd, client, args[0])
			if err != nil {
				return err
			}

			cmd.Println(scene.Name)
			switch {
			case scene.Off:
				cmd.Println("  the lights go off")
			case scene.Colour != "":
				cmd.Printf("  everything is %s\n", scene.Colour)
			}
			for _, a := range scene.Assignments {
				cmd.Printf("  %s is %s\n", a.Target, a.Colour)
			}
			for _, device := range sortedKeys(scene.Effects) {
				cmd.Printf("  %s is doing %s\n", device, scene.Effects[device])
			}
			switch scene.Screen {
			case "":
				cmd.Println("  the screen is left as it is")
			case api.ScreenDashboard:
				cmd.Println("  the screen shows the dashboard")
			case api.ScreenReadout:
				cmd.Println("  the screen shows the cooler's own display")
			default:
				cmd.Printf("  the screen shows %s\n", scene.Screen)
			}
			return nil
		},
	}
}

func sceneWriteCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "write <name> <target>=<colour>...",
		Short: "Write a scene out, rather than capturing one",
		Long: `Write a scene out, rather than capturing one.

	hotaru scene write evening --colour #201040
	hotaru scene write evening kraken=#201040 keychron=#100820 \
	    --effect keychron="Solid Splash" --screen dashboard

--colour is one colour across every device in scope, which is what most scenes
are; targets after it are the exceptions, so "everything blue except the top
fan" stays two words rather than an enumeration.

The same targets as ` + "`hotaru light set`" + `: a device, a zone, an LED range,
or a segment named in the rules file. An effect is a mode the device
advertises, by any part of the device's name; one it does not have costs the
effect rather than the scene, and is reported when the scene is applied.

Writing a scene does not light it. ` + "`hotaru scene apply`" + ` does that,
and ` + "`hotaru scene save`" + ` is the other way to make one: it keeps what
the lights are showing now.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := client(cmd)
			if err != nil {
				return err
			}

			scene := api.Scene{Name: args[0], Effects: map[string]string{}}
			for _, arg := range args[1:] {
				target, colour, ok := strings.Cut(arg, "=")
				if !ok {
					return fmt.Errorf("%q: a scene's lights are written target=colour", arg)
				}
				scene.Assignments = append(scene.Assignments,
					api.SceneAssignment{Target: target, Colour: colour})
			}

			effects, _ := cmd.Flags().GetStringSlice("effect")
			for _, effect := range effects {
				device, mode, ok := strings.Cut(effect, "=")
				if !ok {
					return fmt.Errorf("%q: an effect is written device=mode", effect)
				}
				scene.Effects[device] = mode
			}
			scene.Screen, _ = cmd.Flags().GetString("screen")
			scene.Colour, _ = cmd.Flags().GetString("colour")
			scene.Off, _ = cmd.Flags().GetBool("off")
			if scene.Off && (scene.Colour != "" || len(scene.Assignments) > 0) {
				// Off is not a dark colour, and a scene that says both has
				// not decided what it wants.
				return fmt.Errorf("a scene is either off or a colour, not both")
			}

			if err := client.SaveScene(cmd.Context(), scene); err != nil {
				return quiet(err)
			}
			cmd.Printf("Saved %s. `hotaru scene apply %s` lights it.\n", scene.Name, scene.Name)
			return nil
		},
	}
	cmd.Flags().String("colour", "", "one colour across every device in scope")
	cmd.Flags().Bool("off", false, "turn lighting off rather than colouring it")
	cmd.Flags().StringSlice("effect", nil, `what a device should be doing: device="Mode Name"`)
	cmd.Flags().String("screen", "",
		`what the cooler's screen shows: "dashboard", "readout", or a path to a GIF`)
	return cmd
}

func sceneApplyCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "apply <name>",
		Short: "Light a scene, and keep it",
		Long: `Light a scene, and keep it.

Recorded as what this machine should be showing, so it comes back after a
reboot and is re-sent to hardware that forgets.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := client(cmd)
			if err != nil {
				return err
			}
			done, err := client.ApplyScene(cmd.Context(), args[0], api.SceneRequest{})
			if err != nil {
				return quiet(err)
			}
			report(cmd, done)
			return nil
		},
	}
}

func scenePreviewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "preview <name> | <target>=<colour>...",
		Short: "Look at a scene without keeping it",
		Long: `Look at a scene without keeping it.

The lights change and nothing is recorded. hotaru also stops re-sending
colours to the devices this scene covers, so what you are looking at is not
quietly corrected underneath you while you decide.

It ends when this command does -- press Ctrl-C, or close the terminal, or kill
it -- and the lights go back to what was last applied.

Targets can be given instead of a name, and then nothing needs to have been
saved at all:

	hotaru scene preview kraken=#201040 keychron=black`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := client(cmd)
			if err != nil {
				return err
			}

			/*
				Detached, the preview outlives this command under a lease
				somebody else renews. For a script, and for the case this is
				being run from: a terminal that cannot sit still.
			*/
			if detach, _ := cmd.Flags().GetBool("detach"); detach {
				done, err := detached(cmd, client, args)
				if err != nil {
					return quiet(err)
				}
				report(cmd, done)
				cmd.Printf("Previewing, for about %s. `hotaru preview renew %s` keeps it, "+
					"`hotaru preview release %s` ends it.\n",
					previewFor, done.Preview.Token, done.Preview.Token)
				return nil
			}

			done, release, err := hold(cmd, client, args)
			if err != nil {
				return quiet(err)
			}
			defer release()

			report(cmd, done)
			cmd.Println("Previewing. Press Ctrl-C to put the lights back.")
			<-cmd.Context().Done()
			cmd.Println("\nPutting the lights back.")
			return nil
		},
	}
	cmd.Flags().Bool("detach", false,
		"leave the preview up under a token, instead of holding it here")
	return cmd
}

// previewFor is how long a detached preview lasts unrenewed. The service's
// own number, repeated in the message so nobody has to go and look it up.
const previewFor = "10 seconds"

/*
detached takes a preview and lets go of it.

The token form of the same lease. A command that returns cannot be the thing
holding a draft up, so the expiry is what stops a script that died from leaving
somebody's lights on a colour nobody chose.
*/
func detached(cmd *cobra.Command, client *api.Client, args []string) (api.SceneResponse, error) {
	const who = "hotaru scene preview --detach"
	if len(args) == 1 && !strings.Contains(args[0], "=") {
		return client.PreviewScene(cmd.Context(), args[0], api.SceneRequest{Holder: who})
	}
	draft, err := draftOf(args)
	if err != nil {
		return api.SceneResponse{}, err
	}
	return client.PreviewDraft(cmd.Context(), draft, who)
}

func sceneSaveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "save <name>",
		Short: "Keep what the lights are showing, under a name",
		Long: `Keep what the lights are showing, under a name.

Saves the colour and effect of every device hotaru has been asked to light, as
it is now. A device showing several colours at once cannot be reduced to one
line, and is reported rather than guessed at.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := client(cmd)
			if err != nil {
				return err
			}
			screen, _ := cmd.Flags().GetString("screen")
			scene, err := client.CaptureScene(cmd.Context(), args[0], screen)
			if err != nil {
				return quiet(err)
			}
			cmd.Printf("Saved %s: %d device(s).\n", scene.Name, len(scene.Assignments))
			return nil
		},
	}
	cmd.Flags().String("screen", "",
		`what the cooler's screen shows: "dashboard", "readout", or a path to a GIF`)
	return cmd
}

/*
sceneRecolourCommand builds a scene's lights again from whatever it shows.

For the knob that has no right answer. Somebody sets a separation, looks at
the case, and wants the colours further apart -- and what they are adjusting
is not visible anywhere except the machine itself, so it has to be adjustable
after the fact rather than only at the moment the scene is made.
*/
func sceneRecolourCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "recolour <name>",
		Short: "Build a scene's lights again from the picture or dashboard it shows",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := client(cmd)
			if err != nil {
				return err
			}
			distance, _ := cmd.Flags().GetFloat64("distance")
			scene, err := client.Recolour(cmd.Context(), args[0], distance)
			if err != nil {
				return quiet(err)
			}
			cmd.Printf("%s: %d assignment(s), separation %.1f.\n",
				scene.Name, len(scene.Assignments), distance)
			return nil
		},
	}
	distanceFlag(cmd)
	return cmd
}

/*
sceneAdoptCommand takes the pictures scenes point at into the library.

A scene written before hotaru had a library names a file wherever it happened
to be -- under Pictures, on another disk, in a directory that is about to be
tidied up. It works, until the file moves; and the window cannot offer it,
because the chooser lists what hotaru keeps rather than every GIF on the
machine. Adopting copies the file in and points the scene at the copy, which
is what a scene made today would have.

The original is left alone. Nothing is adopted twice: a scene already pointing
into the library is skipped, and so is one naming a file that is no longer
there -- that is a picture to go and find, not one to copy.
*/
func sceneAdoptCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "adopt [name...]",
		Short: "Copy the pictures scenes point at into the library",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := client(cmd)
			if err != nil {
				return err
			}
			saved, err := client.Scenes(cmd.Context())
			if err != nil {
				return quiet(err)
			}
			stored, err := client.Images(cmd.Context())
			if err != nil {
				return quiet(err)
			}

			kept := map[string]bool{}
			taken := map[string]bool{}
			for _, one := range stored {
				kept[one.Path] = true
				taken[one.Name] = true
			}

			adopted := 0
			for _, scene := range saved {
				if len(args) > 0 && !slices.Contains(args, scene.Name) {
					continue
				}
				if !outside(scene.Screen, kept) {
					continue
				}
				source, err := os.ReadFile(scene.Screen) //nolint:gosec // a path a scene named
				if err != nil {
					cmd.PrintErrf("%s: %v\n", scene.Name, err)
					continue
				}

				name := free(suggested(scene.Screen), taken)
				picture, err := client.AddImage(cmd.Context(), name, source)
				if err != nil {
					return quiet(err)
				}
				taken[name] = true

				was := scene.Screen
				scene.Screen = picture.Path
				if err := client.SaveScene(cmd.Context(), scene); err != nil {
					return quiet(err)
				}
				adopted++
				cmd.Printf("%s: %s is now the picture %q.\n", scene.Name, was, name)
			}

			if adopted == 0 {
				cmd.Println("Every scene already points at a picture hotaru keeps.")
			}
			return nil
		},
	}
}

// outside says whether a scene's screen is a file hotaru does not keep. The
// other screen states -- a dashboard, the readout, nothing at all -- are not
// files and are left alone.
func outside(screen string, kept map[string]bool) bool {
	switch {
	case screen == "", kept[screen]:
		return false
	case screen == api.ScreenDashboard, screen == api.ScreenReadout:
		return false
	case strings.HasPrefix(screen, api.ScreenDashboardPrefix):
		return false
	}
	return filepath.IsAbs(screen)
}

// suggested is what to call a picture, from the file it came out of: the name
// somebody gave it, without the extension.
func suggested(path string) string {
	return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
}

// free is a name nothing else in the library has, because adopting must not
// write over a picture somebody already kept under the same name.
func free(name string, taken map[string]bool) string {
	if !taken[name] {
		return name
	}
	for n := 2; ; n++ {
		next := fmt.Sprintf("%s-%d", name, n)
		if !taken[next] {
			return next
		}
	}
}

/*
sceneStyleCommand gives other scenes the style of one.

A style and a colour are different things: a scene names a colour per light
and a mode per device -- what that device does with the colours once it has
them. Somebody who decides their keyboard should be reactive has decided that
about the keyboard rather than about one scene, and saying so across a bank of
nine used to mean editing nine scenes.
*/
func sceneStyleCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "style <from> <to>...",
		Short: "Give other scenes the effects of this one",
		Long: `Give other scenes the effects of this one.

The effects only: what each device is doing with the colours, not the colours
themselves. A keyboard set to a reactive mode in one scene can be reactive in
all of them without touching what any of them light.

It replaces rather than merges, so every named scene ends up with exactly the
effects the source has -- including none, which is how a bank is put back to
plain colours.`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := client(cmd)
			if err != nil {
				return err
			}
			changed, err := client.CopyEffects(cmd.Context(), args[0], args[1:])
			if err != nil {
				return quiet(err)
			}
			if len(changed) == 0 {
				cmd.Println("Nothing to change.")
				return nil
			}
			cmd.Printf("%s: %s now do what it does.\n", args[0], strings.Join(changed, ", "))
			return nil
		},
	}
}

func sceneDeleteCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <name>",
		Short: "Forget a scene",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := client(cmd)
			if err != nil {
				return err
			}
			if err := client.DeleteScene(cmd.Context(), args[0]); err != nil {
				return quiet(err)
			}
			cmd.Printf("%s is gone.\n", args[0])
			return nil
		},
	}
}

/*
hold starts a preview, from a saved name or from targets typed on the spot.

The unsaved form exists because the editor needs it and parity is symmetrical:
a route the window can reach is a route a terminal can reach. It is also the
quickest way to try a colour without leaving anything behind.
*/
func hold(cmd *cobra.Command, client *api.Client, args []string) (api.SceneResponse, func(), error) {
	const who = "hotaru scene preview"
	if len(args) == 1 && !strings.Contains(args[0], "=") {
		return client.HoldScene(cmd.Context(), args[0], api.SceneRequest{Holder: who})
	}

	draft, err := draftOf(args)
	if err != nil {
		return api.SceneResponse{}, nil, err
	}
	return client.HoldDraft(cmd.Context(), draft, who)
}

// draftOf reads target=colour pairs into a scene nobody saved.
func draftOf(args []string) (api.Scene, error) {
	draft := api.Scene{Name: "a draft"}
	for _, arg := range args {
		target, colour, ok := strings.Cut(arg, "=")
		if !ok {
			return api.Scene{}, fmt.Errorf(
				"%q: a scene name, or target=colour for something unsaved", arg)
		}
		draft.Assignments = append(draft.Assignments,
			api.SceneAssignment{Target: target, Colour: colour})
	}
	return draft, nil
}

// find resolves a name the way the service does, so `show` and `apply` accept
// the same prefixes.
func find(cmd *cobra.Command, client *api.Client, name string) (api.Scene, error) {
	saved, err := client.Scenes(cmd.Context())
	if err != nil {
		return api.Scene{}, quiet(err)
	}
	var matches []api.Scene
	for _, scene := range saved {
		if scene.Name == name {
			return scene, nil
		}
		if strings.HasPrefix(strings.ToLower(scene.Name), strings.ToLower(name)) {
			matches = append(matches, scene)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return api.Scene{}, fmt.Errorf("no scene called %q", name)
	}
	names := make([]string, len(matches))
	for i, scene := range matches {
		names[i] = scene.Name
	}
	sort.Strings(names)
	return api.Scene{}, fmt.Errorf("%q matches %s", name, strings.Join(names, ", "))
}

// report says what a scene did, per device, and what it could not do.
func report(cmd *cobra.Command, done api.SceneResponse) {
	changed := 0
	for _, result := range done.Results {
		if result.Applied {
			changed++
		}
		if result.Skipped != "" {
			cmd.Printf("%s: %s\n", result.Device, result.Skipped)
		}
		if result.Error != "" {
			cmd.Printf("%s: %s\n", result.Device, result.Error)
		}
		for _, problem := range result.Problems {
			// An assignment that named something this device does not have.
			// Said even when the write worked: a scene quietly doing
			// something other than what was asked is the failure this
			// reporting exists for.
			cmd.Printf("%s: %s\n", result.Device, problem)
		}
	}
	cmd.Printf("%s: %d of %d device(s) lit.\n", done.Scene, changed, len(done.Results))
	if done.Screen != "" {
		cmd.Printf("The screen is showing %s.\n", done.Screen)
	}
	for _, problem := range done.Problems {
		cmd.Printf("Not done: %s\n", problem)
	}
}

// lights is the LIGHTS column: what a scene does to the lighting, in as few
// words as it takes to tell two scenes apart in a listing.
func lights(scene api.Scene) string {
	switch {
	case scene.Off:
		return "off"
	case scene.Colour != "" && len(scene.Assignments) > 0:
		return fmt.Sprintf("%s +%d", scene.Colour, len(scene.Assignments))
	case scene.Colour != "":
		return scene.Colour
	case len(scene.Assignments) > 0:
		return fmt.Sprintf("%d target(s)", len(scene.Assignments))
	}
	return "-"
}

func effects(scene api.Scene) string {
	if len(scene.Effects) == 0 {
		return ""
	}
	var out []string
	for _, device := range sortedKeys(scene.Effects) {
		out = append(out, device+": "+scene.Effects[device])
	}
	return strings.Join(out, ", ")
}

func sortedKeys(in map[string]string) []string {
	out := make([]string, 0, len(in))
	for key := range in {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}
