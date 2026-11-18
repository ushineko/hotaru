package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/ushineko/hotaru/internal/config"
)

/*
What was kept of light map (spec 078 R4): the colours parts are lit in to
tell them apart, a name's shape in the rules, and reading the rules a person
has.
*/

// palette is what parts are lit with: four colours nobody has to squint at.
// Rehearsal produced "bottom cyan(?)" from something subtler, and then the
// same mistake again with blue, cyan and teal on adjacent fans.
var palette = []string{"red", "green", "blue", "white"}

// slug is a name as it will appear in the file: what the user said, in the
// shape a target can carry.
func slug(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			return r
		case r == ' ', r == '_', r == '-', r == '/':
			return '-'
		default:
			return -1
		}
	}, name)
	return strings.Trim(strings.ReplaceAll(name, "--", "-"), "-")
}

/*
existingRules is the rules file a person has, for the names in it, and where
it is. A file that will not parse is read as empty, and said.
*/
func existingRules(asker Asker) (*config.Config, string) {
	path, err := config.RulesPath()
	if err != nil {
		return &config.Config{}, ""
	}
	cfg, problems, err := config.Load(path)
	if err != nil {
		asker.Say("Could not read %s (%v), so this starts from nothing.", path, err)
		return &config.Config{}, path
	}
	for _, problem := range problems {
		asker.Say("  %s", problem.Error())
	}
	return cfg, path
}

/*
mapGoneCommand is light map's name, kept to say what replaced it (spec 078
R4.1): a script or a habit that runs it learns where to go.
*/
func mapGoneCommand() *cobra.Command {
	return &cobra.Command{
		Use:    "map",
		Short:  "Removed: see light length, light fans, light strip and light name",
		Hidden: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.Println("light map is gone; each thing it asked is a command that shows its answer:")
			cmd.Println("  hotaru light length <device>/<zone>        a header's length, by looking")
			cmd.Println("  hotaru light fans <device>/<zone> <n>      a header holds n fans")
			cmd.Println("  hotaru light strip <device>/<zone>         a header holds a strip")
			cmd.Println("  hotaru light name <device>                 names for its zones and fans")
			return errStopped
		},
	}
}
